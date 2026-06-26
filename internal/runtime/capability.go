package runtime

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// Slice 2 — Backend Capability / Contract Model (D1-C codified).
//
// This file makes D1-C (capability-scoped first-class host-shell backend) an
// ENFORCED, INTROSPECTABLE CONTRACT rather than scattered conditionals in method
// bodies. Each backend declares a CapabilityMatrix: a single data structure
// mapping every verb (up/down/exec/shell/logs/ps + custom-hook dispatch) to one
// of {Supported, Noop, Unsupported}. Backend methods that are Unsupported
// DERIVE their typed guidance error from that same matrix, so the declaration
// and the runtime behavior cannot drift.
//
// Three properties this enforces (all tested in capability_test.go):
//  1. Introspectable: Backend.Capabilities() returns the matrix; MatrixFor(name)
//     resolves a matrix from a backend name without a manifest.
//  2. Typed-unsupported: an Unsupported verb returns *UnsupportedVerbError
//     (backend + verb + guidance), never a silent fallback, panic, or opaque
//     string. No backend substitutes for another.
//  3. Resolver hardening: MatrixFor() rejects unknown backends with a clear
//     error — it does NOT default to docker_compose and does NOT default to
//     host-shell. The minimal (host-shell) profile therefore cannot reach a
//     docker_compose code path through the matrix.

// Verb is a logical runtime verb the harness dispatches to a backend. Verbs
// correspond to the user-facing subcommands plus custom-hook dispatch; they are
// the units of the capability matrix. Each backend declares, per Verb, one
// Capability (Supported / Noop / Unsupported).
type Verb string

const (
	VerbUp    Verb = "up"
	VerbDown  Verb = "down"
	VerbExec  Verb = "exec"
	VerbShell Verb = "shell"
	VerbLogs  Verb = "logs"
	VerbPs    Verb = "ps"
	// VerbHook is custom-hook dispatch (run-shape verbs.*.kind=hook): the
	// harness fires a project-owned leaf at a fixed lifecycle point. A backend
	// declaring this Supported participates in hook dispatch (the leaf runs on
	// the host for host-shell/bare, or around compose ops for docker_compose).
	VerbHook Verb = "hook"
)

// allVerbs is the fixed, ordered verb set the capability matrix indexes over.
// It is the single place verbs are enumerated; matrices and tests reference
// AllVerbs(). Adding a verb here forces every backend's matrix to declare it
// (TestMatrix_Exhaustive fails if a matrix omits a known verb).
var allVerbs = []Verb{VerbUp, VerbDown, VerbExec, VerbShell, VerbLogs, VerbPs, VerbHook}

// AllVerbs returns a copy of the fixed verb set the matrix is defined over.
// Exported for introspection and tests.
func AllVerbs() []Verb {
	out := make([]Verb, len(allVerbs))
	copy(out, allVerbs)
	return out
}

// Capability is the declared support level of a verb on a backend.
type Capability int

const (
	// CapUnsupported is the zero value ON PURPOSE: an undeclared (zero-value)
	// capability fails closed as Unsupported, so forgetting a declaration can
	// never silently succeed.
	CapUnsupported Capability = iota
	CapSupported
	// CapNoop means the verb is a deliberate no-op: it succeeds (returns nil)
	// and prints guidance so the operator knows the capability boundary rather
	// than seeing silent success. Used by host-shell/bare up/down.
	CapNoop
)

// String renders the capability for diagnostics and table-driven test output.
func (c Capability) String() string {
	switch c {
	case CapSupported:
		return "supported"
	case CapNoop:
		return "noop"
	default:
		return "unsupported"
	}
}

// VerbEntry binds a verb to its declared capability plus the human guidance
// returned when the verb is invoked. Guidance is most relevant for Unsupported
// (it is the body of the typed error) and Noop (it is printed on success); for
// Supported verbs it is a short note and may be empty.
type VerbEntry struct {
	Verb     Verb
	Cap      Capability
	Guidance string
}

// CapabilityMatrix is a backend's first-class, introspectable declaration of
// which verbs it supports. It is DATA, not behavior: each concrete backend
// returns one from Backend.Capabilities(), and MatrixFor(name) resolves one by
// backend name. The matrix is the single source of truth for "what this backend
// can do"; backend methods that are Unsupported derive their typed error from it
// via UnsupportedError().
type CapabilityMatrix struct {
	// Backend is the stable backend identifier (matches Backend.Name()).
	Backend string
	// Entries is the ordered verb → capability + guidance declaration. It SHOULD
	// be exhaustive over AllVerbs() (TestMatrix_Exhaustive enforces this).
	Entries []VerbEntry
}

// Lookup returns the declared capability and guidance for verb. If verb is not
// declared at all, it returns (CapUnsupported, "", false) — an undeclared verb
// is treated as Unsupported with no guidance (fail closed).
func (m CapabilityMatrix) Lookup(v Verb) (cap Capability, guidance string, ok bool) {
	for _, e := range m.Entries {
		if e.Verb == v {
			return e.Cap, e.Guidance, true
		}
	}
	return CapUnsupported, "", false
}

