// Package lineage owns the shim-written S1 render-lineage file
// (.vh-agent-harness/lineage.yml) introduced by decision D3-B.
//
// Per the config-authority model the S1 surface is LINEAGE-ONLY: it records what
// was rendered (template source/commit/ref, Copier version, an answer digest,
// and the render/update identity) and NOTHING else. It must never answer a
// profile question (S3), an update-safety/ownership question (S2), or a runtime
// question (S4). AssertLineageOnly enforces that invariant at parse time.
//
// This file is a throwaway-prototype surface: the prototype does not yet invoke
// Copier, so a seeded Lineage records the embedded corpus as the template source
// and an empty Copier version (see Seed()). It is the faithful placeholder the
// harness owns and will replace with real Copier output once Copier is wired in.
package lineage

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// DirName is the project-owned declaration directory the lineage file lives in.
// It is the same directory the entrypoint model reserves for .vh-agent-harness/.
const DirName = ".vh-agent-harness"

// FileName is the lineage file name inside DirName.
const FileName = "lineage.yml"

// LineageVersion is the schema version written by this binary.
const LineageVersion = "1"

// FilePath returns the absolute path to the lineage file inside targetDir. It
// does not check for existence.
func FilePath(targetDir string) string {
	return filepath.Join(targetDir, DirName, FileName)
}

// Lineage is the full lineage document. Every field is an S1 (render-lineage)
// fact. The struct intentionally carries NO profile / ownership / runtime
// fields so a parsed Lineage can never answer an S2/S3/S4 question.
type Lineage struct {
	// LineageVersion is the lineage schema version ("1").
	LineageVersion string `yaml:"lineage_version"`
	// Template records the template origin the materialization was rendered from.
	Template TemplateRef `yaml:"template"`
	// Copier records the Copier tool identity that performed the render. In the
	// prototype Copier is not yet the substrate, so Copier.Version is "" and
	// the shim records itself as the renderer via Render.RenderedBy.
	Copier CopierRef `yaml:"copier"`
	// Answers records a digest over the render answers that produced the current
	// materialization plus the sorted key list that feeds that digest. In the
	// prototype (Copier not yet the render substrate) it ALSO carries the raw
	// install-identity answer values so doctor/update can re-render the managed
	// baseline faithfully; see AnswersRef.Values.
	Answers AnswersRef `yaml:"answers"`
	// Render records the identity of the last successful render/update.
	Render RenderRef `yaml:"render"`
}

// TemplateRef identifies the template the materialization was rendered from.
type TemplateRef struct {
	// Source is the template origin (path / URL / embedded-corpus URI).
	Source string `yaml:"source"`
	// Commit is the pinned template commit hash; "" when not pinned.
	Commit string `yaml:"commit"`
	// Ref is the symbolic template ref (branch / tag); "" when not set.
	Ref string `yaml:"ref"`
}

// CopierRef records the Copier tool that performed the render.
type CopierRef struct {
	// Version is the Copier version (e.g. "9.16.0"). Empty means the shim
	// rendered directly without Copier (the current prototype state).
	Version string `yaml:"version"`
}

// AnswersRef is the digest + key list over the render answers, plus (in the
// prototype) the raw install-identity values needed for faithful re-render.
type AnswersRef struct {
	// Digest is "sha256:<hex>" over the canonicalized selected answer keys +
	// values. It lets doctor detect answer drift without trusting the raw
	// values as the sole authority.
	Digest string `yaml:"digest"`
	// SelectedKeys is the sorted list of answer keys that feed Digest.
	SelectedKeys []string `yaml:"selected_keys"`
	// Values carries the raw install-identity answer values (project_name,
	// project_slug, coordinator_dir — the caller-supplied install answers, NOT
	// the S3 profile feature answers). doctor and update read these to re-render
	// the managed baseline faithfully: without them, a project whose install
	// name/slug differ from the target dir basename would false-flag managed
	// drift on every token-bearing file, and update would silently rewrite the
	// project name. Copier's own answers cache is the long-term home for raw
	// answer values; in the prototype (Copier not yet wired) lineage.yml is the
	// only durable answer record, so it carries them. This stays S1-only: it
	// records what was rendered and never answers an S2/S3/S4 question (feature
	// decisions live in vh-harness-profile.yml).
	Values map[string]string `yaml:"values"`
}

