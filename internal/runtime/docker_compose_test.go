package runtime

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"
)

// fakeRunner records every Run invocation (name+args+opts) and can be
// configured to return an error for specific arg patterns or capture stdout.
type fakeRunner struct {
	calls    []fakeCall
	stdoutBy map[string]string // arg-join-prefix -> canned stdout
	errOn    func(name string, args []string) error
}

type fakeCall struct {
	name        string
	args        []string
	interactive bool
	dir         string
}

func newFakeRunner() *fakeRunner {
	return &fakeRunner{
		stdoutBy: map[string]string{},
		errOn:    func(string, []string) error { return nil },
	}
}

func (f *fakeRunner) Run(ctx context.Context, name string, args []string, opts RunOpts) error {
	call := fakeCall{name: name, args: append([]string(nil), args...), interactive: opts.Interactive, dir: opts.Dir}
	f.calls = append(f.calls, call)
	if err := f.errOn(name, args); err != nil {
		return err
	}
	// If a canned stdout is registered for a unique argv substring, write it.
	key := strings.Join(args, " ")
	for marker, out := range f.stdoutBy {
		if strings.Contains(key, marker) {
			if opts.Stdout != nil {
				io.WriteString(opts.Stdout, out)
			}
			break
		}
	}
	return nil
}

func (f *fakeRunner) lastArgs() []string {
	if len(f.calls) == 0 {
		return nil
	}
	return f.calls[len(f.calls)-1].args
}

func newTestDC() *DockerCompose {
	dc := &DockerCompose{
		Cfg:    DockerComposeConfig{ComposeFile: "docker-compose.yml", ProjectName: "demo", DefaultService: "dev", Dir: "/proj"},
		Runner: newFakeRunner(),
	}
	dc.Reachable = func(context.Context) error { return nil } // probe succeeds by default
	return dc
}

// TestDC_Argv verifies all argv builders produce the exact expected tokens
// WITHOUT touching a real docker daemon.
func TestDC_Argv(t *testing.T) {
	dc := newTestDC()

	// base
	if got := dc.baseArgs(); !reflect.DeepEqual(got, []string{"compose", "-f", "docker-compose.yml", "-p", "demo"}) {
		t.Errorf("baseArgs = %v", got)
	}
	// up -d
	if got := dc.upArgs(); !reflect.DeepEqual(got, []string{"compose", "-f", "docker-compose.yml", "-p", "demo", "up", "-d"}) {
		t.Errorf("upArgs = %v", got)
	}
	// down
	if got := dc.downArgs(); !reflect.DeepEqual(got, []string{"compose", "-f", "docker-compose.yml", "-p", "demo", "down"}) {
		t.Errorf("downArgs = %v", got)
	}
	// ps --format json
	if got := dc.psArgs(); !reflect.DeepEqual(got, []string{"compose", "-f", "docker-compose.yml", "-p", "demo", "ps", "--format", "json"}) {
		t.Errorf("psArgs = %v", got)
	}
}

// TestDC_ExecArgs verifies the -T / -w / service resolution rules.
func TestDC_ExecArgs(t *testing.T) {
	dc := newTestDC()

	// Non-interactive (default): -T, default service, no -w.
	got, err := dc.execArgs([]string{"echo", "hi"}, ExecOpts{})
	if err != nil {
		t.Fatalf("execArgs: %v", err)
	}
	want := []string{"compose", "-f", "docker-compose.yml", "-p", "demo", "exec", "-T", "dev", "echo", "hi"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("non-interactive exec = %v\nwant %v", got, want)
	}

	// Interactive: no -T.
	got, err = dc.execArgs([]string{"bash"}, ExecOpts{Interactive: true})
	if err != nil {
		t.Fatalf("execArgs interactive: %v", err)
	}
	want = []string{"compose", "-f", "docker-compose.yml", "-p", "demo", "exec", "dev", "bash"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("interactive exec = %v\nwant %v", got, want)
	}

	// Explicit service overrides default; workdir adds -w.
	got, err = dc.execArgs([]string{"ls"}, ExecOpts{Service: "web", Workdir: "/srv"})
	if err != nil {
		t.Fatalf("execArgs service+workdir: %v", err)
	}
	want = []string{"compose", "-f", "docker-compose.yml", "-p", "demo", "exec", "-T", "-w", "/srv", "web", "ls"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("service+workdir exec = %v\nwant %v", got, want)
	}

	// Interactive empty cmd (shell path): no trailing command.
	got, err = dc.execArgs(nil, ExecOpts{Interactive: true})
	if err != nil {
		t.Fatalf("execArgs shell: %v", err)
	}
	want = []string{"compose", "-f", "docker-compose.yml", "-p", "demo", "exec", "dev"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("shell exec = %v\nwant %v", got, want)
	}

	// Non-interactive empty cmd -> error.
	if _, err := dc.execArgs(nil, ExecOpts{}); err == nil {
		t.Errorf("empty non-interactive exec: expected error, got nil")
	}
}

// TestDC_ExecArgs_NoService verifies the fail_with_guidance when neither an
// explicit service nor a DefaultService is configured.
func TestDC_ExecArgs_NoService(t *testing.T) {
	dc := &DockerCompose{
		Cfg:    DockerComposeConfig{ComposeFile: "docker-compose.yml", ProjectName: "demo", Dir: "/proj"},
		Runner: newFakeRunner(),
	}
	dc.Reachable = func(context.Context) error { return nil }
	_, err := dc.execArgs([]string{"echo"}, ExecOpts{})
	if err == nil || !strings.Contains(err.Error(), "requires a service") {
		t.Errorf("expected service-required error, got %v", err)
	}
}

