// Package renderstate owns the persisted record of rendered overlay outputs and
// the report-only orphan detection that derives from it.
//
// # Why this package exists (P1-LINEAGE-002)
//
// The seam renderer builds its overlay contribution list from LIVE overlay paths
// only (internal/cli/seam.go renderSeamStaging). When an overlay skill source is
// removed from its pack, that source simply drops out of the rendered set, so the
// seam never classifies the previously-rendered copy and leaves it on disk —
// invisible to `update --dry-run` and never reported. The renderer's conservative
// non-deleting posture is intentional (it never destroys operator content), but
// the loss of VISIBILITY is the bug: a stale rendered skill keeps being
// auto-discovered (SKILL.md is auto-loaded) and can contradict newly-adopted core
// skills.
//
// # What this package does (v1: REPORT-ONLY)
//
// This package persists a per-FILE manifest of what the last non-dry-run apply
// rendered, keyed by normalized destination path and carrying producer provenance
// (pack name + source-relative path) plus a rendered digest. (The persist is
// gated: it happens only when no currently-rendered, manifest-tracked overlay-skill
// destination reports WriteFailed; non-skill write failures do not gate, and
// substrate.Apply return semantics are unchanged — Apply still returns nil on a
// live-write failure.) On every apply it compares the PRIOR manifest against the
// CURRENT render and surfaces records whose producing SOURCE has gone missing (a
// definite orphan) as long as the destination is still on disk. It NEVER deletes
// anything: the only side effect is reporting findings through
// substrate.ApplyReport and persisting the manifest itself after a non-dry-run
// apply under that gate.
//
// # Spec (locked)
//
// Manifest location: a SEPARATE versioned persisted file at
// <target>/.vh-agent-harness/rendered-outputs.json (JSON). It does NOT live in
// lineage.yml — lineage.yml is the S1-only render-origin authority whose
// AssertLineageOnly guard rejects any new top-level key, so the rendered-outputs
// record must stay a separate file to preserve that safety contract.
//
// Record granularity: per-FILE (not per-directory) so a mixed directory — some
// files whose source was removed and some still rendered — survives correctly.
//
// Fields (Manifest.Entries[]):
//
//	destination_path    normalized repo-relative live path, forward-slashed
//	                     (e.g. ".opencode/skills/tdd-loop/SKILL.md")
//	producer_kind       what produced the output; v1 = "overlay_skill" only
//	overlay_pack        overlay pack NAME that produced the output
//	source_relative_path pack-FS-relative source path
//	                     (e.g. "skills/tdd-loop/SKILL.md")
//	rendered_digest     "sha256:<hex>" of the bytes written to destination_path
//
// Versioning: ManifestVersion is written on every record. A future bump uses the
// manifest_version field to let a reader migrate forward; v1 readers reject an
// unknown version rather than guessing.
//
// Lifecycle / atomicity: the manifest is written ONLY after a NON-dry-run apply
// in which no currently-rendered, manifest-tracked overlay-skill destination
// reports WriteFailed (dry-run reads + compares but never writes; non-skill write
// failures do not gate; substrate.Apply return semantics are unchanged — Apply
// still returns nil on a live-write failure). The manifest must never claim a
// generation whose tracked overlay-skill writes did not all land. Persistence is
// atomic: the new bytes are written to a temp file in the same directory and
// renamed into place, so the on-disk manifest is either the prior generation or
// the new one, never a half-written mix. If the live-tree apply succeeded but the
// manifest replacement fails, the apply is NOT rolled back (its writes are real)
// and the manifest write failure is surfaced as a warning; the manifest stays at
// the prior generation, so the next run compares against a valid (if stale)
// record and never reports anything false. The only consequence of a manifest
// write failure is a one-cycle blind spot for sources removed in that run.
//
// # Provenance (definite-orphan rule)
//
// A record is a DEFINITE orphan only when its producing SOURCE is missing — i.e.
// the source file can no longer be read from the producer pack by name (pack
// opened independently of whether it is currently selected, then the
// source-relative path statted in the pack FS). Pack deselection alone is NOT an
// orphan: a deselected pack whose source still exists is simply not rendered this
// run, and its record is dropped silently. This keeps report-only detection from
// ever flagging an operator-added project skill dir (those were never recorded)
// or a pack the operator merely turned off.
//
// # v1 scope
//
// Only overlay-rendered SKILLS (source path under "skills/") are tracked. Core
// removals and overlay agents/commands are out of scope; this package reserves
// the ProducerKind vocabulary but only emits ProducerOverlaySkill.
package renderstate

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// DirName is the per-project config directory the manifest lives in (shared with
// lineage.yml and run-shape.yml).
const DirName = ".vh-agent-harness"

