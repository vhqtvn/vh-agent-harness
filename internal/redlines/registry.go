package redlines

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// Kind is the redline subject kind. The schema accepts exactly these two
// values; anything else fails validation.
type Kind string

const (
	KindScrubProject      Kind = "scrub-project"
	KindForbiddenRelation Kind = "forbidden-relation"
)

// policyScrubBeforeCommit is the only scrub-project policy recognized in v1.
const policyScrubBeforeCommit = "scrub-before-commit"

// opaqueIDRe constrains a subject id to the ONLY form safe to echo anywhere:
// `subj-` followed by one or more of [A-Za-z0-9_-]. This structurally enforces
// opacity — a real term with spaces, slashes, or punctuation cannot satisfy it,
// and a bare `subj-` (empty suffix) is rejected.
var opaqueIDRe = regexp.MustCompile(`^subj-[A-Za-z0-9_-]+$`)

// Subject is a validated redline subject loaded from a registry file. Fields
// are separated by kind: scrub-project uses Labels/SourceRepos/Policy, while
// forbidden-relation uses SideA/SideB/AmbientRepos/Unit. Kind-inappropriate
// fields are ignored on load (lenient), but every REQUIRED field for the
// declared kind and every enum (Unit, Kind) is enforced (strict).
//
// The Why field is the operator's private one-line rationale. It MUST NEVER
// appear in any output, error, log, or diagnostic produced by this package.
type Subject struct {
	// ID is the opaque subject identifier (subj-...). It is the only field safe
	// to echo in errors and diagnostics.
	ID string
	// Kind is scrub-project or forbidden-relation.
	Kind Kind

	// scrub-project fields.
	Labels      []string // names / aliases / path fragments to scrub
	SourceRepos []string // reserved: globs of the sensitive origin repos; REJECTED at load in v1 (engine matches Labels only)
	Policy      string   // scrub-before-commit (defaulted)

	// forbidden-relation fields.
	SideA        []string // termset A
	SideB        []string // termset B
	AmbientRepos []string // globs where side A is ambient (optional)
	Unit         string   // "file", "diff", or "" meaning both (default)

	// shared fields.
	Repos []string // enforcement-scope globs; nil/empty = all repos
	Why   string   // private rationale; NEVER echoed
}

// rawRegistry / rawSubject are the YAML decode targets. Field tags mirror the
// operator's schema exactly. Unknown YAML fields are ignored (forward-compat);
// required fields and enums are enforced in validate.
type rawRegistry struct {
	Version  int          `yaml:"version"`
	Subjects []rawSubject `yaml:"subjects"`
}

type rawSubject struct {
	ID           string   `yaml:"id"`
	Kind         string   `yaml:"kind"`
	Labels       []string `yaml:"labels"`
	SourceRepos  []string `yaml:"source_repos"`
	Repos        []string `yaml:"repos"`
	Policy       string   `yaml:"policy"`
	SideA        []string `yaml:"side_a"`
	SideB        []string `yaml:"side_b"`
	AmbientRepos []string `yaml:"ambient_repos"`
	Unit         string   `yaml:"unit"`
	Why          string   `yaml:"why"`
}

// Registry is the validated effective redline registry for a repo: the
// user-level subjects plus any additive repo-local subjects. A nil Registry
// means "no redlines apply" (the inert no-op case).
//
// SourcePath is the user-level registry filesystem path (or "<absent>" when no
// user registry exists). It is the only path retained for diagnostics and is
// not sensitive (it is a conventional XDG path).
type Registry struct {
	Version    int
	Subjects   []Subject
	SourcePath string
}

// SupportedVersion is the only registry schema version this build understands.
const SupportedVersion = 1

