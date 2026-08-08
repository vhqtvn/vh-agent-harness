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
//
// The predicate is exercised BOTH ways: a negative/clean-path assertion over
// the live templates/core/ corpus (TestCoreCorpusHasNoTaskIDInCommandContext)
// AND a positive-control assertion over an isolated t.TempDir() fixture
// (TestGuardCatchesTaskIDInCommandContext). The positive control is
// load-bearing defense-in-depth: if the regexes below were ever broken so they
// stopped matching the defect shape, the clean-path test would still pass
// (it only proves the corpus is currently clean, not that the guard catches).
// The positive control injects a synthetic violating span into an isolated
// fixture and asserts the SAME scanner reports it, proving the catching-path
// fires on the defect class. The narrowing is locked by a paired negative
// fixture: a prose ID-shape example outside any command span must NOT be
// flagged.

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
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

// taskIDHit is one finding of a task-ID-shaped token inside an rg/grep command
// span, located by the relative path of the offending file and its line number.
type taskIDHit struct {
	relPath string
	lineNo  int
	token   string
	span    string
}

func (h taskIDHit) String() string {
	return fmt.Sprintf("%s:%d  token=%s  span=%s", h.relPath, h.lineNo, h.token, h.span)
}

// scanTaskIDInCommandContext walks root and reports every task-ID-shaped token
// found inside an rg/grep command span. relBase is the directory the returned
// relPath values are expressed relative to (normally the repo root for stable,
// portable diagnostics). This is the single predicate shared by the
// clean-corpus assertion and the positive control — factoring it out is what
// makes the positive control prove the SAME path the guard uses in production
// rather than a parallel reimplementation.
func scanTaskIDInCommandContext(root, relBase string) ([]taskIDHit, error) {
	var hits []taskIDHit
	walkErr := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
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
		rel, _ := filepath.Rel(relBase, path)
		s := string(body)
		for _, m := range commandSpanRe.FindAllStringIndex(s, -1) {
			span := s[m[0]:m[1]]
			tok := taskIDShapeRe.FindString(span)
			if tok == "" {
				continue
			}
			lineNo := strings.Count(s[:m[0]], "\n") + 1
			hits = append(hits, taskIDHit{
				relPath: rel,
				lineNo:  lineNo,
				token:   tok,
				span:    span,
			})
		}
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}
	return hits, nil
}

func TestCoreCorpusHasNoTaskIDInCommandContext(t *testing.T) {
	root := findModuleRoot(t)
	coreRoot := filepath.Join(root, "templates", "core")
	hits, err := scanTaskIDInCommandContext(coreRoot, root)
	if err != nil {
		t.Fatalf("walk %s: %v", coreRoot, err)
	}
	if len(hits) > 0 {
		bad := make([]string, len(hits))
		for i, h := range hits {
			bad[i] = h.String()
		}
		t.Errorf("templates/core/ must not carry a task-ID-shaped identifier inside an rg/grep command span (defect class: fabricated ID as retrieval guidance). %d hit(s):\n  - %s",
			len(hits), strings.Join(bad, "\n  - "))
	}
}

// TestGuardCatchesTaskIDInCommandContext is the POSITIVE CONTROL for the corpus
// guard. The clean-path test above proves templates/core/ is currently clean but
// would still pass if the regexes were broken (stopped matching) — a clean
// corpus cannot distinguish "the guard works" from "the guard matches nothing."
// This test injects a synthetic violating span into an isolated t.TempDir()
// fixture (NEVER the live tree) and asserts the SAME scanner reports it, so the
// catching-path is proven to fire on the defect class. The narrowing is locked
// by a paired check: a prose ID-shape example outside any command span is NOT
// flagged, confirming the guard does not over-match legitimate format docs.
func TestGuardCatchesTaskIDInCommandContext(t *testing.T) {
	// Synthetic corpus in an isolated temp dir. relBase == the temp dir so the
	// reported relPath is the fixture filename (stable across hosts).
	dir := t.TempDir()

	// Defect-class fixture: a task-ID-shaped token INSIDE a backtick-wrapped
	// rg/grep command span. This is exactly the shape defect two shipped.
	const violatingBody = "# retrieval help\n" +
		"find it with `rg \"P0-XXX-001\" docs/` then read the matches.\n"
	if err := os.WriteFile(filepath.Join(dir, "violating.md"), []byte(violatingBody), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	// Narrowing fixture: a task-ID-shaped token in PROSE only, beside `e.g.`,
	// with NO surrounding rg/grep command span. This is the legitimate shape
	// format docs (e.g. backlog.md) carry and the guard must NOT flag it.
	const cleanBody = "# format guide\n" +
		"Use a stable task ID shaped like e.g. P1-CORE-001 (PHASE-AREA-NNN).\n"
	if err := os.WriteFile(filepath.Join(dir, "clean.md"), []byte(cleanBody), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	hits, err := scanTaskIDInCommandContext(dir, dir)
	if err != nil {
		t.Fatalf("scan tempdir: %v", err)
	}

	// Positive assertion: the defect-class span MUST be caught. If the regexes
	// broke (stopped matching the command span or the ID shape), len(hits) == 0
	// and this fails — which is the whole point of a positive control.
	var violating, clean []taskIDHit
	for _, h := range hits {
		switch h.relPath {
		case "violating.md":
			violating = append(violating, h)
		case "clean.md":
			clean = append(clean, h)
		}
	}
	if len(violating) == 0 {
		t.Errorf("guard FAILED to catch the defect-class span: a task-ID-shaped " +
			"token inside an rg/grep command span was not reported. The catching-" +
			"path regex is broken (this positive control is what makes the guard " +
			"self-proving).")
	} else {
		// Strengthen the positive control: report exactly the injected token so a
		// silent predicate change (matching the span but not extracting the token)
		// is also caught.
		gotTok := violating[0].token
		if gotTok != "P0-XXX-001" {
			t.Errorf("guard reported the defect-class span but extracted token %q, "+
				"want %q (the predicate matched the span but not the injected ID — "+
				"the narrowing is not catching the right shape)", gotTok, "P0-XXX-001")
		}
	}

	// Narrowing assertion: the prose-only ID-shape example must NOT be flagged.
	// If it is, the guard has lost the command-context narrowing and would
	// false-positive on legitimate format docs.
	if len(clean) > 0 {
		t.Errorf("guard over-matched: the prose ID-shape example in clean.md was "+
			"flagged (%d hit(s)), but it is NOT inside an rg/grep command span. "+
			"The command-context narrowing is broken.", len(clean))
	}

	// Sanity: at least the violating fixture must have produced a hit, or the
	// guard is a no-op. Belt-and-braces alongside the per-file assertions above.
	if len(hits) == 0 {
		t.Fatalf("guard reported zero hits across the whole fixture corpus — the " +
			"scanner is a no-op and cannot be trusted on the live corpus either.")
	}
}