// UnsupportedError returns the typed *UnsupportedVerbError for verb, derived
// from this matrix. It is what Unsupported backend methods return, so the
// declaration and the runtime behavior share one source. If verb is not in the
// matrix, guidance falls back to a fail-closed message.
func (m CapabilityMatrix) UnsupportedError(v Verb) *UnsupportedVerbError {
	_, guidance, declared := m.Lookup(v)
	if !declared {
		guidance = "verb not declared by this backend's capability matrix"
	}
	return &UnsupportedVerbError{Backend: m.Backend, Verb: v, Guidance: guidance}
}

// UnsupportedVerbError is the typed error returned when a verb is invoked on a
// backend that declares it CapUnsupported. It carries the backend name, the
// verb, and human guidance. It is a distinct type so callers can detect
// "unsupported" via IsUnsupportedVerbError and branch on guidance, rather than
// parsing an opaque error string. It is NEVER used to silently fall back to
// another backend — the resolver rejects unknown backends and no backend
// substitutes for another.
type UnsupportedVerbError struct {
	Backend  string
	Verb     Verb
	Guidance string
}

// Error implements error. The message names the backend, the verb, and carries
// the guidance, so an unsupported verb is always a clear, typed signal.
func (e *UnsupportedVerbError) Error() string {
	return fmt.Sprintf("runtime backend %q does not support verb %q: %s", e.Backend, e.Verb, e.Guidance)
}

// IsUnsupportedVerbError reports whether err is (or wraps) an
// *UnsupportedVerbError.
func IsUnsupportedVerbError(err error) bool {
	var uve *UnsupportedVerbError
	return errors.As(err, &uve)
}

// CheckVerb consults be's declared capability matrix and returns the typed
// *UnsupportedVerbError if verb is Unsupported; nil otherwise (Supported/Noop).
// It lets a dispatch layer enforce the contract BEFORE touching the backend
// method, and is the manifest-independent enforcement entry point. The concrete
// backend methods ALSO enforce Unsupported independently (defense in depth) by
// deriving the same typed error from the same matrix.
func CheckVerb(be Backend, v Verb) error {
	m := be.Capabilities()
	cap, _, _ := m.Lookup(v)
	if cap == CapUnsupported {
		return m.UnsupportedError(v)
	}
	return nil
}

// --- Per-backend matrix declarations (single source of truth) ---------------

// knownMatrices is the runtime-level capability registry: backend name → its
// declared CapabilityMatrix. It is the runtime's own assertion of what each
// backend can do, independent of any manifest. MatrixFor(name) uses it to
// reject unknown backends with a clear error (no default substitution).
var knownMatrices = map[string]CapabilityMatrix{
	"host-shell":     hostShellMatrix(),
	"docker_compose": dockerComposeMatrix(),
	"bare":           bareMatrix(),
	"proxy":          proxyMatrix(),
}

