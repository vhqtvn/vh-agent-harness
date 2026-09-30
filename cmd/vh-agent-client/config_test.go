// config_test.go — slice P2 step 1 (red): flag parsing, validation,
// mode resolution, and the exit-code mapping (2 = usage/validate,
// 1 = everything else, 0 = clean). The daemon argv assembly rules
// (default `vh-agentd`, --session-dir forwarding, `--` passthrough)
// are contract-tested here so the e2e battery can rely on them.
package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func discard() io.Writer { return io.Discard }

func fixedTTY(tty bool) func() bool { return func() bool { return tty } }

func TestUnknownFlagIsUsageError(t *testing.T) {
	_, err := parseArgs([]string{"--nope"}, fixedTTY(true), discard())
	if err == nil {
		t.Fatal("unknown flag must error")
	}
	if code := exitCodeFor(err); code != 2 {
		t.Fatalf("exitCodeFor(unknown flag) = %d, want 2", code)
	}
	if !strings.Contains(err.Error(), "nope") {
		t.Fatalf("error should name the flag: %v", err)
	}
}

func TestPromptSelectsOneShotMode(t *testing.T) {
	cfg, err := parseArgs([]string{"--prompt", "hello there"}, fixedTTY(false), discard())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cfg.Mode != ModeOneShot || cfg.Prompt != "hello there" {
		t.Fatalf("cfg = %+v, want one-shot prompt", cfg)
	}
}

func TestEmptyPromptIsUsageError(t *testing.T) {
	_, err := parseArgs([]string{"--prompt", ""}, fixedTTY(false), discard())
	if err == nil || exitCodeFor(err) != 2 {
		t.Fatalf("empty --prompt must be a usage error (exit 2), got %v", err)
	}
}

func TestResumeAndNewAreMutuallyExclusive(t *testing.T) {
	_, err := parseArgs([]string{"--prompt", "x", "--resume", "--new"}, fixedTTY(false), discard())
	if err == nil || exitCodeFor(err) != 2 {
		t.Fatalf("--resume + --new must be a usage error (exit 2), got %v", err)
	}
}

func TestReplForcedWithoutTTY(t *testing.T) {
	cfg, err := parseArgs([]string{"--repl"}, fixedTTY(false), discard())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cfg.Mode != ModeRepl {
		t.Fatalf("mode = %v, want REPL", cfg.Mode)
	}
}

func TestTTYAloneSelectsRepl(t *testing.T) {
	cfg, err := parseArgs([]string{}, fixedTTY(true), discard())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cfg.Mode != ModeRepl {
		t.Fatalf("mode = %v, want REPL on TTY", cfg.Mode)
	}
}

func TestNoTTYNoPromptNoReplIsUsageError(t *testing.T) {
	_, err := parseArgs([]string{}, fixedTTY(false), discard())
	if err == nil || exitCodeFor(err) != 2 {
		t.Fatalf("no input mode must be a usage error (exit 2), got %v", err)
	}
}

func TestPromptWinsOverTTY(t *testing.T) {
	cfg, err := parseArgs([]string{"--prompt", "one"}, fixedTTY(true), discard())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cfg.Mode != ModeOneShot {
		t.Fatalf("mode = %v, want one-shot (explicit --prompt wins over TTY)", cfg.Mode)
	}
}

func TestDefaultsForSessionDirAndDaemonArgv(t *testing.T) {
	cfg, err := parseArgs([]string{"--prompt", "x"}, fixedTTY(false), discard())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cfg.SessionDir == "" {
		t.Fatal("session dir must have a default")
	}
	argv := cfg.daemonArgv()
	if len(argv) < 3 || argv[0] != "vh-agentd" {
		t.Fatalf("default daemon argv = %v, want vh-agentd ...", argv)
	}
	sawDir := false
	for i, a := range argv {
		if a == "--session-dir" && i+1 < len(argv) && argv[i+1] == cfg.SessionDir {
			sawDir = true
		}
	}
	if !sawDir {
		t.Fatalf("default argv must forward --session-dir %s: %v", cfg.SessionDir, argv)
	}
}