// UserRegistryPath resolves the user-level registry path using the SAME XDG
// convention the harness already uses for plugin config
// (templates/overlays/auto-classifier-pilot/plugins/auto-tool-gate.js
// userConfigDir):
//
//	$XDG_CONFIG_HOME/vh-agent-harness/redlines/registry.yml   (when XDG set, non-empty, absolute)
//	else $HOME/.config/vh-agent-harness/redlines/registry.yml
//
// The XDG spec is followed strictly: a RELATIVE XDG_CONFIG_HOME is ignored
// (falls back to $HOME/.config), matching spec §1.4. This resolution is
// implemented explicitly (rather than via os.UserConfigDir, which varies by
// GOOS) so the path is identical to the established harness convention on
// every platform.
//
// Returns an error if neither XDG_CONFIG_HOME nor HOME yields a usable base.
func UserRegistryPath() (string, error) {
	base, err := userConfigBase()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "vh-agent-harness", "redlines", "registry.yml"), nil
}

func userConfigBase() (string, error) {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" && filepath.IsAbs(xdg) {
		return xdg, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("redlines: resolve config base: %w", err)
	}
	if home == "" {
		return "", fmt.Errorf("redlines: resolve config base: HOME is empty and XDG_CONFIG_HOME is unset/relative")
	}
	return filepath.Join(home, ".config"), nil
}

// RepoLocalRegistryPath returns the OPTIONAL repo-local additive registry
// path: <repoRoot>/.vh-agent-harness/redlines.local.yml. It is gitignored and
// ADDITIVE/tightening-only (see Load). Existence is not required.
func RepoLocalRegistryPath(repoRoot string) string {
	return filepath.Join(repoRoot, ".vh-agent-harness", "redlines.local.yml")
}

// Load reads and validates the effective registry for a repo: the user-level
// registry plus, if present, the repo-local additive file.
//
// Inert guarantee (zero footprint):
//
//   - If the user-level registry is ABSENT, Load returns (nil, nil) — no error,
//     no output, and no file created anywhere. This is the no-registry no-op.
//   - If the user-level registry is present but valid and binds nothing, Load
//     returns the (non-nil) registry and nil error; still no files are created.
//
// Fail-closed guarantee:
//
//   - A present-but-unreadable user registry, an invalid schema, a duplicate
//     or non-opaque id, or any malformed entry returns a non-nil error. The
//     repo-local file is held to the same standard.
//
// Additive/tightening-only (raise-only, mirroring internal/ownership/doc.go
// §D2-A): the repo-local file may only INTRODUCE NEW subjects. If a repo-local
// subject's id collides with a user-level subject's id, Load fails closed — a
// repo-local file may never redefine, weaken, or mask a user-level entry. (A
// subject with a new id can only ADD restrictions; it cannot weaken an
// existing one because subjects are independent and a scanner checks all that
// bind.)
//
// All errors are OPAQUE: they reference paths, opaque subject ids, and generic
// reason codes. They never contain a label, a termset, a remote, or a `why`.
func Load(repoRoot string) (*Registry, error) {
	userReg, userPresent, err := loadFileAtUserLevel()
	if err != nil {
		return nil, err
	}
	if !userPresent {
		// No user registry at all. The repo-local file is irrelevant: even if
		// present, it can only ADD to a user registry, and there is none. This
		// is the inert no-op: success, no files, nil registry.
		return nil, nil
	}

	// Repo-local additive layer.
	repoReg, repoPresent, err := loadFileAt(RepoLocalRegistryPath(repoRoot), "repo-local")
	if err != nil {
		return nil, err
	}

	effective := &Registry{
		Version:    userReg.Version,
		Subjects:   append([]Subject(nil), userReg.Subjects...),
		SourcePath: userReg.SourcePath,
	}

	if repoPresent {
		// Additive/tightening-only: reject any id collision with the user
		// level. A colliding id would be an attempt to redefine/weaken a
		// user-level entry.
		existing := map[string]struct{}{}
		for _, s := range effective.Subjects {
			existing[s.ID] = struct{}{}
		}
		// Also enforce uniqueness WITHIN the repo-local file (loadFileAt already
		// checks intra-file uniqueness; this guards the cross-file case).
		seenLocal := map[string]struct{}{}
		for _, s := range repoReg.Subjects {
			if _, dup := seenLocal[s.ID]; dup {
				return nil, fmt.Errorf("redlines: repo-local registry %q: duplicate subject id %q", repoReg.SourcePath, s.ID)
			}
			seenLocal[s.ID] = struct{}{}
			if _, clash := existing[s.ID]; clash {
				return nil, fmt.Errorf("redlines: repo-local registry %q: subject id %q collides with user-level entry (repo-local is additive/tightening-only: cannot redefine a user-level subject)", repoReg.SourcePath, s.ID)
			}
			effective.Subjects = append(effective.Subjects, s)
		}
	}
	return effective, nil
}

