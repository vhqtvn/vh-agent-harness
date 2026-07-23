package cli

// release_inject.go implements `vh-agent-harness release inject-errata` — the
// machine step that closes the "staged erratum never actually injected" hole
// (the third failure mode from the release-defer reconciliation matrix).
//
// Doctor check #13 (staged-errata-content, release_gate.go) FAILs when a staged
// errata card's correction body is absent from the about-to-release migration
// note. This subcommand is the recovery action the FAIL message points at: it
// reads each staged errata card, injects its staged_path erratum body into the
// about-to-release note as a "## Errata for vX.Y.Z" section, and flips the card
// status staged→completed — in the same slice, with zero human copy-paste.
//
// After inject, check #13 sees no staged cards (they are now completed) and
// passes; check #12 (defer-liveness) treats completed as closed and passes too.
// If inject was forgotten, the card stays staged and #13 FAILs at the tag
// boundary (release-tag.sh G0c runs `vh-agent-harness doctor`).
//
// The mechanism lives in the BINARY (not prompt prose) so it is effective this
// session — opencode caches the releaser subagent prompt per-process, so a
// prompt-only step would be stale-cached and inactive for the current ceremony
// run. The binary reads/writes disk immediately.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/vhqtvn/vh-agent-harness/internal/memory/claims"
)

// releaseInjectFlags holds the inputs to `vh-agent-harness release inject-errata`.
type releaseInjectFlags struct {
	target string // repo root (default: current directory)
	note   string // override: explicit about-to-release note path
	dryRun bool   // preview without writing
}

var releaseInjectFl *releaseInjectFlags

// releaseCmd is the parent for release-ceremony machine verbs.
// `release inject-errata` is the first subcommand; future release-time verbs
// (e.g. a release-preflight) may hang off this parent.
var releaseCmd = &cobra.Command{
	Use:   "release",
	Short: "Release-ceremony machine steps (errata injection, release-time gates)",
	Long: `Release-ceremony machine steps. These verbs are the MACHINE layer of the
release ceremony — they read/write disk and are effective immediately (unlike
prompt prose, which opencode caches per-process). The enforcement authority for
the ceremony lives in scripts/release-tag.sh (the G0c doctor gate) and the
doctor checks (#12 defer-liveness, #13 staged-errata-content); these subcommands
are the recovery actions the gates point at.

Subcommands:
  inject-errata   Inject each staged erratum into the about-to-release note,
                  flipping its card staged→completed (zero copy-paste).`,
}

// releaseInjectErrataCmd is `vh-agent-harness release inject-errata`.
var releaseInjectErrataCmd = &cobra.Command{
	Use:           "inject-errata",
	Short:         "Inject staged errata into the about-to-release note and flip cards to completed",
	SilenceUsage:  true,
	SilenceErrors: true,
	Long: `Inject each errata card whose status is "staged" into the about-to-release
(untagged) migration note, then flip the card status staged→completed.

For each staged errata card:
  1. Read its staged_path erratum file.
  2. Extract the corrective body (everything after the title + status blockquote).
  3. Append a "## Errata for vX.Y.Z" section (version from the erratum title) to
     the about-to-release note, unless the note already contains the body
     (idempotent — the section is not duplicated).
  4. Flip the card status to "completed", append a history entry, and update
     updated_at.

The about-to-release note is auto-detected as the highest-version untagged
templates/migrations/v<semver>.md (sorted by numeric semver, not lexicographic).
The --note flag is an OPTIONAL validation: when provided, it MUST resolve to the
same auto-detected highest note — it cannot override into a lower or stale note.
This prevents injecting the correction into the wrong note while the shipping
note lacks it.

After this command, doctor check #13 (staged-errata-content) passes (no staged
cards remain) and the release ceremony can proceed through release-tag.sh.`,
	Args: cobra.NoArgs,
	RunE: runReleaseInjectErrata,
}

