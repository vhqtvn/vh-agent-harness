package cli

// overlay_new.go implements `vh-agent-harness overlay new` — the one-command
// scaffolder that takes a consumer agent from "I need a new agent/command/skill"
// to a renderable overlay pack, fail-closed preview included.
//
// It writes a pack directory under .vh-agent-harness/overlays/<name>/ carrying
// per-unit skeletons (agents/commands/skills), an opencode-append.jsonc that
// wires the agent into the opencode config, and the optional permission-pack +
// callable-graph doc. It then appends <name> to the `overlays:` list in
// vh-harness-profile.yml — via the SAME schema load/marshal path the Reconciler
// uses (schema.HarnessProfile.AppendOverlay), NEVER a text/regex edit on the
// platform_armed file.
//
// --dry-run prints the full file-creation manifest AND the expected profile
// diff and writes nothing.

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	corpus "github.com/vhqtvn/vh-agent-harness"
	"github.com/vhqtvn/vh-agent-harness/internal/schema"
)

// overlayNewFlags holds the inputs to `vh-agent-harness overlay new`.
type overlayNewFlags struct {
	target  string
	agent   string
	command string
	skill   string
	dryRun  bool
}

var overlayNewFl *overlayNewFlags

// overlayCmd is the parent for overlay-management verbs. `overlay new` is the
// first subcommand; future verbs (e.g. `overlay list`) hang off this parent.
var overlayCmd = &cobra.Command{
	Use:   "overlay",
	Short: "Manage overlay packs (agents/commands/skills contributed by overlays)",
	Long: `Manage overlay packs under .vh-agent-harness/overlays/. An overlay pack
carries project-specific agents, commands, and/or skills plus an
opencode-append.jsonc that wires them into the opencode config. A pack is
activated by listing its name under ` + "`overlays:`" + ` in
.vh-agent-harness/vh-harness-profile.yml; ` + "`vh-agent-harness update`" + `
then renders it into .opencode/.

Subcommands:
  new <name>   Scaffold a new overlay pack and wire it into the profile.`,
}

var overlayNewCmd = &cobra.Command{
	Use:           "new <name>",
	Short:         "Scaffold a new overlay pack and wire it into vh-harness-profile.yml",
	SilenceUsage:  true,
	SilenceErrors: true,
	Long: `Scaffold a new overlay pack at .vh-agent-harness/overlays/<name>/ and append
<name> to the ` + "`overlays:`" + ` list in .vh-agent-harness/vh-harness-profile.yml.

The pack is created from domain-free embedded skeletons. Pass at least one unit
flag so the pack contributes something renderable:

  --agent <n>     create agents/<n>.md (subagent) + wire it into opencode-append.jsonc
  --command <n>   create commands/<n>.md (slash command)
  --skill <n>     create skills/<n>/SKILL.md

A pack may carry one of each unit type. Every pack also gets an
opencode-append.jsonc (active agent wiring when --agent is given), a
permission-pack.jsonc (LIVE self-descriptor; effective on the next ` + "`update`" + `
once the pack is listed under ` + "`overlays:`" + `), and a callable-graph-snippet.md
(fully HTML-commented, inert until you uncomment it).

The profile edit is structural, not textual: <name> is appended through the
schema's own load/marshal path (the same one ` + "`update`" + ` reconciles with),
so the armed file stays reconcile-clean — a subsequent ` + "`update`" + ` raises
no conflict/proposal on it ("clean" means no armed-file conflict, NOT that the
whole ` + "`update`" + ` is a no-op; a first ` + "`update`" + ` still emits normal
platform-seed/managed applies for the new pack).

--dry-run prints the full file-creation manifest plus the exact profile diff and
writes nothing. Existing files are never overwritten: if the pack directory or
any target file already exists, the command fails with a clear message and
writes nothing.

Run ` + "`vh-agent-harness update`" + ` afterward to render the pack into .opencode/.`,
	Args: cobra.ExactArgs(1),
	RunE: runOverlayNew,
}

