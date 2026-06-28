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

// projectConfigAnswers reads the target's .vh-agent-harness/project.config.json
// (if present) and returns the render-token values it contributes:
//
//	mission_summary       -> {{MISSION_SUMMARY}}      (verbatim string)
//	architecture_summary  -> {{ARCHITECTURE_SUMMARY}} (list -> "- line\n- line", or a string verbatim)
//	db_user               -> {{DB_USER}}
//	db_name               -> {{DB_NAME}}
//
// project.config.json is project_owned: the user creates it (e.g.
// `vh-agent-harness example .vh-agent-harness/project.config.json > …`) and
// fills it BEFORE install so the seeded CLAUDE.md/Makefile pick up the values.
// An absent file is the greenfield default (empty map -> tokens resolve empty).
// A present-but-malformed file warns and is ignored (never fails the render).
func projectConfigAnswers(target string) map[string]string {
	out := map[string]string{}
	if target == "" {
		return out
	}
	path := filepath.Join(target, runshape.DirName, "project.config.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		return out // absent (or unreadable): greenfield default
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
		return out
	}
	p := doc.Project
	if p.MissionSummary != "" {
		out["mission_summary"] = p.MissionSummary
	}
	if arch := formatArchitectureSummary(p.ArchitectureSummary); arch != "" {
		out["architecture_summary"] = arch
	}
	if p.DBUser != "" {
		out["db_user"] = p.DBUser
	}
	if p.DBName != "" {
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
// The warning is NON-FATAL: it never blocks install/update/guide, but it makes
// the incomplete render visible and points the operator at the command that
// creates the config. It surfaces under --dry-run too (install/update invoke it
// before the dry-run branch). projectConfigAnswers only records a field when its
// value is non-empty, so a resolved-empty token is simply absent from the
// returned map.
func warnUnresolvedProjectConfigTokens(w io.Writer, target string) {
	path := filepath.Join(target, runshape.DirName, "project.config.json")
	_, statErr := os.Stat(path)
	absent := statErr != nil

	answers := projectConfigAnswers(target)
	type issue struct {
		field    string
		sentinel bool
	}
	var unresolved []issue
	for _, f := range consumedProjectConfigFieldsOrdered {
		v := answers[f]
		switch {
		case v == "":
			unresolved = append(unresolved, issue{field: f})
		case strings.Contains(v, "{{"):
			unresolved = append(unresolved, issue{field: f, sentinel: true})
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
	fmt.Fprintln(w, "  This warning is non-fatal — the render proceeded, but the output is incomplete.")
	fmt.Fprintln(w, "--------------------------------------------------------------------------------")
}
