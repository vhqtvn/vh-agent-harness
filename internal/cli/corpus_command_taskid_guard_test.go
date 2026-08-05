package cli

// corpus_command_taskid_guard_test.go — narrowed corpus guard for the defect
// class "a task-ID-shaped identifier inside a runnable command presented as
// retrieval guidance, where following it returns nothing."
//
// Defect two shipped a fabricated task ID (`P0-DOCS-006`) into every adopter's
// generated archive index as an `rg "P0-DOCS-006" ...` retrieval example. The
// defect is NOT "a task-ID-shaped string anywhere in prose" (format docs like
// `backlog.md` legitimately show `P1-CORE-001` as an ID-shape example beside
// `e.g.`). The defect is a task-ID-shaped token INSIDE A COMMAND CONTEXT —
// specifically within an `rg`/`grep` invocation that is itself wrapped in
// backticks. Following such a command returns nothing (the ID is fabricated),
// which is the harm.
//
// This test narrows the predicate to that exact shape so legitimate format
// examples are left alone on their own merits (no allowlist). It does NOT
// validate the rendered mirror — that is already covered by the JS test
// `the rendered archive index contains no task-ID-shaped identifier`
// (tests/scripts/normalize-backlog.test.js), which asserts the rendered
// archive-index output. This guard asserts the SOURCE corpus under
// templates/core/ so a regression that re-introduces a literal task-ID command
// anywhere in core (not only the archive index) fails before render.

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// taskIDShapeRe matches the harness task-ID shape <PHASE><N>-<AREA>-<NNN>
// (e.g. P0-DOCS-006, P1-CORE-001, P2-API-003). Identical to the shape the JS
// archive-index test uses.
var taskIDShapeRe = regexp.MustCompile(`[A-Z]\d+-[A-Z]+-\d+`)

// commandSpanRe matches a backtick-wrapped span that contains an `rg` or `grep`
// invocation as a whole word. `[^`]` matches any character including newlines,
// so multi-line command spans are covered without a DOTALL flag. This is the
// "command context" narrowing: only retrieval commands presented as runnable
// guidance, not prose mentions of an ID.
var commandSpanRe = regexp.MustCompile("`[^`]*\\b(?:rg|grep)\\b[^`]*`")

func TestCoreCorpusHasNoTaskIDInCommandContext(t *testing.T) {
	root := findModuleRoot(t)
	coreRoot := filepath.Join(root, "templates", "core")
	var bad []string
	walkErr := filepath.Walk(coreRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		body, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		rel, _ := filepath.Rel(root, path)
		s := string(body)
		for _, m := range commandSpanRe.FindAllStringIndex(s, -1) {
			span := s[m[0]:m[1]]
			tok := taskIDShapeRe.FindString(span)
			if tok == "" {
				continue
			}
			lineNo := strings.Count(s[:m[0]], "\n") + 1
			bad = append(bad, rel+":"+strconv.Itoa(lineNo)+"  token="+tok+"  span="+span)
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk %s: %v", coreRoot, walkErr)
	}
	if len(bad) > 0 {
		t.Errorf("templates/core/ must not carry a task-ID-shaped identifier inside an rg/grep command span (defect class: fabricated ID as retrieval guidance). %d hit(s):\n  - %s",
			len(bad), strings.Join(bad, "\n  - "))
	}
}