// TestSessionDirResolvedAbsolute (hotfix b-F2): the daemon hard-rejects
// relative session dirs (Landlock RWDir resolution against the
// sandboxed child's cwd), and the documented DEFAULT is relative — so
// the client resolves the session dir to an absolute, cleaned path
// (from its own cwd) BEFORE daemon argv assembly: the default, a
// user-supplied relative dir, and an already-absolute dir alike.
func TestSessionDirResolvedAbsolute(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"default", []string{"--prompt", "x"}, filepath.Join(cwd, defaultSessionDir)},
		{"user-relative", []string{"--prompt", "x", "--session-dir", "rel/sessions"}, filepath.Join(cwd, "rel", "sessions")},
		{"user-relative-dirty", []string{"--prompt", "x", "--session-dir", "rel/../sessions"}, filepath.Join(cwd, "sessions")},
		{"already-absolute", []string{"--prompt", "x", "--session-dir", filepath.Join(cwd, "s")}, filepath.Join(cwd, "s")},
	}
	for _, tc := range cases {
		cfg, err := parseArgs(tc.args, fixedTTY(false), discard())
		if err != nil {
			t.Fatalf("%s: parse: %v", tc.name, err)
		}
		if cfg.SessionDir != tc.want {
			t.Fatalf("%s: SessionDir = %q, want %q", tc.name, cfg.SessionDir, tc.want)
		}
		if !filepath.IsAbs(cfg.SessionDir) {
			t.Fatalf("%s: SessionDir must be absolute, got %q", tc.name, cfg.SessionDir)
		}
		// The daemon argv carries the resolved path — the daemon never
		// sees a relative dir from this client.
		argv := cfg.daemonArgv()
		found := false
		for i, a := range argv {
			if a == "--session-dir" && i+1 < len(argv) && argv[i+1] == tc.want {
				found = true
			}
		}
		if !found {
			t.Fatalf("%s: daemon argv must forward the absolute dir: %v", tc.name, argv)
		}
	}
}

func TestExecSpecTakesRestOfArgs(t *testing.T) {
	cfg, err := parseArgs([]string{"--prompt", "x", "--exec", "myd", "--adapter", "openai", "--model", "m"}, fixedTTY(false), discard())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	want := []string{"myd", "--adapter", "openai", "--model", "m"}
	if strings.Join(cfg.Exec, " ") != strings.Join(want, " ") {
		t.Fatalf("Exec = %v, want %v", cfg.Exec, want)
	}
}

func TestExecSpecStripsLeadingDoubleDash(t *testing.T) {
	cfg, err := parseArgs([]string{"--exec", "myd", "--", "--weird-flag", "v"}, fixedTTY(true), discard())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	want := []string{"myd", "--weird-flag", "v"}
	if strings.Join(cfg.Exec, " ") != strings.Join(want, " ") {
		t.Fatalf("Exec = %v, want %v (leading -- stripped, rest passthrough)", cfg.Exec, want)
	}
}

func TestSessionDirInjectedOnlyWhenSpecLacksIt(t *testing.T) {
	cfg, err := parseArgs([]string{"--prompt", "x", "--exec", "myd", "--session-dir", "/custom"}, fixedTTY(false), discard())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	argv := cfg.daemonArgv()
	count := 0
	for _, a := range argv {
		if a == "--session-dir" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("daemon argv must carry exactly one --session-dir: %v", argv)
	}
	if !contains(argv, "/custom") {
		t.Fatalf("user-provided --session-dir must win: %v", argv)
	}
}

func TestExecRequiresACommand(t *testing.T) {
	_, err := parseArgs([]string{"--prompt", "x", "--exec"}, fixedTTY(false), discard())
	if err == nil || exitCodeFor(err) != 2 {
		t.Fatalf("--exec with no command must be a usage error, got %v", err)
	}
}

func TestExitCodeForMapsErrors(t *testing.T) {
	if code := exitCodeFor(nil); code != 0 {
		t.Fatalf("nil error → %d, want 0", code)
	}
	if code := exitCodeFor(io.EOF); code != 1 {
		t.Fatalf("non-usage error → %d, want 1", code)
	}
	if code := exitCodeFor(&usageError{msg: "bad"}); code != 2 {
		t.Fatalf("usageError → %d, want 2", code)
	}
}

