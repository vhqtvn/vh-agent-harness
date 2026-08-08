// Package originhash owns the persisted per-file origin-hash store used by the
// managed-file update seam's three-way divergence check. It is the harness's
// port of the hermes-agent skills_sync origin-hash mechanism.
//
// # Why this package exists
//
// The seam's platform_managed ownership class is a wholesale-overwrite class:
// a plain re-render overwrites it unconditionally (modulo a byte-identical
// no-op). Without a record of what bytes the platform last wrote, a consumer's
// hand-edits to a platform_managed file are silently destroyed on every update
// — the recurring "adoption migration" tax documented in
// researches/decisions/origin-hash-update-sync.md. The origin-hash store closes
// that gap: it records the hash of the bytes the platform last successfully
// wrote for every platform_managed file, and the apply path
// (internal/substrate/apply.go planOutcome) compares the on-disk hash to the
// recorded origin hash to decide whether an update is safe (on-disk still
// matches origin → overwrite) or must be skipped (on-disk diverged →
// ownership-transferred, never clobber).
//
// The mechanism is the three-way comparison hermes uses
// (refs/hermes-agent/tools/skills_sync.py): update ONLY when on-disk hash ==
// recorded origin hash; divergence (user edited) → skip + mark user-modified;
// deletion → respected; upstream-removed → de-manifest only, never delete the
// on-disk copy.
//
// # Storage
//
// A separate versioned persisted file at
// <target>/.vh-agent-harness/origin-hashes.json (JSON). It is NOT the legacy
// manifest (.opencode/harness-manifest.json) — that file is no longer written
// by the seam path and its hashes are stale for any seam-era install. The
// origin-hash store is purpose-built, written only by substrate.Apply after a
// fully-applied generation (the same gate lineage.yml uses), and read at the
// start of every Apply.
//
// # Atomicity / failure model
//
// Persistence mirrors lineage.yml: the store is written ONLY after a fully-
// applied generation (no live-write failure), via an atomic temp-file + rename
// in the same directory, so the on-disk store is either the prior generation or
// the new one, never a half-written mix. A write failure is surfaced as an
// error from substrate.Apply (consistent with lineage) because a successful
// generation whose state record did not persist is a real inconsistency; the
// live tree writes are real and a re-run after fixing the underlying write
// failure re-records origin hashes idempotently.
//
// # Scope (v1)
//
// Only platform_managed files are tracked. overlay_extension (active pack),
// platform_armed (schema reconcile), project_owned, external_generated, and
// local_only are out of scope: overlay_extension's three-way treatment is a
// separate question (deferred), and the latter four are never wholesale-
// overwritten by the platform so an origin hash would add nothing.
package originhash

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// DirName is the per-project config directory the store lives in (shared with
// lineage.yml and rendered-outputs.json).
const DirName = ".vh-agent-harness"

// FileName is the origin-hash store file name inside DirName.
const FileName = "origin-hashes.json"

// SchemaVersion is the store schema version written by this binary.
const SchemaVersion = "1"

// DigestPrefix is the hash-algorithm tag prefixing every origin-hash value, so
// the format is self-describing and consistent with the drift / manifest /
// renderstate digest formats.
const DigestPrefix = "sha256:"

// FilePath returns the absolute path to the origin-hash store inside targetDir.
// It does not check for existence.
func FilePath(targetDir string) string {
	return filepath.Join(targetDir, DirName, FileName)
}

// Digest computes the "sha256:<hex>" digest of a byte slice. Exported so the
// apply path (internal/substrate) can compute staged/live digests without
// re-implementing the format, keeping a single hash representation across the
// harness.
func Digest(data []byte) string {
	sum := sha256.Sum256(data)
	return DigestPrefix + hex.EncodeToString(sum[:])
}

// Store is the persisted per-file origin-hash record from one fully-applied
// generation. It is the durable input to the next apply's three-way divergence
// check. OriginHashes maps a repo-relative platform_managed path to the
// "sha256:<hex>" hash of the bytes the platform last successfully wrote there.
type Store struct {
	SchemaVersion string            `json:"schema_version"`
	OriginHashes  map[string]string `json:"origin_hashes"`
}

// New returns a Store prefilled with the current schema version and an empty
// origin_hashes map. Callers fill OriginHashes.
func New() *Store {
	return &Store{SchemaVersion: SchemaVersion, OriginHashes: map[string]string{}}
}

