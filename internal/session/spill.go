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
//   - creation is exclusive (O_CREATE|O_EXCL): an existing path — file
//     OR symlink — is never written through (anti-symlink);
//   - reads are FAIL-CLOSED: the file's size and sha256 must match the
//     locator, else the read refuses.
//
// Replay contract: spill files are durable SIDECAR state. The session
// log already carries the preview + locator, so log replay (and
// DeriveMessages) never touches the store — losing spill files degrades
// RETRIEVAL (spill_read fails closed), never replay integrity.
package session

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
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
// (0700) at first write; files are created exclusive (0600). A
// FileSpillStore is safe for concurrent use: writes are
// content-addressed (same-content races converge on one file via the
// exclusive-create + verify dedup path) and reads are stateless.
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

// Write implements SpillStore.
func (s *FileSpillStore) Write(kind string, content []byte) (SpillLocator, error) {
	sum := sha256.Sum256(content)
	full := hex.EncodeToString(sum[:])
	name := sanitizeKind(kind) + "-" + full[:16]
	loc := SpillLocator{File: name, SHA256: full, Size: int64(len(content))}

	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return SpillLocator{}, fmt.Errorf("session: spill mkdir %s: %w", s.dir, err)
	}
	path := filepath.Join(s.dir, name)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if os.IsExist(err) {
			// Exclusive-create refuses EVERY existing path — file or
			// symlink (anti-symlink). For the content-addressed name the
			// common case is a dedup hit; verify the stored bytes before
			// trusting it, and refuse on any mismatch (a collision or a
			// pre-planted path is never written through).
			if existing, rerr := os.ReadFile(path); rerr == nil && bytes.Equal(existing, content) {
				return loc, nil
			}
			return SpillLocator{}, fmt.Errorf("session: spill file %s already exists with different content (refusing to overwrite)", path)
		}
		return SpillLocator{}, fmt.Errorf("session: spill create %s: %w", path, err)
	}
	if _, err := f.Write(content); err != nil {
		_ = f.Close()
		_ = os.Remove(path) // never leave a partial file under the content's name
		return SpillLocator{}, fmt.Errorf("session: spill write %s: %w", path, err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(path)
		return SpillLocator{}, fmt.Errorf("session: spill close %s: %w", path, err)
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
	var candidates []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err // fail-closed: an unreadable subtree aborts the walk
		}
		if !d.Type().IsRegular() {
			return nil // directories, symlinks, and specials never match
		}
		if d.Name() == loc.File && strings.HasSuffix(filepath.Base(filepath.Dir(path)), spillDirSuffix) {
			candidates = append(candidates, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("session: spill retrieval walk over %s: %w", root, err)
	}
	for _, path := range candidates {
		if b, err := readSpillValidated(path, loc); err == nil {
			return b, nil
		}
	}
	return nil, fmt.Errorf("session: spill file %q not found under any %q store beneath %s", loc.File, "*"+spillDirSuffix, root)
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
	notice := fmt.Sprintf("... [spilled %d bytes: %s — read via spill/read]", len(content), locJSON)
	budget := int(maxInline) - 1 - len(notice) // 1 = the newline before the notice
	if budget <= 0 {
		return content, nil, false // the notice alone exceeds the cap: keep inline
	}
	spilled := &loc
	return content[:budget] + "\n" + notice, spilled, true
}
