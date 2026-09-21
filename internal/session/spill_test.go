// spill_test.go — the oversize tool-result spill storage seam (dsh
// spill/spill-local pattern, see
// researches/sources/deepseek-harness/session-cognition.md §spill/):
// session-scoped content-addressed files with exclusive-create
// anti-symlink semantics, fail-closed hash-validated reads, the
// commit-time SpillPolicy decision (preview budget reserved INSIDE the
// inline cap), and the daemon-wide retrieval walk.
package session

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// locJSON is the canonical compact locator rendering embedded in the
// spill notice (deterministic struct order).
func locJSON(t *testing.T, loc SpillLocator) string {
	t.Helper()
	b, err := json.Marshal(loc)
	if err != nil {
		t.Fatalf("marshal locator: %v", err)
	}
	return string(b)
}

func shaHex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func TestSpillLocatorJSONShape(t *testing.T) {
	loc := SpillLocator{File: "sp-0123456789abcdef", SHA256: strings.Repeat("a", 64), Size: 42}
	got := locJSON(t, loc)
	want := `{"file":"sp-0123456789abcdef","sha256":"` + strings.Repeat("a", 64) + `","size":42}`
	if got != want {
		t.Fatalf("locator JSON = %s, want %s", got, want)
	}
}

func TestFileSpillStoreWriteReadRoundtrip(t *testing.T) {
	dir := t.TempDir()
	s := NewFileSpillStore(dir, "sess-rt")
	content := []byte("the full tool result bytes")
	loc, err := s.Write("", content)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if loc.Size != int64(len(content)) || loc.SHA256 != shaHex(content) {
		t.Fatalf("locator = %+v", loc)
	}
	if !strings.HasPrefix(loc.File, "sp-") || len(loc.File) != len("sp-")+16 {
		t.Fatalf("file name = %q, want sp-<sha256[:16]>", loc.File)
	}

	// Storage layout + permissions: 0700 dir, 0600 file, under
	// <sessionDir>/<sessionID>.spill/.
	spillDir := filepath.Join(dir, "sess-rt.spill")
	if loc.File != "" {
		fi, err := os.Stat(spillDir)
		if err != nil {
			t.Fatalf("spill dir: %v", err)
		}
		if fi.Mode().Perm() != 0o700 {
			t.Fatalf("spill dir perms = %o, want 0700", fi.Mode().Perm())
		}
		fi, err = os.Stat(filepath.Join(spillDir, loc.File))
		if err != nil {
			t.Fatalf("spill file: %v", err)
		}
		if fi.Mode().Perm() != 0o600 {
			t.Fatalf("spill file perms = %o, want 0600", fi.Mode().Perm())
		}
	}

	back, err := s.Read(loc)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if !bytes.Equal(back, content) {
		t.Fatalf("roundtrip = %q, want %q", back, content)
	}
}

func TestFileSpillStoreDedupAndKinds(t *testing.T) {
	s := NewFileSpillStore(t.TempDir(), "sess-dedup")
	a := []byte("identical output")
	loc1, err := s.Write("", a)
	if err != nil {
		t.Fatalf("Write 1: %v", err)
	}
	loc2, err := s.Write("", a)
	if err != nil {
		t.Fatalf("Write 2: %v", err)
	}
	if loc1 != loc2 {
		t.Fatalf("identical content must dedupe to one locator: %v vs %v", loc1, loc2)
	}
	// One file on disk.
	entries, err := os.ReadDir(s.Dir())
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("files = %d, want 1 (content-addressed dedup)", len(entries))
	}

	// A different kind namespaces the same content separately.
	loc3, err := s.Write("future-kind", a)
	if err != nil {
		t.Fatalf("Write kind: %v", err)
	}
	if loc3.File == loc1.File || !strings.HasPrefix(loc3.File, "future-kind-") {
		t.Fatalf("kind-prefixed name = %q", loc3.File)
	}

	// Different content gets a different name.
	loc4, err := s.Write("", []byte("other"))
	if err != nil {
		t.Fatalf("Write other: %v", err)
	}
	if loc4.File == loc1.File {
		t.Fatal("different content must not share a file name")
	}
}

func TestFileSpillStoreReadFailClosed(t *testing.T) {
	dir := t.TempDir()
	s := NewFileSpillStore(dir, "sess-fc")
	content := []byte("hash-validated bytes")
	loc, err := s.Write("", content)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}

	// Tampered bytes: sha mismatch must refuse.
	path := filepath.Join(s.Dir(), loc.File)
	if err := os.WriteFile(path, []byte("tampered bytes!!"), 0o600); err != nil {
		t.Fatalf("tamper: %v", err)
	}
	if _, err := s.Read(loc); err == nil {
		t.Fatal("Read after tamper must fail closed (hash mismatch)")
	}

	// Wrong size in the locator: refuse.
	loc2, err := s.Write("", []byte("size probe"))
	if err != nil {
		t.Fatalf("Write 2: %v", err)
	}
	bad := loc2
	bad.Size++
	if _, err := s.Read(bad); err == nil {
		t.Fatal("Read with wrong size must fail closed")
	}

	// Missing file: refuse.
	ghost := SpillLocator{File: "sp-0000000000000000", SHA256: shaHex([]byte("x")), Size: 1}
	if _, err := s.Read(ghost); err == nil {
		t.Fatal("Read of a missing file must fail closed")
	}
}