// RenderRef records the identity of the last successful render/update.
type RenderRef struct {
	// LastSuccessfulUpdateID is an opaque, content-addressed id of the last
	// successful render/update (derived from the digest + vh-agent-harness version).
	LastSuccessfulUpdateID string `yaml:"last_successful_update_id"`
	// LastSuccessfulRenderAt is the RFC3339 timestamp of the last successful
	// render/update.
	LastSuccessfulRenderAt string `yaml:"last_successful_render_at"`
	// RenderedBy names what performed the render. In the prototype this is
	// "shim" (Copier not yet wired); in the real harness it becomes "copier".
	RenderedBy string `yaml:"rendered_by"`
}

// New returns a Lineage prefilled with the current schema version.
func New() *Lineage {
	return &Lineage{LineageVersion: LineageVersion}
}

// FilePathNote is the leading comment block prepended to a written lineage.yml
// so a human reading the file understands it is shim-owned lineage, not policy.
const fileHeader = `# lineage.yml - S1 render lineage (decision D3-B).
# Shim-owned and lineage-ONLY. This is NOT policy authority: it cannot answer
# "which profile?" (S3), "is this file safe to overwrite?" (S2), or "which
# services run?" (S4). See the config-authority model.
# Do not hand-edit; the harness rewrites this file on render/update.
`

// Write serializes the lineage as YAML to DirName/FileName under targetDir,
// creating the directory as needed. The output is deterministic for a given
// Lineage value (struct field order is fixed; SelectedKeys is sorted first).
func (l *Lineage) Write(targetDir string) error {
	if l == nil {
		return fmt.Errorf("lineage: write nil lineage")
	}
	// Normalize SelectedKeys so identical lineages produce identical bytes.
	l.Answers.SelectedKeys = sortedDedup(l.Answers.SelectedKeys)
	if err := os.MkdirAll(filepath.Join(targetDir, DirName), 0o755); err != nil {
		return err
	}
	data, err := yaml.Marshal(l)
	if err != nil {
		return err
	}
	data = append([]byte(fileHeader), data...)
	return os.WriteFile(FilePath(targetDir), data, 0o644)
}

// Read loads the lineage file under targetDir. A missing file returns (nil, nil)
// (mirrors manifest.Find's "absent vs broken" contract); a present-but-
// unparseable file returns an error so callers can distinguish the two.
// Read also runs AssertLineageOnly and surfaces any S2/S3/S4 leak as an error,
// so a lineage file that has drifted into policy territory is never silently
// honored.
func Read(targetDir string) (*Lineage, error) {
	data, err := os.ReadFile(FilePath(targetDir))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if err := AssertLineageOnly(data); err != nil {
		return nil, fmt.Errorf("lineage authority hygiene: %w", err)
	}
	var l Lineage
	if err := yaml.Unmarshal(data, &l); err != nil {
		return nil, fmt.Errorf("parse %s: %w", FilePath(targetDir), err)
	}
	return &l, nil
}

