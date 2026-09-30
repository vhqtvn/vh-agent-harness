// spill.go — the oversize tool-result spill storage seam (the dsh
// spill/spill-local pattern; see
// researches/sources/deepseek-harness/session-cognition.md §spill/):
// oversized tool-result content is written FULL to a session-scoped
// file and the committed event carries a bounded preview plus an
// opaque locator, instead of losing the overflow to truncation.
//
// Storage contract (dsh spill-local, restated for this engine):
//   - one store per session, rooted at <session-dir>/<session-id>.spill/
//     (dir 0700, files 0600);
//   - files are CONTENT-ADDRESSED (<kind>-<sha256[:16]>, kind default
//     "sp"), so identical output de-duplicates and concurrent writes of
//     the same content are idempotent;
//   - files materialize by TEMP + ATOMIC RENAME: the full content is
//     staged in a private temp file (0600, `tmp-<sha16>-<rand>`,
//     exclusive-create), fsynced, then renamed onto the content-
//     addressed name. The final name NEVER hosts a partial file (the
//     concurrent same-content partial-read window — writer B observing
//     writer A's half-written file — is closed by construction), a
//     same-content rename is a harmless byte-identical overwrite, and
//     rename REPLACES a pre-existing path without ever writing through
//     it (anti-symlink);
//   - reads are FAIL-CLOSED: the file's size and full-content sha256
//     must match the locator, else the read refuses. Windowed reads
//     validate the FULL file (streaming hash) and then serve only the
//     requested byte range — the whole file is never buffered.
//
// Replay contract: spill files are durable SIDECAR state. The session
// log already carries the preview + locator, so log replay (and
// DeriveMessages) never touches the store — losing spill files degrades
// RETRIEVAL (spill_read fails closed), never replay integrity.
package session

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// DefaultSpillMaxInline is the default inline budget for spilled tool
// results. It deliberately matches the run_shell default capture cap
// (internal/tools/shell DefaultMaxCapturedBytes = 64 KiB) so the commit-
// time spill threshold and the capture threshold describe the same
// order of magnitude; the two caps remain independent knobs (capture
// bounds in-memory buffering per stream, spill bounds the committed
// event content as a whole).
const DefaultSpillMaxInline = 64 * 1024

// spillDirSuffix names the per-session spill directory: <id>.spill.
const spillDirSuffix = ".spill"