func TestFileSpillStoreRejectsTraversal(t *testing.T) {
	s := NewFileSpillStore(t.TempDir(), "sess-trav")
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	for _, name := range []string{"../sess-other.spill/sp-x", "a/b", "..", ".", "", "/etc/passwd"} {
		loc := SpillLocator{File: name, SHA256: shaHex([]byte("secret")), Size: 6}
		if _, err := s.Read(loc); err == nil {
			t.Fatalf("Read(loc.File=%q) must reject a non-basename locator", name)
		}
	}
	if b, err := os.ReadFile(outside); err != nil || string(b) != "secret" {
		t.Fatalf("traversal attempt touched the outside file: %q %v", b, err)
	}
}

func TestFileSpillStoreNeverWritesThroughSymlink(t *testing.T) {
	dir := t.TempDir()
	s := NewFileSpillStore(dir, "sess-sym")
	victim := filepath.Join(dir, "victim.txt")
	if err := os.WriteFile(victim, []byte("victim"), 0o600); err != nil {
		t.Fatalf("seed victim: %v", err)
	}
	content := []byte("content whose name collides with the symlink")
	name := "sp-" + shaHex(content)[:16]
	if err := os.MkdirAll(s.Dir(), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.Symlink(victim, filepath.Join(s.Dir(), name)); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	loc, err := s.Write("", content)
	if err != nil {
		t.Fatalf("Write over a pre-planted symlink must succeed by REPLACING the symlink entry (temp+atomic-rename): %v", err)
	}
	// The victim behind the symlink was never written THROUGH.
	if b, _ := os.ReadFile(victim); string(b) != "victim" {
		t.Fatalf("symlink target was modified: %q", b)
	}
	// The final name is now a REGULAR file holding exactly our content.
	fi, err := os.Lstat(filepath.Join(s.Dir(), name))
	if err != nil || !fi.Mode().IsRegular() {
		t.Fatalf("final path must be a regular file after rename, got mode %v err=%v", fi, err)
	}
	if back, err := s.Read(loc); err != nil || !bytes.Equal(back, content) {
		t.Fatalf("readback = %d bytes err=%v, want the written content", len(back), err)
	}
}

// failingStore arms the save-failure fallback path.
type failingStore struct{ calls int }

func (f *failingStore) Write(string, []byte) (SpillLocator, error) {
	f.calls++
	return SpillLocator{}, fmt.Errorf("disk on fire")
}
func (f *failingStore) Read(SpillLocator) ([]byte, error) {
	return nil, fmt.Errorf("unreachable")
}

func TestSpillPolicyApply(t *testing.T) {
	t.Run("nil store and nil policy keep content inline", func(t *testing.T) {
		var nilPolicy *SpillPolicy
		if c, loc, spilled := nilPolicy.Apply("", "x"); c != "x" || loc != nil || spilled {
			t.Fatalf("nil policy = %q %v %v", c, loc, spilled)
		}
		p := &SpillPolicy{MaxInlineBytes: 10}
		if c, loc, spilled := p.Apply("", "0123456789A"); c != "0123456789A" || loc != nil || spilled {
			t.Fatalf("no store = %q %v %v", c, loc, spilled)
		}
	})

	t.Run("at or below cap is inline and byte-stable", func(t *testing.T) {
		s := NewFileSpillStore(t.TempDir(), "sess-p1")
		p := &SpillPolicy{MaxInlineBytes: 100, Store: s}
		for _, n := range []int{0, 1, 99, 100} {
			content := strings.Repeat("x", n)
			c, loc, spilled := p.Apply("", content)
			if c != content || loc != nil || spilled {
				t.Fatalf("n=%d: %q %v %v — inline must be byte-identical", n, c, loc, spilled)
			}
		}
	})

	t.Run("oversize spills with preview budget inside the cap", func(t *testing.T) {
		s := NewFileSpillStore(t.TempDir(), "sess-p2")
		const cap = 4096
		p := &SpillPolicy{MaxInlineBytes: cap, Store: s}
		content := strings.Repeat("y", cap+1000)
		c, loc, spilled := p.Apply("", content)
		if !spilled || loc == nil {
			t.Fatalf("spilled=%v loc=%v", spilled, loc)
		}
		if len(c) > cap {
			t.Fatalf("spilled content = %d bytes, must stay <= cap %d", len(c), cap)
		}
		wantNotice := fmt.Sprintf("... [spilled %d bytes: %s — read via spill_read]", len(content), locJSON(t, *loc))
		if !strings.HasSuffix(c, "\n"+wantNotice) {
			t.Fatalf("content must end with the notice line, got tail %q", c[len(c)-min(len(c), len(wantNotice)+40):])
		}
		// The preview is the FIRST bytes of the original content.
		preview := strings.TrimSuffix(c, "\n"+wantNotice)
		if !strings.HasPrefix(content, preview) || len(preview) == 0 {
			t.Fatalf("preview %q is not a prefix of the content", preview[:min(len(preview), 40)])
		}
		// The spill file holds the FULL content, hash-validated.
		back, err := s.Read(*loc)
		if err != nil || string(back) != content {
			t.Fatalf("spill readback = %d bytes err=%v", len(back), err)
		}
		if loc.SHA256 != shaHex([]byte(content)) || loc.Size != int64(len(content)) {
			t.Fatalf("locator = %+v", loc)
		}
	})

	t.Run("save failure keeps the full content inline", func(t *testing.T) {
		f := &failingStore{}
		p := &SpillPolicy{MaxInlineBytes: 100, Store: f}
		content := strings.Repeat("z", 500)
		c, loc, spilled := p.Apply("", content)
		if c != content || loc != nil || spilled {
			t.Fatalf("fallback = %d bytes loc=%v spilled=%v, want full inline", len(c), loc, spilled)
		}
		if f.calls != 1 {
			t.Fatalf("store calls = %d, want 1", f.calls)
		}
	})

	t.Run("cap too small for the notice keeps content inline", func(t *testing.T) {
		s := NewFileSpillStore(t.TempDir(), "sess-p3")
		p := &SpillPolicy{MaxInlineBytes: 10, Store: s}
		content := strings.Repeat("q", 200)
		c, loc, spilled := p.Apply("", content)
		if c != content || loc != nil || spilled {
			t.Fatalf("tiny cap = %d bytes loc=%v spilled=%v, want keep-inline fallback", len(c), loc, spilled)
		}
	})

	t.Run("zero cap normalizes to the default", func(t *testing.T) {
		s := NewFileSpillStore(t.TempDir(), "sess-p4")
		p := &SpillPolicy{Store: s}
		big := strings.Repeat("d", DefaultSpillMaxInline+1)
		if _, _, spilled := p.Apply("", big); !spilled {
			t.Fatal("MaxInlineBytes<=0 must normalize to DefaultSpillMaxInline and still spill")
		}
	})
}

// TestFileSpillStoreWindowedRead — windowed retrieval (the storage half
// of the spill_read no-op fix): the seam serves ONLY the
// [offset, offset+length) window, but validates the FULL file against
// the locator (streaming sha256 — the whole file is never buffered),
// so a tamper ANYWHERE in the file refuses even a healthy window.
func TestFileSpillStoreWindowedRead(t *testing.T) {
	dir := t.TempDir()
	s := NewFileSpillStore(dir, "sess-win")
	content := []byte(strings.Repeat("W", 100000))
	loc, err := s.Write("", content)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}

	// First window.
	w, err := s.ReadWindow(loc, 0, 4096)
	if err != nil {
		t.Fatalf("ReadWindow head: %v", err)
	}
	if !bytes.Equal(w, content[:4096]) {
		t.Fatalf("head window = %d bytes, want content[:4096]", len(w))
	}
	// Mid window.
	w, err = s.ReadWindow(loc, 65536, 100)
	if err != nil {
		t.Fatalf("ReadWindow mid: %v", err)
	}
	if !bytes.Equal(w, content[65536:65636]) {
		t.Fatalf("mid window = %d bytes, want content[65536:65636]", len(w))
	}
	// A window running past EOF serves exactly the remainder.
	w, err = s.ReadWindow(loc, 99990, 4096)
	if err != nil {
		t.Fatalf("ReadWindow tail: %v", err)
	}
	if !bytes.Equal(w, content[99990:]) {
		t.Fatalf("tail window = %d bytes, want the %d-byte remainder", len(w), len(content)-99990)
	}
	// offset == size: the empty terminal window (a pager landing exactly
	// at the end), not an error.
	w, err = s.ReadWindow(loc, int64(len(content)), 4096)
	if err != nil || len(w) != 0 {
		t.Fatalf("terminal window = %d bytes err=%v, want empty no-error", len(w), err)
	}

	// Fail-closed args: offset beyond size, negative offset, and
	// non-positive length all refuse.
	if _, err := s.ReadWindow(loc, int64(len(content))+1, 10); err == nil {
		t.Fatal("offset beyond size must fail closed")
	}
	if _, err := s.ReadWindow(loc, -1, 10); err == nil {
		t.Fatal("negative offset must fail closed")
	}
	if _, err := s.ReadWindow(loc, 0, 0); err == nil {
		t.Fatal("zero length must fail closed (callers resolve defaults)")
	}
	if _, err := s.ReadWindow(loc, 0, -5); err == nil {
		t.Fatal("negative length must fail closed")
	}

	// The daemon-wide walk serves the same windows (before the tamper
	// below invalidates the file).
	w, err = ReadSpillWindowUnder(dir, loc, 65536, 100)
	if err != nil || !bytes.Equal(w, content[65536:65636]) {
		t.Fatalf("ReadSpillWindowUnder = %d bytes err=%v, want the mid window", len(w), err)
	}
	// ... and keeps the fail-closed posture over the walk.
	if _, err := ReadSpillWindowUnder(dir, loc, -1, 10); err == nil {
		t.Fatal("ReadSpillWindowUnder negative offset must fail closed")
	}

	// Full-file hash validation guards the window: tamper a byte FAR
	// outside a healthy window — the windowed read must still refuse.
	path := filepath.Join(s.Dir(), loc.File)
	tampered := append([]byte(nil), content...)
	tampered[99999] ^= 0xFF
	if err := os.WriteFile(path, tampered, 0o600); err != nil {
		t.Fatalf("tamper: %v", err)
	}
	if _, err := s.ReadWindow(loc, 0, 10); err == nil {
		t.Fatal("windowed read over a tampered file must fail closed (full-file hash)")
	}
}