// FileName is the rendered-outputs manifest file name inside DirName.
const FileName = "rendered-outputs.json"

// ManifestVersion is the rendered-outputs manifest schema version. The version
// is carried in every written manifest via the manifest_version field so a future
// bump can be detected and migrated; v1 readers reject an unknown version rather
// than guessing at compatibility.
const ManifestVersion = "1"

// ProducerKind labels what produced a rendered output. v1 tracks
// ProducerOverlaySkill only; the vocabulary is reserved so a future version can
// extend to overlay agents/commands or core removals without reusing a value.
type ProducerKind string

const (
	// ProducerOverlaySkill labels a file rendered from an overlay pack's skills/
	// subtree (overlay_pack = pack name, source_relative_path is under "skills/").
	ProducerOverlaySkill ProducerKind = "overlay_skill"
)

// DigestPrefix is the hash-algorithm tag prefixing every rendered_digest value.
const DigestPrefix = "sha256:"

// Manifest is the persisted record of rendered overlay outputs from one
// successful (non-dry-run) apply. It is the durable input to the next run's
// report-only orphan detection. Entries are normalized per-FILE.
type Manifest struct {
	// ManifestVersion is the schema version (see ManifestVersion constant).
	ManifestVersion string `json:"manifest_version"`
	// SuccessfulRenderID is the lineage content-addressed render id this manifest
	// reflects, carried for forward-compat and diagnostics. It is informational
	// only; orphan detection does not depend on it.
	SuccessfulRenderID string `json:"successful_render_id,omitempty"`
	// Entries is the per-file rendered-output record set, keyed by destination.
	Entries []Record `json:"entries"`
}

// Record is one rendered output file. It is normalized per-FILE so a mixed
// directory (some files from an active source, some from a removed source)
// survives correctly.
type Record struct {
	// DestinationPath is the normalized repo-relative LIVE path, forward-slashed
	// (e.g. ".opencode/skills/tdd-loop/SKILL.md"). It is the comparison key.
	DestinationPath string `json:"destination_path"`
	// ProducerKind labels what produced this output.
	ProducerKind ProducerKind `json:"producer_kind"`
	// OverlayPack is the overlay pack NAME that produced this output. Source
	// existence is re-checked by opening this pack by name, independently of
	// whether it is currently selected.
	OverlayPack string `json:"overlay_pack"`
	// SourceRelativePath is the pack-FS-relative source path
	// (e.g. "skills/tdd-loop/SKILL.md"). It is statted in the producer pack's FS
	// to decide source-missing.
	SourceRelativePath string `json:"source_relative_path"`
	// RenderedDigest is "sha256:<hex>" of the bytes last written to
	// DestinationPath by a successful render. It is used to label an orphan's
	// destination as unchanged or modified relative to the last render.
	RenderedDigest string `json:"rendered_digest"`
}

// FilePath returns the absolute path to the rendered-outputs manifest inside a
// target directory. It does not check for existence.
func FilePath(targetDir string) string {
	return filepath.Join(targetDir, DirName, FileName)
}

// New returns a Manifest prefilled with the current schema version and an empty
// entry set. Callers fill SuccessfulRenderID and Entries.
func New(renderID string) *Manifest {
	return &Manifest{
		ManifestVersion:    ManifestVersion,
		SuccessfulRenderID: renderID,
		Entries:            []Record{},
	}
}

// Digest computes the "sha256:<hex>" digest of a rendered byte slice. Exported so
// the seam can stamp records at render time without re-implementing the format.
func Digest(data []byte) string {
	sum := sha256.Sum256(data)
	return DigestPrefix + hex.EncodeToString(sum[:])
}

// NormalizeDestination canonicalizes a repo-relative path into the manifest's
// destination_path form: forward-slashed (Windows backslashes coerced too, so a
// manifest written on one OS compares cleanly on another), cleaned of leading
// "./", and with no backslashes. It is the single normalization site so writes
// and comparisons agree.
func NormalizeDestination(p string) string {
	// Coerce Windows-style backslashes on every OS (filepath.ToSlash is a no-op
	// for "\" on Unix, where it is a legal filename char, so do it by hand).
	cleaned := filepath.Clean(strings.ReplaceAll(p, "\\", "/"))
	// filepath.Clean leaves a leading "./" only for relative "."; strip it so
	// ".opencode/..." never carries a redundant prefix.
	return strings.TrimPrefix(cleaned, "./")
}

