package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/vhqtvn/vh-agent-harness/internal/drift"
	"github.com/vhqtvn/vh-agent-harness/internal/manifest"
	"github.com/vhqtvn/vh-agent-harness/internal/permission"
)

// preflightCmd is a read-only environment + integrity gate. It verifies the
// host environment and the current installation are ready for the harness to
// function and prints a PASS/FAIL/WARN table. It exits 0 only when no check is
// in the FAIL tier.
//
// Tiering (documented fork):
//   - FAIL (exit non-zero): node missing/<18, eval.js missing, manifest
//     unparseable, managed-file drift present. These block correct operation.
//   - WARN (exit 0): node_modules absent (npm install needed), runtime backend
//     unreachable (e.g. docker daemon down / docker not installed). These are
//     environment-dependent and do not block the CLI itself.
//
// The runtime-reachability check is WARN rather than FAIL because the harness
// CLI is usable without docker (bare backend) and because CI/dev containers
// frequently do not expose a docker daemon; surfacing it as a failure would make
// preflight unrunnable in those environments.
var preflightCmd = &cobra.Command{
	Use:           "preflight",
	Short:         "Verify environment and installation integrity before install/upgrade",
	SilenceUsage:  true,
	SilenceErrors: true,
	Long: `Read-only environment + integrity gate.

Checks (PASS/FAIL/WARN/SKIP), exit non-zero if any check FAILs:

  node           node >= ` + "`" + ` + fmt.Sprintf("%d", permission.NodeMinMajor) + ` + "`" + ` on PATH               FAIL if missing/old
  eval.js        .opencode/plugins/shell-guard/eval.js present       FAIL if absent
  node_modules   .opencode/node_modules present                      WARN if absent
  lineage        .vh-agent-harness/lineage.yml present + parseable    FAIL if leaked
  runtime        backend reachable (docker_compose probes daemon)    WARN if unreachable
  managed-drift  no platform-managed drift vs the re-rendered corpus  FAIL if drift present

On a seam install (lineage present) the lineage/runtime/drift checks run against
the seam authorities (S1 lineage, S4 run-shape, re-rendered corpus). On an older
legacy-manifest install they run against the manifest instead. WARNs do not fail
the command; SKIP means there is nothing to check yet (e.g. not installed).`,
	Args: cobra.NoArgs,
	RunE: runPreflight,
}

// check tiers.
const (
	tierPass = "PASS"
	tierFail = "FAIL"
	tierWarn = "WARN"
	tierSkip = "SKIP"
	// tierInfo is a non-failing, non-warning informational signal. It neither
	// increments the problem count nor the warning count, and never blocks a
	// command. doctor's managed-drift check uses it to surface ownership-preserved
	// divergences (a path raised to project_owned via harness-ownership.yml whose
	// live bytes intentionally differ from the re-rendered template — update
	// correctly preserves them, so they are not drift). A tierInfo result is
	// treated as a pass for exit-code / blocking purposes.
	tierInfo = "INFO"
)

type checkResult struct {
	name   string
	tier   string
	detail string
}

func (c checkResult) String() string {
	return fmt.Sprintf("%-13s %-4s %s", c.name, c.tier, c.detail)
}

func runPreflight(cmd *cobra.Command, _ []string) error {
	out := cmd.OutOrStdout()

	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}

	var results []checkResult
	if isSeamInstalled(cwd) {
		// Seam-aware gate: integrity is judged against the S1 lineage, the S4
		// run-shape backend, and the re-rendered corpus (the same surfaces doctor
		// uses), not a legacy manifest a seam install never writes.
		results = []checkResult{
			checkNode(),
			checkEvalJS(cwd),
			checkNodeModules(cwd),
			checkSeamLineage(cwd),
			checkRuntimeSeam(),
			checkManagedDrift(cwd),
			checkRuntimeStateGitignored(cwd),
		}
	} else {
		root, lm, manifestErr := resolveHarnessRoot()
		results = []checkResult{
			checkNode(),
			checkEvalJS(root),
			checkNodeModules(root),
			checkManifest(lm, manifestErr),
			checkRuntime(lm),
			checkDrift(lm),
		}
	}

	fmt.Fprintln(out, "preflight:")
	failed := 0
	warned := 0
	for _, r := range results {
		fmt.Fprintln(out, "  "+r.String())
		switch r.tier {
		case tierFail:
			failed++
		case tierWarn:
			warned++
		}
	}
	// tierInfo is a non-failing signal treated as a pass for blocking and summary
	// purposes (e.g. an ownership-preserved managed file). Counting it with passed
	// keeps the summary arithmetic honest: passed+warned+failed == len(results).
	fmt.Fprintf(out, "summary: %d passed, %d warned, %d failed\n",
		countTier(results, tierPass)+countTier(results, tierInfo), warned, failed)
	if failed > 0 {
		fmt.Fprintf(out, "result: FAIL — fix the failing checks before proceeding.\n")
		return errSilent{}
	}
	fmt.Fprintf(out, "result: PASS\n")
	return nil
}

func countTier(rs []checkResult, tier string) int {
	n := 0
	for _, r := range rs {
		if r.tier == tier {
			n++
		}
	}
	return n
}

// resolveHarnessRoot returns the harness project root, the governing manifest
// (if any), and a manifest error (if a manifest exists but is unparseable). The
// root is the manifest's project dir when a manifest is found, otherwise the
// current working directory (pre-install use). A malformed manifest surfaces as
// manifestErr so the gate can FAIL instead of silently SKIPping.
func resolveHarnessRoot() (root string, lm *loadedManifest, manifestErr error) {
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}
	path, m, ferr := manifest.Find(cwd)
	if path == "" && ferr == nil {
		// no manifest anywhere up the tree
		return cwd, nil, nil
	}
	root = dirFromManifestPath(path)
	if ferr != nil {
		// manifest exists but is unparseable
		return root, nil, ferr
	}
	return root, &loadedManifest{path: path, dir: root, m: m}, nil
}

