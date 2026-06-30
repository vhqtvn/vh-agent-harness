package cli

// Phase 4 capability-installer: post-render fail-closed reference validation.
//
// renderSeamStaging (internal/cli/seam.go) calls validateRenderedRefs AFTER
// permconfig.Emit has written the canonical opencode.jsonc into staging. It is
// defense-in-depth on top of Phase 3's present-agent filter
// (internal/permconfig/emit.go computeTaskBlock): by the time this runs, the
// OPTIONAL task edges to capability-gated-out agents have already been pruned,
// so any reference that still dangles is a HARD inconsistency — a capability
// manifest declaring a hard dependency whose agent cluster did not fully render,
// or a prompt reference to a file conditional rendering removed — and MUST fail
// closed before the broken artifact reaches the live tree.
//
// The check is a strict no-op (returns nil) when every reference resolves,
// which is the invariant the real dogfood render satisfies today.

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// promptFileRefRe matches an opencode "{file:<path>}" reference as it appears
// in a parsed JSON string value (the surrounding quotes are already gone after
// json.Unmarshal). The captured group is the inner path. Used to resolve agent
// prompt references against the rendered staging tree. Per-agent model files
// ({file:./.local/config/agent-model/<agent>}) live in the "model" field, not
// "prompt", and are seeded empty separately — they are intentionally out of
// scope here.
var promptFileRefRe = regexp.MustCompile(`^\{file:(.+)\}$`)

// validateRenderedRefs asserts the emitted opencode.jsonc contains no reference
// to an agent that did not render and no prompt file reference to a file that
// conditional rendering removed. It parses the emitted bytes (already
// canonical, comment-free JSON from permconfig.Emit) and cross-checks every
// reference against (a) the rendered "agent" map and (b) the staging tree on
// disk for prompt file targets.
//
// Hard references validated (each fails closed with a message naming the
// dangling reference and its missing target):
//
//   - permission.task: every non-"*" target in a rendered agent's task block
//     must itself be a rendered agent. A dangling entry means an orchestrator
//     carries an allow/ref to a delegate that is not there. (Phase 3's
//     permconfig filter already prunes these for capability-gated agents; this
//     catches a render path that bypassed the filter or a hard-dep cluster that
//     did not fully render.)
//   - agent.<name>.prompt: every "{file:<path>}" prompt reference (regardless
//     of path) must resolve to a file present in the rendered staging tree. A
//     missing target means the agent's prompt would fail to load at runtime.
//
// "present" is the set of agent names that rendered (the "agent" object keys).
// Returns nil when every reference resolves; otherwise returns a single error
// listing every dangling reference and the rendered roster so one failed render
// surfaces the full inconsistency rather than just the first.
func validateRenderedRefs(staging string, emitted []byte) error {
	var doc struct {
		Agent map[string]struct {
			Prompt     string `json:"prompt"`
			Permission *struct {
				Task map[string]string `json:"task"`
			} `json:"permission"`
		} `json:"agent"`
	}
	if err := json.Unmarshal(emitted, &doc); err != nil {
		// permconfig.Emit just produced this blob via json.MarshalIndent, so a
		// parse failure here is a render-pipeline bug; surface it rather than
		// silently skipping the validation pass.
		return fmt.Errorf("validate rendered refs: parse emitted opencode.jsonc: %w", err)
	}

	present := make(map[string]bool, len(doc.Agent))
	for name := range doc.Agent {
		present[name] = true
	}

	type danglingRef struct {
		from, kind, target string
	}
	var refs []danglingRef

	// Task edges: every non-"*" target must be a rendered agent. The "*"
	// wildcard is a decision applied to ALL delegates, not a reference to a
	// named one, so it is never dangling.
	for name, agent := range doc.Agent {
		if agent.Permission == nil {
			continue
		}
		for target := range agent.Permission.Task {
			if target == "*" {
				continue
			}
			if !present[target] {
				refs = append(refs, danglingRef{from: name, kind: "permission.task", target: target})
			}
		}
	}

	// Prompt file references: EVERY "{file:<path>}" prompt target must resolve
	// under the staging tree, regardless of path (the fail-closed contract: a
	// prompt whose target was removed or never rendered must not reach the live
	// tree). The "model" field is intentionally not parsed here, so per-agent
	// model files under .local/ stay out of scope (seeded empty separately).
	for name, agent := range doc.Agent {
		if agent.Prompt == "" {
			continue
		}
		m := promptFileRefRe.FindStringSubmatch(agent.Prompt)
		if m == nil {
			continue // inline prompt text (no file reference) — nothing to resolve
		}
		inner := strings.TrimPrefix(m[1], "./") // tolerate a leading "./"
		// Stat the staged target. A genuinely-missing file (fs.ErrNotExist) is a
		// dangling ref; any OTHER stat error (e.g. permission denied) is surfaced
		// by os and is not misreported as dangling here (ORCH-F2).
		if _, err := os.Stat(filepath.Join(staging, filepath.FromSlash(inner))); err != nil && errors.Is(err, fs.ErrNotExist) {
			refs = append(refs, danglingRef{from: name, kind: "prompt", target: inner})
		}
	}

	if len(refs) == 0 {
		return nil
	}
	// Deterministic ordering for the error message (Go map iteration is
	// randomized); sort by source agent, then ref kind, then target.
	sort.Slice(refs, func(i, j int) bool {
		if refs[i].from != refs[j].from {
			return refs[i].from < refs[j].from
		}
		if refs[i].kind != refs[j].kind {
			return refs[i].kind < refs[j].kind
		}
		return refs[i].target < refs[j].target
	})
	rendered := make([]string, 0, len(present))
	for name := range present {
		rendered = append(rendered, name)
	}
	sort.Strings(rendered)
	var b strings.Builder
	fmt.Fprintf(&b, "validate rendered refs: %d dangling reference(s) point at a non-rendered agent or a missing prompt file; rendered agents = %v",
		len(refs), rendered)
	for _, r := range refs {
		fmt.Fprintf(&b, "\n  - agent %q %s -> %q (not rendered / missing)", r.from, r.kind, r.target)
	}
	return fmt.Errorf("%s", b.String())
}