// loadFileAtUserLevel resolves the user registry path, returns (nil, false,
// nil) when absent, and (reg, true, nil) when present+valid. Present-but-
// unreadable/invalid returns (nil, false, err).
func loadFileAtUserLevel() (*Registry, bool, error) {
	path, err := UserRegistryPath()
	if err != nil {
		return nil, false, err
	}
	return loadFileAt(path, "user")
}

// loadFileAt decodes and validates a single registry file at path. level is
// "user" or "repo-local" and only affects error attribution. Returns
// (nil, false, nil) when the file is absent.
func loadFileAt(path, level string) (*Registry, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("redlines: %s registry %q: unreadable: %w", level, path, err)
	}
	var raw rawRegistry
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, false, fmt.Errorf("redlines: %s registry %q: invalid YAML: %w", level, path, err)
	}
	subjects, err := validateRegistry(&raw, path, level)
	if err != nil {
		return nil, false, err
	}
	return &Registry{
		Version:    raw.Version,
		Subjects:   subjects,
		SourcePath: path,
	}, true, nil
}

// validateRegistry enforces the schema described in the package doc and the
// operator's Decision 2. It is strict on required fields, enums, version, id
// opacity, and id uniqueness; lenient on kind-inappropriate fields and unknown
// YAML fields (forward-compat). Every error string is opaque.
func validateRegistry(raw *rawRegistry, path, level string) ([]Subject, error) {
	if raw.Version != SupportedVersion {
		return nil, fmt.Errorf("redlines: %s registry %q: unsupported version %d (want %d)", level, path, raw.Version, SupportedVersion)
	}
	seen := map[string]struct{}{}
	out := make([]Subject, 0, len(raw.Subjects))
	for i, rs := range raw.Subjects {
		subj, err := validateSubject(rs)
		if err != nil {
			return nil, fmt.Errorf("redlines: %s registry %q: subject #%d: %w", level, path, i+1, err)
		}
		if _, dup := seen[subj.ID]; dup {
			return nil, fmt.Errorf("redlines: %s registry %q: duplicate subject id %q", level, path, subj.ID)
		}
		seen[subj.ID] = struct{}{}
		out = append(out, subj)
	}
	return out, nil
}

