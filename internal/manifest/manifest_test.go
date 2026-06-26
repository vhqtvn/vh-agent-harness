package manifest

import (
	"encoding/json"
	"reflect"
	"testing"
)

func newTestManifest() *Manifest {
	return &Manifest{
		SchemaVersion:     SchemaVersion,
		EnabledComponents: []string{"plugin:shell-guard"},
		Files:             make(map[string]File),
	}
}

func TestManifest_IsEnabled(t *testing.T) {
	m := newTestManifest()
	if !m.IsEnabled("plugin:shell-guard") {
		t.Errorf("IsEnabled(plugin:shell-guard) = false, want true")
	}
	if m.IsEnabled("agent:researcher") {
		t.Errorf("IsEnabled(agent:researcher) = true, want false")
	}
}

func TestManifest_EnableComponent(t *testing.T) {
	m := newTestManifest()

	// Add a new component -> mutated, appended.
	if !m.EnableComponent("agent:researcher") {
		t.Errorf("EnableComponent(agent:researcher) = false, want true (should mutate)")
	}
	want := []string{"plugin:shell-guard", "agent:researcher"}
	if !reflect.DeepEqual(m.EnabledComponents, want) {
		t.Errorf("after enable: EnabledComponents = %v, want %v", m.EnabledComponents, want)
	}

	// Enable again -> no-op, not mutated, no duplicate.
	if m.EnableComponent("agent:researcher") {
		t.Errorf("EnableComponent(already-enabled) = true, want false (idempotent)")
	}
	if !reflect.DeepEqual(m.EnabledComponents, want) {
		t.Errorf("after re-enable: EnabledComponents = %v, want %v (no duplicate)", m.EnabledComponents, want)
	}
}

func TestManifest_DisableComponent(t *testing.T) {
	m := newTestManifest()
	m.EnableComponent("agent:researcher")
	m.EnableComponent("agent:planner")
	// order is now [plugin:shell-guard, agent:researcher, agent:planner]

	// Remove a middle entry -> mutated, order preserved.
	if !m.DisableComponent("agent:researcher") {
		t.Errorf("DisableComponent(agent:researcher) = false, want true (should mutate)")
	}
	want := []string{"plugin:shell-guard", "agent:planner"}
	if !reflect.DeepEqual(m.EnabledComponents, want) {
		t.Errorf("after disable: EnabledComponents = %v, want %v", m.EnabledComponents, want)
	}

	// Disable again -> no-op.
	if m.DisableComponent("agent:researcher") {
		t.Errorf("DisableComponent(not-enabled) = true, want false (idempotent)")
	}
}

func TestManifest_SetRemoveFile(t *testing.T) {
	m := newTestManifest()
	m.SetFile(".opencode/agents/researcher.md", File{Hash: "sha256:abc", Class: ClassManaged})
	got, ok := m.Files[".opencode/agents/researcher.md"]
	if !ok || got.Hash != "sha256:abc" {
		t.Errorf("SetFile did not store entry: %+v", got)
	}
	// Replace.
	m.SetFile(".opencode/agents/researcher.md", File{Hash: "sha256:xyz", Class: ClassManaged})
	if m.Files[".opencode/agents/researcher.md"].Hash != "sha256:xyz" {
		t.Errorf("SetFile did not replace hash")
	}
	// Remove.
	m.RemoveFile(".opencode/agents/researcher.md")
	if _, ok := m.Files[".opencode/agents/researcher.md"]; ok {
		t.Errorf("RemoveFile did not delete entry")
	}
	// Remove absent is a no-op.
	m.RemoveFile("does/not/exist")
}

// TestRuntime_AdditiveFields verifies the slice-4a additive Runtime extension is
// backwards compatible: an older manifest JSON without compose_file /
// default_service still parses (zero-value defaults), and the new fields round-
// trip through marshal/unmarshal. schema_version stays "1".
func TestRuntime_AdditiveFields(t *testing.T) {
	// Older slice-2 manifest: only backend + fallback.
	old := []byte(`{
	  "schema_version": "1",
	  "harness_version": "0.1.0-dev (slice-2)",
	  "project": {"name": "Demo", "slug": "demo"},
	  "runtime": {"backend": "docker_compose", "fallback": ""},
	  "enabled_components": ["plugin:shell-guard"],
	  "files": {}
	}`)
	var m Manifest
	if err := json.Unmarshal(old, &m); err != nil {
		t.Fatalf("parse older manifest: %v", err)
	}
	if m.Runtime.Backend != "docker_compose" {
		t.Errorf("backend = %q, want docker_compose", m.Runtime.Backend)
	}
	if m.Runtime.ComposeFile != "" || m.Runtime.DefaultService != "" {
		t.Errorf("additive fields should default empty; got compose_file=%q default_service=%q",
			m.Runtime.ComposeFile, m.Runtime.DefaultService)
	}

	// Newer slice-4a manifest carries the optional fields; they round-trip.
	m.Runtime.ComposeFile = "compose.yaml"
	m.Runtime.DefaultService = "dev"
	data, err := json.Marshal(&m)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m2 Manifest
	if err := json.Unmarshal(data, &m2); err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	if m2.Runtime.ComposeFile != "compose.yaml" || m2.Runtime.DefaultService != "dev" {
		t.Errorf("round-trip lost additive fields: %+v", m2.Runtime)
	}
	if m2.SchemaVersion != SchemaVersion {
		t.Errorf("schema_version changed to %q", m2.SchemaVersion)
	}
}
