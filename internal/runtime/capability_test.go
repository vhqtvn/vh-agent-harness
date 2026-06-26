package runtime

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// Slice 2 — capability-matrix tests. These assert the matrix is an enforced,
// introspectable contract (not scattered conditionals): the declared surface,
// typed-unsupported enforcement, resolver hardening, matrix↔behavior
// consistency, and minimal-profile cleanliness (host-shell never reaches docker).

// expectedMatrix is the per-backend × per-verb table the prototype declares. It
// is the literal acceptance matrix from D1-C and is asserted verbatim against
// each backend's Capabilities() so the declaration cannot drift unnoticed.
var expectedMatrix = map[string]map[Verb]Capability{
	"host-shell": {
		VerbUp: CapNoop, VerbDown: CapNoop,
		VerbExec: CapSupported, VerbShell: CapSupported,
		VerbLogs: CapUnsupported, VerbPs: CapUnsupported,
		VerbHook: CapSupported,
	},
	"docker_compose": {
		VerbUp: CapSupported, VerbDown: CapSupported,
		VerbExec: CapSupported, VerbShell: CapSupported,
		VerbLogs: CapSupported, VerbPs: CapSupported,
		VerbHook: CapSupported,
	},
	"bare": {
		VerbUp: CapNoop, VerbDown: CapNoop,
		VerbExec: CapSupported, VerbShell: CapSupported,
		VerbLogs: CapUnsupported, VerbPs: CapUnsupported,
		VerbHook: CapSupported,
	},
}

// TestMatrix_PerBackend asserts each backend's declared Capabilities() matches
// the expected D1-C matrix exactly, for every verb. This reproduces the matrix
// as built and locks the declared surface.
func TestMatrix_PerBackend(t *testing.T) {
	backends := map[string]Backend{
		"host-shell":     &HostShell{Cfg: HostShellConfig{Dir: "/p"}, Runner: newFakeRunner()},
		"docker_compose": newTestDC(),
		"bare":           &Bare{Cfg: BareConfig{Dir: "/p"}, Runner: newFakeRunner()},
	}
	for name, be := range backends {
		if be.Name() != name {
			t.Errorf("backend %q Name() = %q", name, be.Name())
			continue
		}
		m := be.Capabilities()
		if m.Backend != name {
			t.Errorf("backend %q matrix.Backend = %q", name, m.Backend)
		}
		exp, ok := expectedMatrix[name]
		if !ok {
			t.Fatalf("no expected matrix entry for backend %q (test table drift)", name)
		}
		for _, v := range AllVerbs() {
			gotCap, _, gotOK := m.Lookup(v)
			if !gotOK {
				t.Errorf("backend %q: verb %q not declared in matrix (must be exhaustive)", name, v)
				continue
			}
			if want := exp[v]; gotCap != want {
				t.Errorf("backend %q verb %q: declared %s, want %s", name, v, gotCap, want)
			}
		}
	}
}

// TestMatrix_Exhaustive asserts every known backend's matrix declares exactly
// the AllVerbs() set: no missing verb (which would silently fail-closed) and no
// phantom/typo verb. Adding a verb to AllVerbs without declaring it on every
// backend fails here.
func TestMatrix_Exhaustive(t *testing.T) {
	want := map[Verb]bool{}
	for _, v := range AllVerbs() {
		want[v] = true
	}
	for _, name := range KnownBackends() {
		m, err := MatrixFor(name)
		if err != nil {
			t.Fatalf("MatrixFor(%q): %v", name, err)
		}
		got := map[Verb]bool{}
		for _, e := range m.Entries {
			if !want[e.Verb] {
				t.Errorf("backend %q matrix declares phantom verb %q (not in AllVerbs)", name, e.Verb)
			}
			if got[e.Verb] {
				t.Errorf("backend %q matrix declares verb %q twice", name, e.Verb)
			}
			got[e.Verb] = true
		}
		for v := range want {
			if !got[v] {
				t.Errorf("backend %q matrix missing verb %q", name, v)
			}
		}
	}
}