func validateSubject(rs rawSubject) (Subject, error) {
	// id: required, opaque form.
	if rs.ID == "" {
		return Subject{}, fmt.Errorf("missing id")
	}
	if !opaqueIDRe.MatchString(rs.ID) {
		return Subject{}, fmt.Errorf("id %q is not opaque (must match subj-[A-Za-z0-9_-]+; real terms are not allowed here)", rs.ID)
	}
	// kind: required, enum.
	kind := Kind(rs.Kind)
	switch kind {
	case KindScrubProject, KindForbiddenRelation:
	default:
		return Subject{}, fmt.Errorf("id %q: kind %q invalid (want %q or %q)", rs.ID, rs.Kind, KindScrubProject, KindForbiddenRelation)
	}

	// unit enum (validated always when present, regardless of kind, to catch
	// typos; it is only semantically used by forbidden-relation). v1 supports
	// file-level scanning only; "diff" is rejected here so an operator cannot
	// silently get weaker file-level protection by configuring unit: diff.
	switch rs.Unit {
	case "", "file":
		// valid (v1: file-level only)
	case "diff":
		return Subject{}, fmt.Errorf("id %q: unit %q not yet implemented (v1 supports file-level only; use \"file\" or omit for both)", rs.ID, rs.Unit)
	default:
		return Subject{}, fmt.Errorf("id %q: unit %q invalid (want file or omitted for both)", rs.ID, rs.Unit)
	}

	s := Subject{
		ID:           rs.ID,
		Kind:         kind,
		Labels:       append([]string(nil), rs.Labels...),
		SourceRepos:  append([]string(nil), rs.SourceRepos...),
		Repos:        append([]string(nil), rs.Repos...),
		Policy:       rs.Policy,
		SideA:        append([]string(nil), rs.SideA...),
		SideB:        append([]string(nil), rs.SideB...),
		AmbientRepos: append([]string(nil), rs.AmbientRepos...),
		Unit:         rs.Unit,
		Why:          rs.Why,
	}

	switch kind {
	case KindScrubProject:
		// source_repos is NOT yet implemented in v1: the engine's
		// scrubUnitMatches only consults s.Labels, so an operator relying on
		// source_repos would get zero protection with no warning (the same
		// honesty violation as unit: diff). Reject at load (fail-closed) so the
		// operator must encode source-identifying fragments as labels instead.
		if len(rs.SourceRepos) > 0 {
			return Subject{}, fmt.Errorf("id %q: source_repos not yet implemented (v1 matches labels only; encode source-identifying fragments as labels)", rs.ID)
		}
		if len(s.Labels) == 0 {
			return Subject{}, fmt.Errorf("id %q (scrub-project): labels required (non-empty)", rs.ID)
		}
		// Per-element emptiness/whitespace check. The scanner's
		// scrubUnitMatches `continue`s on "" (it skips empty labels rather
		// than matching), so labels: [""] would load as a seemingly-valid
		// subject that NEVER FIRES — a silent false-negative letting
		// otherwise-blocked material into the acquired tree. This is the same
		// honesty-contract failure mode rejected above for source_repos and
		// unit: diff; reject at load (fail-closed).
		if containsEmptyOrWhitespaceTerm(s.Labels) {
			return Subject{}, fmt.Errorf("id %q: labels contains an empty or whitespace-only term (terms must be non-empty)", rs.ID)
		}
		// policy: default + restrict. If omitted, default to the only v1 value.
		if s.Policy == "" {
			s.Policy = policyScrubBeforeCommit
		} else if s.Policy != policyScrubBeforeCommit {
			return Subject{}, fmt.Errorf("id %q (scrub-project): policy %q invalid (want %q)", rs.ID, s.Policy, policyScrubBeforeCommit)
		}
	case KindForbiddenRelation:
		if len(s.SideA) == 0 {
			return Subject{}, fmt.Errorf("id %q (forbidden-relation): side_a required (non-empty)", rs.ID)
		}
		if containsEmptyOrWhitespaceTerm(s.SideA) {
			return Subject{}, fmt.Errorf("id %q: side_a contains an empty or whitespace-only term (terms must be non-empty)", rs.ID)
		}
		if len(s.SideB) == 0 {
			return Subject{}, fmt.Errorf("id %q (forbidden-relation): side_b required (non-empty)", rs.ID)
		}
		if containsEmptyOrWhitespaceTerm(s.SideB) {
			return Subject{}, fmt.Errorf("id %q: side_b contains an empty or whitespace-only term (terms must be non-empty)", rs.ID)
		}
	}
	return s, nil
}

// containsEmptyOrWhitespaceTerm reports whether terms contains any element
// that is empty or whitespace-only (empty after strings.TrimSpace). Such an
// element is silently skipped by the scanner — scrubUnitMatches and
// anyTermInContent both `continue` on "" (they skip empty terms rather than
// matching) — so a term slice that is non-empty by length but contains an
// empty/whitespace element would load as a seemingly-valid subject that NEVER
// FIRES. That is a silent false-negative: it lets otherwise-blocked material
// into the acquired tree with no warning, the same honesty-contract failure
// mode the registry rejects for source_repos and unit: diff. Used to fail
// closed at load time for labels, side_a, and side_b.
func containsEmptyOrWhitespaceTerm(terms []string) bool {
	for _, t := range terms {
		if strings.TrimSpace(t) == "" {
			return true
		}
	}
	return false
}