func init() {
	releaseInjectFl = &releaseInjectFlags{}
	releaseInjectErrataCmd.Flags().StringVarP(&releaseInjectFl.target, "target", "o", "",
		"target directory (default: current directory)")
	releaseInjectErrataCmd.Flags().StringVar(&releaseInjectFl.note, "note", "",
		"explicit about-to-release note path (default: auto-detect highest untagged note)")
	releaseInjectErrataCmd.Flags().BoolVar(&releaseInjectFl.dryRun, "dry-run", false,
		"preview the injection + card flips without writing")
	releaseCmd.AddCommand(releaseInjectErrataCmd)
}

// injectReport is the structured output of one staged-erratum injection.
type injectReport struct {
	Card       string `json:"card"`
	StagedPath string `json:"staged_path"`
	TargetVer  string `json:"target_version"`
	Note       string `json:"note"`
	Injected   bool   `json:"injected"` // false = body already present (idempotent skip)
	Flipped    bool   `json:"flipped"`  // false = dry-run or flip failed
}

func runReleaseInjectErrata(cmd *cobra.Command, _ []string) (err error) {
	defer func() { reportRunErrToStderr(cmd, err) }()
	out := cmd.OutOrStdout()

	target := releaseInjectFl.target
	if target == "" {
		cwd, gerr := os.Getwd()
		if gerr != nil {
			return fmt.Errorf("getcwd: %w", gerr)
		}
		target = cwd
	}
	abs, aerr := filepath.Abs(target)
	if aerr != nil {
		return fmt.Errorf("resolve target: %w", aerr)
	}

	reg, derr := claims.Derive(abs)
	if derr != nil {
		return fmt.Errorf("read claim sources: %w", derr)
	}

	// No tasks transport dir → no staged cards possible → clean no-op. This must
	// NOT be a hard error: a repo with no .local/coordinator/tasks/ state simply
	// has nothing to inject, same as a repo with tasks but no staged errata.
	staged := stagedErrataCards(reg.Cards)
	if !reg.TasksPresent || len(staged) == 0 {
		fmt.Fprintf(out, "no staged errata cards — nothing to inject\n")
		return nil
	}

	// Resolve the target note. Always derive the highest about-to-release note
	// from claims (the single note that check #13 gates on), regardless of
	// whether --note is provided. When --note is given, VALIDATE that it
	// resolves to the SAME highest about-to-release note — otherwise an operator
	// could inject into a stale/lower untagged note, flip the card to completed,
	// and have check #13 PASS on the now-empty staged set while the note that
	// actually ships lacks the correction (commit-review tier1b-F1 round 3).
	about := aboutToReleaseNotes(reg.Notes)
	if len(about) == 0 {
		return fmt.Errorf("no about-to-release (untagged) migration note under templates/migrations/ — create the note first")
	}
	// Highest version = last after sort (claims.Derive sorts ascending).
	expectedNote := about[len(about)-1].Path

	var notePath string
	if releaseInjectFl.note != "" {
		np := releaseInjectFl.note
		if !filepath.IsAbs(np) {
			np = filepath.Join(abs, filepath.FromSlash(np))
		}
		// Reject --note unless it resolves to the highest about-to-release
		// note. This prevents injecting into a stale/lower note while the
		// shipping note (the highest) lacks the correction.
		if !samePath(np, expectedNote) {
			return fmt.Errorf("--note %s does not resolve to the highest about-to-release note %s — inject-errata must target the note that check #13 gates on; do not use --note to override into a lower or stale note",
				releaseInjectFl.note, expectedNote)
		}
		notePath = np
	} else {
		notePath = expectedNote
	}

	noteBytes, nerr := os.ReadFile(notePath)
	if nerr != nil {
		return fmt.Errorf("read target note %s: %w", notePath, nerr)
	}
	noteText := string(noteBytes)
	noteNorm := normalizeForContentMatch(noteText)

	// TWO-PHASE fail-closed ordering (commit-review tier1c-F1 fix):
	//
	// Phase 1 builds the updated note text in memory and collects which cards to
	// flip — NOTHING is written to disk. Phase 2 writes the note FIRST, and only
	// after that write succeeds does it flip the cards staged→completed.
	//
	// Why this ordering is load-bearing: "status=completed" is the invariant
	// "the erratum body is durably in the about-to-release note." If the card
	// were flipped before the note write and the write then failed, the card
	// would be completed while the body is absent — check #13 (which only holds
	// STAGED cards) would PASS on the now-empty staged set, and a re-run would
	// be a no-op ("no staged errata cards"). That re-opens the exact "staged
	// erratum never injected" hole this command exists to close. Writing the
	// note first means a write failure leaves every card staged and the ceremony
	// blocked (fail-closed).
	var reports []injectReport
	type pendingFlip struct {
		reportIdx int    // index into reports
		cardPath  string // absolute path to the card JSON
		targetVer string // version for the history entry
	}
	var flips []pendingFlip
	var failErrs []string
	noteDirty := false
	for _, c := range staged {
		rpt := injectReport{Card: cardLabel(c), StagedPath: strings.TrimSpace(c.StagedPath)}
		sp := rpt.StagedPath
		if sp == "" {
			failErrs = append(failErrs, fmt.Sprintf("%s: no staged_path set (card is malformed)", rpt.Card))
			reports = append(reports, rpt)
			continue
		}
		erratumPath := sp
		if !filepath.IsAbs(erratumPath) {
			erratumPath = filepath.Join(abs, filepath.FromSlash(sp))
		}
		raw, rerr := os.ReadFile(erratumPath)
		if rerr != nil {
			failErrs = append(failErrs, fmt.Sprintf("%s: staged_path %s unreadable: %v", rpt.Card, sp, rerr))
			reports = append(reports, rpt)
			continue
		}
		targetVer, rawBody := splitErratumBody(string(raw))
		rpt.TargetVer = targetVer
		rpt.Note = notePath
		if rawBody == "" {
			failErrs = append(failErrs, fmt.Sprintf("%s: staged_path %s has no corrective body", rpt.Card, sp))
			reports = append(reports, rpt)
			continue
		}
		bodyNorm := normalizeForContentMatch(rawBody)
		if strings.Contains(noteNorm, bodyNorm) {
			// Idempotent: the erratum body is already in the note. Do not
			// duplicate the section; still flip the card so #13 stops holding it.
			rpt.Injected = false
		} else {
			rpt.Injected = true
			if !releaseInjectFl.dryRun {
				section := buildErrataSection(targetVer, rawBody)
				noteText = appendErrataSection(noteText, section)
				noteNorm = normalizeForContentMatch(noteText)
				noteDirty = true
			}
		}
		rptIdx := len(reports)
		reports = append(reports, rpt)
		if !releaseInjectFl.dryRun {
			flips = append(flips, pendingFlip{reportIdx: rptIdx, cardPath: c.Path, targetVer: targetVer})
		}
	}

	// Phase 2a — write the note FIRST (fail-closed). If this fails, no card is
	// flipped, so every staged card stays staged and check #13 keeps blocking
	// the ceremony.
	if !releaseInjectFl.dryRun && noteDirty {
		if werr := atomicWriteFile(notePath, []byte(noteText)); werr != nil {
			return fmt.Errorf("write target note %s (no cards were flipped — all staged cards remain staged): %w", notePath, werr)
		}
	}

	// Phase 2b — only now that the note is durably written, flip each card
	// staged→completed (read full JSON to preserve rich-schema fields, mutate,
	// write back).
	for _, fl := range flips {
		flipped, ferr := flipCardToCompleted(fl.cardPath, fl.targetVer)
		if ferr != nil {
			failErrs = append(failErrs, fmt.Sprintf("%s: flip card failed: %v", reports[fl.reportIdx].Card, ferr))
		} else {
			reports[fl.reportIdx].Flipped = flipped
		}
	}

	// Report.
	if releaseInjectFl.dryRun {
		fmt.Fprintln(out, "(dry-run — no files written)")
	}
	for _, r := range reports {
		verb := "already present (idempotent)"
		if r.Injected {
			if releaseInjectFl.dryRun {
				verb = "would inject"
			} else {
				verb = "injected"
			}
		}
		flip := ""
		if r.Flipped {
			flip = "; card flipped staged→completed"
		} else if releaseInjectFl.dryRun && r.Injected {
			flip = "; would flip card staged→completed"
		}
		fmt.Fprintf(out, "  %s: %s section for %s into %s%s\n", r.Card, verb, r.TargetVer, filepath.Base(r.Note), flip)
	}
	if len(failErrs) > 0 {
		fmt.Fprintln(out, "errors:")
		for _, e := range failErrs {
			fmt.Fprintf(out, "  - %s\n", e)
		}
		return errSilent{}
	}
	return nil
}