func TestFileSpillStoreConcurrentUse(t *testing.T) {
	dir := t.TempDir()
	s := NewFileSpillStore(dir, "sess-conc")
	// 256 KiB of shared content: large enough that a same-content writer
	// genuinely overlaps another writer's create→complete window (the
	// partial-read race the temp+atomic-rename Write closes). Distinct
	// writers stay kilobyte-scale.
	same := strings.Repeat("S", 256<<10)
	const goroutines = 16
	var wg sync.WaitGroup
	locs := make([]SpillLocator, goroutines)
	// Half the goroutines write the SAME content (idempotent dedup under
	// race), half write distinct contents.
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			var content string
			if i%2 == 0 {
				content = same
			} else {
				content = fmt.Sprintf("distinct output %d %s", i, strings.Repeat("k", 1000))
			}
			loc, err := s.Write("", []byte(content))
			if err != nil {
				t.Errorf("Write %d: %v", i, err)
				return
			}
			if _, err := s.Read(loc); err != nil {
				t.Errorf("Read %d: %v", i, err)
			}
			locs[i] = loc
		}(i)
	}
	wg.Wait()
	// Same-content writers agree on one locator; every locator reads back.
	for i := 0; i+2 < goroutines; i += 2 {
		if locs[i] != locs[i+2] {
			t.Fatalf("same-content locators differ: %v vs %v", locs[i], locs[i+2])
		}
	}
	entries, err := os.ReadDir(s.Dir())
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	// 8 distinct + 1 shared = 9 files.
	if len(entries) != 9 {
		t.Fatalf("files = %d, want 9", len(entries))
	}
}