// TestMatrixFor_RejectsUnknown is the resolver-hardening test: unknown and empty
// backend names yield a clear error and MatrixFor does NOT default to
// docker_compose or host-shell (no silent substitution).
func TestMatrixFor_RejectsUnknown(t *testing.T) {
	for _, bad := range []string{"kubernetes", "", "nvidia-docker", "DOCKER_COMPOSE", "host_shell"} {
		if _, err := MatrixFor(bad); err == nil {
			t.Errorf("MatrixFor(%q): expected error for unknown backend, got nil", bad)
		}
	}
	// Known backends resolve to their matrix with the right Backend name.
	for _, name := range KnownBackends() {
		m, err := MatrixFor(name)
		if err != nil {
			t.Errorf("MatrixFor(%q): %v", name, err)
			continue
		}
		if m.Backend != name {
			t.Errorf("MatrixFor(%q).Backend = %q", name, m.Backend)
		}
	}
	// The error must name the unknown backend and the known set.
	_, err := MatrixFor("kubernetes")
	if err == nil || !strings.Contains(err.Error(), "kubernetes") || !strings.Contains(err.Error(), "known") {
		t.Errorf("unknown-backend error should name backend + known set; got %v", err)
	}
}

// TestUnsupportedVerbError_Typed asserts Unsupported verbs return the typed
// *UnsupportedVerbError (not an opaque string), carrying the backend name and
// verb. Also asserts Supported verbs on docker_compose do NOT yield an
// unsupported error (probe stubbed to pass so the method itself succeeds).
func TestUnsupportedVerbError_Typed(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name string
		be   Backend
		verb Verb
		call func(Backend) error
	}{
		{"host-shell/logs", &HostShell{Cfg: HostShellConfig{}, Runner: newFakeRunner()}, VerbLogs, func(b Backend) error { return b.Logs(ctx, "", false) }},
		{"host-shell/ps", &HostShell{Cfg: HostShellConfig{}, Runner: newFakeRunner()}, VerbPs, func(b Backend) error { _, e := b.Ps(ctx); return e }},
		{"bare/logs", &Bare{Cfg: BareConfig{}, Runner: newFakeRunner()}, VerbLogs, func(b Backend) error { return b.Logs(ctx, "", false) }},
		{"bare/ps", &Bare{Cfg: BareConfig{}, Runner: newFakeRunner()}, VerbPs, func(b Backend) error { _, e := b.Ps(ctx); return e }},
	}
	for _, c := range cases {
		err := c.call(c.be)
		if err == nil {
			t.Errorf("%s: expected typed unsupported error, got nil", c.name)
			continue
		}
		if !IsUnsupportedVerbError(err) {
			t.Errorf("%s: error is not *UnsupportedVerbError; got %T: %v", c.name, err, err)
			continue
		}
		var uve *UnsupportedVerbError
		if !errors.As(err, &uve) {
			t.Errorf("%s: errors.As to *UnsupportedVerbError failed", c.name)
			continue
		}
		if uve.Backend != c.be.Name() {
			t.Errorf("%s: typed error Backend = %q, want %q", c.name, uve.Backend, c.be.Name())
		}
		if uve.Verb != c.verb {
			t.Errorf("%s: typed error Verb = %q, want %q", c.name, uve.Verb, c.verb)
		}
		if uve.Guidance == "" {
			t.Errorf("%s: typed error Guidance is empty", c.name)
		}
	}

	// docker_compose logs/ps are Supported — with a reachable daemon they must
	// NOT return an unsupported error.
	dc := newTestDC()
	if err := dc.Logs(ctx, "", false); IsUnsupportedVerbError(err) {
		t.Errorf("docker_compose logs: must not be unsupported; got %v", err)
	}
	if _, err := dc.Ps(ctx); IsUnsupportedVerbError(err) {
		t.Errorf("docker_compose ps: must not be unsupported; got %v", err)
	}
}

// TestCapabilityMatrix_DeclaredMatchesBehavior is the core contract-consistency
// test: for each backend, the matrix's declared capability for {up, down, logs,
// ps} must match the method's actual behavior. Unsupported → typed error; else
// → not an unsupported error. This proves the declaration is honest (methods
// derive from the matrix rather than contradicting it).
func TestCapabilityMatrix_DeclaredMatchesBehavior(t *testing.T) {
	ctx := context.Background()
	mkBackends := func() map[string]Backend {
		return map[string]Backend{
			"host-shell":     &HostShell{Cfg: HostShellConfig{}, Runner: newFakeRunner()},
			"docker_compose": newTestDC(), // probe stubbed to pass
			"bare":           &Bare{Cfg: BareConfig{}, Runner: newFakeRunner()},
		}
	}
	// call invokes a verb's method and returns its error (noop/supported happy
	// path or unsupported typed error). exec/shell/hook are all Supported across
	// every backend, so the contract surface reduces to up/down/logs/ps.
	call := func(be Backend, v Verb) error {
		switch v {
		case VerbUp:
			return be.Up(ctx)
		case VerbDown:
			return be.Down(ctx)
		case VerbLogs:
			return be.Logs(ctx, "", false)
		case VerbPs:
			_, e := be.Ps(ctx)
			return e
		}
		return nil
	}
	for name, be := range mkBackends() {
		m := be.Capabilities()
		for _, v := range []Verb{VerbUp, VerbDown, VerbLogs, VerbPs} {
			declared, _, _ := m.Lookup(v)
			err := call(be, v)
			switch declared {
			case CapUnsupported:
				if !IsUnsupportedVerbError(err) {
					t.Errorf("backend %q verb %q declared Unsupported but method returned non-typed error: %v", name, v, err)
				}
			case CapSupported, CapNoop:
				if IsUnsupportedVerbError(err) {
					t.Errorf("backend %q verb %q declared %s but method returned an unsupported error: %v", name, v, declared, err)
				}
			}
		}
	}
}

