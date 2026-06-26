package ownership

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

// --- helpers ----------------------------------------------------------------

// defRule / ovRule are test-local input rows for the ModuleDefaults / Overrides
// builders, so call sites read as plain value literals instead of fighting
// anonymous-struct field types.
type defRule struct {
	path, provenance string
	class            Class
}

type ovRule struct {
	path, reason string
	class        Class
}

func defaultsFrom(rules ...defRule) ModuleDefaults {
	m := ModuleDefaults{}
	for _, r := range rules {
		m[r.path] = PathRule{Class: r.class, Provenance: r.provenance}
	}
	return m
}

func overridesFrom(rules ...ovRule) Overrides {
	m := Overrides{}
	for _, r := range rules {
		m[r.path] = Override{Class: r.class, Reason: r.reason}
	}
	return m
}

// --- Resolve: accepted raises ----------------------------------------------

// TestResolve_BriefRaiseTable runs the brief's explicit accepted (raise)
// transitions through Resolve and confirms the effective map + Origin.
func TestResolve_BriefRaiseTable(t *testing.T) {
	cases := []struct {
		name string
		from Class
		to   Class
		path string
	}{
		{"platform_managed->platform_armed", ClassPlatformManaged, ClassPlatformArmed, ".opencode/scripts/commit-gate.sh"},
		{"platform_managed->project_owned", ClassPlatformManaged, ClassProjectOwned, ".opencode/agents/researcher.md"},
		{"overlay_extension->project_owned", ClassOverlayExtension, ClassProjectOwned, "opencode.jsonc"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			def := defaultsFrom(defRule{path: c.path, class: c.from, provenance: "core.test"})
			ov := overridesFrom(ovRule{path: c.path, class: c.to, reason: "project raise"})
			eff, err := Resolve(def, ov)
			if err != nil {
				t.Fatalf("Resolve err = %v, want nil (raise accepted)", err)
			}
			got, ok := eff.ClassOf(c.path)
			if !ok {
				t.Fatalf("path %q missing from effective map", c.path)
			}
			if got != c.to {
				t.Errorf("effective class = %q, want %q (project-wins-on-raise)", got, c.to)
			}
			if e := eff[c.path]; e.Origin != OriginOverrideRaise {
				t.Errorf("Origin = %q, want override-raise", e.Origin)
			}
			if e := eff[c.path]; e.Provenance != "core.test" {
				t.Errorf("Provenance = %q, want inherited 'core.test'", e.Provenance)
			}
		})
	}
}

// TestResolve_SameClassNoopAccepted: an override setting the same class as the
// default is accepted as a benign no-op (protection unchanged).
func TestResolve_SameClassNoopAccepted(t *testing.T) {
	def := defaultsFrom(defRule{path: "AGENTS.md", class: ClassProjectOwned, provenance: "project.policy"})
	ov := overridesFrom(ovRule{path: "AGENTS.md", class: ClassProjectOwned, reason: "explicitly reaffirming"})
	eff, err := Resolve(def, ov)
	if err != nil {
		t.Fatalf("Resolve err = %v, want nil (same-class no-op accepted)", err)
	}
	if got, _ := eff.ClassOf("AGENTS.md"); got != ClassProjectOwned {
		t.Errorf("class = %q, want project_owned", got)
	}
	if e := eff["AGENTS.md"]; e.Origin != OriginOverrideNoop {
		t.Errorf("Origin = %q, want override-noop", e.Origin)
	}
}

// TestResolve_DefaultsOnly: no overrides -> every path at its default.
func TestResolve_DefaultsOnly(t *testing.T) {
	def := defaultsFrom(
		defRule{path: ".opencode/agents/researcher.md", class: ClassPlatformManaged, provenance: "core.research_workflow"},
		defRule{path: "AGENTS.md", class: ClassProjectOwned, provenance: "project.policy"},
	)
	eff, err := Resolve(def, nil)
	if err != nil {
		t.Fatalf("Resolve err = %v, want nil", err)
	}
	if got, _ := eff.ClassOf(".opencode/agents/researcher.md"); got != ClassPlatformManaged {
		t.Errorf("researcher default = %q, want platform_managed", got)
	}
	if e := eff["AGENTS.md"]; e.Origin != OriginDefault {
		t.Errorf("Origin = %q, want default", e.Origin)
	}
}