func TestReadSpillUnderFindsAnySessionStore(t *testing.T) {
	root := t.TempDir()
	// Two session stores under the root, one nested (subagent layout).
	s1 := NewFileSpillStore(root, "sess-a")
	s2 := NewFileSpillStore(filepath.Join(root, "subagents", "parent"), "child-1")
	content := []byte("walk-retrievable")
	loc1, err := s1.Write("", content)
	if err != nil {
		t.Fatalf("Write 1: %v", err)
	}
	loc2, err := s2.Write("", content)
	if err != nil {
		t.Fatalf("Write 2: %v", err)
	}
	if loc1 != loc2 {
		t.Fatalf("content-addressed locators must match across stores: %v vs %v", loc1, loc2)
	}
	back, err := ReadSpillUnder(root, loc1)
	if err != nil {
		t.Fatalf("ReadSpillUnder: %v", err)
	}
	if !bytes.Equal(back, content) {
		t.Fatalf("walk readback = %q", back)
	}

	// Unknown name: fail closed.
	if _, err := ReadSpillUnder(root, SpillLocator{File: "sp-1111111111111111", SHA256: shaHex(content), Size: int64(len(content))}); err == nil {
		t.Fatal("unknown spill file must fail closed")
	}
	// Traversal: fail closed.
	if _, err := ReadSpillUnder(root, SpillLocator{File: "../../etc/passwd", SHA256: shaHex(content), Size: 1}); err == nil {
		t.Fatal("traversal locator must fail closed")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
