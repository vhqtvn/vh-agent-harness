package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