// dirFromManifestPath turns <root>/.opencode/harness-manifest.json into <root>.
func dirFromManifestPath(p string) string {
	d := filepath.Dir(p)
	return filepath.Dir(d)
}

// checkNode verifies node is on PATH and at least NodeMinMajor.
func checkNode() checkResult {
	bin, major, err := permission.ProbeNode()
	if err != nil {
		return checkResult{name: "node", tier: tierFail, detail: err.Error()}
	}
	if major < permission.NodeMinMajor {
		return checkResult{name: "node", tier: tierFail,
			detail: fmt.Sprintf("node %s is major %d; need >= %d", bin, major, permission.NodeMinMajor)}
	}
	return checkResult{name: "node", tier: tierPass,
		detail: fmt.Sprintf("node %s (major %d >= %d)", bin, major, permission.NodeMinMajor)}
}

// checkEvalJS verifies the shell-guard eval.js bridge exists on disk.
func checkEvalJS(root string) checkResult {
	p := filepath.Join(root, permission.EvalRelPath)
	if _, err := os.Stat(p); err != nil {
		return checkResult{name: "eval.js", tier: tierFail,
			detail: fmt.Sprintf("missing %s (run `vh-agent-harness install`)", permission.EvalRelPath)}
	}
	return checkResult{name: "eval.js", tier: tierPass, detail: permission.EvalRelPath}
}

// checkNodeModules verifies .opencode/node_modules exists; missing is a WARN.
func checkNodeModules(root string) checkResult {
	p := filepath.Join(root, manifest.DirName, "node_modules")
	if _, err := os.Stat(p); err != nil {
		return checkResult{name: "node_modules", tier: tierWarn,
			detail: "missing .opencode/node_modules — run `npm install` in .opencode/"}
	}
	return checkResult{name: "node_modules", tier: tierPass, detail: ".opencode/node_modules"}
}

// checkManifest verifies the manifest parses, if present. A present-but-
// unparseable manifest is a FAIL (blocks correct operation); absence is a SKIP.
func checkManifest(lm *loadedManifest, manifestErr error) checkResult {
	if manifestErr != nil {
		return checkResult{name: "manifest", tier: tierFail,
			detail: fmt.Sprintf("unparseable: %v", manifestErr)}
	}
	if lm == nil {
		return checkResult{name: "manifest", tier: tierSkip, detail: "no manifest (not installed)"}
	}
	return checkResult{name: "manifest", tier: tierPass,
		detail: fmt.Sprintf("harness-manifest.json (v%s, %d files)", lm.m.SchemaVersion, len(lm.m.Files))}
}

// checkRuntime verifies the runtime backend is reachable. docker_compose probes
// the daemon; bare always passes; absence of a manifest skips. Unreachable is a
// WARN (documented fork), never a FAIL.
func checkRuntime(lm *loadedManifest) checkResult {
	if lm == nil {
		return checkResult{name: "runtime", tier: tierSkip, detail: "no manifest (backend unknown)"}
	}
	backend, err := backendSelector(lm.m, lm.dir)
	if err != nil {
		return checkResult{name: "runtime", tier: tierFail,
			detail: fmt.Sprintf("backend misconfigured: %v", err)}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := backend.Healthcheck(ctx); err != nil {
		return checkResult{name: "runtime", tier: tierWarn,
			detail: fmt.Sprintf("%s backend unreachable: %v", backend.Name(), err)}
	}
	return checkResult{name: "runtime", tier: tierPass,
		detail: fmt.Sprintf("%s backend reachable", backend.Name())}
}

// checkRuntimeSeam resolves the runtime backend via the seam-aware authority
// (S4 run-shape preferred, legacy manifest fallback) and probes reachability.
// A misconfigured backend is a FAIL; an unreachable-but-configured backend is a
// WARN (same documented fork as checkRuntime: the CLI is usable without a live
// daemon). host-shell/bare/proxy always pass the healthcheck.
func checkRuntimeSeam() checkResult {
	be, _, err := resolveBackend()
	if err != nil {
		return checkResult{name: "runtime", tier: tierFail,
			detail: fmt.Sprintf("backend misconfigured: %v", err)}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if herr := be.Healthcheck(ctx); herr != nil {
		return checkResult{name: "runtime", tier: tierWarn,
			detail: fmt.Sprintf("%s backend unreachable: %v", be.Name(), herr)}
	}
	return checkResult{name: "runtime", tier: tierPass,
		detail: fmt.Sprintf("%s backend reachable", be.Name())}
}

// checkDrift verifies no managed-file drift exists, if a manifest is present.
func checkDrift(lm *loadedManifest) checkResult {
	if lm == nil {
		return checkResult{name: "drift", tier: tierSkip, detail: "no manifest"}
	}
	report, err := drift.Compute(lm.dir, lm.m)
	if err != nil {
		return checkResult{name: "drift", tier: tierFail, detail: fmt.Sprintf("compute failed: %v", err)}
	}
	if report.HasProblems() {
		return checkResult{name: "drift", tier: tierFail,
			detail: fmt.Sprintf("%d drifted, %d missing, %d unexpected — run `vh-agent-harness update`",
				report.Counts[drift.Drifted], report.Counts[drift.Missing], report.Counts[drift.Unexpected])}
	}
	return checkResult{name: "drift", tier: tierPass,
		detail: fmt.Sprintf("%d files in sync", report.Counts[drift.OK])}
}