// buildErrataSection renders the markdown section injected into the note. The
// heading mirrors the erratum's own convention ("## Errata for vX.Y.Z"); the
// body is the verbatim corrective content (title + status blockquote stripped
// by splitErratumBody).
func buildErrataSection(targetVersion, rawBody string) string {
	ver := targetVersion
	if ver == "" {
		ver = "(unknown version)"
	}
	return fmt.Sprintf("\n\n## Errata for %s\n\n%s\n", ver, rawBody)
}

// appendErrataSection appends an errata section to the note text, ensuring the
// note ends with exactly one blank line before the section.
func appendErrataSection(noteText, section string) string {
	return strings.TrimRight(noteText, "\n") + "\n" + strings.TrimLeft(section, "\n")
}

// flipCardToCompleted reads the full card JSON at cardPath (preserving all
// rich-schema fields), sets status="completed", appends a history entry, updates
// updated_at, and writes it back. Returns true if the write succeeded.
func flipCardToCompleted(cardPath, targetVersion string) (bool, error) {
	raw, err := os.ReadFile(cardPath)
	if err != nil {
		return false, fmt.Errorf("read card: %w", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return false, fmt.Errorf("parse card: %w", err)
	}
	doc["status"] = "completed"
	now := time.Now().UTC().Format(time.RFC3339)
	doc["updated_at"] = now
	ver := targetVersion
	if ver == "" {
		ver = "(unknown version)"
	}
	histEntry := map[string]any{
		"at":           now,
		"event":        "erratum_injected",
		"session_name": nil,
		"status":       "completed",
		"note":         fmt.Sprintf("Erratum body injected into the about-to-release note by `vh-agent-harness release inject-errata` (target %s); card flipped staged→completed.", ver),
	}
	if existing, ok := doc["history"].([]any); ok {
		doc["history"] = append(existing, histEntry)
	} else {
		doc["history"] = []any{histEntry}
	}
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return false, fmt.Errorf("marshal card: %w", err)
	}
	if err := atomicWriteFile(cardPath, append(out, '\n')); err != nil {
		return false, fmt.Errorf("write card: %w", err)
	}
	return true, nil
}

// samePath compares two paths for equivalence after cleaning both to their
// canonical absolute form. This is used to validate the --note override
// resolves to the same file the gate would target.
func samePath(a, b string) bool {
	return filepath.Clean(a) == filepath.Clean(b)
}

// atomicWriteFile writes data to path atomically by first writing to a
// same-directory temp file, syncing, and renaming over the destination. This
// ensures a write failure or interruption cannot corrupt the destination
// (os.WriteFile opens with O_TRUNC — a partial write would destroy authored
// content that the doctor gates can detect but cannot restore).
func atomicWriteFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".tmp-release-inject-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			tmp.Close()
			os.Remove(tmpName)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Chmod(0o644); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	cleanup = false
	return os.Rename(tmpName, path)
}