// --- hotfix B-F1: --resume pre-scan vs the --exec boundary -------------------
//
// The client's --resume pre-scan must stop at the --exec boundary:
// everything after the daemon binary name is the DAEMON's command line
// and passes through VERBATIM (the documented UX contract). Pre-fix,
// the scan stripped --resume tokens from the ENTIRE raw argv, so
// `vh-agent-client --exec some-daemon --resume daemon-value` lost the
// daemon's --resume pair AND the client mis-parsed it as its own.

// TestExecPassesDaemonResumeVerbatim: a daemon carrying its own
// `--resume <id>` after the --exec boundary keeps it, and the client
// does NOT adopt it as its resume posture.
func TestExecPassesDaemonResumeVerbatim(t *testing.T) {
	cfg, err := parseArgs([]string{"--prompt", "x", "--exec", "some-daemon", "--resume", "daemon-value", "--flag", "v"}, fixedTTY(false), discard())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	want := []string{"some-daemon", "--resume", "daemon-value", "--flag", "v"}
	if strings.Join(cfg.Exec, " ") != strings.Join(want, " ") {
		t.Fatalf("Exec = %v, want %v (daemon argv passes through verbatim)", cfg.Exec, want)
	}
	if cfg.Resume || cfg.ResumeID != "" {
		t.Fatalf("the daemon's --resume must NOT become the client's: Resume=%v ResumeID=%q", cfg.Resume, cfg.ResumeID)
	}
	// The assembled daemon argv forwards the daemon's --resume pair
	// verbatim (the client's --session-dir is injected at the end).
	argv := cfg.daemonArgv()
	found := false
	for i := 0; i+2 < len(argv); i++ {
		if argv[i] == "--resume" && argv[i+1] == "daemon-value" {
			found = true
		}
	}
	if !found {
		t.Fatalf("daemonArgv must forward the daemon's --resume daemon-value verbatim: %v", argv)
	}
}

// TestExecPassesBareDaemonResumeFlag: a bare post-boundary --resume is
// the daemon's flag, not the client's pointer-resume request.
func TestExecPassesBareDaemonResumeFlag(t *testing.T) {
	cfg, err := parseArgs([]string{"--exec", "myd", "--resume"}, fixedTTY(true), discard())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cfg.Resume || cfg.ResumeID != "" {
		t.Fatalf("post-boundary bare --resume is the daemon's: Resume=%v ResumeID=%q", cfg.Resume, cfg.ResumeID)
	}
	want := []string{"myd", "--resume"}
	if strings.Join(cfg.Exec, " ") != strings.Join(want, " ") {
		t.Fatalf("Exec = %v, want %v", cfg.Exec, want)
	}
}

// TestClientResumeFormsBeforeExecStillParse: the three client-side
// forms — bare, `--resume=<id>`, `--resume <id>` — still parse when
// they appear BEFORE the --exec boundary, while the daemon's own
// --resume usage after the boundary is left alone.
func TestClientResumeFormsBeforeExecStillParse(t *testing.T) {
	cfg, err := parseArgs([]string{"--resume", "--exec", "myd", "--resume", "daemon-id"}, fixedTTY(true), discard())
	if err != nil {
		t.Fatalf("bare form: parse: %v", err)
	}
	if !cfg.Resume || cfg.ResumeID != "" {
		t.Fatalf("bare client --resume: Resume=%v ResumeID=%q", cfg.Resume, cfg.ResumeID)
	}
	if want := "myd --resume daemon-id"; strings.Join(cfg.Exec, " ") != want {
		t.Fatalf("bare form: Exec = %v, want %v", cfg.Exec, want)
	}

	cfg, err = parseArgs([]string{"--resume=sess-9", "--exec", "myd"}, fixedTTY(true), discard())
	if err != nil {
		t.Fatalf("eq form: parse: %v", err)
	}
	if cfg.ResumeID != "sess-9" || cfg.Resume {
		t.Fatalf("eq form: Resume=%v ResumeID=%q, want explicit sess-9", cfg.Resume, cfg.ResumeID)
	}

	cfg, err = parseArgs([]string{"--resume", "sess-7", "--exec", "myd"}, fixedTTY(true), discard())
	if err != nil {
		t.Fatalf("space form: parse: %v", err)
	}
	if cfg.ResumeID != "sess-7" || cfg.Resume {
		t.Fatalf("space form: Resume=%v ResumeID=%q, want explicit sess-7", cfg.Resume, cfg.ResumeID)
	}
	if want := "myd"; strings.Join(cfg.Exec, " ") != want {
		t.Fatalf("space form: Exec = %v, want [myd]", cfg.Exec)
	}
}