// Validate checks that every record is well-formed: a known producer kind, a
// non-empty destination/source/pack, a sha256-prefixed digest, and no duplicate
// destination. It is called by Read (defensive) and Write (fail-loud) so a
// malformed manifest is never silently trusted or persisted.
func (m *Manifest) Validate() error {
	if m == nil {
		return fmt.Errorf("renderstate: manifest is nil")
	}
	if m.ManifestVersion == "" {
		return fmt.Errorf("renderstate: manifest_version is empty")
	}
	if m.ManifestVersion != ManifestVersion {
		return fmt.Errorf("renderstate: manifest_version %q is unsupported (this build understands %q only)",
			m.ManifestVersion, ManifestVersion)
	}
	seen := make(map[string]bool, len(m.Entries))
	for i, e := range m.Entries {
		switch e.ProducerKind {
		case ProducerOverlaySkill:
			// only v1 kind
		default:
			return fmt.Errorf("renderstate: entry %d (%s): unsupported producer_kind %q",
				i, e.DestinationPath, e.ProducerKind)
		}
		if e.DestinationPath == "" {
			return fmt.Errorf("renderstate: entry %d: empty destination_path", i)
		}
		if e.OverlayPack == "" {
			return fmt.Errorf("renderstate: entry %d (%s): empty overlay_pack", i, e.DestinationPath)
		}
		if e.SourceRelativePath == "" {
			return fmt.Errorf("renderstate: entry %d (%s): empty source_relative_path", i, e.DestinationPath)
		}
		if !strings.HasPrefix(e.RenderedDigest, DigestPrefix) {
			return fmt.Errorf("renderstate: entry %d (%s): rendered_digest %q must start with %q",
				i, e.DestinationPath, e.RenderedDigest, DigestPrefix)
		}
		if seen[e.DestinationPath] {
			return fmt.Errorf("renderstate: duplicate destination_path %q", e.DestinationPath)
		}
		seen[e.DestinationPath] = true
	}
	return nil
}

// Read loads and validates the manifest at targetDir. A missing manifest (no
// prior generation) returns (nil, nil) — the caller treats this as a
// forward-looking bootstrap. A present-but-unreadable or schema-invalid manifest
// returns an error so a corrupted file is never silently trusted.
func Read(targetDir string) (*Manifest, error) {
	data, err := os.ReadFile(FilePath(targetDir))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("renderstate: read manifest: %w", err)
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("renderstate: parse manifest: %w", err)
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}
	return &m, nil
}

// Write persists the manifest as indented JSON via an atomic temp-file + rename,
// after validating it. Entries are sorted by destination_path so the output is
// deterministic across idempotent re-applies. This function is the single
// persistence site; it is reached only after a non-dry-run apply in which no
// currently-rendered, manifest-tracked overlay-skill destination reports
// WriteFailed (the caller gates; non-skill write failures do not gate, and
// substrate.Apply return semantics are unchanged).
func (m *Manifest) Write(targetDir string) error {
	if err := m.Validate(); err != nil {
		return err
	}
	// Deterministic ordering: sort a copy so the in-memory slice order is not
	// mutated (callers may inspect it after Write).
	entries := make([]Record, len(m.Entries))
	copy(entries, m.Entries)
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].DestinationPath < entries[j].DestinationPath
	})
	out := Manifest{
		ManifestVersion:    m.ManifestVersion,
		SuccessfulRenderID: m.SuccessfulRenderID,
		Entries:            entries,
	}
	data, err := json.MarshalIndent(&out, "", "  ")
	if err != nil {
		return fmt.Errorf("renderstate: marshal manifest: %w", err)
	}
	data = append(data, '\n')

	dir := filepath.Join(targetDir, DirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("renderstate: ensure manifest dir: %w", err)
	}
	// Atomic write: stage in a sibling temp file in the SAME directory (so the
	// rename is atomic on the same filesystem), then rename into place. A crash
	// between write and rename leaves the temp behind, not a half-written
	// manifest.
	final := FilePath(targetDir)
	tmp, err := os.CreateTemp(dir, ".rendered-outputs.*.tmp")
	if err != nil {
		return fmt.Errorf("renderstate: create temp manifest: %w", err)
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
		return fmt.Errorf("renderstate: write temp manifest: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("renderstate: close temp manifest: %w", err)
	}
	if err := os.Rename(tmpName, final); err != nil {
		return fmt.Errorf("renderstate: replace manifest: %w", err)
	}
	// Rename succeeded: the temp name no longer exists to clean up.
	tmpName = ""
	return nil
}
