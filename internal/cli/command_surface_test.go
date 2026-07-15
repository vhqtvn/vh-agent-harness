package cli

// command_surface_test.go is a structural DRIFT GATE over the vh-agent-harness
// command surface. It does not test behavior; it pins two invariants so the
// registered command tree cannot silently drift away from what users and agents
// are shown:
//
//  1. Help-listing completeness — `vh-agent-harness` (no args) help output must
//     mention every non-hidden registered command by name. This auto-extends as
//     commands are added: a newly-registered command that is forgotten in the
//     help surface fails this test (zero future maintenance).
//
//  2. README command-table consistency — the human-curated command table in
//     README.md must (a) never claim a command that is not registered (fiction
//     guard), and (b) always document a small core set of primary, always-
//     present commands (omission guard). Advanced commands stay optional in the
//     human table by design; the fiction guard alone keeps it honest.
//
// These tests drive the real, shared rootCmd via executeCapture, so they MUST
// NOT use t.Parallel() (global command tree + writers are not parallel-safe).

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// registeredCommandNames returns the set of non-hidden, top-level command names
// registered on rootCmd (the same tree the binary dispatches). This includes
// cobra's auto-added `completion` and the routed `help` command.
func registeredCommandNames(t *testing.T) map[string]bool {
	t.Helper()
	names := map[string]bool{}
	for _, c := range rootCmd.Commands() {
		if c.Hidden {
			continue
		}
		names[c.Name()] = true
	}
	if len(names) == 0 {
		t.Fatal("rootCmd has no non-hidden commands; the command tree is empty")
	}
	return names
}

