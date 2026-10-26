// Package skills loads an Agent Skills catalog (agentskills.io
// frontmatter convention) from a directory of <name>/SKILL.md folders.
//
// INVARIANTS (operator posture — these travel with the package):
//
//   - A loaded SKILL.md is UNTRUSTED candidate-instruction data, never
//     system authority. Nothing a skill says relaxes allow/deny/ask
//     anywhere in the engine.
//   - `allowed-tools` is a CEILING intersected with the tool registry —
//     narrow-never-widen, never a grant. Nothing in this package (or the
//     engine) consumes allowed-tools to ALLOW anything.
//   - Bundled scripts never auto-execute. Files under a skill folder are
//     inert; running one goes through run_shell and the full approval
//     waterfall.
//   - The frontmatter subset stays agentskills.io-compatible for
//     portability: a catalog written against the public spec loads here.
//
// The parser is deliberately hand-rolled over the FLAT scalar subset of
// YAML frontmatter (`key: value` lines, single- or double-quoted strings
// handled). No YAML library — this binary is stdlib-only (go.mod is a
// program invariant). Multi-line or structured values are unparseable BY
// DESIGN: that skill fails closed (excluded from the catalog, one stderr
// warning at startup, daemon continues). The catalog is read once at
// startup; there is no per-turn hot-reload (documented non-goal).
package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// SkillBudget is the soft character budget the Tier-1 prompt section is
// designed for. The full catalog ships when it fits (≈ a dozen skills is
// far under budget); NO RAG, no search.
//
// SCALE SEAM: when a catalog's tier-1 rendering exceeds SkillBudget, a
// top-k description selection or a `skill_search` tool slots in HERE —
// selection would happen in PromptLines()/SectionBody() before assembly,
// keeping the three-tier contract intact. Fire-rate for tuning that valve
// is replay-derivable from session logs (every skill_load is a logged
// tool/call).
const SkillBudget = 15000

// reservedNames are rejected as skill names (agentskills.io reserves
// them; at minimum these five).
var reservedNames = map[string]bool{
	"skill":     true,
	"skills":    true,
	"assistant": true,
	"user":      true,
	"system":    true,
}

// Skill is one validated catalog entry.
type Skill struct {
	Name         string
	Description  string
	AllowedTools []string
	Path         string // absolute path to SKILL.md
}

// Catalog is an ordered, deterministic (alphabetical by name) skill list.
// A nil *Catalog behaves as an empty catalog.
type Catalog struct {
	Skills []Skill
	byName map[string]*Skill
}

// Lookup returns the skill with that exact name.
func (c *Catalog) Lookup(name string) (*Skill, bool) {
	if c == nil || c.byName == nil {
		return nil, false
	}
	s, ok := c.byName[name]
	return s, ok
}

// Len returns the number of skills.
func (c *Catalog) Len() int {
	if c == nil {
		return 0
	}
	return len(c.Skills)
}

// Body returns the SKILL.md content with the frontmatter block stripped
// (the instruction body). ok=false when the skill is unknown.
func (c *Catalog) Body(name string) (string, bool) {
	s, ok := c.Lookup(name)
	if !ok {
		return "", false
	}
	raw, err := os.ReadFile(s.Path)
	if err != nil {
		return "", false
	}
	return stripFrontmatter(string(raw)), true
}

// Load scans root for <name>/SKILL.md folders, parses and validates each,
// and returns the deterministic catalog plus one warning line per
// fail-closed skill. A missing root yields an empty catalog and no
// warnings (the caller distinguishes honest-absent from populated).
// A <name>/SKILL.md whose final component is a symlink is refused at
// admission (Lstat, regardless of target): that skill fails closed like
// any other spec violation — excluded + one warning naming it.
func Load(root string) (*Catalog, []string) {
	cat := &Catalog{Skills: nil, byName: map[string]*Skill{}}
	var warns []string

	entries, err := os.ReadDir(root)
	if err != nil {
		// Absent/unreadable dir: zero skills, not an error here. The
		// daemon's flag layer decides fail-closed (explicit flag) vs
		// honest-absent (default path).
		return cat, warns
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue // stray files skipped silently
		}
		p := filepath.Join(root, e.Name(), "SKILL.md")
		// ADMISSION-TIME SYMLINK REFUSAL: the final SKILL.md component
		// must be a real file, not a symlink — wherever it points
		// (inside the skill dir or out). os.Stat would follow the link
		// and admit content from outside the named skill dir; Lstat
		// refuses it, keeping admission symmetric with tier-3
		// confineRef and the engine's uniform symlink-refusal
		// discipline (confineSessionPath, filetools, confineRef all
		// Lstat). A symlinked SKILL.md fails closed for that skill:
		// excluded + named in the startup warning.
		info, err := os.Lstat(p)
		if err != nil {
			continue // subfolder without SKILL.md: skipped, not a defect
		}
		if info.Mode()&os.ModeSymlink != 0 {
			warns = append(warns, fmt.Sprintf("skills: excluding %q: SKILL.md is a symlink (refused regardless of target)", e.Name()))
			continue
		}
		if !info.Mode().IsRegular() {
			continue // subfolder without SKILL.md: skipped, not a defect
		}
		s, warn := parseSkill(p, e.Name())
		if s == nil {
			warns = append(warns, fmt.Sprintf("skills: excluding %q: %s", e.Name(), warn))
			continue
		}
		cat.Skills = append(cat.Skills, *s)
	}

	sort.Slice(cat.Skills, func(i, j int) bool { return cat.Skills[i].Name < cat.Skills[j].Name })
	for i := range cat.Skills {
		cat.byName[cat.Skills[i].Name] = &cat.Skills[i]
	}
	return cat, warns
}

