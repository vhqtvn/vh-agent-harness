package originhash

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStore_RoundTripPreservesEntries(t *testing.T) {
	dir := t.TempDir()
	s := New()
	s.OriginHashes[".vh-agent-harness/AGENTS.core.md"] = Digest([]byte("core v1"))
	s.OriginHashes[".opencode/agents/build.md"] = Digest([]byte("agent v1"))

	if err := s.Write(dir); err != nil {
		t.Fatalf("Write: %v", err)
	}

	got, err := Read(dir)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got == nil {
		t.Fatal("Read returned nil store after a Write")
	}
	if got.SchemaVersion != SchemaVersion {
		t.Errorf("schema version: want %q, got %q", SchemaVersion, got.SchemaVersion)
	}
	if len(got.OriginHashes) != 2 {
		t.Fatalf("entry count: want 2, got %d (%+v)", len(got.OriginHashes), got.OriginHashes)
	}
	for _, rel := range []string{".vh-agent-harness/AGENTS.core.md", ".opencode/agents/build.md"} {
		h, ok := got.Lookup(rel)
		if !ok {
			t.Errorf("Lookup(%q): missing after round-trip", rel)
			continue
		}
		if !strings.HasPrefix(h, DigestPrefix) {
			t.Errorf("Lookup(%q): digest %q missing %q prefix", rel, h, DigestPrefix)
		}
	}
}

func TestStore_MissingFileReturnsNilBootstrap(t *testing.T) {
	dir := t.TempDir() // no store written
	got, err := Read(dir)
	if err != nil {
		t.Fatalf("Read on missing store: want nil error, got %v", err)
	}
	if got != nil {
		t.Fatalf("Read on missing store: want nil store (bootstrap), got %+v", got)
	}
	// Lookup on a nil store must be safe and report "not recorded".
	if h, ok := got.Lookup("anything"); ok || h != "" {
		t.Errorf("Lookup on nil store: want (\"\",false), got (%q,%v)", h, ok)
	}
}

func TestStore_WriteIsDeterministic(t *testing.T) {
	write := func(dir string) string {
		s := New()
		// Insert in non-sorted order to prove Write sorts.
		s.OriginHashes[".opencode/agents/build.md"] = Digest([]byte("b"))
		s.OriginHashes[".vh-agent-harness/AGENTS.core.md"] = Digest([]byte("a"))
		if err := s.Write(dir); err != nil {
			t.Fatalf("Write: %v", err)
		}
		b, err := os.ReadFile(FilePath(dir))
		if err != nil {
			t.Fatalf("ReadFile: %v", err)
		}
		return string(b)
	}
	first := write(t.TempDir())
	second := write(t.TempDir())
	if first != second {
		t.Errorf("origin-hash store is not deterministic across writes:\n--- first ---\n%s\n--- second ---\n%s", first, second)
	}
}

func TestStore_UnsupportedSchemaVersionIsError(t *testing.T) {
	dir := t.TempDir()
	p := FilePath(dir)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(`{"schema_version":"99","origin_hashes":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(dir); err == nil {
		t.Fatal("Read on unsupported schema_version: want error, got nil")
	}
}
