package corpus

import (
	"bytes"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"testing"
)

// TestCommitReviewerSeatsByteIdentical guards the load-bearing property that
// the four commit-reviewer seat prompts (a/b/c/d) never silently drift apart.
// Apart from each leaf's distinct `description:` frontmatter line, the four
// embedded sources must be byte-identical.
//
// This is a deliberate drift-prevention test, chosen over a generator (which
// would add maintenance machinery for four rarely-changing files) and over a
// renderer change (no per-invocation payoff). If a future edit changes one seat
// without the others, this test fails and surfaces the divergence.
func TestCommitReviewerSeatsByteIdentical(t *testing.T) {
	seats := []string{"a", "b", "c", "d"}
	base := "templates/core/.opencode/agents/commit-reviewer-"
	stripped := make([][]byte, len(seats))
	for i, s := range seats {
		b, err := CoreFS.ReadFile(base + s + ".md")
		if err != nil {
			t.Fatalf("read commit-reviewer-%s: %v", s, err)
		}
		stripped[i] = stripDescriptionLine(b)
	}
	for i := 1; i < len(stripped); i++ {
		if !bytes.Equal(stripped[0], stripped[i]) {
			t.Errorf("commit-reviewer-%s drifts from commit-reviewer-%s after stripping the description: line", seats[i], seats[0])
		}
	}
}

// stripDescriptionLine drops the single frontmatter line whose first column is
// `description:`. It is anchored to line start, so indented body occurrences
// (e.g. `    - evidence.description:`) are preserved and the only stripped line
// is the seat-specific frontmatter description.
func stripDescriptionLine(b []byte) []byte {
	lines := bytes.Split(b, []byte("\n"))
	out := make([][]byte, 0, len(lines))
	for _, ln := range lines {
		if bytes.HasPrefix(ln, []byte("description:")) {
			continue
		}
		out = append(out, ln)
	}
	return bytes.Join(out, []byte("\n"))
}

// skillDescriptionSoftCap is the SOFT, forward-looking BYTE ceiling for a
// SKILL.md frontmatter `description` value (length measured with Go len, i.e.
// bytes — not runes). It is intentionally stricter than the HARD cap enforced
// at runtime by internal/schema.ValidateSkillFrontmatter
// (skillDescriptionMaxLen = 1024), which rejects malformed/absurd descriptions.
// This cap guards the more ergonomic property that descriptions stay short
// enough for the skill catalog to consume cheaply: descriptions much over ~250
// bytes bloat the catalog every agent turn.
//
// It is a regression guard, not a runtime validation. Lengths are measured in
// bytes (len(string), Go idiom) over the EMBEDDED token form (CoreFS/OverlaysFS
// carry the unrendered {{PROJECT_NAME}} tokens); byte vs rune only differs by a
// few for the multibyte em-dashes/arrows present in some descriptions and does
// not move any skill across the 250 boundary.
const skillDescriptionSoftCap = 250

// grandfatheredSkillDescriptionCap records the embedded-token byte length of
// every shipped skill whose description currently EXCEEDS skillDescriptionSoftCap.
//
// These are INTENTIONALLY grandfathered: trimming them is a separate, motivated
// decision (see desc-cap-guard card non-goals), so the soft-cap guard does NOT
// require them under 250 today. The guard instead enforces three properties:
//
//  1. a skill NOT in this map must be <= skillDescriptionSoftCap (catches a new
//     or newly-lengthened description crossing the ceiling);
//  2. a skill IN this map must not GROW beyond its recorded value (catches the
//     realistic regression — an already-long description creeping longer);
//  3. a skill IN this map whose ACTUAL length has since dropped to <= the soft
//     cap is a positive signal — its entry is now stale and must be removed so
//     the skill is thereafter held to the cap (prevents it from later regrowing
//     toward the stale, higher ceiling).
//
// An entry whose RECORDED value is <= skillDescriptionSoftCap is itself a
// failure (a stale entry that should never have shipped above the cap).
//
// Values are byte lengths over the embedded token form, re-derived by the test.
var grandfatheredSkillDescriptionCap = map[string]int{
	"think-mode":            520,
	"repo-recon":            433,
	"media-perception":      372,
	"debugging-loop":        335,
	"compaction-discipline": 328,
	"backlog":               322,
	"diagnostics-export":    306,
	"tdd-loop":              303,
	"skill-creator":         282,
	"harness-operator":      265,
	"gated-commit":          260,
}