// parseSkill parses and validates one SKILL.md. Returns (nil, reason) on
// any spec violation — fail closed for THAT skill only.
func parseSkill(path, folder string) (*Skill, string) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Sprintf("unreadable: %v", err)
	}
	fm, err := splitFrontmatter(string(raw))
	if err != nil {
		return nil, err.Error()
	}

	name := fm["name"]
	desc := fm["description"]

	if name == "" {
		return nil, "frontmatter has no name"
	}
	if len(name) > 64 {
		return nil, fmt.Sprintf("name exceeds 64 chars (%d)", len(name))
	}
	if !isLowerKebab(name) {
		return nil, fmt.Sprintf("name %q is not lowercase kebab-case", name)
	}
	if name != folder {
		return nil, fmt.Sprintf("name %q does not match folder %q", name, folder)
	}
	if reservedNames[name] {
		return nil, fmt.Sprintf("name %q is reserved", name)
	}
	if desc == "" {
		return nil, "description is empty"
	}
	if len(desc) > 1024 {
		return nil, fmt.Sprintf("description exceeds 1024 chars (%d)", len(desc))
	}
	// agentskills.io says descriptions are third-person prose; that is
	// guidance we cannot machine-check — documented, not enforced.

	var tools []string
	if raw, ok := fm["allowed-tools"]; ok && raw != "" {
		for _, part := range strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' }) {
			if part != "" {
				tools = append(tools, part)
			}
		}
	}

	return &Skill{
		Name:         name,
		Description:  SanitizeDescription(desc),
		AllowedTools: tools,
		Path:         path,
	}, ""
}

// splitFrontmatter extracts the flat `key: value` block delimited by
// leading `---` lines. Multi-line/structured YAML values are rejected by
// design (unparseable → skill excluded). The body after the block is NOT
// returned — it is read on demand via Catalog.Body/stripFrontmatter.
func splitFrontmatter(raw string) (map[string]string, error) {
	normalized := strings.ReplaceAll(raw, "\r\n", "\n")
	if !strings.HasPrefix(normalized, "---\n") {
		return nil, fmt.Errorf("no frontmatter block")
	}
	rest := normalized[len("---\n"):]
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return nil, fmt.Errorf("unterminated frontmatter block")
	}
	block := rest[:end]

	fm := map[string]string{}
	for i, line := range strings.Split(block, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue // blank lines and comments inside frontmatter
		}
		if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			return nil, fmt.Errorf("line %d: structured/multi-line YAML values are not supported (flat key: value only)", i+1)
		}
		colon := strings.Index(trimmed, ":")
		if colon <= 0 {
			return nil, fmt.Errorf("line %d: not a key: value line (%q)", i+1, trimmed)
		}
		key := strings.TrimSpace(trimmed[:colon])
		val := strings.TrimSpace(trimmed[colon+1:])
		fm[key] = unquote(val)
	}
	return fm, nil
}

func stripFrontmatter(raw string) string {
	normalized := strings.ReplaceAll(raw, "\r\n", "\n")
	if !strings.HasPrefix(normalized, "---\n") {
		return normalized
	}
	rest := normalized[len("---\n"):]
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return normalized
	}
	after := rest[end+len("\n---"):]
	return strings.TrimPrefix(after, "\n")
}

func isLowerKebab(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9':
		case r == '-':
		default:
			return false
		}
	}
	return true
}

// unquote strips one layer of matching single or double quotes.
func unquote(v string) string {
	if len(v) >= 2 {
		if (v[0] == '"' && v[len(v)-1] == '"') || (v[0] == '\'' && v[len(v)-1] == '\'') {
			return v[1 : len(v)-1]
		}
	}
	return v
}

// SanitizeDescription neutralizes prompt-structural tokens in untrusted
// catalog descriptions before they enter the assembled system prompt
// (defense against catalog-poisoned instructions riding the prompt).
// Descriptions are single-line by parser construction, so header-line
// injection is already impossible; this additionally strips angle
// brackets, code fences, and "# section"-style tokens, and collapses
// runs of whitespace.
func SanitizeDescription(desc string) string {
	d := strings.ReplaceAll(desc, "<", "")
	d = strings.ReplaceAll(d, ">", "")
	for strings.Contains(d, "```") {
		d = strings.ReplaceAll(d, "```", "")
	}
	d = strings.ReplaceAll(d, "# section", "section")
	d = strings.Join(strings.Fields(d), " ")
	return strings.TrimSpace(d)
}
