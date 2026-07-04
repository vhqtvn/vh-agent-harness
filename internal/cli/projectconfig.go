package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/vhqtvn/vh-agent-harness/internal/runshape"
)

// blessedNASentinels are the case-insensitive values that mean "this consumed
// field is intentionally unset" in project.config.json (e.g. db_user / db_name
// for a project with no database). A field whose value is one of these:
//
//   - renders as EMPTY — the token ({{DB_USER}} etc.) substitutes to "" in a
//     seeded CLAUDE.md/Makefile, so no literal sentinel ever ships; and
//   - is treated as RESOLVED by warnUnresolvedProjectConfigTokens (no UNRESOLVED
//     notice), because the operator explicitly recorded "not applicable".
//
// Leave a field blank ("") when you want the warning to KEEP reminding you to
// fill it in; the N/A sentinel is the "I considered it, there is none" answer.
var blessedNASentinels = map[string]bool{
	"none": true,
	"n/a":  true,
	"null": true,
	"na":   true,
}

// isBlessedNA reports whether v is a blessed N/A sentinel (case-insensitive,
// surrounding whitespace trimmed).
func isBlessedNA(v string) bool {
	return blessedNASentinels[strings.ToLower(strings.TrimSpace(v))]
}

// projectConfigDoc is the parsed `project` block of project.config.json.
type projectConfigDoc struct {
	MissionSummary      string
	ArchitectureSummary json.RawMessage
	DBUser              string
	DBName              string
}

