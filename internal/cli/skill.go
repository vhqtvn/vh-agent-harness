package cli

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	corpus "github.com/vhqtvn/vh-agent-harness"
	"github.com/vhqtvn/vh-agent-harness/internal/overlay"
	"github.com/vhqtvn/vh-agent-harness/internal/schema"
)

// skillCmd exposes native skill discoverability + validation. Skills are generic
// corpus files (no skill-specific manifest); this command gives operators a
// python-free, glob-free way to see and check them — and a doctor-visible health
// signal. It is the Slice-1 "Skill Visibility & Health" surface.
//
// After `vh-agent-harness update` adds or changes skills under .opencode/skills/,
// a running opencode session will NOT see them until restart (opencode caches the
// skill list per-process — the D1 staleness bug). The list/validate reads here
// are always fresh (they walk the embed/rendered trees directly), but that does
// not refresh opencode's in-process cache.
var skillCmd = &cobra.Command{
	Use:           "skill",
	Short:         "List and validate OpenCode skills",
	SilenceUsage:  true,
	SilenceErrors: true,
	Long: `Inspect the OpenCode skills available in this project.

Subcommands:
  list      Show every skill (core, overlay-pack, and rendered) with its source,
            whether it is rendered to .opencode/skills/, and whether its SKILL.md
            frontmatter is valid.
  validate  Validate one or more skill directories' SKILL.md frontmatter
            (name rules, description limits, compatibility). With no arguments,
            validates every rendered skill under .opencode/skills/.

Skills are generic corpus files, not opencode-specific to this binary. After
` + "`vh-agent-harness update`" + ` adds or changes skills, restart opencode so the
running session picks them up (opencode caches the skill list per-process).`,
}

var skillTargetFlag string

var skillListCmd = &cobra.Command{
	Use:           "list",
	Short:         "List installed skills (core, overlay, rendered) with validity",
	Args:          cobra.NoArgs,
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE:          runSkillList,
}

var skillValidateCmd = &cobra.Command{
	Use:   "validate [skill-dir...]",
	Short: "Validate SKILL.md frontmatter for one or more skill directories",
	Long: `Validate the SKILL.md frontmatter of one or more skill directories.

Each argument is a skill directory containing a SKILL.md file (e.g.
.opencode/skills/backlog). With no arguments, every rendered skill under
.opencode/skills/ is validated. Exits non-zero if any skill fails.

Checks (ported from the skill-creator's quick_validate.py):
  - YAML frontmatter is present and parses
  - name: matches ^[a-z0-9]+(-[a-z0-9]+)*$, <=64 chars, equals the directory name
  - description: non-empty, <=1024 chars, no angle brackets
  - compatibility: if present, must equal "opencode"`,
	Args:          cobra.ArbitraryArgs,
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE:          runSkillValidate,
}

func init() {
	skillCmd.AddCommand(skillListCmd, skillValidateCmd)
	skillCmd.PersistentFlags().StringVarP(&skillTargetFlag, "target", "o", "",
		"target directory (default: current directory)")
}

func runSkillList(cmd *cobra.Command, _ []string) (err error) {
	defer func() { reportRunErrToStderr(cmd, err) }()
	return listSkills(cmd.OutOrStdout(), resolveSkillTarget())
}

func runSkillValidate(cmd *cobra.Command, args []string) (err error) {
	defer func() { reportRunErrToStderr(cmd, err) }()
	return validateSkills(cmd.OutOrStdout(), resolveSkillTarget(), args)
}