func init() {
	overlayNewFl = &overlayNewFlags{target: "."}
	overlayNewCmd.Flags().StringVarP(&overlayNewFl.target, "target", "o", overlayNewFl.target,
		"project root containing .vh-agent-harness/ (default: current directory)")
	overlayNewCmd.Flags().StringVar(&overlayNewFl.agent, "agent", "",
		"name of an agent skeleton to create (agents/<name>.md)")
	overlayNewCmd.Flags().StringVar(&overlayNewFl.command, "command", "",
		"name of a slash-command skeleton to create (commands/<name>.md)")
	overlayNewCmd.Flags().StringVar(&overlayNewFl.skill, "skill", "",
		"name of a skill skeleton to create (skills/<name>/SKILL.md)")
	overlayNewCmd.Flags().BoolVar(&overlayNewFl.dryRun, "dry-run", false,
		"preview the file-creation manifest + profile diff without writing anything")

	overlayCmd.AddCommand(overlayNewCmd)
}

// nameRe is the filesystem-safe name contract for pack names and unit names:
// lowercase alphanumerics with internal dots/dashes/underscores, starting and
// ending alphanumeric. Rejects empty, uppercase, slashes, leading separators,
// and path-traversal sequences.
var nameRe = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9._-]*[a-z0-9])?$`)

// skeletonEmbed maps a unit type to its embedded skeleton file name.
var skeletonEmbed = map[string]string{
	"agent":   "agent.md",
	"command": "command.md",
	"skill":   "skill.md",
}

// plannedFile is one file the scaffolder intends to create.
type plannedFile struct {
	relPath string // repo-relative (pack-rooted) path, e.g. overlays/<pack>/agents/foo.md
	content []byte
}

// runOverlayNew is the RunE for `vh-agent-harness overlay new`.
func runOverlayNew(cmd *cobra.Command, args []string) error {
	out := cmd.OutOrStdout()
	errOut := cmd.ErrOrStderr()

	name := args[0]
	if err := validateOverlayName("pack", name); err != nil {
		fmt.Fprintln(errOut, "error:", err)
		return errSilent{}
	}

	target, err := filepath.Abs(overlayNewFl.target)
	if err != nil {
		fmt.Fprintln(errOut, "error:", fmt.Errorf("resolve target: %w", err))
		return errSilent{}
	}
	if !isExistingDir(filepath.Join(target, ".vh-agent-harness")) {
		fmt.Fprintf(errOut, "error: no .vh-agent-harness/ at %s\n", target)
		fmt.Fprintln(errOut, "       run `vh-agent-harness install` here first, then re-run `overlay new`.")
		return errSilent{}
	}

	// Validate every requested unit name up front (fail-closed before any plan).
	units := map[string]string{}
	for kind, val := range map[string]string{
		"agent":   overlayNewFl.agent,
		"command": overlayNewFl.command,
		"skill":   overlayNewFl.skill,
	} {
		if strings.TrimSpace(val) == "" {
			continue
		}
		if err := validateOverlayName(kind, val); err != nil {
			fmt.Fprintln(errOut, "error:", err)
			return errSilent{}
		}
		units[kind] = val
	}

	packDir := filepath.Join(target, ".vh-agent-harness", "overlays", name)
	packDirExists := isExistingDir(packDir)

	plan, err := buildOverlayPlan(name, units)
	if err != nil {
		fmt.Fprintln(errOut, "error:", err)
		return errSilent{}
	}

	// Never overwrite an existing file. Check the whole plan before writing the
	// first byte. This is also the rejection path for a pack that already exists
	// WITH files in it (a populated pack dir surfaces as per-file conflicts).
	var conflicts []string
	for _, pf := range plan {
		if _, statErr := os.Stat(filepath.Join(packDir, filepath.FromSlash(pf.relPath))); statErr == nil {
			conflicts = append(conflicts, pf.relPath)
		}
	}
	if len(conflicts) > 0 {
		fmt.Fprintf(errOut, "error: refusing to overwrite existing file(s) in pack %q:\n", name)
		for _, c := range conflicts {
			fmt.Fprintf(errOut, "       overlays/%s/%s\n", name, c)
		}
		fmt.Fprintln(errOut, "       nothing was written. Remove the conflicting file(s) or pick a new pack name.")
		return errSilent{}
	}
	// A pack dir that exists but holds none of the plan files (empty dir) is still
	// a collision: the pack name is already taken on disk.
	if packDirExists {
		fmt.Fprintf(errOut, "error: overlay pack %q already exists at %s\n", name, relFromTarget(target, packDir))
		fmt.Fprintln(errOut, "       choose a different name, or run with --dry-run to preview a fresh pack.")
		return errSilent{}
	}

	// --- Profile diff computation (always; dry-run prints it, apply applies it) ---
	profilePath := filepath.Join(target, harnessProfileName)
	rawBefore, rErr := os.ReadFile(profilePath)
	if rErr != nil && !os.IsNotExist(rErr) {
		// An absent profile is fine (-> fresh instance via AppendOverlay); any
		// OTHER read error (unreadable file, permission denied) must abort
		// rather than silently overwrite the profile with a fresh instance.
		fmt.Fprintln(errOut, "error:", fmt.Errorf("read profile %s: %w", relFromTarget(target, profilePath), rErr))
		return errSilent{}
	}
	beforeOverlays := extractOverlays(rawBefore)
	merged, added, err := (schema.HarnessProfile{}).AppendOverlay(rawBefore, name)
	if err != nil {
		fmt.Fprintln(errOut, "error:", err)
		return errSilent{}
	}
	afterOverlays := extractOverlays(merged)

	if overlayNewFl.dryRun {
		printOverlayNewDryRun(out, target, name, plan, beforeOverlays, afterOverlays, added)
		return nil
	}

	if len(units) == 0 {
		fmt.Fprintf(errOut, "warning: no --agent/--command/--skill given; creating a minimal pack %q\n", name)
		fmt.Fprintln(errOut, "         (no .md unit skeletons — only opencode-append.jsonc [no-op shell],")
		fmt.Fprintln(errOut, "          permission-pack.jsonc, callable-graph-snippet.md are written.)")
	}

	// Apply: write the pack files, then the profile.
	packAbs := filepath.Join(target, ".vh-agent-harness", "overlays", name)
	var created []string
	for _, pf := range plan {
		abs := filepath.Join(packAbs, filepath.FromSlash(pf.relPath))
		if mkErr := os.MkdirAll(filepath.Dir(abs), 0o755); mkErr != nil {
			fmt.Fprintln(errOut, "error:", mkErr)
			return errSilent{}
		}
		if wErr := os.WriteFile(abs, pf.content, 0o644); wErr != nil {
			fmt.Fprintln(errOut, "error:", wErr)
			return errSilent{}
		}
		created = append(created, "overlays/"+name+"/"+pf.relPath)
	}

	if added {
		if wErr := os.WriteFile(profilePath, merged, 0o644); wErr != nil {
			fmt.Fprintln(errOut, "error:", wErr)
			return errSilent{}
		}
	}

	// Report.
	fmt.Fprintf(out, "scaffolded pack %q into %s\n", name, relFromTarget(target, packAbs))
	fmt.Fprintln(out, "created:")
	for _, c := range created {
		fmt.Fprintf(out, "  .vh-agent-harness/%s\n", c)
	}
	if added {
		fmt.Fprintf(out, "vh-harness-profile.yml: overlays += %q (via schema reconcile path)\n", name)
	} else {
		fmt.Fprintf(out, "vh-harness-profile.yml: overlays already contained %q (no change)\n", name)
	}
	fmt.Fprintln(out, "\nNext steps:")
	fmt.Fprintf(out, "  1. Preview the render: `vh-agent-harness update --dry-run` (expect 0 conflicts).\n")
	fmt.Fprintf(out, "  2. Apply: `vh-agent-harness update` (renders the pack into .opencode/).\n")
	fmt.Fprintf(out, "  3. Verify health: `vh-agent-harness doctor`.\n")
	return nil
}

// validateOverlayName enforces the filesystem-safe name contract for pack and
// unit names. label is used only for the error message.
func validateOverlayName(label, name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("%s name must be non-empty", label)
	}
	if !nameRe.MatchString(name) {
		return fmt.Errorf("%s name %q must be lowercase alphanumerics (internal . - _ allowed), starting and ending alphanumeric",
			label, name)
	}
	return nil
}

// buildOverlayPlan assembles the ordered list of files to create inside the
// pack directory. relPath is pack-rooted (e.g. "agents/foo.md"); the caller
// prepends the pack dir. Every pack gets opencode-append.jsonc,
// permission-pack.jsonc, and callable-graph-snippet.md.
func buildOverlayPlan(name string, units map[string]string) ([]plannedFile, error) {
	var plan []plannedFile

	skelSub, err := fs.Sub(corpus.OverlaySkeletonFS, corpus.OverlaySkeletonDir)
	if err != nil {
		return nil, fmt.Errorf("open embedded overlay-skeleton: %w", err)
	}

	// Per-unit skeletons in a stable order: agent, command, skill.
	for _, kind := range []string{"agent", "command", "skill"} {
		unitName, ok := units[kind]
		if !ok {
			continue
		}
		embedName := skeletonEmbed[kind]
		tmpl, rErr := fs.ReadFile(skelSub, embedName)
		if rErr != nil {
			return nil, fmt.Errorf("read embedded skeleton %s: %w", embedName, rErr)
		}
		body := renderUnit(string(tmpl), unitName)
		rel := unitRelPath(kind, unitName)
		plan = append(plan, plannedFile{relPath: rel, content: []byte(body)})
	}

	// opencode-append.jsonc: active agent wiring if --agent, else no-op shell.
	agentName := units["agent"]
	plan = append(plan, plannedFile{
		relPath: "opencode-append.jsonc",
		content: []byte(buildOpencodeAppend(name, agentName)),
	})
	// permission-pack.jsonc: LIVE self-descriptor (parsed as-is once the pack
	// is active). Name substituted where meaningful.
	plan = append(plan, plannedFile{
		relPath: "permission-pack.jsonc",
		content: []byte(buildPermissionPack(name, agentName)),
	})
	// callable-graph-snippet.md: fully HTML-commented (inert until edited).
	plan = append(plan, plannedFile{
		relPath: "callable-graph-snippet.md",
		content: []byte(buildCallableSnippet(name, agentName)),
	})

	return plan, nil
}

// unitRelPath returns the pack-rooted path for a unit skeleton.
func unitRelPath(kind, unitName string) string {
	switch kind {
	case "agent":
		return "agents/" + unitName + ".md"
	case "command":
		return "commands/" + unitName + ".md"
	case "skill":
		return "skills/" + unitName + "/SKILL.md"
	}
	return ""
}

// renderUnit substitutes the per-invocation placeholder __UNIT_NAME__ in an
// embedded skeleton. Render tokens ({{PROJECT_NAME}} etc.) are left intact —
// they resolve at `update` render time.
func renderUnit(tmpl, unitName string) string {
	return strings.ReplaceAll(tmpl, "__UNIT_NAME__", unitName)
}

// buildOpencodeAppend returns the opencode-append.jsonc body. When agentName is
// non-empty the agent block + task allow-injections are ACTIVE (the pack is
// immediately functional after `update`). Otherwise it is a commented no-op
// shell (valid JSONC, deep-merge-safe).
func buildOpencodeAppend(pack, agentName string) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "// %s/opencode-append.jsonc — pack contribution (ownership overlay_extension)\n", pack)
	fmt.Fprintf(&sb, "//\n")
	if agentName == "" {
		fmt.Fprintf(&sb, "// Deep-merged into the rendered opencode.jsonc when this pack is listed under\n")
		fmt.Fprintf(&sb, "// `overlays:` in vh-harness-profile.yml. Currently a NO-OP: no --agent was\n")
		fmt.Fprintf(&sb, "// passed to `overlay new`, so no agent roster entry is contributed. Add an\n")
		fmt.Fprintf(&sb, "// agent block (see `vh-agent-harness example` + the _pack-skeleton shape) or\n")
		fmt.Fprintf(&sb, "// any other opencode.jsonc keys you need, then `vh-agent-harness update`.\n")
		fmt.Fprintf(&sb, "//\n")
		fmt.Fprintf(&sb, "// Generated by `vh-agent-harness overlay new`. Edit freely.\n")
		fmt.Fprintf(&sb, "{\n}\n")
		return sb.String()
	}
	fmt.Fprintf(&sb, "// Deep-merged into the rendered opencode.jsonc when this pack is listed under\n")
	fmt.Fprintf(&sb, "// `overlays:` in vh-harness-profile.yml. The merge recurses nested maps: the\n")
	fmt.Fprintf(&sb, "// %q agent block is INSERTED into `agent`, and the build/coordination/\n", agentName)
	fmt.Fprintf(&sb, "// project-coordinator blocks ADD their `task` allow entries (no other fields\n")
	fmt.Fprintf(&sb, "// touched). Tokens like {{PROJECT_NAME}} resolve at render time.\n")
	fmt.Fprintf(&sb, "//\n")
	fmt.Fprintf(&sb, "// Generated by `vh-agent-harness overlay new`. Edit freely.\n")
	fmt.Fprintf(&sb, "{\n")
	fmt.Fprintf(&sb, "  \"agent\": {\n")
	fmt.Fprintf(&sb, "    %q: {\n", agentName)
	fmt.Fprintf(&sb, "      \"description\": \"TODO: what %s does and when to call it\",\n", agentName)
	fmt.Fprintf(&sb, "      \"mode\": \"subagent\",\n")
	fmt.Fprintf(&sb, "      \"prompt\": \"{file:.opencode/agents/%s.md}\",\n", agentName)
	fmt.Fprintf(&sb, "      \"permission\": {\n")
	fmt.Fprintf(&sb, "        \"edit\": \"allow\",\n")
	fmt.Fprintf(&sb, "        \"webfetch\": \"deny\",\n")
	fmt.Fprintf(&sb, "        \"task\": { \"*\": \"deny\", \"committer\": \"allow\" },\n")
	fmt.Fprintf(&sb, "        \"bash\": {\n")
	fmt.Fprintf(&sb, "          \"*\": \"ask\",\n")
	fmt.Fprintf(&sb, "          \"rg *\": \"allow\",\n")
	fmt.Fprintf(&sb, "          \"ls *\": \"allow\",\n")
	fmt.Fprintf(&sb, "          \"git diff *\": \"allow\"\n")
	fmt.Fprintf(&sb, "        }\n")
	fmt.Fprintf(&sb, "      }\n")
	fmt.Fprintf(&sb, "    },\n")
	fmt.Fprintf(&sb, "    \"build\": {\n")
	fmt.Fprintf(&sb, "      \"permission\": { \"task\": { %q: \"allow\" } }\n", agentName)
	fmt.Fprintf(&sb, "    },\n")
	fmt.Fprintf(&sb, "    \"coordination\": {\n")
	fmt.Fprintf(&sb, "      \"permission\": { \"task\": { %q: \"allow\" } }\n", agentName)
	fmt.Fprintf(&sb, "    },\n")
	fmt.Fprintf(&sb, "    \"project-coordinator\": {\n")
	fmt.Fprintf(&sb, "      \"permission\": { \"task\": { %q: \"allow\" } }\n", agentName)
	fmt.Fprintf(&sb, "    }\n")
	fmt.Fprintf(&sb, "  }\n")
	fmt.Fprintf(&sb, "}\n")
	return sb.String()
}

// buildPermissionPack returns the pack's permission-pack.jsonc — a LIVE JSONC
// self-description of the permission entries the pack contributes. When
// agentName is set it is substituted into the self-description; otherwise the
// literal <name> placeholder is kept so the operator knows to fill it in. The
// file is valid JSONC either way and is parsed as-is by the seam.
//
// The scaffolded agent is a committer-delegator specialist: it carries
// gateExempt: true and a task allow on `committer`, so its `location` block
// MUST omit `gate`. This is the contract update-opencode-config.js
// validateRules() enforces (a gateExempt agent's gate deny would bleed into the
// committer subagent session via deriveSubagentSessionPermission). Keep `gate`
// omitted whenever `gateExempt` is true.
func buildPermissionPack(pack, agentName string) string {
	name := agentName
	if name == "" {
		name = "<name>"
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "// %s/permission-pack.jsonc — pack permission self-description (LIVE)\n", pack)
	fmt.Fprintf(&sb, "//\n")
	fmt.Fprintf(&sb, "// This file is the pack's SELF-DESCRIPTION of the permission entries it\n")
	fmt.Fprintf(&sb, "// contributes. It is LIVE JSONC (not commented out): on the next\n")
	fmt.Fprintf(&sb, "// `vh-agent-harness update` the seam materializes it verbatim to\n")
	fmt.Fprintf(&sb, "//   .opencode/sys-scripts/permission-packs/%s.jsonc\n", pack)
	fmt.Fprintf(&sb, "// (this pack is already listed under `overlays:` in vh-harness-profile.yml —\n")
	fmt.Fprintf(&sb, "// `overlay new` appends it for you), and update-opencode-config.js reads that\n")
	fmt.Fprintf(&sb, "// directory dynamically to resolve the active roster. So the entries below\n")
	fmt.Fprintf(&sb, "// take effect once you run `update` then\n")
	fmt.Fprintf(&sb, "// `vh-agent-harness exec node .opencode/sys-scripts/update-opencode-config.js`.\n")
	fmt.Fprintf(&sb, "//\n")
	fmt.Fprintf(&sb, "// Contract (enforced by update-opencode-config.js validateRules): this agent\n")
	fmt.Fprintf(&sb, "// is a committer-delegator (gateExempt: true), so its `location` block must NOT\n")
	fmt.Fprintf(&sb, "// carry a `gate` decision — a gate deny would bleed into the committer\n")
	fmt.Fprintf(&sb, "// subagent. Keep `gate` omitted while `gateExempt` is true; if you stop\n")
	fmt.Fprintf(&sb, "// delegating to committer, set gateExempt: false and add a `gate` decision.\n")
	fmt.Fprintf(&sb, "//\n")
	fmt.Fprintf(&sb, "// Generated by `vh-agent-harness overlay new`. Edit freely.\n")
	fmt.Fprintf(&sb, "{\n")
	fmt.Fprintf(&sb, "  \"agents\": {\n")
	fmt.Fprintf(&sb, "    %q: {\n", name)
	fmt.Fprintf(&sb, "      \"location\": {\n")
	fmt.Fprintf(&sb, "        \"wildcard\": \"ask\",\n")
	fmt.Fprintf(&sb, "        \"readonly\": \"allow\",\n")
	fmt.Fprintf(&sb, "        \"git_readonly\": \"allow\",\n")
	fmt.Fprintf(&sb, "        \"devSh\": \"allow\"\n")
	fmt.Fprintf(&sb, "      },\n")
	fmt.Fprintf(&sb, "      \"task\": { \"*\": \"deny\", \"committer\": \"allow\" },\n")
	fmt.Fprintf(&sb, "      \"gateExempt\": true,\n")
	fmt.Fprintf(&sb, "      \"delegateFrom\": [\n")
	fmt.Fprintf(&sb, "        \"build\",\n")
	fmt.Fprintf(&sb, "        \"coordination\",\n")
	fmt.Fprintf(&sb, "        \"project-coordinator\"\n")
	fmt.Fprintf(&sb, "      ]\n")
	fmt.Fprintf(&sb, "    }\n")
	fmt.Fprintf(&sb, "  }\n")
	fmt.Fprintf(&sb, "}\n")
	return sb.String()
}

// buildCallableSnippet returns a fully HTML-commented callable-graph-snippet.md
// so appending it is harmless until the operator uncomments/edits it.
func buildCallableSnippet(pack, agentName string) string {
	name := agentName
	if name == "" {
		name = "<name>"
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "<!-- %s/callable-graph-snippet.md — optional pack routing doc -->\n", pack)
	fmt.Fprintf(&sb, "<!-- Generated by `vh-agent-harness overlay new`. This file is appended to the -->\n")
	fmt.Fprintf(&sb, "<!-- rendered callable-graph when the pack is active. Uncomment the lines below -->\n")
	fmt.Fprintf(&sb, "<!-- to document this pack's specialists and routing. -->\n")
	fmt.Fprintf(&sb, "\n")
	fmt.Fprintf(&sb, "<!-- ## %s specialists -->\n", pack)
	fmt.Fprintf(&sb, "\n")
	fmt.Fprintf(&sb, "<!-- - **%s** (scope): one line describing what this specialist does and its edit scope. -->\n", name)
	fmt.Fprintf(&sb, "\n")
	fmt.Fprintf(&sb, "<!-- ### %s routing -->\n", pack)
	fmt.Fprintf(&sb, "\n")
	fmt.Fprintf(&sb, "<!-- - `build`, `coordination`, and `project-coordinator` may delegate to `%s` (task allow). -->\n", name)
	fmt.Fprintf(&sb, "<!-- - `%s` stays within its scope; cross-scope work is handed back. -->\n", name)
	fmt.Fprintf(&sb, "<!-- - Commands surfaced by this pack: `<cmd-1>`, `<cmd-2>`. -->\n")
	return sb.String()
}

// extractOverlays reads just the overlays slice from a vh-harness-profile.yml
// blob (used for dry-run before/after diff display). Returns nil on any parse
// problem (the schema validate/append path is the authority).
func extractOverlays(raw []byte) []string {
	if len(strings.TrimSpace(string(raw))) == 0 {
		return nil
	}
	var d struct {
		Overlays []string `yaml:"overlays"`
	}
	if err := yaml.Unmarshal(raw, &d); err != nil {
		return nil
	}
	return d.Overlays
}

// printOverlayNewDryRun prints the full file-creation manifest + profile diff
// and writes nothing.
func printOverlayNewDryRun(out interface {
	Write(p []byte) (n int, err error)
}, target, name string, plan []plannedFile, before, after []string, added bool) {
	fmt.Fprintf(out, "DRY RUN — overlay new %q into %s\n", name, target)
	fmt.Fprintln(out, "Nothing was written.")
	fmt.Fprintln(out)

	fmt.Fprintf(out, "Would CREATE (under .vh-agent-harness/overlays/%s/):\n", name)
	for _, pf := range plan {
		fmt.Fprintf(out, "  %s\n", pf.relPath)
	}
	fmt.Fprintln(out)

	fmt.Fprintln(out, "vh-harness-profile.yml overlays (before):", overlayListOrNone(before))
	fmt.Fprintln(out, "vh-harness-profile.yml overlays (after): ", overlayListOrNone(after))
	if added {
		fmt.Fprintf(out, "  -> +%q (appended via schema reconcile path)\n", name)
	} else {
		fmt.Fprintf(out, "  -> %q already selected (no change)\n", name)
	}
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Re-run `vh-agent-harness overlay new` without --dry-run to apply.")
}

// overlayListOrNone renders an overlays slice for the dry-run diff.
func overlayListOrNone(vals []string) string {
	if len(vals) == 0 {
		return "[]"
	}
	return "[" + strings.Join(vals, ", ") + "]"
}

// relFromTarget renders a path relative to target for human output; falls back
// to the absolute path if the path is not under target.
func relFromTarget(target, path string) string {
	rel, err := filepath.Rel(target, path)
	if err != nil {
		return path
	}
	return filepath.Join("<target>", rel)
}