// --- Resolve: rejected downgrades ------------------------------------------

// TestResolve_BriefDowngradeTable runs the brief's explicit rejected
// (downgrade) transitions through Resolve and confirms a typed DowngradeError.
func TestResolve_BriefDowngradeTable(t *testing.T) {
	cases := []struct {
		name string
		from Class
		to   Class
		path string
	}{
		{"project_owned->platform_managed", ClassProjectOwned, ClassPlatformManaged, "AGENTS.md"},
		{"platform_armed->platform_managed", ClassPlatformArmed, ClassPlatformManaged, ".opencode/scripts/commit-gate.sh"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			def := defaultsFrom(defRule{path: c.path, class: c.from, provenance: "core.test"})
			ov := overridesFrom(ovRule{path: c.path, class: c.to, reason: "careless downgrade"})
			eff, err := Resolve(def, ov)
			if err == nil {
				t.Fatalf("Resolve err = nil, want a DowngradeError")
			}
			var de *DowngradeError
			if !errors.As(err, &de) {
				t.Fatalf("err must be *DowngradeError; got %T: %v", err, err)
			}
			if de.Path != c.path {
				t.Errorf("Path = %q, want %q", de.Path, c.path)
			}
			if de.From != c.from || de.To != c.to {
				t.Errorf("From/To = %s -> %s, want %s -> %s", de.From, de.To, c.from, c.to)
			}
			// The path must remain at its higher-protection default in the
			// partial effective map (downgrade is not partially applied).
			if got, _ := eff.ClassOf(c.path); got != c.from {
				t.Errorf("partial effective class = %q, want unchanged default %q", got, c.from)
			}
		})
	}
}

// TestDowngradeError_MessageAndGuidance reproduces the error shape + message +
// guidance an operator sees, and confirms it points to the scary workflow.
func TestDowngradeError_MessageAndGuidance(t *testing.T) {
	de := &DowngradeError{
		Path:   "AGENTS.md",
		From:   ClassProjectOwned,
		To:     ClassPlatformManaged,
		Reason: "protection lowered on the raise/lower lattice",
	}
	msg := de.Error()
	for _, want := range []string{
		`AGENTS.md`,
		`project_owned`,
		`platform_managed`,
		`downgrade rejected`,
		`raise-only (D2-A)`,
		`harness ownership downgrade --path`,
		`vh-agent-harness update --propose`,
		`Not implemented in v0`,
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("DowngradeError message missing %q\nfull message:\n%s", want, msg)
		}
	}
}

// TestDowngradeError_DetectableViaWrapAndJoin confirms errors.As / IsDowngradeError
// still match when the error is wrapped or joined with others.
func TestDowngradeError_DetectableViaWrapAndJoin(t *testing.T) {
	de := &DowngradeError{Path: "x", From: ClassProjectOwned, To: ClassPlatformManaged}
	// Wrapped.
	wrapped := fmt.Errorf("resolve failed: %w", de)
	if !IsDowngradeError(wrapped) {
		t.Error("IsDowngradeError(wrapped) = false, want true")
	}
	// Joined with unrelated errors.
	joined := errors.Join(
		errors.New("unrelated"),
		de,
		&UnknownPathError{Path: "y"},
	)
	if !IsDowngradeError(joined) {
		t.Error("IsDowngradeError(joined) = false, want true")
	}
	if !IsUnknownPathError(joined) {
		t.Error("IsUnknownPathError(joined) = false, want true (multi-error traversal)")
	}
}

// --- Resolve: unknown-path fail-closed -------------------------------------

// TestResolve_UnknownPathFailClosed: an override for a path with NO platform
// default is rejected (fail closed), never silently inventing a class.
func TestResolve_UnknownPathFailClosed(t *testing.T) {
	def := defaultsFrom(defRule{path: ".opencode/agents/researcher.md", class: ClassPlatformManaged, provenance: "core.research_workflow"})
	ov := overridesFrom(ovRule{path: ".opencode/agents/never-declared.md", class: ClassProjectOwned, reason: "inventing a path"})
	_, err := Resolve(def, ov)
	if err == nil {
		t.Fatal("Resolve err = nil, want UnknownPathError")
	}
	var u *UnknownPathError
	if !errors.As(err, &u) {
		t.Fatalf("err must be *UnknownPathError; got %T: %v", err, err)
	}
	if u.Path != ".opencode/agents/never-declared.md" {
		t.Errorf("Path = %q, want the unknown override path", u.Path)
	}
	if !IsUnknownPathError(err) {
		t.Error("IsUnknownPathError = false, want true")
	}
}