func resolveSkillTarget() string {
	if skillTargetFlag != "" {
		return skillTargetFlag
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return cwd
}

// skillRow is one row of the skill-list table.
type skillRow struct {
	name     string
	source   string // "core" | "overlay:<pack>" | "rendered"
	rendered bool
	valid    bool
	note     string // empty when valid; one-line reason when not
}

// listSkills walks the embedded core corpus, the active overlay packs, and the
// live rendered .opencode/skills/ tree, then prints a provenance+validity table.
// Provenance priority for the SOURCE column is overlay > core > rendered-only.
func listSkills(out io.Writer, target string) error {
	coreSrc, err := coreSkillSources()
	if err != nil {
		return err
	}
	// Overlay skill sources: walk each active overlay pack (declared order).
	type ovlEntry struct {
		source  string
		content []byte
	}
	overlaySrc := map[string]ovlEntry{}
	for _, pack := range activeOverlays(target) {
		p, perr := overlay.OpenPackFor(target, pack)
		if perr != nil {
			continue
		}
		skills, _ := readEmbedSkills(p.FS, "skills")
		for name, content := range skills {
			overlaySrc[name] = ovlEntry{source: "overlay:" + pack, content: content}
		}
	}
	rendered := renderedSkillNames(target)

	// Union of all skill names.
	names := map[string]bool{}
	for n := range coreSrc {
		names[n] = true
	}
	for n := range overlaySrc {
		names[n] = true
	}
	for n := range rendered {
		names[n] = true
	}
	sorted := make([]string, 0, len(names))
	for n := range names {
		sorted = append(sorted, n)
	}
	sort.Strings(sorted)

	rows := make([]skillRow, 0, len(sorted))
	renderedCount, validCount := 0, 0
	for _, n := range sorted {
		var row skillRow
		row.name = n
		row.rendered = rendered[n]
		if row.rendered {
			renderedCount++
		}
		// Resolve source + content: overlay > core > rendered-only.
		switch {
		case overlaySrc[n].content != nil:
			row.source = overlaySrc[n].source
			row.valid, row.note = checkSkillContent(overlaySrc[n].content, n)
		case coreSrc[n] != nil:
			row.source = "core"
			row.valid, row.note = checkSkillContent(coreSrc[n], n)
		default:
			// Rendered-only (no embed/overlay source): validate the live file.
			row.source = "rendered"
			content, rerr := os.ReadFile(filepath.Join(target, ".opencode", "skills", n, "SKILL.md"))
			if rerr != nil {
				row.valid = false
				row.note = oneLine(rerr.Error())
			} else {
				row.valid, row.note = checkSkillContent(content, n)
			}
		}
		if row.valid {
			validCount++
		}
		rows = append(rows, row)
	}

	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tSOURCE\tRENDERED\tFRONTMATTER")
	for _, r := range rows {
		renderedStr := yesNo(r.rendered, "yes", "no")
		fmStr := "yes"
		if !r.valid {
			fmStr = "no"
			if r.note != "" {
				fmStr = "no: " + oneLine(r.note)
			}
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", r.name, r.source, renderedStr, fmStr)
	}
	tw.Flush()
	fmt.Fprintf(out, "\n%d skill(s): %d rendered, %d valid.\n", len(rows), renderedCount, validCount)
	return nil
}

// checkSkillContent runs the frontmatter validator and maps the result to a
// (valid, note) pair.
func checkSkillContent(content []byte, dirName string) (bool, string) {
	if err := schema.ValidateSkillFrontmatter(content, dirName); err != nil {
		return false, err.Error()
	}
	return true, ""
}

// validateSkills validates one or more skill directories. With no dirs, it
// validates every rendered skill under <target>/.opencode/skills/.
func validateSkills(out io.Writer, target string, dirs []string) error {
	if len(dirs) == 0 {
		rendered := renderedSkillNames(target)
		if len(rendered) == 0 {
			fmt.Fprintf(out, "no rendered skills found under %s\n", filepath.Join(target, ".opencode", "skills"))
			return nil
		}
		for n := range rendered {
			dirs = append(dirs, filepath.Join(target, ".opencode", "skills", n))
		}
		sort.Strings(dirs)
	}

	failures := 0
	for _, dir := range dirs {
		name := filepath.Base(dir)
		skillMD := filepath.Join(dir, "SKILL.md")
		content, err := os.ReadFile(skillMD)
		if err != nil {
			fmt.Fprintf(out, "FAIL  %s  — SKILL.md not found: %s\n", name, oneLine(err.Error()))
			failures++
			continue
		}
		if verr := schema.ValidateSkillFrontmatter(content, name); verr != nil {
			fmt.Fprintf(out, "FAIL  %s  — %s\n", name, oneLine(verr.Error()))
			failures++
			continue
		}
		fmt.Fprintf(out, "OK    %s  — frontmatter valid\n", name)
	}
	if failures > 0 {
		return fmt.Errorf("%d of %d skill(s) failed validation", failures, len(dirs))
	}
	return nil
}

// coreSkillSources walks the embedded core corpus and returns the SKILL.md bytes
// for every core skill, keyed by skill directory name.
func coreSkillSources() (map[string][]byte, error) {
	sub, err := fs.Sub(corpus.CoreFS, corpus.CoreDir)
	if err != nil {
		return nil, fmt.Errorf("core skills: %w", err)
	}
	return readEmbedSkills(sub, ".opencode/skills")
}

// readEmbedSkills walks an embed/overlay FS for "<skillsRel>/<name>/SKILL.md"
// files and returns them keyed by skill name. A missing skillsRel directory
// yields an empty map (not an error) so overlay packs with no skills/ are
// tolerated.
func readEmbedSkills(fsys fs.FS, skillsRel string) (map[string][]byte, error) {
	out := map[string][]byte{}
	walkErr := fs.WalkDir(fsys, skillsRel, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			// Root or a subtree is missing (e.g. an overlay pack with no
			// skills/ dir) — nothing to walk there.
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if path.Base(p) != "SKILL.md" {
			return nil
		}
		name := path.Base(path.Dir(p))
		content, rerr := fs.ReadFile(fsys, p)
		if rerr != nil {
			return nil
		}
		out[name] = content
		return nil
	})
	return out, walkErr
}

// renderedSkillNames lists skill directory names that have a SKILL.md on disk
// under <target>/.opencode/skills/. A missing directory yields an empty set.
//
// This is the shared helper backing BOTH `skill list` (the RENDERED column) and
// `validateSkills` (which validates only present skills). Its "SKILL.md must be
// present" filter is correct for those paths: a half-deleted skill (dir present,
// SKILL.md deleted) is legitimately not rendered and is out of scope for those
// verbs. doctor's checkSkillValidity must NOT use this helper — a deleted
// SKILL.md is a health-gate FAIL there — so it uses renderedSkillDirNames
// instead. Do not change this function's semantics without auditing both
// consumers; the list path is intentionally unchanged by the F2 doctor fix.
func renderedSkillNames(target string) map[string]bool {
	out := map[string]bool{}
	dir := filepath.Join(target, ".opencode", "skills")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return out
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if isRegularFile(filepath.Join(dir, e.Name(), "SKILL.md")) {
			out[e.Name()] = true
		}
	}
	return out
}

// renderedSkillDirNames lists EVERY skill directory name under
// <target>/.opencode/skills/, regardless of whether a SKILL.md is present. A
// missing .opencode/skills/ directory yields nil. It is the directory set
// doctor's checkSkillValidity enumerates INDEPENDENTLY of renderedSkillNames so
// a half-deleted skill (directory present, SKILL.md absent) is not silently
// invisible to the health gate. `skill list` continues to use
// renderedSkillNames (SKILL.md-present filter) for its RENDERED column, so this
// helper does not affect list output.
func renderedSkillDirNames(target string) []string {
	dir := filepath.Join(target, ".opencode", "skills")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			out = append(out, e.Name())
		}
	}
	return out
}

// oneLine collapses a message to a single line so it never breaks table rows.
func oneLine(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "\n", " "), "\r", "")
}