// DigestOf returns the "sha256:<hex>" digest over the given answers map. The
// canonical form is the answers sorted by key, each as "key=value\n". It is
// deterministic and order-independent. Both the seed path (installer tokens)
// and the drift path (Copier answers) route through here so digests are
// comparable apples-to-apples.
func DigestOf(answers map[string]string) string {
	keys := make([]string, 0, len(answers))
	for k := range answers {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var sb strings.Builder
	for _, k := range keys {
		sb.WriteString(k)
		sb.WriteByte('=')
		sb.WriteString(answers[k])
		sb.WriteByte('\n')
	}
	sum := sha256.Sum256([]byte(sb.String()))
	return "sha256:" + hex.EncodeToString(sum[:])
}

// UpdateID derives a short, content-addressed id for a render/update from the
// answer digest and a vh-agent-harness version. Identical inputs yield identical ids so
// an idempotent re-install produces a stable lineage (the render timestamp may
// still move; the id is content-addressed, the timestamp is wall-clock).
func UpdateID(digest, harnessVersion string) string {
	h := sha256.Sum256([]byte(digest + "|" + harnessVersion))
	return hex.EncodeToString(h[:])[:16]
}

// --- Authority hygiene (the S1-only invariant) -------------------------------

// allowedTopLevel is the exhaustive set of top-level keys a lineage.yml may
// carry. Every entry is an S1 (render-lineage) fact. Anything else is a leak of
// S2 (ownership), S3 (profile), or S4 (runtime) authority and is rejected.
var allowedTopLevel = map[string]bool{
	"lineage_version": true,
	"template":        true,
	"copier":          true,
	"answers":         true,
	"render":          true,
}

// forbiddenTopLevel maps a forbidden top-level key to the authority surface it
// would leak. AssertLineageOnly reports these by surface so the failure is
// self-explaining. This is the mechanical guard for the config-authority model's
// "one-line test": reading the lineage must not answer an S2/S3/S4 question.
var forbiddenTopLevel = map[string]string{
	// S3 - feature-surface / profile selection
	"profile": "S3", "profile_ref": "S3", "modules": "S3", "overlays": "S3", "policy_packs": "S3",
	// S2 - update-safety / ownership classification
	"ownership": "S2", "ownership_overrides": "S2", "classes": "S2", "class": "S2",
	// S4 - runtime execution shape
	"services": "S4", "runtime": "S4", "runners": "S4", "proxies": "S4",
	"env": "S4", "hooks": "S4", "lifecycle": "S4", "backend": "S4",
}

// AssertLineageOnly parses raw YAML into a generic map and verifies every
// top-level key is an allowed S1 lineage fact. Any forbidden key is reported
// with the authority surface it would leak. It is the authority-hygiene guard
// the lineage invariant rests on: the lineage file must never carry the facts
// another surface owns.
func AssertLineageOnly(raw []byte) error {
	var root map[string]any
	if err := yaml.Unmarshal(raw, &root); err != nil {
		return fmt.Errorf("parse: %w", err)
	}
	var leaked []string
	for k := range root {
		if allowedTopLevel[k] {
			continue
		}
		if surface, ok := forbiddenTopLevel[k]; ok {
			leaked = append(leaked, fmt.Sprintf("%q leaks %s authority", k, surface))
		} else {
			leaked = append(leaked, fmt.Sprintf("%q is not a known lineage fact", k))
		}
	}
	if len(leaked) == 0 {
		return nil
	}
	sort.Strings(leaked)
	return fmt.Errorf("lineage file is not lineage-only: %s", strings.Join(leaked, "; "))
}

// Seed builds a faithful placeholder Lineage for a shim-rendered materialization
// (the prototype state: Copier is not yet the render substrate). The template
// source records the embedded corpus, the Copier version is empty, and the
// answer digest is computed from the given answers. When the harness wires in
// real Copier, install/update will replace this with the actual Copier version
// and template source/commit.
func Seed(templateSource string, answers map[string]string, harnessVersion string) *Lineage {
	keys := make([]string, 0, len(answers))
	for k := range answers {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	digest := DigestOf(answers)
	return &Lineage{
		LineageVersion: LineageVersion,
		Template:       TemplateRef{Source: templateSource},
		Copier:         CopierRef{Version: ""}, // empty = shim-rendered, Copier not yet invoked
		Answers: AnswersRef{
			Digest:       digest,
			SelectedKeys: keys,
			Values:       copyMap(answers),
		},
		Render: RenderRef{
			LastSuccessfulUpdateID: UpdateID(digest, harnessVersion),
			LastSuccessfulRenderAt: time.Now().UTC().Format(time.RFC3339),
			RenderedBy:             "shim",
		},
	}
}

// copyMap returns a shallow copy of in so the lineage does not alias the
// caller's answer map (mirrors mergeRenderAnswers's "caller's map is never
// retained" discipline). A nil input yields a fresh empty map so the seeded
// AnswersRef.Values is never nil (YAML-serializes as a stable empty map).
func copyMap(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// sortedDedup returns a sorted, de-duplicated copy of in.
func sortedDedup(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}
