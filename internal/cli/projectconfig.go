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

// warnEmptyProjectConfigTokens writes a LOUD warning to w (stderr in production)
// when .vh-agent-harness/project.config.json is absent OR any of the four
// consumed tokens would resolve to the empty string at render time.
//
// This closes the W3 adoption footgun: previously an absent/empty
// project.config.json was SILENT (projectConfigAnswers returns an empty map and
// the renderer resolves {{MISSION_SUMMARY}} etc. to "" — renderer.go
// SubstituteHarnessTokens), so a consumer would ship a CLAUDE.md/Makefile with
// blank sections and no signal that anything was missing. The warning is
// NON-FATAL: it never blocks install/update/guide, but it makes the incomplete
// render visible and points the operator at the command that creates the config.
//
// It surfaces under --dry-run too because the caller (install/update) invokes it
// before the dry-run branch, so an operator previewing a render learns the
// seeded files would come out token-empty. projectConfigAnswers only records a
// field when its value is non-empty, so a resolved-empty token is simply absent
// from the returned map — which is exactly what this checks.
func warnEmptyProjectConfigTokens(w io.Writer, target string) {
	path := filepath.Join(target, runshape.DirName, "project.config.json")
	_, statErr := os.Stat(path)
	absent := statErr != nil

	answers := projectConfigAnswers(target)
	var missing []string
	for _, f := range consumedProjectConfigFieldsOrdered {
		if answers[f] == "" {
			missing = append(missing, f)
		}
	}
	if len(missing) == 0 {
		return
	}
	sort.Strings(missing)

	fmt.Fprintln(w, "--------------------------------------------------------------------------------")
	fmt.Fprintln(w, "vh-agent-harness WARNING: project.config.json token(s) resolve EMPTY.")
	if absent {
		fmt.Fprintf(w, "  No %s found — ALL of the consumed tokens below are empty.\n", filepath.Join(runshape.DirName, "project.config.json"))
	} else {
		fmt.Fprintf(w, "  %s exists but these field(s) are unset/empty:\n", filepath.Join(runshape.DirName, "project.config.json"))
	}
	for _, f := range missing {
		fmt.Fprintf(w, "    - %s  (renders as {{%s}})\n", f, strings.ToUpper(f))
	}
	fmt.Fprintln(w, "  The seeded CLAUDE.md/Makefile will have BLANK sections for these.")
	fmt.Fprintln(w, "  Create/fill the config BEFORE install (project_owned seeds are written once):")
	fmt.Fprintf(w, "    vh-agent-harness example %s > %s\n",
		filepath.Join(runshape.DirName, "project.config.json"),
		filepath.Join(runshape.DirName, "project.config.json"))
	fmt.Fprintln(w, "  This warning is non-fatal — the render proceeded, but the output is incomplete.")
	fmt.Fprintln(w, "--------------------------------------------------------------------------------")
}