// TestSkillDescriptionSoftCap guards the per-skill description length against
// silent growth. It reads every embedded SKILL.md from CoreFS (the curated core
// skills) and OverlaysFS (the shipped overlay-pack pilot skills), mirroring how
// TestCommitReviewerSeatsByteIdentical reads the corpus through the embed FSes.
//
// On failure it reports each offending skill with its actual length and the
// applicable ceiling, so a maintainer can either trim the description or update
// the grandfather map deliberately.
func TestSkillDescriptionSoftCap(t *testing.T) {
	skills, err := readEmbeddedSkillFiles()
	if err != nil {
		t.Fatalf("read embedded skills: %v", err)
	}
	if len(skills) == 0 {
		t.Fatal("readEmbeddedSkillFiles returned no skills (embed walk regression)")
	}

	// Stable order for readable failure output.
	names := make([]string, 0, len(skills))
	for n := range skills {
		names = append(names, n)
	}
	sort.Strings(names)

	var failures []string
	for _, name := range names {
		desc, ok := skillDescriptionValue(skills[name])
		if !ok {
			failures = append(failures, fmt.Sprintf("%s: no frontmatter `description:` line found", name))
			continue
		}
		if isYAMLBlockScalarIndicator(desc) {
			// A block/fold scalar (`>-`, `|`, ...) is measured here as a 2-byte
			// indicator and would pass the cap vacuously regardless of the real
			// (indented, multi-line) description — an evasion the guard cannot
			// see. Require an inline scalar so length is always meaningful.
			failures = append(failures, fmt.Sprintf(
				"%s: description uses a YAML block/fold scalar indicator (%q); the soft-cap guard measures inline scalars only — convert to an inline (bare or double-quoted) scalar",
				name, desc))
			continue
		}
		got := len(desc)
		ceiling, grandfathered := grandfatheredSkillDescriptionCap[name]
		switch {
		case grandfathered && ceiling <= skillDescriptionSoftCap:
			// Stale entry: the RECORDED ceiling never exceeded the cap, so the
			// grandfather record must be removed (it should never have shipped).
			failures = append(failures, fmt.Sprintf(
				"%s: grandfathered ceiling %d <= soft cap %d; remove the stale grandfather entry (the recorded value never exceeded the cap)",
				name, ceiling, skillDescriptionSoftCap))
		case grandfathered && got <= skillDescriptionSoftCap:
			// Whittle-down positive signal: the description has since been
			// trimmed below the cap, so the exemption is no longer needed and
			// the entry is stale — remove it so the skill is held to the cap.
			failures = append(failures, fmt.Sprintf(
				"%s: description is now %d bytes (<= soft cap %d); remove the stale grandfather entry so the skill is held to the cap",
				name, got, skillDescriptionSoftCap))
		case grandfathered:
			if got > ceiling {
				failures = append(failures, fmt.Sprintf(
					"%s: description grew to %d bytes, exceeding its grandfathered ceiling of %d (trim it or raise the ceiling deliberately)",
					name, got, ceiling))
			}
		default:
			if got > skillDescriptionSoftCap {
				failures = append(failures, fmt.Sprintf(
					"%s: description is %d bytes, over the soft cap of %d (trim it, or add a deliberate grandfather entry if untrimmable)",
					name, got, skillDescriptionSoftCap))
			}
		}
	}
	for _, f := range failures {
		t.Error(f)
	}
}

// readEmbeddedSkillFiles walks CoreFS and OverlaysFS for SKILL.md files and
// returns their bytes keyed by skill directory name. It mirrors the embed
// access in internal/cli/skill.go (coreSkillSources / readEmbedSkills) but
// stays self-contained in this test so the guard does not couple to CLI code.
func readEmbeddedSkillFiles() (map[string][]byte, error) {
	out := map[string][]byte{}

	coreSub, err := fs.Sub(CoreFS, CoreDir)
	if err != nil {
		return nil, fmt.Errorf("core sub: %w", err)
	}
	if err := walkSkills(coreSub, ".opencode/skills", out); err != nil {
		return nil, fmt.Errorf("core walk: %w", err)
	}

	ovlSub, err := fs.Sub(OverlaysFS, OverlaysDir)
	if err != nil {
		return nil, fmt.Errorf("overlays sub: %w", err)
	}
	// Overlay packs live one level under the overlays root
	// (<pack>/skills/<name>/SKILL.md). Walk the whole overlays sub-tree.
	if err := walkSkills(ovlSub, ".", out); err != nil {
		return nil, fmt.Errorf("overlays walk: %w", err)
	}
	return out, nil
}

