package cli

// Slice 2 — resolver-level capability enforcement.
//
// These tests prove the capability contract holds at the CLI resolver layer,
// not only inside the runtime package:
//
//   - A host-shell manifest resolves to a concrete *runtime.HostShell (NOT a
//     silently-substituted *runtime.DockerCompose). This is the minimal-profile
//     cleanliness proof: host-shell can never reach a docker_compose code path.
//   - runLogs on host-shell returns the typed UnsupportedVerbError end-to-end
//     (no silent host exec, no opaque error).
//   - An unknown backend is rejected by the real resolver (no default
//     substitution to docker_compose or host-shell).
//
// The per-verb matrix values themselves are asserted exhaustively in
// internal/runtime/capability_test.go; here we assert the CLI plumbing honours
// them.

import (
	"errors"
	"testing"

	"github.com/vhqtvn/vh-agent-harness/internal/runtime"
)

// TestResolver_HostShellNotDockerSubstituted proves the minimal (host-shell)
// profile resolves to a concrete host-shell backend and never substitutes a
// docker_compose backend. The capability matrix would make any such leak
// observable, but this test pins the resolver directly.
func TestResolver_HostShellNotDockerSubstituted(t *testing.T) {
	root := t.TempDir()
	writeFixtureManifest(t, root, "host-shell", "")
	defer resetRuntimeDeps(t) // restore default backendFor (real resolver)

	runWithCwd(t, root, func() {
		be, _, err := resolveBackend()
		if err != nil {
			t.Fatalf("resolveBackend host-shell: %v", err)
		}

		// Concrete type must be *runtime.HostShell, never *runtime.DockerCompose.
		switch be.(type) {
		case *runtime.HostShell:
			// expected
		case *runtime.DockerCompose:
			t.Fatalf("host-shell manifest resolved to *runtime.DockerCompose: minimal profile leaked into docker code path")
		default:
			t.Fatalf("host-shell manifest resolved to unexpected backend type %T", be)
		}

		// The resolved backend must report a host-shell capability matrix with
		// exec/shell supported and logs/ps unsupported (D1-C declared surface).
		mat := be.Capabilities()
		if mat.Backend != "host-shell" {
			t.Fatalf("capability matrix backend = %q, want %q", mat.Backend, "host-shell")
		}
		if cap, _, _ := mat.Lookup(runtime.VerbExec); cap != runtime.CapSupported {
			t.Errorf("host-shell Exec capability = %v, want CapSupported", cap)
		}
		if cap, _, _ := mat.Lookup(runtime.VerbShell); cap != runtime.CapSupported {
			t.Errorf("host-shell Shell capability = %v, want CapSupported", cap)
		}
		if cap, _, _ := mat.Lookup(runtime.VerbLogs); cap != runtime.CapUnsupported {
			t.Errorf("host-shell Logs capability = %v, want CapUnsupported", cap)
		}
		if cap, _, _ := mat.Lookup(runtime.VerbPs); cap != runtime.CapUnsupported {
			t.Errorf("host-shell Ps capability = %v, want CapUnsupported", cap)
		}
	})
}

// TestRunLogs_HostShellReturnsTypedUnsupported proves an unsupported verb
// flows through the CLI runLogs path as a typed UnsupportedVerbError, not a
// silent host exec and not an opaque error. This is the Slice 2 acceptance
// criterion "unsupported verb -> typed error, not silent host exec".
func TestRunLogs_HostShellReturnsTypedUnsupported(t *testing.T) {
	root := t.TempDir()
	writeFixtureManifest(t, root, "host-shell", "")
	defer resetRuntimeDeps(t)

	runWithCwd(t, root, func() {
		cmd, _ := newOutCmd()
		err := runLogs(cmd, nil)
		if err == nil {
			t.Fatalf("runLogs host-shell: expected typed unsupported error, got nil")
		}
		if !runtime.IsUnsupportedVerbError(err) {
			t.Fatalf("runLogs host-shell: error is not UnsupportedVerbError: %T %v", err, err)
		}

		var uve *runtime.UnsupportedVerbError
		if !errors.As(err, &uve) {
			t.Fatalf("errors.As to *UnsupportedVerbError failed: %v", err)
		}
		if uve.Backend != "host-shell" {
			t.Errorf("UnsupportedVerbError.Backend = %q, want %q", uve.Backend, "host-shell")
		}
		if uve.Verb != runtime.VerbLogs {
			t.Errorf("UnsupportedVerbError.Verb = %v, want VerbLogs", uve.Verb)
		}
		if uve.Guidance == "" {
			t.Errorf("UnsupportedVerbError.Guidance is empty; typed guidance is required")
		}
	})
}

// TestResolver_RejectsUnknownBackend proves the real resolver (default
// backendFor) rejects an unrecognized backend rather than defaulting to
// docker_compose or host-shell. Selector-level rejection is also covered by
// TestBackendSelection_UnknownBackend; this confirms it via resolveBackend().
func TestResolver_RejectsUnknownBackend(t *testing.T) {
	root := t.TempDir()
	writeFixtureManifest(t, root, "kubernetes", "")
	defer resetRuntimeDeps(t)

	runWithCwd(t, root, func() {
		be, _, err := resolveBackend()
		if err == nil {
			t.Fatalf("resolveBackend unknown backend: expected error, got backend %T", be)
		}
		// Must not have substituted a known backend.
		switch be.(type) {
		case *runtime.DockerCompose:
			t.Fatalf("unknown backend silently resolved to *runtime.DockerCompose")
		case *runtime.HostShell:
			t.Fatalf("unknown backend silently resolved to *runtime.HostShell")
		}
	})
}
