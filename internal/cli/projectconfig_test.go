package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func writeProjectConfig(t *testing.T, root, body string) {
	t.Helper()
	dir := filepath.Join(root, ".vh-agent-harness")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "project.config.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestProjectConfigAnswers_ListArchitecture(t *testing.T) {
	root := t.TempDir()
	writeProjectConfig(t, root, `{
	  "project": {
	    "mission_summary": "Do the thing.",
	    "architecture_summary": ["apps/api - backend", "apps/web - frontend"],
	    "db_user": "u", "db_name": "d"
	  }
	}`)
	got := projectConfigAnswers(root)
	if got["mission_summary"] != "Do the thing." {
		t.Errorf("mission_summary = %q", got["mission_summary"])
	}
	if got["architecture_summary"] != "- apps/api - backend\n- apps/web - frontend" {
		t.Errorf("architecture_summary = %q", got["architecture_summary"])
	}
	if got["db_user"] != "u" || got["db_name"] != "d" {
		t.Errorf("db = %q/%q", got["db_user"], got["db_name"])
	}
}

func TestProjectConfigAnswers_StringArchitecture(t *testing.T) {
	root := t.TempDir()
	writeProjectConfig(t, root, `{"project":{"architecture_summary":"freeform text"}}`)
	if got := projectConfigAnswers(root)["architecture_summary"]; got != "freeform text" {
		t.Errorf("string architecture_summary = %q", got)
	}
}

func TestProjectConfigAnswers_AbsentAndMalformed(t *testing.T) {
	if got := projectConfigAnswers(t.TempDir()); len(got) != 0 {
		t.Errorf("absent config must yield empty map, got %v", got)
	}
	root := t.TempDir()
	writeProjectConfig(t, root, `{not json`)
	if got := projectConfigAnswers(root); len(got) != 0 {
		t.Errorf("malformed config must be ignored (empty), got %v", got)
	}
}