// walkSkills walks fsys from root and records every SKILL.md whose path crosses
// a `skills/<name>/` directory, keyed by <name>.
func walkSkills(fsys fs.FS, root string, out map[string][]byte) error {
	return fs.WalkDir(fsys, root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // a missing subtree is non-fatal
		}
		if d.IsDir() || path.Base(p) != "SKILL.md" {
			return nil
		}
		// Path shape: .../skills/<name>/SKILL.md
		if path.Base(path.Dir(path.Dir(p))) != "skills" {
			return nil
		}
		name := path.Base(path.Dir(p))
		content, rerr := fs.ReadFile(fsys, p)
		if rerr != nil {
			return rerr
		}
		out[name] = content
		return nil
	})
}

// skillDescriptionValue returns the frontmatter `description` scalar of a
// SKILL.md. It finds the FIRST line anchored at column 0 with prefix
// `description:` (the frontmatter key, which always precedes the body), then
// strips a single pair of surrounding double quotes if present (mirroring
// stripDescriptionLine's line-start anchoring). Indented body occurrences such
// as `    - evidence.description:` are never matched.
func skillDescriptionValue(b []byte) (string, bool) {
	for _, ln := range bytes.Split(b, []byte("\n")) {
		if !bytes.HasPrefix(ln, []byte("description:")) {
			continue
		}
		rest := bytes.TrimSpace(ln[len("description:"):])
		if len(rest) >= 2 && rest[0] == '"' && rest[len(rest)-1] == '"' {
			rest = rest[1 : len(rest)-1]
		}
		return string(rest), true
	}
	return "", false
}

// yamlBlockScalarIndicators are the YAML block/fold scalar introducers. When a
// description value is exactly one of these (the unquoted indicator that begins
// an indented continuation block), skillDescriptionValue returns it as the
// 2-char value; measuring that would pass the cap vacuously. The full set with
// optional chomping/keep indicators (`>`, `>-`, `>+`, `|`, `|-`, `|+`) is closed.
var yamlBlockScalarIndicators = map[string]bool{
	">":  true,
	">-": true,
	">+": true,
	"|":  true,
	"|-": true,
	"|+": true,
}

// isYAMLBlockScalarIndicator reports whether s is a YAML block/fold scalar
// introducer — the evasion form the inline-scalar guard cannot length-measure.
func isYAMLBlockScalarIndicator(s string) bool {
	return yamlBlockScalarIndicators[s]
}

// TestSkillDescriptionValueParser is a focused unit test for the line-level
// extraction helper, covering bare and quoted scalars and the no-match case.
func TestSkillDescriptionValueParser(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   string
		want string
		ok   bool
	}{
		{"bare", "---\nname: x\ndescription: hello world\n---\nbody\n", "hello world", true},
		{"quoted", "---\ndescription: \"hello, world\"\n---\n", "hello, world", true},
		{"empty_quoted", "description: \"\"", "", true},
		{"indented_body_ignored", "---\ndescription: top\n---\n    description: indented\n", "top", true},
		{"missing", "---\nname: x\n---\n", "", false},
		{"trailing_spaces_trimmed", "description:   pad   ", "pad", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := skillDescriptionValue([]byte(tc.in))
			if ok != tc.ok {
				t.Fatalf("ok = %v, want %v", ok, tc.ok)
			}
			if ok && got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// TestYAMLBlockScalarIndicatorDetection covers the closed set of block/fold
// scalar introducers and confirms ordinary description text is not mis-flagged.
func TestYAMLBlockScalarIndicatorDetection(t *testing.T) {
	for _, indicator := range []string{">", ">-", ">+", "|", "|-", "|+"} {
		if !isYAMLBlockScalarIndicator(indicator) {
			t.Errorf("expected %q to be detected as a block scalar indicator", indicator)
		}
	}
	for _, text := range []string{"hello world", "", "- leading dash body", "pipe|in middle", "\"quoted\"", "> not bare (has trailing text)"} {
		if isYAMLBlockScalarIndicator(text) {
			t.Errorf("did not expect %q to be flagged as a block scalar indicator", text)
		}
	}
}