// TestHostShell_NeverInvokesDocker proves minimal-profile cleanliness: running
// EVERY host-shell verb through a recording runner never issues a "docker"
// command. The capability matrix makes host-shell logs/ps Unsupported (typed
// error before any docker path), and exec/up/down run on the host directly. The
// minimal (host-shell) profile therefore cannot reach a docker_compose code
// path.
func TestHostShell_NeverInvokesDocker(t *testing.T) {
	fr := newFakeRunner()
	h := &HostShell{Cfg: HostShellConfig{Dir: "/proj"}, Runner: fr}
	ctx := context.Background()

	// Exercise every host-shell verb. Errors from Logs/Ps (typed unsupported)
	// are expected; Up/Down/Exec/Healthcheck must succeed.
	if err := h.Up(ctx); err != nil {
		t.Fatalf("Up: %v", err)
	}
	if err := h.Down(ctx); err != nil {
		t.Fatalf("Down: %v", err)
	}
	if err := h.Exec(ctx, []string{"echo", "ok"}, ExecOpts{}); err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if err := h.Healthcheck(ctx); err != nil {
		t.Fatalf("Healthcheck: %v", err)
	}
	if err := h.Logs(ctx, "", false); !IsUnsupportedVerbError(err) {
		t.Fatalf("Logs: expected typed unsupported error, got %v", err)
	}
	if _, err := h.Ps(ctx); !IsUnsupportedVerbError(err) {
		t.Fatalf("Ps: expected typed unsupported error, got %v", err)
	}

	// Critical: no recorded runner call may touch docker.
	for _, c := range fr.calls {
		if c.name == "docker" {
			t.Errorf("host-shell must NEVER invoke docker (minimal-profile cleanliness); saw call name=%q args=%v", c.name, c.args)
		}
	}
	// And exec/shell/logs/ps must be declared in a way that cannot reach docker.
	m := h.Capabilities()
	for _, v := range []Verb{VerbLogs, VerbPs} {
		if cap, _, _ := m.Lookup(v); cap != CapUnsupported {
			t.Errorf("host-shell verb %q must be Unsupported (cannot reach docker); declared %s", v, cap)
		}
	}
}

// TestCheckVerb asserts the manifest-independent enforcement helper returns the
// typed error for Unsupported verbs and nil otherwise. This is the dispatch-
// layer entry point that lets a caller enforce the contract before touching the
// backend method.
func TestCheckVerb(t *testing.T) {
	hostShell := &HostShell{Cfg: HostShellConfig{}, Runner: newFakeRunner()}
	dc := newTestDC()
	bare := &Bare{Cfg: BareConfig{}, Runner: newFakeRunner()}

	// Unsupported verbs → typed error.
	for _, c := range []struct {
		be   Backend
		verb Verb
	}{
		{hostShell, VerbLogs}, {hostShell, VerbPs},
		{bare, VerbLogs}, {bare, VerbPs},
	} {
		err := CheckVerb(c.be, c.verb)
		if !IsUnsupportedVerbError(err) {
			t.Errorf("CheckVerb(%s, %s): expected typed unsupported error, got %v", c.be.Name(), c.verb, err)
		}
	}
	// Supported/noop verbs → nil.
	for _, c := range []struct {
		be   Backend
		verb Verb
	}{
		{hostShell, VerbExec}, {hostShell, VerbShell}, {hostShell, VerbHook},
		{hostShell, VerbUp}, {hostShell, VerbDown},
		{dc, VerbLogs}, {dc, VerbPs}, {dc, VerbUp}, {dc, VerbExec},
		{bare, VerbExec}, {bare, VerbUp},
	} {
		if err := CheckVerb(c.be, c.verb); err != nil {
			t.Errorf("CheckVerb(%s, %s): expected nil, got %v", c.be.Name(), c.verb, err)
		}
	}
}
