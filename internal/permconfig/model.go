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
	"fmt"
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

// HarnessPolicy is the first-class policy governing how the fixed
// "vh-agent-harness *" entry (and its canonical read-only exceptions) is
// emitted in a permission.bash block. It replaces the inconsistent legacy
// practice of assigning one of three scalar decisions (allow/ask/deny) to
// the "vh-agent-harness *" wildcard for read-only specialist agents.
//
// opencode resolves permission.bash with findLast (last-match-wins)
// semantics (refs/opencode/packages/core/src/permission.ts). The four states:
//
//   - allow    → emit "vh-agent-harness *": "allow" (broad — build,
//     coordination, project-coordinator). No exceptions region.
//   - ask      → emit "vh-agent-harness *": "ask" (broad — external only).
//     No exceptions region.
//   - deny     → emit "vh-agent-harness *": "deny" (broad — committer keeps
//     its gated command surface). No exceptions region.
//   - read_only → emit "vh-agent-harness *": "deny" FIRST (region 4a, the
//     catch-all), THEN emit each canonical safe read-only harness verb as
//     "allow" AFTER (region 4b). Under findLast the later specific allows
//     win over the earlier broad deny, so read-only agents get prompt-free
//     access to safe inspection verbs while every other verb (mutations,
//     shell, diagnostics-export, unknown future verbs) stays denied.
//
// The deny-first + allows-after ordering is the ONLY correct shape under
// findLast: allows-first + deny-last would deny everything (the bug the
// original report's suggested pattern would have produced), and a bare
// allow/ask scalar with no exceptions region is what non-read-only agents
// already had.
type HarnessPolicy string

const (
	HarnessPolicyAllow    HarnessPolicy = "allow"
	HarnessPolicyAsk      HarnessPolicy = "ask"
	HarnessPolicyDeny     HarnessPolicy = "deny"
	HarnessPolicyReadOnly HarnessPolicy = "read_only"
)

func validHarnessPolicy(p HarnessPolicy) bool {
	return p == HarnessPolicyAllow || p == HarnessPolicyAsk || p == HarnessPolicyDeny || p == HarnessPolicyReadOnly
}