// TestCommandSurface_HelpListsAllRegisteredCommands asserts that the no-args
// root help output mentions every non-hidden registered command by name. This
// is the structural drift gate for help completeness: a newly-registered command
// that never reaches the help surface (e.g. because a future custom template or
// availability change hides it) fails here.
//
// NOTE: executeCapture drives the shared global rootCmd, so this test MUST NOT
// use t.Parallel().
func TestCommandSurface_HelpListsAllRegisteredCommands(t *testing.T) {
	out, err := executeCapture(t, []string{})
	if err != nil {
		t.Fatalf("no-args help: want nil error (exit 0), got %v", err)
	}
	var missing []string
	for _, c := range rootCmd.Commands() {
		if c.Hidden {
			continue
		}
		if !strings.Contains(out, c.Name()) {
			missing = append(missing, c.Name())
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		t.Errorf("root help output does not mention registered command(s): %s\n--- help output ---\n%s",
			strings.Join(missing, ", "), out)
	}
}

// TestCommandSurface_EveryCommandIsGrouped asserts that every non-hidden,
// non-completion registered command carries a non-empty GroupID drawn from the
// four harness group constants (lifecycle/orientation/health/runtime). This
// closes the regression gap the prior Slice-2 review flagged as DEFER candidate
// cF3: a future `rootCmd.AddCommand(...)` that forgets `assignGroup(...)`
// would silently render the command under cobra's catch-all "Additional
// Commands:" bucket (alongside the auto-added `completion`) instead of a titled
// group — escaping TestCommandSurface_HelpListsAllRegisteredCommands, which
// only checks that the name appears in help, not that it is grouped.
//
// The ONE allowed ungrouped visible command is cobra's auto-added `completion`
// (it is not harness-registered and has no GroupID by design). `help` IS
// harness-registered and grouped under orientation, so it is deliberately NOT
// exempted — losing its group would be a real regression this test must catch.
//
// NOTE: executeCapture drives the shared global rootCmd, so this test MUST NOT
// use t.Parallel().
func TestCommandSurface_EveryCommandIsGrouped(t *testing.T) {
	// Drive rootCmd once so cobra's auto-added `completion` is registered,
	// making the iterated tree identical to what `--help` renders.
	if _, err := executeCapture(t, []string{}); err != nil {
		t.Fatalf("no-args help: want nil error (exit 0), got %v", err)
	}

	validGroups := map[string]bool{
		groupLifecycle:   true,
		groupOrientation: true,
		groupHealth:      true,
		groupRuntime:     true,
	}

	// completion is the one allowed ungrouped visible command (cobra auto-adds
	// it; not harness-registered, no GroupID by design).
	allowedUngrouped := map[string]bool{"completion": true}

	// Build a table of every candidate command (non-hidden, non-exempt), then
	// range over it so failures name the offending command clearly.
	type groupRow struct {
		name    string
		groupID string
	}
	var rows []groupRow
	for _, c := range rootCmd.Commands() {
		if c.Hidden {
			continue
		}
		if allowedUngrouped[c.Name()] {
			continue
		}
		rows = append(rows, groupRow{name: c.Name(), groupID: c.GroupID})
	}
	if len(rows) == 0 {
		t.Fatal("rootCmd has no non-hidden, non-completion commands; the command tree is empty")
	}

	for _, r := range rows {
		if r.groupID == "" {
			t.Errorf("registered command %q has empty GroupID (would render under \"Additional Commands:\" instead of a titled group)", r.name)
			continue
		}
		if !validGroups[r.groupID] {
			t.Errorf("registered command %q carries unknown GroupID %q (not one of lifecycle/orientation/health/runtime)", r.name, r.groupID)
		}
	}
}

// readmeTableTokens extracts the first word of every backtick-quoted command
// token found inside the README.md "## Command surface" table rows, skipping
// flags (tokens whose first word starts with '-'). The surviving first word is
// the command verb a reader would type. Subcommands written as `overlay new`
// reduce to `overlay` (the registered top-level parent). `help [command]` and
// `help migrate [version]` reduce to `help`.
func readmeTableTokens(t *testing.T) []string {
	t.Helper()
	readmePath := filepath.Join("..", "..", "README.md")
	body, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("read %s: %v", readmePath, err)
	}

	// Isolate the "## Command surface" section: from its heading up to (but not
	// including) the next column-0 "## " heading.
	lines := strings.Split(string(body), "\n")
	var section []string
	inSection := false
	for _, ln := range lines {
		if strings.HasPrefix(ln, "## ") {
			if inSection {
				break // next top-level section ends the command-surface section
			}
			if strings.Contains(ln, "Command surface") {
				inSection = true
			}
			continue
		}
		if inSection {
			section = append(section, ln)
		}
	}
	if len(section) == 0 {
		t.Fatalf("README.md: could not locate the \"## Command surface\" section")
	}

	tokRe := regexp.MustCompile("`([^`]+)`")
	seen := map[string]bool{}
	var tokens []string
	for _, ln := range section {
		// Only parse actual table rows (the prose around the table also uses
		// backticks; restricting to rows keeps the token set command-shaped).
		if !strings.HasPrefix(strings.TrimSpace(ln), "|") {
			continue
		}
		for _, m := range tokRe.FindAllStringSubmatch(ln, -1) {
			fields := strings.Fields(m[1])
			if len(fields) == 0 {
				continue
			}
			word := fields[0] // first word: the command verb
			if strings.HasPrefix(word, "-") {
				continue // a flag like --dry-run / -f, not a command name
			}
			if !seen[word] {
				seen[word] = true
				tokens = append(tokens, word)
			}
		}
	}
	if len(tokens) == 0 {
		t.Fatalf("README.md: no command tokens found in the \"## Command surface\" table")
	}
	sort.Strings(tokens)
	return tokens
}

// TestCommandSurface_ReadmeTableConsistency asserts the README.md command table
// is consistent with the registered command tree in two directions:
//
//  1. Fiction guard — every command token the table claims (first word of each
//     backtick token, flags excluded) must be a registered command name. Catches
//     a doc advertising a command that does not exist.
//
//  2. Omission guard — a small set of primary, always-present commands must
//     appear in the table. Catches the table silently dropping a core verb.
//
// Advanced commands are intentionally OPTIONAL in the human table (this test
// does not require them all to appear); the fiction guard alone keeps the table
// honest.
//
// NOTE: this test does not call executeCapture, but it shares package-level
// state conventions, so it also avoids t.Parallel() for consistency.
func TestCommandSurface_ReadmeTableConsistency(t *testing.T) {
	registered := registeredCommandNames(t)
	tokens := readmeTableTokens(t)

	// 1. Fiction guard: every table token must be a registered command.
	for _, tok := range tokens {
		if !registered[tok] {
			t.Errorf("README.md command table references %q, which is not a registered command", tok)
		}
	}

	// 2. Omission guard: primary, always-present commands must be documented.
	primaryCommands := []string{
		"install", "update", "guide", "self-update", "version",
		"status", "exec", "shell", "help",
	}
	present := map[string]bool{}
	for _, tok := range tokens {
		present[tok] = true
	}
	for _, want := range primaryCommands {
		if !present[want] {
			t.Errorf("README.md command table omits primary command %q", want)
		}
	}
}