// KnownBackends returns the sorted, stable set of backend names the runtime
// declares a capability matrix for. It is the authority on which backend
// identifiers are valid (used by MatrixFor's error message and by tests).
func KnownBackends() []string {
	names := make([]string, 0, len(knownMatrices))
	for k := range knownMatrices {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

// MatrixFor returns the declared capability matrix for name. It is the
// runtime-level resolver hardening: an unknown/unrecognized backend yields a
// clear error and there is NO default — it does not default to docker_compose
// and does not default to host-shell. Callers MUST handle the error; there is no
// silent substitution. This complements the CLI-level backendSelector (which
// rejects unknown backends at manifest-load time) with a manifest-independent
// resolver.
func MatrixFor(name string) (CapabilityMatrix, error) {
	m, ok := knownMatrices[name]
	if !ok {
		return CapabilityMatrix{}, fmt.Errorf(
			"unknown runtime backend %q (known: %s); refusing to default to any backend",
			name, strings.Join(KnownBackends(), ", "),
		)
	}
	// Return a shallow copy so callers cannot mutate the registry's matrix.
	out := m
	out.Entries = append([]VerbEntry(nil), m.Entries...)
	return out, nil
}

// hostShellMatrix is the D1-C capability surface for host-shell: a first-class,
// capability-scoped backend. exec/shell/hook WORK on the host (the real value);
// up/down are deliberate Noop-with-guidance (no services to manage); logs/ps are
// typed-Unsupported (no service observability). This MUST match the actual
// HostShell method behavior — asserted by TestCapabilityMatrix_DeclaredMatchesBehavior.
func hostShellMatrix() CapabilityMatrix {
	return CapabilityMatrix{
		Backend: "host-shell",
		Entries: []VerbEntry{
			{Verb: VerbUp, Cap: CapNoop, Guidance: "host-shell manages no services; nothing to start (commands run directly on the host)"},
			{Verb: VerbDown, Cap: CapNoop, Guidance: "host-shell manages no services; nothing to stop (commands run directly on the host)"},
			{Verb: VerbExec, Cap: CapSupported, Guidance: "commands run directly on the host"},
			{Verb: VerbShell, Cap: CapSupported, Guidance: "interactive host shell"},
			{Verb: VerbLogs, Cap: CapUnsupported, Guidance: "host-shell backend does not manage services and has no logs; declare a lifecycle hook (run-shape.yml) or use runtime.backend=docker_compose/proxy for service logs"},
			{Verb: VerbPs, Cap: CapUnsupported, Guidance: "host-shell backend does not manage services and has no ps; declare a lifecycle hook (run-shape.yml) or use runtime.backend=docker_compose/proxy for service status"},
			{Verb: VerbHook, Cap: CapSupported, Guidance: "custom-hook leaves run on the host (project-owned .vh-agent-harness/scripts/*.sh)"},
		},
	}
}

// dockerComposeMatrix is the full service-capable surface: every verb is
// Supported. docker_compose never returns an Unsupported error for a core verb.
func dockerComposeMatrix() CapabilityMatrix {
	return CapabilityMatrix{
		Backend: "docker_compose",
		Entries: []VerbEntry{
			{Verb: VerbUp, Cap: CapSupported, Guidance: "docker compose up -d"},
			{Verb: VerbDown, Cap: CapSupported, Guidance: "docker compose down"},
			{Verb: VerbExec, Cap: CapSupported, Guidance: "docker compose exec <service> ..."},
			{Verb: VerbShell, Cap: CapSupported, Guidance: "docker compose exec <service> (interactive)"},
			{Verb: VerbLogs, Cap: CapSupported, Guidance: "docker compose logs [--follow] [service]"},
			{Verb: VerbPs, Cap: CapSupported, Guidance: "docker compose ps"},
			{Verb: VerbHook, Cap: CapSupported, Guidance: "custom-hook leaves run around compose ops (project-owned .vh-agent-harness/scripts/*.sh)"},
		},
	}
}

// bareMatrix is bare's honest surface: exec/shell/hook run on the host (no
// isolation, with a loud warning); up/down are deliberate Noop-with-warning;
// logs/ps are typed-Unsupported (no managed services). bare is a real backend,
// never a silent substitute for docker_compose.
func bareMatrix() CapabilityMatrix {
	return CapabilityMatrix{
		Backend: "bare",
		Entries: []VerbEntry{
			{Verb: VerbUp, Cap: CapNoop, Guidance: "bare has no managed services; nothing to start (commands run directly on the host, no isolation)"},
			{Verb: VerbDown, Cap: CapNoop, Guidance: "bare has no managed services; nothing to stop (commands run directly on the host, no isolation)"},
			{Verb: VerbExec, Cap: CapSupported, Guidance: "commands run directly on the host (no isolation)"},
			{Verb: VerbShell, Cap: CapSupported, Guidance: "interactive host shell (no isolation)"},
			{Verb: VerbLogs, Cap: CapUnsupported, Guidance: "bare backend has no managed services to log; use runtime.backend=docker_compose for service logs"},
			{Verb: VerbPs, Cap: CapUnsupported, Guidance: "bare backend has no managed services to list; use runtime.backend=docker_compose for service status"},
			{Verb: VerbHook, Cap: CapSupported, Guidance: "custom-hook leaves run on the host (project-owned .vh-agent-harness/scripts/*.sh)"},
		},
	}
}

// proxyMatrix is the capability matrix for the proxy backend: exec/shell are
// delegated to the project wrapper command (Supported), service lifecycle is
// owned by that wrapper (up/down Noop), and the harness does not observe
// services (logs/ps Unsupported). The shell-guard gate still runs before any
// delegate is invoked (enforced in the cli layer), so proxy is gate-equivalent
// to the other backends.
func proxyMatrix() CapabilityMatrix {
	return CapabilityMatrix{
		Backend: "proxy",
		Entries: []VerbEntry{
			{Verb: VerbUp, Cap: CapNoop, Guidance: "proxy delegates to the project wrapper; run its own up (e.g. `./dev.sh up`)"},
			{Verb: VerbDown, Cap: CapNoop, Guidance: "proxy delegates to the project wrapper; run its own down (e.g. `./dev.sh down`)"},
			{Verb: VerbExec, Cap: CapSupported, Guidance: "delegates to runtime.proxy_command + the user command (e.g. `./dev.sh exec ...`)"},
			{Verb: VerbShell, Cap: CapSupported, Guidance: "delegates to runtime.proxy_command with $SHELL (e.g. `./dev.sh exec /bin/bash`)"},
			{Verb: VerbLogs, Cap: CapUnsupported, Guidance: "proxy does not observe services; run the project wrapper's logs (e.g. `./dev.sh logs`)"},
			{Verb: VerbPs, Cap: CapUnsupported, Guidance: "proxy does not observe services; run the project wrapper's status (e.g. `./dev.sh status`)"},
			{Verb: VerbHook, Cap: CapSupported, Guidance: "custom-hook leaves run on the host (project-owned .vh-agent-harness/scripts/*.sh)"},
		},
	}
}