// TestDC_ComposeFileDefault verifies an empty ComposeFile resolves to the
// docker-compose.yml default.
func TestDC_ComposeFileDefault(t *testing.T) {
	dc := &DockerCompose{Cfg: DockerComposeConfig{ProjectName: "p"}, Runner: newFakeRunner()}
	if got := dc.composeFile(); got != "docker-compose.yml" {
		t.Errorf("composeFile() = %q, want docker-compose.yml", got)
	}
	if got := dc.baseArgs(); !reflect.DeepEqual(got, []string{"compose", "-f", "docker-compose.yml", "-p", "p"}) {
		t.Errorf("baseArgs default = %v", got)
	}
}

// TestDC_FailWithGuidance verifies docker_compose NEVER silently falls back to
// bare when the daemon is unreachable. The probe is stubbed to fail; Up must
// return the canonical guidance error containing "No fallback configured".
func TestDC_FailWithGuidance(t *testing.T) {
	dc := newTestDC()
	probeErr := errors.New("connection refused")
	dc.Reachable = func(context.Context) error { return probeErr }

	err := dc.Up(context.Background())
	if err == nil {
		t.Fatalf("Up with unreachable daemon: expected guidance error, got nil")
	}
	for _, want := range []string{"docker_compose", "not reachable", "No fallback configured"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("guidance error missing %q; got %q", want, err.Error())
		}
	}
	// Critical: the up command itself must NOT have been issued (no fallback).
	for _, c := range dc.Runner.(*fakeRunner).calls {
		if c.name == "docker" && len(c.args) > 0 && c.args[0] == "compose" && contains(c.args, "up") {
			t.Errorf("docker compose up was issued despite unreachable daemon (silent fallback!) call=%v", c.args)
		}
	}
}

// TestDC_HealthcheckOK verifies a successful probe reports reachability.
func TestDC_HealthcheckOK(t *testing.T) {
	dc := newTestDC() // probe succeeds
	if err := dc.Healthcheck(context.Background()); err != nil {
		t.Fatalf("Healthcheck: %v", err)
	}
}

// TestDC_LogsArgs verifies --follow and optional service.
func TestDC_LogsArgs(t *testing.T) {
	dc := newTestDC()
	if got := dc.logsArgs("", false); !reflect.DeepEqual(got, []string{"compose", "-f", "docker-compose.yml", "-p", "demo", "logs"}) {
		t.Errorf("logs no-follow no-service = %v", got)
	}
	if got := dc.logsArgs("web", true); !reflect.DeepEqual(got, []string{"compose", "-f", "docker-compose.yml", "-p", "demo", "logs", "--follow", "web"}) {
		t.Errorf("logs follow web = %v", got)
	}
}

// TestDC_PsParse verifies `docker compose ps --format json` output is parsed
// into normalized ServiceStatus rows.
func TestDC_PsParse(t *testing.T) {
	raw := strings.Join([]string{
		`{"Service":"dev","State":"running","Status":"Up 2 minutes"}`,
		`{"Service":"db","State":"exited","Status":"Exited 0"}`,
		``,               // blank line skipped
		`not-json-noise`, // unparseable skipped
	}, "\n")
	rows := parsePsJSON([]byte(raw))
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2: %+v", len(rows), rows)
	}
	if rows[0].Name != "dev" || rows[0].State != "running" || rows[0].Status != "Up 2 minutes" {
		t.Errorf("row0 = %+v", rows[0])
	}
	if rows[1].Name != "db" || rows[1].State != "exited" {
		t.Errorf("row1 = %+v", rows[1])
	}
}

// TestDC_PsLive runs Ps against a fake runner that returns canned JSON, proving
// the Ps -> Runner -> parse path works end to end without a real daemon.
func TestDC_PsLive(t *testing.T) {
	dc := newTestDC()
	fr := dc.Runner.(*fakeRunner)
	fr.stdoutBy["ps --format json"] = `{"Service":"dev","State":"running","Status":"Up 1 second"}` + "\n"

	rows, err := dc.Ps(context.Background())
	if err != nil {
		t.Fatalf("Ps: %v", err)
	}
	if len(rows) != 1 || rows[0].Name != "dev" {
		t.Fatalf("Ps rows = %+v", rows)
	}
	// Confirm the issued argv.
	last := fr.lastArgs()
	if last[len(last)-2] != "--format" || last[len(last)-1] != "json" {
		t.Errorf("Ps argv did not end with --format json: %v", last)
	}
}

// TestDC_PsUnavailable verifies Ps also prefights (daemon down -> guidance).
func TestDC_PsUnavailable(t *testing.T) {
	dc := newTestDC()
	dc.Reachable = func(context.Context) error { return fmt.Errorf("down") }
	if _, err := dc.Ps(context.Background()); err == nil || !strings.Contains(err.Error(), "No fallback configured") {
		t.Errorf("expected guidance error, got %v", err)
	}
}

// contains is a tiny local helper to avoid pulling sort/search for one check.
func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

// ensure bytes is used (kept for future stdout-buffer assertions).
var _ = bytes.Buffer{}