// --- Resolve: off-lattice fail-closed --------------------------------------

// TestResolve_OffLatticeFailClosed: overrides touching external_generated or
// local_only (as from OR to) are rejected with NotHandOverridableError.
func TestResolve_OffLatticeFailClosed(t *testing.T) {
	t.Run("override_external_generated_path", func(t *testing.T) {
		// A path the platform declares external_generated cannot be hand-overridden,
		// even toward project_owned.
		def := defaultsFrom(defRule{path: ".opencode/skills/acme-cockpit/skill.md", class: ClassExternalGenerated, provenance: "provider.acme-cockpit"})
		ov := overridesFrom(ovRule{path: ".opencode/skills/acme-cockpit/skill.md", class: ClassProjectOwned, reason: "trying to claim it"})
		_, err := Resolve(def, ov)
		if err == nil {
			t.Fatal("Resolve err = nil, want NotHandOverridableError")
		}
		if !IsNotHandOverridableError(err) {
			t.Fatalf("err must be NotHandOverridableError; got %T: %v", err, err)
		}
	})
	t.Run("override_local_only_path", func(t *testing.T) {
		def := defaultsFrom(defRule{path: ".local/state.bin", class: ClassLocalOnly, provenance: "core.local_state"})
		ov := overridesFrom(ovRule{path: ".local/state.bin", class: ClassProjectOwned, reason: "trying to raise local state"})
		_, err := Resolve(def, ov)
		if !IsNotHandOverridableError(err) {
			t.Fatalf("override on local_only path must be NotHandOverridableError; got %v", err)
		}
	})
	t.Run("override_to_off_lattice_class", func(t *testing.T) {
		// Trying to SET a path to external_generated/local_only by hand is also
		// rejected (you cannot hand-set an off-lattice class).
		def := defaultsFrom(defRule{path: ".opencode/agents/researcher.md", class: ClassPlatformManaged, provenance: "core.research_workflow"})
		ov := overridesFrom(ovRule{path: ".opencode/agents/researcher.md", class: ClassLocalOnly, reason: "trying to hide it"})
		_, err := Resolve(def, ov)
		if !IsNotHandOverridableError(err) {
			t.Fatalf("override TO off-lattice class must be NotHandOverridableError; got %v", err)
		}
	})
}

// --- Resolve: invalid class literal -----------------------------------------

// TestResolve_InvalidOverrideClass: a garbage class literal in an override is
// rejected before any lattice reasoning.
func TestResolve_InvalidOverrideClass(t *testing.T) {
	def := defaultsFrom(defRule{path: ".opencode/agents/researcher.md", class: ClassPlatformManaged, provenance: "core.research_workflow"})
	ov := Overrides{".opencode/agents/researcher.md": Override{Class: Class("totally_made_up")}}
	_, err := Resolve(def, ov)
	var ice *InvalidClassError
	if !errors.As(err, &ice) {
		t.Fatalf("err must be *InvalidClassError; got %T: %v", err, err)
	}
	if !strings.Contains(err.Error(), "totally_made_up") {
		t.Errorf("error should name the bad literal; got: %v", err)
	}
}

// TestResolve_InvalidDefaultClassAborts: an invalid class in the PLATFORM
// DEFAULTS aborts immediately (corrupted upstream input, not partially honored).
func TestResolve_InvalidDefaultClassAborts(t *testing.T) {
	def := ModuleDefaults{
		"x": PathRule{Class: Class("bogus")},
	}
	_, err := Resolve(def, nil)
	var ice *InvalidClassError
	if !errors.As(err, &ice) {
		t.Fatalf("err must be *InvalidClassError; got %T: %v", err, err)
	}
}

// --- Resolve: multi-violation join -----------------------------------------