// SpillLocator is the opaque handle to one spilled result. The wire/log
// format is exactly {file, sha256, size}: the content-addressed file
// name (unique within a session store), the full content hash (the
// fail-closed read check), and the byte size. It carries no paths a
// reader could steer — retrieval resolves the name inside a store root.
type SpillLocator struct {
	File   string `json:"file"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

// SpillStore is the spill storage seam: content-addressed durable
// writes and hash-validated reads. Write is a hint-namespaced,
// idempotent-for-identical-content operation; Read is fail-closed on
// any mismatch between the locator and the stored bytes.
type SpillStore interface {
	// Write durably stores content under the sanitized kind prefix and
	// returns its locator. Identical content (same kind) de-duplicates.
	Write(kind string, content []byte) (SpillLocator, error)
	// Read returns the content named by loc, verifying size and sha256.
	Read(loc SpillLocator) ([]byte, error)
}

// FileSpillStore is the filesystem SpillStore for one session, rooted
// at <sessionDir>/<sessionID>.spill/. The directory is created lazily
// (0700) at first write; files materialize by temp+atomic-rename
// (0600). A FileSpillStore is safe for concurrent use: each Write
// stages a private temp file and atomically renames it onto the shared
// content-addressed name, so same-content races converge on one
// COMPLETE file (an overwrite with identical bytes is harmless — the
// name is content-addressed) and a reader can never observe a partial
// file under the final name. Reads are stateless and hash-validated.
type FileSpillStore struct {
	dir string
}

// NewFileSpillStore returns the store for one session. It performs no
// filesystem work (the directory is created at first Write), so wiring
// it cannot fail.
func NewFileSpillStore(sessionDir, sessionID string) *FileSpillStore {
	return &FileSpillStore{dir: filepath.Join(sessionDir, sessionID+spillDirSuffix)}
}

// Dir returns the store's root directory (<...>/<id>.spill).
func (s *FileSpillStore) Dir() string { return s.dir }

// Write implements SpillStore. The content is staged in a private temp
// file, fsynced, then atomically renamed onto the content-addressed
// name — after any rename the file at the final name is always
// COMPLETE, so a concurrent same-content writer can never observe (or
// spuriously reject) a partial file.
func (s *FileSpillStore) Write(kind string, content []byte) (SpillLocator, error) {
	sum := sha256.Sum256(content)
	full := hex.EncodeToString(sum[:])
	name := sanitizeKind(kind) + "-" + full[:16]
	loc := SpillLocator{File: name, SHA256: full, Size: int64(len(content))}

	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return SpillLocator{}, fmt.Errorf("session: spill mkdir %s: %w", s.dir, err)
	}
	// Stage the FULL content in a temp file inside the spill dir
	// (exclusive-create + random suffix: never a symlink, never
	// observable at the final content-addressed name), then rename it
	// ATOMICALLY into place. os.Rename never writes THROUGH an existing
	// path — it replaces the directory entry — so a pre-planted symlink
	// at the final name is displaced, its target untouched.
	tmp, err := os.CreateTemp(s.dir, "tmp-"+full[:16]+"-")
	if err != nil {
		return SpillLocator{}, fmt.Errorf("session: spill create temp in %s: %w", s.dir, err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(content); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return SpillLocator{}, fmt.Errorf("session: spill write %s: %w", tmpName, err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return SpillLocator{}, fmt.Errorf("session: spill fsync %s: %w", tmpName, err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return SpillLocator{}, fmt.Errorf("session: spill close %s: %w", tmpName, err)
	}
	final := filepath.Join(s.dir, name)
	if err := os.Rename(tmpName, final); err != nil {
		_ = os.Remove(tmpName)
		return SpillLocator{}, fmt.Errorf("session: spill rename %s -> %s: %w", tmpName, final, err)
	}
	return loc, nil
}

// Read implements SpillStore: fail-closed on any size/hash mismatch.
func (s *FileSpillStore) Read(loc SpillLocator) ([]byte, error) {
	if err := validateSpillName(loc.File); err != nil {
		return nil, err
	}
	return readSpillValidated(filepath.Join(s.dir, loc.File), loc)
}

// ReadSpillUnder retrieves a spill file by locator from ANY per-session
// store beneath root — the retrieval seam for a tool that has no
// session context (spill_read): it walks root for `*.spill`
// directories containing loc.File and returns the first candidate that
// validates against the locator. Content addressing makes the name
// globally unique in practice, and the hash check makes an accidental
// name collision unreturnable. Unknown names fail closed.
func ReadSpillUnder(root string, loc SpillLocator) ([]byte, error) {
	if err := validateSpillName(loc.File); err != nil {
		return nil, err
	}
	candidates, err := walkSpillCandidates(root, loc.File)
	if err != nil {
		return nil, err
	}
	for _, path := range candidates {
		if b, err := readSpillValidated(path, loc); err == nil {
			return b, nil
		}
	}
	return nil, fmt.Errorf("session: spill file %q not found under any %q store beneath %s", loc.File, "*"+spillDirSuffix, root)
}

// ReadWindow returns the [offset, offset+length) window of the content
// named by loc, after validating the FULL file against the locator:
// the file is streamed through sha256 (never buffered whole) and the
// window is then served by a bounded section read. The window serves
// min(length, size-offset) bytes; offset == size yields the empty
// terminal window. Fail-closed: negative offset, offset beyond size,
// non-positive length, or any size/hash mismatch refuses.
func (s *FileSpillStore) ReadWindow(loc SpillLocator, offset int64, length int64) ([]byte, error) {
	if err := validateSpillName(loc.File); err != nil {
		return nil, err
	}
	return readSpillWindowValidated(filepath.Join(s.dir, loc.File), loc, offset, length)
}

// ReadSpillWindowUnder is ReadWindow over the daemon-wide walk: it
// retrieves a window of loc's content from ANY per-session store
// beneath root, full-file hash-validated, serving only the window
// bytes (the whole file is never buffered).
func ReadSpillWindowUnder(root string, loc SpillLocator, offset int64, length int64) ([]byte, error) {
	if err := validateSpillName(loc.File); err != nil {
		return nil, err
	}
	candidates, err := walkSpillCandidates(root, loc.File)
	if err != nil {
		return nil, err
	}
	for _, path := range candidates {
		if w, err := readSpillWindowValidated(path, loc, offset, length); err == nil {
			return w, nil
		}
	}
	return nil, fmt.Errorf("session: spill file %q not found under any %q store beneath %s", loc.File, "*"+spillDirSuffix, root)
}

// walkSpillCandidates collects the regular files named file inside any
// `*.spill` directory beneath root (fail-closed: an unreadable subtree
// aborts the walk).
func walkSpillCandidates(root, file string) ([]string, error) {
	var candidates []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err // fail-closed: an unreadable subtree aborts the walk
		}
		if !d.Type().IsRegular() {
			return nil // directories, symlinks, and specials never match
		}
		if d.Name() == file && strings.HasSuffix(filepath.Base(filepath.Dir(path)), spillDirSuffix) {
			candidates = append(candidates, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("session: spill retrieval walk over %s: %w", root, err)
	}
	return candidates, nil
}

// readSpillValidated reads path and enforces the locator's size and
// sha256 (fail-closed on mismatch or unreadable bytes).
func readSpillValidated(path string, loc SpillLocator) ([]byte, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("session: spill read %s: %w", path, err)
	}
	if int64(len(b)) != loc.Size {
		return nil, fmt.Errorf("session: spill %s size mismatch: locator says %d, file holds %d", path, loc.Size, len(b))
	}
	sum := sha256.Sum256(b)
	if hex.EncodeToString(sum[:]) != loc.SHA256 {
		return nil, fmt.Errorf("session: spill %s hash mismatch: content does not match the locator (fail-closed)", path)
	}
	return b, nil
}

// readSpillWindowValidated validates the FULL file at path against the
// locator (stat size + streaming sha256 — the file is never buffered
// whole), then serves ONLY the [offset, offset+length) window via a
// bounded section read. A tamper ANYWHERE in the file refuses even a
// healthy window; a validated file serves its bytes at any offset.
func readSpillWindowValidated(path string, loc SpillLocator, offset int64, length int64) ([]byte, error) {
	if offset < 0 {
		return nil, fmt.Errorf("session: spill window offset %d is negative", offset)
	}
	if length <= 0 {
		return nil, fmt.Errorf("session: spill window length %d must be positive (callers resolve defaults)", length)
	}
	if offset > loc.Size {
		return nil, fmt.Errorf("session: spill window offset %d is beyond the content size %d", offset, loc.Size)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("session: spill read %s: %w", path, err)
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("session: spill stat %s: %w", path, err)
	}
	if !fi.Mode().IsRegular() {
		return nil, fmt.Errorf("session: spill %s is not a regular file (fail-closed)", path)
	}
	if fi.Size() != loc.Size {
		return nil, fmt.Errorf("session: spill %s size mismatch: locator says %d, file holds %d", path, loc.Size, fi.Size())
	}
	// Validate the full content hash by STREAMING the file through
	// sha256 (bounded memory; the bytes are never held whole).
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return nil, fmt.Errorf("session: spill read %s: %w", path, err)
	}
	if n != loc.Size {
		return nil, fmt.Errorf("session: spill %s size mismatch: locator says %d, file holds %d", path, loc.Size, n)
	}
	if hex.EncodeToString(h.Sum(nil)) != loc.SHA256 {
		return nil, fmt.Errorf("session: spill %s hash mismatch: content does not match the locator (fail-closed)", path)
	}
	// Serve the window: a bounded section read off the still-open file.
	winLen := length
	if rem := loc.Size - offset; rem < winLen {
		winLen = rem
	}
	window := make([]byte, winLen)
	if _, err := io.ReadFull(io.NewSectionReader(f, offset, winLen), window); err != nil {
		return nil, fmt.Errorf("session: spill window read %s@%d: %w", path, offset, err)
	}
	return window, nil
}

// validateSpillName confines the locator's file component to a strict
// single base name: no separators, no "." / "..", no emptiness — a
// locator can never steer a read outside its store.
func validateSpillName(name string) error {
	if name == "" || name == "." || name == ".." || filepath.Base(name) != name || strings.ContainsRune(name, '/') || strings.ContainsRune(name, '\\') {
		return fmt.Errorf("session: spill locator file %q is not a strict base name", name)
	}
	return nil
}

// sanitizeKind maps a kind hint onto the filename prefix: lowercase
// [a-z0-9-], others collapse to '-', trimmed, capped at 16 runes; the
// empty (default) kind is "sp". Kinds namespace dedup (the same bytes
// under two kinds are two files) — pass "" unless a real namespace is
// wanted.
func sanitizeKind(kind string) string {
	if kind == "" {
		return "sp"
	}
	var b strings.Builder
	for _, r := range kind {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r + ('a' - 'A'))
		default:
			b.WriteRune('-')
		}
	}
	s := strings.Trim(b.String(), "-")
	if len(s) > 16 {
		s = s[:16]
	}
	if s == "" {
		return "sp"
	}
	return s
}

// SpillPolicy is the commit-time oversize decision: content at or
// below MaxInlineBytes stays inline byte-for-byte; content above it is
// written FULL to the store and replaced by a bounded preview plus the
// spill notice carrying the opaque locator. The notice's bytes are
// reserved INSIDE the cap (preview + notice ≤ MaxInlineBytes), so a
// spilled event never exceeds the inline budget.
//
// Fallbacks (dsh keep-inline discipline, silent-but-deterministic —
// a spill must never fail the tool result because the sidecar failed):
//   - nil policy or nil Store ⇒ inline, byte-identical to pre-spill
//     behavior (the library default);
//   - MaxInlineBytes <= 0 ⇒ DefaultSpillMaxInline;
//   - Store.Write error ⇒ the FULL content stays inline;
//   - a cap so small the notice alone would exceed it ⇒ inline.
type SpillPolicy struct {
	MaxInlineBytes int64
	Store          SpillStore
}

// Apply decides one content's fate and returns the effective content,
// the locator when spilled, and whether the spill happened.
func (p *SpillPolicy) Apply(kind, content string) (string, *SpillLocator, bool) {
	if p == nil || p.Store == nil {
		return content, nil, false
	}
	maxInline := p.MaxInlineBytes
	if maxInline <= 0 {
		maxInline = DefaultSpillMaxInline
	}
	if int64(len(content)) <= maxInline {
		return content, nil, false
	}
	loc, err := p.Store.Write(kind, []byte(content))
	if err != nil {
		return content, nil, false // save failure keeps the content inline
	}
	locJSON, merr := json.Marshal(loc)
	if merr != nil {
		return content, nil, false // unreachable for this struct; same fallback
	}
	notice := fmt.Sprintf("... [spilled %d bytes: %s — read via spill_read]", len(content), locJSON)
	budget := int(maxInline) - 1 - len(notice) // 1 = the newline before the notice
	if budget <= 0 {
		return content, nil, false // the notice alone exceeds the cap: keep inline
	}
	spilled := &loc
	return content[:budget] + "\n" + notice, spilled, true
}