// Read loads the store at targetDir. A missing file returns (nil, nil) — the
// caller (substrate.Apply) treats this as the bootstrap case: no prior origin
// recorded, every platform_managed file is treated as unedited. A present-but-
// unreadable, schema-incompatible, or structurally-malformed store (including a
// schema-version-valid file whose origin_hashes field is missing/null) returns
// an error so a corrupted file is never silently trusted — see the nil-map
// rejection in the body for why this is fail-closed, not bootstrap.
func Read(targetDir string) (*Store, error) {
	data, err := os.ReadFile(FilePath(targetDir))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("originhash: read store: %w", err)
	}
	var s Store
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("originhash: parse store: %w", err)
	}
	if s.SchemaVersion != SchemaVersion {
		return nil, fmt.Errorf("originhash: schema_version %q unsupported (this build understands %q only); remove %s to re-bootstrap",
			s.SchemaVersion, SchemaVersion, FileName)
	}
	// A store this binary wrote always carries origin_hashes as a (possibly
	// empty) map — Write and New both initialize it non-nil, and JSON marshal
	// of a non-nil map yields "{}", never "null". A nil map after unmarshal
	// therefore means the field was absent or explicitly null — i.e. the file
	// was NOT produced by this binary's Write (hand-edited, foreign, or
	// truncated). Reject it fail-closed: a nil map treated as an empty bootstrap
	// store would make Lookup report no prior origin for every platform_managed
	// path, skipping the three-way check for ALL such files and wholesale-
	// overwriting every consumer hand-edit — exactly the silent-clobber data
	// loss this feature exists to prevent.
	if s.OriginHashes == nil {
		return nil, fmt.Errorf("originhash: store %q has a missing or null origin_hashes field (this binary never writes that form); remove %s to re-bootstrap",
			FileName, FileName)
	}
	return &s, nil
}

// Write persists the store as indented JSON via an atomic temp-file + rename.
// Origin-hash keys are sorted so the output is deterministic across idempotent
// re-applies. This is the single persistence site; it is reached only after a
// fully-applied generation (the caller gates, mirroring lineage.yml).
func (s *Store) Write(targetDir string) error {
	if s == nil {
		return fmt.Errorf("originhash: write nil store")
	}
	if s.OriginHashes == nil {
		s.OriginHashes = map[string]string{}
	}
	// Deterministic ordering: emit keys in sorted order so two stores with the
	// same contents are byte-identical (stable in git across idempotent updates).
	keys := make([]string, 0, len(s.OriginHashes))
	for k := range s.OriginHashes {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := struct {
		SchemaVersion string            `json:"schema_version"`
		OriginHashes  map[string]string `json:"origin_hashes"`
	}{SchemaVersion, make(map[string]string, len(keys))}
	for _, k := range keys {
		out.OriginHashes[k] = s.OriginHashes[k]
	}
	data, err := json.MarshalIndent(&out, "", "  ")
	if err != nil {
		return fmt.Errorf("originhash: marshal store: %w", err)
	}
	data = append(data, '\n')

	dir := filepath.Join(targetDir, DirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("originhash: ensure store dir: %w", err)
	}
	// Atomic write: stage in a sibling temp file in the SAME directory (so the
	// rename is atomic on the same filesystem), then rename into place. A crash
	// between write and rename leaves the temp behind, not a half-written store.
	final := FilePath(targetDir)
	tmp, err := os.CreateTemp(dir, ".origin-hashes.*.tmp")
	if err != nil {
		return fmt.Errorf("originhash: create temp store: %w", err)
	}
	tmpName := tmp.Name()
	// Best-effort cleanup if anything below fails before the rename succeeds.
	defer func() {
		if tmpName != "" {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("originhash: write temp store: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("originhash: close temp store: %w", err)
	}
	if err := os.Rename(tmpName, final); err != nil {
		return fmt.Errorf("originhash: replace store: %w", err)
	}
	// Rename succeeded: the temp name no longer exists to clean up.
	tmpName = ""
	return nil
}

// Lookup returns the recorded origin hash for rel and whether one was recorded.
// A nil store (the bootstrap case after a missing-file Read) returns
// ("", false) for every path, so callers can treat "no prior origin" and
// "origin not recorded for this path" uniformly.
func (s *Store) Lookup(rel string) (string, bool) {
	if s == nil || s.OriginHashes == nil {
		return "", false
	}
	h, ok := s.OriginHashes[rel]
	return h, ok
}