// wildcardDecision returns the scalar Decision implied for the
// "vh-agent-harness *" wildcard under this policy. Allow/Ask/Deny map to
// themselves; ReadOnly maps to Deny (the catch-all that the canonical
// read-only exceptions in region 4b override under findLast).
func (p HarnessPolicy) wildcardDecision() Decision {
	switch p {
	case HarnessPolicyAllow:
		return Allow
	case HarnessPolicyAsk:
		return Ask
	case HarnessPolicyDeny:
		return Deny
	case HarnessPolicyReadOnly:
		return Deny
	}
	return Deny // unknown → fail-closed scalar
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
	// HarnessPolicy is the first-class policy governing how the fixed
	// "vh-agent-harness *" entry (and its canonical read-only exceptions
	// under read_only) is emitted. It is the PREFERRED declaration channel:
	// core tables and new permission-pack.jsonc entries set this field.
	// Legacy scalar channels (Harness, DevSh below) are normalized into a
	// HarnessPolicy by normalizeHarnessFields at resolve/validate time.
	// read_only is the only policy NOT expressible via the legacy scalars.
	HarnessPolicy HarnessPolicy
	// Harness is the legacy preferred scalar spelling for the fixed
	// "vh-agent-harness *" entry. It mirrors DevSh for backward compatibility:
	// permission-pack.jsonc may use "harness" (preferred legacy) or "devSh"
	// (deprecated alias); both must agree if present. After normalization,
	// Harness carries the wildcardDecision() of HarnessPolicy.
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

// normalizeHarnessFields reconciles the three harness declaration channels
// (HarnessPolicy [new, preferred], Harness [legacy scalar], DevSh [deprecated
// alias]) into a single coherent state. After normalization, when any channel
// was non-empty:
//
//   - HarnessPolicy is non-empty and valid (the canonical policy).
//   - Harness and DevSh both carry wildcardDecision(HarnessPolicy) (the
//     implied scalar for the "vh-agent-harness *" entry).
//
// Conflict detection (fail-closed):
//   - If HarnessPolicy is set AND Harness/DevSh implies a different policy,
//     return an error. Each policy's implied scalar: allow→Allow, ask→Ask,
//     deny→Deny, read_only→Deny.
//   - If Harness and DevSh are both set with different values, return an
//     error (preserves the legacy both-different rejection).
//
// Derivation (when no conflict):
//   - HarnessPolicy set, Harness/DevSh empty: derive scalar from policy.
//   - HarnessPolicy empty, Harness/DevSh set: derive policy from scalar
//     (read_only is NOT derivable from a scalar — it requires the explicit
//     HarnessPolicy field/declaration).
//   - All empty: return nil without modification (the downstream validate's
//     validDecision(rule.DevSh) check surfaces the missing declaration; core
//     tables and parseLocation always set at least one channel).
//
// normalizeHarnessFields is idempotent: calling it on an already-normalized
// rule is a no-op. It is called from resolveRules (for core-table-sourced
// rules) and validate (defensively, for directly-constructed rules in tests).
func normalizeHarnessFields(rule *LocationRule) error {
	// Reconcile Harness and DevSh (legacy scalar channels). Both set → agree.
	if rule.Harness != "" && rule.DevSh != "" && rule.Harness != rule.DevSh {
		return fmt.Errorf(
			"conflicting harness declarations: Harness (%q) and DevSh (%q) specify different decisions — "+
				"use a single channel (HarnessPolicy preferred; Harness/DevSh are legacy scalar aliases)",
			rule.Harness, rule.DevSh)
	}
	// scalarFromLegacy is the scalar decision declared via the legacy channel
	// (empty if neither Harness nor DevSh was set).
	var scalarFromLegacy Decision
	if rule.Harness != "" {
		scalarFromLegacy = rule.Harness
	} else if rule.DevSh != "" {
		scalarFromLegacy = rule.DevSh
	}

	if rule.HarnessPolicy != "" {
		if !validHarnessPolicy(rule.HarnessPolicy) {
			return fmt.Errorf("harnessPolicy %q is not a valid policy (allow, ask, deny, read_only)", rule.HarnessPolicy)
		}
		implied := rule.HarnessPolicy.wildcardDecision()
		if scalarFromLegacy != "" && scalarFromLegacy != implied {
			return fmt.Errorf(
				"conflicting harness declarations: HarnessPolicy %q implies scalar %q but Harness/DevSh is %q — "+
					"use HarnessPolicy only, or keep them consistent",
				rule.HarnessPolicy, implied, scalarFromLegacy)
		}
		// Derive scalar from policy.
		rule.Harness = implied
		rule.DevSh = implied
		return nil
	}

	// HarnessPolicy empty: derive from legacy scalar if present. read_only is
	// NOT expressible via a scalar Decision (only allow/ask/deny are).
	if scalarFromLegacy != "" {
		switch scalarFromLegacy {
		case Allow:
			rule.HarnessPolicy = HarnessPolicyAllow
		case Ask:
			rule.HarnessPolicy = HarnessPolicyAsk
		case Deny:
			rule.HarnessPolicy = HarnessPolicyDeny
		default:
			return fmt.Errorf("legacy Harness/DevSh decision %q is not allow/deny/ask (read_only requires the HarnessPolicy field)", scalarFromLegacy)
		}
		rule.Harness = scalarFromLegacy
		rule.DevSh = scalarFromLegacy
		return nil
	}

	// All channels empty: leave fields as-is. validate's validDecision check
	// catches the missing declaration downstream.
	return nil
}

// parseHarnessValue converts one raw declaration value (from any of the
// harnessPolicy, harness, or devSh JSON keys) into a (policy, scalar) pair.
// All four values are accepted by all three channels so that a pack author
// can write "harness": "read_only" (legacy scalar key with the new policy
// value) and get HarnessPolicyReadOnly. The scalar is the implied
// wildcardDecision for the policy. Returns ok=false for an unrecognized value.
func parseHarnessValue(raw string) (policy HarnessPolicy, scalar Decision, ok bool) {
	switch HarnessPolicy(raw) {
	case HarnessPolicyAllow:
		return HarnessPolicyAllow, Allow, true
	case HarnessPolicyAsk:
		return HarnessPolicyAsk, Ask, true
	case HarnessPolicyDeny:
		return HarnessPolicyDeny, Deny, true
	case HarnessPolicyReadOnly:
		return HarnessPolicyReadOnly, Deny, true
	}
	return "", "", false
}