// TestResolve_MultipleViolationsJoined: several bad overrides in one pass all
// surface, each reachable via errors.As on the joined error.
func TestResolve_MultipleViolationsJoined(t *testing.T) {
	def := defaultsFrom(
		defRule{path: "AGENTS.md", class: ClassProjectOwned, provenance: "project.policy"},
		defRule{path: ".opencode/agents/researcher.md", class: ClassPlatformManaged, provenance: "core.research_workflow"},
		defRule{path: ".opencode/skills/acme-cockpit/x.md", class: ClassExternalGenerated, provenance: "provider.acme-cockpit"},
	)
	ov := overridesFrom(
		// downgrade
		ovRule{path: "AGENTS.md", class: ClassPlatformManaged},
		// unknown path
		ovRule{path: "docs/ghost.md", class: ClassProjectOwned},
		// off-lattice override
		ovRule{path: ".opencode/skills/acme-cockpit/x.md", class: ClassProjectOwned},
	)
	eff, err := Resolve(def, ov)
	if err == nil {
		t.Fatal("Resolve err = nil, want joined violations")
	}
	if !IsDowngradeError(err) {
		t.Error("joined err should contain a DowngradeError")
	}
	if !IsUnknownPathError(err) {
		t.Error("joined err should contain an UnknownPathError")
	}
	if !IsNotHandOverridableError(err) {
		t.Error("joined err should contain a NotHandOverridableError")
	}
	// The one accepted-able path stays at default; downgraded/off-lattice paths
	// are NOT partially applied.
	if got, _ := eff.ClassOf("AGENTS.md"); got != ClassProjectOwned {
		t.Errorf("AGENTS.md partial effective = %q, want unchanged project_owned", got)
	}
	// Message should mention every offending path.
	msg := err.Error()
	for _, want := range []string{"AGENTS.md", "docs/ghost.md", "acme-cockpit"} {
		if !strings.Contains(msg, want) {
			t.Errorf("joined message missing %q\nfull:\n%s", want, msg)
		}
	}
}

// --- Resolve: full spike1-shaped scenario ----------------------------------

// TestResolve_Spike1Scenario wires a realistic slice of spike1's
// harness-ownership.yml through Resolve and confirms the invariants hold end to
// end: a project raises researcher.md to project_owned (accepted), AGENTS.md
// stays project_owned (no-op), and a careless downgrade of opencode.jsonc is
// rejected while the external_generated skill path refuses a hand override.
func TestResolve_Spike1Scenario(t *testing.T) {
	def := ModuleDefaults{
		".opencode/agents/researcher.md":         {Class: ClassPlatformManaged, Provenance: "core.research_workflow"},
		".opencode/scripts/commit-gate.sh":       {Class: ClassPlatformArmed, Provenance: "core.commit_gate"},
		"AGENTS.md":                              {Class: ClassProjectOwned, Provenance: "project.policy"},
		"opencode.jsonc":                         {Class: ClassOverlayExtension, Provenance: "core.permissions"},
		".opencode/skills/acme-cockpit/skill.md": {Class: ClassExternalGenerated, Provenance: "provider.acme-cockpit"},
	}
	ov := Overrides{
		// raise: accepted
		".opencode/agents/researcher.md": {Class: ClassProjectOwned, Reason: "pin our researcher edits"},
		// no-op: accepted
		"AGENTS.md": {Class: ClassProjectOwned, Reason: "reaffirm"},
		// downgrade: rejected
		"opencode.jsonc": {Class: ClassPlatformManaged, Reason: "want full overwrite"},
		// off-lattice: rejected
		".opencode/skills/acme-cockpit/skill.md": {Class: ClassProjectOwned, Reason: "claim it"},
	}
	eff, err := Resolve(def, ov)
	if err == nil {
		t.Fatal("expected joined downgrade + off-lattice violations")
	}
	// Accepted raises / no-ops land.
	if got, _ := eff.ClassOf(".opencode/agents/researcher.md"); got != ClassProjectOwned {
		t.Errorf("researcher raise not applied: %q", got)
	}
	if got, _ := eff.ClassOf("AGENTS.md"); got != ClassProjectOwned {
		t.Errorf("AGENTS.md no-op changed class: %q", got)
	}
	// Rejected overrides leave the path at its (higher) default.
	if got, _ := eff.ClassOf("opencode.jsonc"); got != ClassOverlayExtension {
		t.Errorf("opencode.jsonc downgrade partially applied: %q", got)
	}
	// Both violation kinds surface.
	if !IsDowngradeError(err) {
		t.Error("expected a DowngradeError in the join")
	}
	if !IsNotHandOverridableError(err) {
		t.Error("expected a NotHandOverridableError in the join")
	}
}