// readProjectConfig reads and parses .vh-agent-harness/project.config.json under
// target. Returns the parsed doc and true on success. Returns zero and false
// when the file is absent or unreadable (the greenfield default — an empty doc,
// so every token resolves empty) OR when it is malformed (a warning is written
// to stderr and the doc is ignored, never failing the caller).
func readProjectConfig(target string) (projectConfigDoc, bool) {
	if target == "" {
		return projectConfigDoc{}, false
	}
	path := filepath.Join(target, runshape.DirName, "project.config.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		return projectConfigDoc{}, false // absent/unreadable: greenfield default
	}
	var doc struct {
		Project struct {
			MissionSummary      string          `json:"mission_summary"`
			ArchitectureSummary json.RawMessage `json:"architecture_summary"`
			DBUser              string          `json:"db_user"`
			DBName              string          `json:"db_name"`
		} `json:"project"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		fmt.Fprintf(os.Stderr, "warning: ignoring unparseable %s: %v\n", path, err)
		return projectConfigDoc{}, false
	}
	return projectConfigDoc{
		MissionSummary:      doc.Project.MissionSummary,
		ArchitectureSummary: doc.Project.ArchitectureSummary,
		DBUser:              doc.Project.DBUser,
		DBName:              doc.Project.DBName,
	}, true
}

// projectConfigAnswers reads the target's .vh-agent-harness/project.config.json
// (if present) and returns the render-token values it contributes:
//
//	mission_summary       -> {{MISSION_SUMMARY}}      (verbatim string)
//	architecture_summary  -> {{ARCHITECTURE_SUMMARY}} (list -> "- line\n- line", or a string verbatim)
//	db_user               -> {{DB_USER}}
//	db_name               -> {{DB_NAME}}
//
// A field set to a blessed N/A sentinel (none / n/a / null / na — see
// blessedNASentinels) is OMITTED from the returned map, so the renderer
// substitutes "" (empty) for its token — "intentionally unset" renders as
// blank rather than the literal sentinel. The W3 warning treats such a field
// as resolved (see warnUnresolvedProjectConfigTokens).
//
// project.config.json is project_owned: the user creates it (e.g.
// `vh-agent-harness example .vh-agent-harness/project.config.json > …`) and
// fills it BEFORE install so the seeded CLAUDE.md/Makefile pick up the values.
// An absent file is the greenfield default (empty map -> tokens resolve empty).
// A present-but-malformed file warns and is ignored (never fails the render).
func projectConfigAnswers(target string) map[string]string {
	out := map[string]string{}
	p, ok := readProjectConfig(target)
	if !ok {
		return out
	}
	if p.MissionSummary != "" && !isBlessedNA(p.MissionSummary) {
		out["mission_summary"] = p.MissionSummary
	}
	if arch := formatArchitectureSummary(p.ArchitectureSummary); arch != "" && !isBlessedNA(arch) {
		out["architecture_summary"] = arch
	}
	if p.DBUser != "" && !isBlessedNA(p.DBUser) {
		out["db_user"] = p.DBUser
	}
	if p.DBName != "" && !isBlessedNA(p.DBName) {
		out["db_name"] = p.DBName
	}
	return out
}

// formatArchitectureSummary renders the architecture_summary field, accepting
// either a JSON array of strings (-> markdown bullet lines) or a plain string
// (verbatim). Anything else (or empty) yields "".
func formatArchitectureSummary(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var list []string
	if err := json.Unmarshal(raw, &list); err == nil {
		lines := make([]string, 0, len(list))
		for _, a := range list {
			if strings.TrimSpace(a) == "" {
				continue
			}
			lines = append(lines, "- "+a)
		}
		return strings.Join(lines, "\n")
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	return ""
}

// consumedProjectConfigFieldsOrdered is the ordered list of project.config.json
// fields the seam resolves into render tokens. It mirrors
// consumedProjectConfigFields (the contract-test set) and projectConfigAnswers'
// struct tags. Keep all three in sync when adding a consumed field.
var consumedProjectConfigFieldsOrdered = []string{
	"mission_summary",
	"architecture_summary",
	"db_user",
	"db_name",
}

// projectFieldStatus classifies a consumed project.config.json field by its
// would-be-rendered value, for the W3 unresolved-token warning.
type projectFieldStatus int

const (
	projectFieldEmpty    projectFieldStatus = iota // absent or "" — needs filling
	projectFieldSentinel                           // still a literal {{...}} — verbatim-save footgun
	projectFieldNA                                 // blessed N/A sentinel — intentionally unset
	projectFieldValue                              // a real value
)

// classifyProjectValue classifies a field's would-be-rendered value. The N/A
// check runs on the rendered form, so for architecture_summary pass its
// formatted value (a plain string or the "- bullet\n- bullet" join), not the
// raw JSON. This means an N/A sentinel on architecture_summary is recognized in
// string form ("none") but not as a single-element list (["none"] renders as
// "- none" and is treated as a real value) — scalar fields (db_user/db_name,
// and mission_summary if you choose) are the normal home for the sentinel.
func classifyProjectValue(v string) projectFieldStatus {
	switch {
	case v == "":
		return projectFieldEmpty
	case strings.Contains(v, "{{"):
		return projectFieldSentinel
	case isBlessedNA(v):
		return projectFieldNA
	default:
		return projectFieldValue
	}
}

// warnUnresolvedProjectConfigTokens writes a LOUD warning to w (stderr in
// production) when .vh-agent-harness/project.config.json is absent OR any of the
// four consumed tokens would resolve to an EMPTY string OR an UNRESOLVED
// SENTINEL (e.g. the field still literally contains "{{MISSION_SUMMARY}}") at
// render time.
//
// This closes the W3 adoption footgun from two directions:
//   - absent/empty config was previously SILENT (projectConfigAnswers returns an
//     empty map; the renderer resolves {{MISSION_SUMMARY}} etc. to ""), so a
//     consumer shipped a CLAUDE.md/Makefile with blank sections and no signal.
//   - a config saved verbatim from `vh-agent-harness example` still contains the
//     literal {{...}} sentinels; projectConfigAnswers records them as NON-empty
//     values, so the prior empty-only check missed them and the renderer
//     substituted each sentinel for itself — shipping literal {{TOKEN}}s in the
//     rendered file (the original incident this epic addresses).
//
// A field set to a blessed N/A sentinel (none / n/a / null / na) is treated as
// RESOLVED — it is the operator's explicit "not applicable" answer for a field
// the project does not use (e.g. db_user / db_name when there is no database).
// It renders empty and does NOT trip this notice.
//
// The warning is NON-FATAL: it never blocks install/update/guide, but it makes
// the incomplete render visible and points the operator at the command that
// creates the config. It surfaces under --dry-run too (install/update invoke it
// before the dry-run branch).
func warnUnresolvedProjectConfigTokens(w io.Writer, target string) {
	path := filepath.Join(target, runshape.DirName, "project.config.json")
	_, statErr := os.Stat(path)
	absent := statErr != nil

	// Classify each consumed field from the RAW config (not projectConfigAnswers'
	// render map, which omits N/A sentinels by design). This is what lets a
	// blessed N/A count as resolved while a blank or literal-{{...}} value still
	// warns.
	doc, parsed := readProjectConfig(target)
	statuses := map[string]projectFieldStatus{}
	if parsed {
		statuses["mission_summary"] = classifyProjectValue(doc.MissionSummary)
		statuses["architecture_summary"] = classifyProjectValue(formatArchitectureSummary(doc.ArchitectureSummary))
		statuses["db_user"] = classifyProjectValue(doc.DBUser)
		statuses["db_name"] = classifyProjectValue(doc.DBName)
	} else {
		for _, f := range consumedProjectConfigFieldsOrdered {
			statuses[f] = projectFieldEmpty
		}
	}

	type issue struct {
		field    string
		sentinel bool
	}
	var unresolved []issue
	for _, f := range consumedProjectConfigFieldsOrdered {
		switch statuses[f] {
		case projectFieldEmpty:
			unresolved = append(unresolved, issue{field: f})
		case projectFieldSentinel:
			unresolved = append(unresolved, issue{field: f, sentinel: true})
			// projectFieldNA, projectFieldValue: resolved — no warning.
		}
	}
	if len(unresolved) == 0 {
		return
	}
	sort.Slice(unresolved, func(i, j int) bool { return unresolved[i].field < unresolved[j].field })

	fmt.Fprintln(w, "--------------------------------------------------------------------------------")
	fmt.Fprintln(w, "vh-agent-harness WARNING: project.config.json token(s) UNRESOLVED.")
	if absent {
		fmt.Fprintf(w, "  No %s found — ALL of the consumed tokens below are empty.\n", filepath.Join(runshape.DirName, "project.config.json"))
	} else {
		fmt.Fprintf(w, "  %s exists but these field(s) are empty OR still contain a literal {{...}} sentinel:\n", filepath.Join(runshape.DirName, "project.config.json"))
	}
	for _, u := range unresolved {
		if u.sentinel {
			fmt.Fprintf(w, "    - %s  (still literal {{%s}} — fill in a real value)\n", u.field, strings.ToUpper(u.field))
		} else {
			fmt.Fprintf(w, "    - %s  (renders as {{%s}})\n", u.field, strings.ToUpper(u.field))
		}
	}
	fmt.Fprintln(w, "  The seeded CLAUDE.md/Makefile will ship BLANK/LITERAL-TOKEN sections for these.")
	fmt.Fprintln(w, "  Fill the config BEFORE install (project_owned seeds are written once):")
	fmt.Fprintf(w, "    vh-agent-harness example %s > %s\n",
		filepath.Join(runshape.DirName, "project.config.json"),
		filepath.Join(runshape.DirName, "project.config.json"))
	fmt.Fprintln(w, "  A field that does NOT apply (e.g. db_user/db_name when there is no database) may")
	fmt.Fprintln(w, "  be set to a blessed N/A sentinel (none / n/a / null / na) — it renders empty and")
	fmt.Fprintln(w, "  silences this notice for that field.")
	fmt.Fprintln(w, "  This warning is non-fatal — the render proceeded, but the output is incomplete.")
	fmt.Fprintln(w, "--------------------------------------------------------------------------------")
}
