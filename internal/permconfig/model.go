// Package permconfig implements the O5 canonical permission-pipeline emitter:
// a Go-native, deterministic rewriter that collapses the dual permission source
// of truth (corpus template bash/task literals AND the legacy Node resolver's
// CORE_*_RULES tables) into ONE Go-owned emitter.
//
// The emitter takes a rendered opencode.jsonc (template conditionals already
// expanded), overwrites every permission.bash and permission.task block from
// Go-canonical tables (command groups + per-agent location/task rules), injects
// overlay-contributed delegateFrom edges, adds feature-gated entries
// (features.backlog), and re-serializes as normalized deterministic JSONC
// (sorted top-level keys via encoding/json, controlled-order permission blocks
// via orderedMap, 4-space indent, comments dropped — Q1b canonical form).
//
// This is distinct from internal/permission/ which is the EXEC-TIME shell-guard
// Hook (enforces bash allow/deny at runtime). permconfig is BUILD-TIME emission
// (produces the config that permission/ reads at runtime).
package permconfig

import (
	"bytes"
	"encoding/json"
)

// Decision is a permission verdict: "allow", "deny", or "ask".
type Decision string

const (
	Allow Decision = "allow"
	Deny  Decision = "deny"
	Ask   Decision = "ask"
)

func validDecision(d Decision) bool {
	return d == Allow || d == Deny || d == Ask
}

// LocationRule is the per-agent (or top-level "default") bash permission rule:
// the decision applied to the wildcard, each command group, and the
// "vh-agent-harness *" devSh entry. Gate is optional — gate-exempt agents
// (build, coordination, project-coordinator, docs-steward) omit the gate key
// entirely so OpenCode's deriveSubagentSessionPermission does not bleed a
// parent gate deny into a committer subagent session.
//
// Edit is the flat edit-tool decision emitted for the agent when EditOverrides
// is empty (the common case — every agent except the committer). When
// EditOverrides is non-empty, edit is emitted as an OBJECT map
// {"<pattern>": "<action>"}: the "*" entry carries the Edit decision (typically
// Deny) FIRST, then each EditRule override LAST. OpenCode evaluates object-form
// edit with findLast semantics (permission/evaluate.ts), so a path matching a
// narrow allow override resolves to allow while everything else denies. This is
// how the committer gets Write access to ONE scoped message-file path
// (tmp/commit-gate-message/**) while edit stays denied everywhere else.
type LocationRule struct {
	Wildcard    Decision // "*" entry
	Readonly    Decision // readonly group decision
	GitReadonly Decision // git_readonly group decision
	Gate        Decision // gate group decision (only emitted when HasGate)
	HasGate     bool     // whether the gate key is present
	// Harness is the preferred spelling for the fixed "vh-agent-harness *"
	// entry (the vh-agent-harness binary's own decision). It mirrors DevSh for
	// backward compatibility: permission-pack.jsonc may use either "harness"
	// (preferred) or "devSh" (deprecated alias); both must agree if present.
	// At emit time Harness and DevSh carry the same normalized value.
	Harness       Decision
	DevSh         Decision   // "vh-agent-harness *" entry (DEPRECATED input alias; mirrors Harness)
	Edit          Decision   // flat edit decision; also the "*" action when EditOverrides is non-empty
	EditOverrides []EditRule // when non-empty, edit is emitted as an object map (deny-* first, overrides last)
	// ExtraBash holds transform-contributed extra bash entries merged into this
	// agent's bash block AFTER the command-group region and BEFORE the
	// "vh-agent-harness *" entry. Populated by EmitWithExtra from validated
	// transform output; nil when no transform is active. OpenCode uses findLast
	// (last-match-wins), so an extra allow placed after "*": "deny" wins.
	ExtraBash []BashEntry
}

// BashEntry is one extra pattern→decision pair contributed by the permission
// transform (config-transform.mjs) for a specific agent. These are merged into
// the agent's bash block AFTER the command-group sort region and BEFORE the
// fixed "vh-agent-harness *" entry, sorted by length-then-locale for
// determinism. Protected-key collisions (with "*", command-group commands, or
// "vh-agent-harness *") and duplicate patterns are rejected at validate time.
type BashEntry struct {
	Pattern  string
	Decision Decision
}

// EditRule is one pattern→decision pair in an agent's object-form edit block.
// Ordering is load-bearing: overrides are appended AFTER the "*" deny so that
// OpenCode's findLast evaluation picks the narrow allow for matching paths.
type EditRule struct {
	Pattern  string
	Decision Decision
}

// TaskEntry is one target→decision pair in an agent's permission.task block.
// The first entry is always the "*" wildcard.
type TaskEntry struct {
	Target   string
	Decision Decision
}

// Features carries the feature-flag state that drives conditional permission
// entries. Currently only Backlog is modeled (Q3a — first-class structured
// input; the legacy resolver loaded features.backlog then never used it,
// wiping the normalize-backlog permission via lines.splice).
type Features struct {
	Backlog bool
}

// PackAgent is one overlay agent's self-described permission contribution,
// parsed from a materialized permission-pack.jsonc.
type PackAgent struct {
	Location     LocationRule // bash decisions (wildcard, groups, devSh)
	Task         []TaskEntry  // task allowlist (insertion-ordered; first is "*")
	GateExempt   bool         // if true, agent omits the gate key
	DelegateFrom []string     // orchestrators that may delegate to this agent
}

// Pack is one overlay pack's materialized permission-pack descriptor.
type Pack struct {
	Name   string               // pack name (filename stem of the .jsonc)
	Agents map[string]PackAgent // agent name → definition
}

// orderedMap is an insertion-ordered string→string map that serializes in
// insertion order (NOT alphabetically). It implements json.Marshaler so that
// when embedded inside a map[string]any root marshaled by json.MarshalIndent,
// its key order is preserved (encoding/json sorts map[string]any keys but
// respects Marshaler output, then re-indents it to the surrounding depth).
//
// This is essential for permission.bash blocks: the canonical order is
// "*" first, then command entries sorted by length-then-locale, then
// "vh-agent-harness *" last — which is NOT alphabetical. Similarly,
// permission.task blocks use insertion order (the resolver's
// Object.entries order) so delegateFrom edges and core task allowlists
// appear in the declared order.
type orderedMap struct {
	entries []orderedEntry
}

type orderedEntry struct {
	key string
	val string
}

func newOrderedMap() *orderedMap {
	return &orderedMap{}
}

func (o *orderedMap) set(key, val string) {
	o.entries = append(o.entries, orderedEntry{key: key, val: val})
}

// MarshalJSON emits the map as compact JSON `{"k":"v",...}` in insertion order.
// The outer json.MarshalIndent call will compact and re-indent this output to
// match the surrounding indentation depth.
func (o orderedMap) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, e := range o.entries {
		if i > 0 {
			buf.WriteByte(',')
		}
		kb, err := json.Marshal(e.key)
		if err != nil {
			return nil, err
		}
		vb, err := json.Marshal(e.val)
		if err != nil {
			return nil, err
		}
		buf.Write(kb)
		buf.WriteByte(':')
		buf.Write(vb)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}