// TestResumeEmptyExplicitFormStillUsageError: an explicit EMPTY value
// stays a fail-closed usage error (exit 2) in the CLIENT region, while
// a post-boundary `--resume=` is the daemon's empty value and passes
// through without error.
func TestResumeEmptyExplicitFormStillUsageError(t *testing.T) {
	for _, args := range [][]string{
		{"--prompt", "x", "--resume="},
		{"--resume=", "--exec", "myd"},
	} {
		_, err := parseArgs(args, fixedTTY(false), discard())
		if err == nil || exitCodeFor(err) != 2 {
			t.Fatalf("args %v: explicit empty --resume must be a usage error (exit 2), got %v", args, err)
		}
	}
	cfg, err := parseArgs([]string{"--prompt", "x", "--exec", "myd", "--resume="}, fixedTTY(false), discard())
	if err != nil {
		t.Fatalf("post-boundary --resume= belongs to the daemon, not a client error: %v", err)
	}
	if cfg.Resume || cfg.ResumeID != "" {
		t.Fatalf("post-boundary --resume= must not touch the client posture: Resume=%v ResumeID=%q", cfg.Resume, cfg.ResumeID)
	}
}

// TestResumeEmptySpaceSeparatedFormUsageError (F5 rider): the
// SPACE-SEPARATED empty id (`--resume ""`) is the same usage error as
// the equals form — it used to set id="" silently, turning the flag
// into a no-op instead of exit 2.
func TestResumeEmptySpaceSeparatedFormUsageError(t *testing.T) {
	for _, args := range [][]string{
		{"--prompt", "x", "--resume", ""},
		{"--resume", "", "--exec", "myd"},
	} {
		_, err := parseArgs(args, fixedTTY(false), discard())
		if err == nil || exitCodeFor(err) != 2 {
			t.Fatalf("args %v: space-separated empty --resume must be a usage error (exit 2), got %v", args, err)
		}
	}
	// Post-boundary (daemon region): untouched, verbatim passthrough.
	cfg, err := parseArgs([]string{"--prompt", "x", "--exec", "myd", "--resume", ""}, fixedTTY(false), discard())
	if err != nil {
		t.Fatalf("post-boundary empty --resume belongs to the daemon, not a client error: %v", err)
	}
	if cfg.Resume || cfg.ResumeID != "" {
		t.Fatalf("post-boundary empty --resume must not touch the client posture: Resume=%v ResumeID=%q", cfg.Resume, cfg.ResumeID)
	}
}

// TestResumeConflictingExplicitIDsUsageError (hotfix R6): two explicit
// --resume forms naming DIFFERENT sessions used to last-win silently;
// they are now a fail-closed usage error (exit 2), consistent with the
// bare+explicit conflict. Repeating the SAME id is idempotent and stays
// legal.
func TestResumeConflictingExplicitIDsUsageError(t *testing.T) {
	for _, args := range [][]string{
		{"--prompt", "x", "--resume", "sess-1", "--resume", "sess-2"},
		{"--prompt", "x", "--resume=sess-1", "--resume", "sess-2"},
		{"--prompt", "x", "--resume", "sess-1", "--resume=sess-2"},
		{"--prompt", "x", "--resume=sess-2", "--resume=sess-1"},
	} {
		_, err := parseArgs(args, fixedTTY(false), discard())
		if err == nil || exitCodeFor(err) != 2 {
			t.Fatalf("args %v: conflicting --resume ids must be a usage error (exit 2), got %v", args, err)
		}
	}
	cfg, err := parseArgs([]string{"--prompt", "x", "--resume", "sess-1", "--resume=sess-1"}, fixedTTY(false), discard())
	if err != nil {
		t.Fatalf("repeating the SAME id must stay legal (idempotent): %v", err)
	}
	if cfg.ResumeID != "sess-1" || cfg.Resume {
		t.Fatalf("same-id repeat: Resume=%v ResumeID=%q, want explicit sess-1", cfg.Resume, cfg.ResumeID)
	}
}

func contains(list []string, want string) bool {
	for _, x := range list {
		if x == want {
			return true
		}
	}
	return false
}
