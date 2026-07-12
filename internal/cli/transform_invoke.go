package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/vhqtvn/vh-agent-harness/internal/permconfig"
)

// transformTimeout caps the wall-clock time for the project config-transform.
// The transform is trusted project-owned code (same trust model as
// forbidden-patterns.project.js); a 10s timeout catches accidental hangs. The
// advisory source lint catches obvious host-API misuse but is NOT a security
// boundary — the real gate is Go validation of the typed output. A timeout is a
// loud failure.
const transformTimeout = 10 * time.Second

// configTransformRelPath is the project-owned transform file path relative to
// the target (live tree) root. The .mjs extension makes Node 18+ treat it as
// ESM unconditionally (no package.json required).
const configTransformRelPath = ".vh-agent-harness/config-transform.mjs"

// configTransformRunner is the harness-owned ESM runner script written to a temp
// dir at invocation time (NOT committed, NOT part of the rendered tree). It
// reads the context JSON, imports the project transform, calls the exported
// function, and writes JSON to stdout. The runner MAY use Node built-ins (it is
// harness infrastructure); the source lint (permconfig.LintTransformSource)
// applies only to the PROJECT transform file.
const configTransformRunner = `import { readFileSync } from "node:fs";
import { pathToFileURL } from "node:url";

const ctxPath = process.argv[2];
const transformPath = process.argv[3];
// ctx is the parsed TransformInput JSON: { context: { packs, features, agents } }.
// It is ALREADY the wrapped shape the transform function expects as its sole
// argument, so we pass it directly — do NOT wrap again as fn({ context: ctx }),
// which would produce the double-wrapped { context: { context: {...} } } and
// break any transform reading context.agents / context.packs / context.features.
const ctx = JSON.parse(readFileSync(ctxPath, "utf8"));
const mod = await import(pathToFileURL(transformPath).href);
const fn = typeof mod.default === "function" ? mod.default : mod.transform;
if (typeof fn !== "function") {
  throw new Error("config-transform.mjs must export a default function or a named 'transform' function");
}
const result = await fn(ctx);
process.stdout.write(JSON.stringify(result ?? {}));
`

// applyConfigTransform reads the project's config-transform.mjs (if present),
// invokes it via Node with a strict context, validates the typed permission
// intent, and returns the extra bash entries per agent.
//
// Returns (nil, nil) when the transform file is absent — this is the normal
// no-op path for projects that do not maintain a transform. When the file IS
// present, any failure (malformed source, forbidden host API, Node not found,
// timeout, non-JSON output, validation error) returns a non-nil error so the
// render fails LOUD rather than silently emitting unvalidated permissions.
//
// The transform runs AFTER pack materialization and BEFORE canonical emission
// so the emitter (sole writer of opencode.jsonc) sees the merged intent. doctor
// re-renders via the same renderSeamStaging pipeline, so a malformed transform
// surfaces as a doctor FAIL (not silent drift).
func applyConfigTransform(target string, stagedData []byte, roster []string, packNames []string, renderAnswers map[string]string) (map[string][]permconfig.BashEntry, error) {
	transformPath := filepath.Join(target, configTransformRelPath)
	source, err := os.ReadFile(transformPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // no transform — no-op
		}
		return nil, fmt.Errorf("read config-transform: %w", err)
	}

	// Source lint: reject obvious host-API usage (advisory, not a sandbox).
	if err := permconfig.LintTransformSource(source); err != nil {
		return nil, fmt.Errorf("config-transform source lint: %w", err)
	}

	// Locate Node (>=18 required for top-level await + dynamic import).
	nodeBin, err := exec.LookPath("node")
	if err != nil {
		return nil, fmt.Errorf("config-transform requires Node.js (>=18) but 'node' was not found in PATH: %w", err)
	}

	// Build the context from the staged config. NO ambient env, NO secrets, NO
	// file paths — only active pack names, resolved features, and the agent roster.
	features := make(map[string]string)
	for k, v := range renderAnswers {
		if strings.HasPrefix(k, "features.") {
			features[strings.TrimPrefix(k, "features.")] = v
		}
	}
	ctxInput, err := permconfig.BuildTransformInputFromConfig(stagedData, packNames, features)
	if err != nil {
		return nil, fmt.Errorf("build transform context: %w", err)
	}
	ctxJSON, err := permconfig.MarshalTransformInput(ctxInput)
	if err != nil {
		return nil, fmt.Errorf("marshal transform context: %w", err)
	}

	// Write the runner + context to a temp dir (cleaned up after invocation).
	tmpDir, err := os.MkdirTemp("", "vh-agent-harness-transform-")
	if err != nil {
		return nil, fmt.Errorf("create temp dir for transform runner: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	runnerPath := filepath.Join(tmpDir, "runner.mjs")
	if err := os.WriteFile(runnerPath, []byte(configTransformRunner), 0o644); err != nil {
		return nil, fmt.Errorf("write transform runner: %w", err)
	}
	ctxPath := filepath.Join(tmpDir, "context.json")
	if err := os.WriteFile(ctxPath, ctxJSON, 0o644); err != nil {
		return nil, fmt.Errorf("write transform context: %w", err)
	}

	// Invoke Node with a hard timeout. The context kill ensures a hung
	// transform cannot block the render indefinitely.
	runCtx, cancel := context.WithTimeout(context.Background(), transformTimeout)
	defer cancel()
	cmd := exec.CommandContext(runCtx, nodeBin, runnerPath, ctxPath, transformPath)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if runCtx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("config-transform timed out after %s — the transform should be a fast function; note: the transform is trusted project-owned code, not sandboxed", transformTimeout)
		}
		stderrTrim := strings.TrimSpace(stderr.String())
		if stderrTrim != "" {
			return nil, fmt.Errorf("config-transform execution failed: %w\nstderr: %s", err, stderrTrim)
		}
		return nil, fmt.Errorf("config-transform execution failed: %w", err)
	}

	// Validate the typed output against the strict contract before feeding it
	// to the emitter.
	extra, err := permconfig.ValidateTransformOutput([]byte(stdout.String()), roster)
	if err != nil {
		return nil, fmt.Errorf("config-transform output validation: %w", err)
	}
	return extra, nil
}
