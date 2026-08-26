// Package skillload implements the skill_load tool: tier 2 of the
// three-tier progressive-disclosure skills delivery (tier 1 = the
// sanitized name+description lines in the assembled prompt; tier 2 =
// this guarded tool returning a skill's full body; tier 3 = reference
// files under the skill's folder via the ref argument).
//
// This is a GUARDED TOOL, never a model filesystem-read: every call goes
// through the engine's normal ToolDefinition pipeline — the same
// waterfall, guards, timeout, and audit logging as run_shell / read.
//
// INVARIANTS (operator posture — the whole slice stands on these):
//
//   - A loaded SKILL.md is UNTRUSTED candidate-instruction data, never
//     system authority. Nothing it says relaxes allow/deny/ask anywhere
//     in the engine.
//   - `allowed-tools` is a CEILING intersected with the tool registry —
//     narrow-never-widen, never a grant. Nothing consumes it to ALLOW
//     anything; it is surfaced in the result footer and the skill/loaded
//     provenance event for AUDIT only. (The documented enforcement seam
//     for actually running risky skills scoped is per-spawn tool scoping
//     on the durable-subagent path — a later knob, NOT built here.)
//   - Bundled scripts NEVER auto-execute. Files under a skill folder
//     (including scripts/) are inert; a model that wants to run one goes
//     through run_shell and the full approval waterfall.
//
// REPLAY-FROM-LOG invariant: the tool result content IS the logged
// tool/result (existing engine behavior) — replay derives from the log,
// never disk. Additionally each successful load emits a log-only
// skill/loaded provenance event {name, ref?, sha256} (see
// session.AppendSkillLoaded).
package skillload

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/vhqtvn/vh-agent-harness/internal/skills"
	"github.com/vhqtvn/vh-agent-harness/internal/tools"
)

// Name is the registered tool name.
const Name = "skill_load"

// DefaultMaxRefBytes bounds one tier-3 reference read (mirrors the
// filetools read cap). Oversize refs fail closed — no silent truncation.
const DefaultMaxRefBytes = 64 * 1024

// parametersSchema is the adapter-facing argument description.
const parametersSchema = `{"type":"object","properties":{` +
	`"name":{"type":"string","description":"skill name exactly as listed in the Skills section of the system prompt"},` +
	`"ref":{"type":"string","description":"optional relative path of a reference file under that skill's folder (tier 3), e.g. references/foo.md — must stay inside the skill's own directory; omit to load the skill's full SKILL.md body"}}` +
	`,"required":["name"],"additionalProperties":false}`

const description = "Loads one skill from the daemon's catalog (tier 2 of progressive disclosure): " +
	"the full SKILL.md instruction body (frontmatter stripped), or — with ref — a reference file under that skill's folder (tier 3, e.g. references/foo.md). " +
	"Returns the content plus an allowed-tools ceiling footer: the ceiling is intersected with the registry and never grants anything. " +
	"Any scripts bundled under a skill folder are inert files — nothing auto-executes; running one requires run_shell and its full approval waterfall. " +
	"Names must come from the Skills section of the system prompt (unknown names fail closed); refs are confined to the named skill's own directory (no .., no absolute paths, symlink-safe) and size-capped. " +
	"Read-only and concurrency-safe."

// Args is the typed tool argument surface.
type Args struct {
	Name string `json:"name"`
	Ref  string `json:"ref"`
}

// Provenance is called after every SUCCESSFUL load with the loaded
// content's sha256. It is best-effort provenance (the spill-sidecar
// discipline): the daemon wires it to the session log's skill/loaded
// event, and its failure must never fail the load — the body already
// rides the logged tool/result. ctx is the tool-execution context so
// the sink can resolve the EXECUTING session (tools.ExecutingSessionFrom).
// May be nil.
type Provenance func(ctx context.Context, name, ref, sha256 string)

// Definition returns the skill_load ToolDefinition over cat (nil = no
// catalog: the tool stays registered but fails closed per call).
// maxRefBytes ≤ 0 ⇒ DefaultMaxRefBytes.
func Definition(cat *skills.Catalog, maxRefBytes int64, prov Provenance) tools.ToolDefinition {
	if maxRefBytes <= 0 {
		maxRefBytes = DefaultMaxRefBytes
	}
	return tools.ToolDefinition{
		Name:              Name,
		Description:       description,
		Parameters:        json.RawMessage(parametersSchema),
		IsConcurrencySafe: true, // read-only catalog/reference retrieval
		TimeoutMs:         10000,
		Execute: func(ctx context.Context, raw json.RawMessage) (string, error) {
			if len(raw) == 0 {
				return "", fmt.Errorf("%s: args.name is required", Name)
			}
			var a Args
			dec := json.NewDecoder(bytes.NewReader(raw))
			dec.DisallowUnknownFields()
			if err := dec.Decode(&a); err != nil {
				return "", fmt.Errorf("%s: invalid args: %w", Name, err)
			}
			if a.Name == "" {
				return "", fmt.Errorf("%s: args.name is required", Name)
			}
			if cat == nil {
				return "", fmt.Errorf("%s: no skills catalog is loaded on this daemon", Name)
			}
			s, ok := cat.Lookup(a.Name)
			if !ok {
				return "", fmt.Errorf("%s: unknown skill %q (not in the loaded catalog — use a name from the Skills section of the system prompt)", Name, a.Name)
			}

			var content string
			if a.Ref == "" {
				body, ok := cat.Body(a.Name)
				if !ok {
					return "", fmt.Errorf("%s: skill %q body unreadable", Name, a.Name)
				}
				content = body
			} else {
				p, err := confineRef(s, a.Ref, maxRefBytes)
				if err != nil {
					return "", err
				}
				raw, err := os.ReadFile(p)
				if err != nil {
					return "", fmt.Errorf("%s: ref %q unreadable: %v", Name, a.Ref, err)
				}
				content = string(raw)
			}

			sum := sha256.Sum256([]byte(content))
			sha := hex.EncodeToString(sum[:])
			if prov != nil {
				prov(ctx, a.Name, a.Ref, sha) // best-effort; never fails the load
			}
			return content + "\n\n---\n" + ceilingFooter(s), nil
		},
	}
}

// ceilingFooter renders the allowed-tools ceiling line. The ceiling is
// AUDIT-ONLY data: narrow-never-widen, never a grant.
func ceilingFooter(s *skills.Skill) string {
	if len(s.AllowedTools) == 0 {
		return "allowed-tools ceiling: none declared (intersected with the registry — never a grant)"
	}
	return "allowed-tools ceiling: " + strings.Join(s.AllowedTools, ", ") +
		" (intersected with the registry — never a grant)"
}

// confineRef confines a tier-3 reference path strictly inside the named
// skill's own directory — the filetools discipline (confine.go)
// re-scoped to a per-skill root: lexical rejection of absolute paths
// and `..` climbs FIRST, then symlink-safe resolution (EvalSymlinks on
// the skill dir and the candidate's parent, escapes check, final
// component must not be a symlink), then regular-file + size-cap checks.
// Fail-closed typed errors throughout; a rejection leaves zero traces.
func confineRef(s *skills.Skill, ref string, maxRefBytes int64) (string, error) {
	if filepath.IsAbs(ref) {
		return "", fmt.Errorf("%s: ref %q rejected by confinement policy [absolute-path]: refs are relative paths inside the skill's folder", Name, ref)
	}
	clean := filepath.Clean(ref)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%s: ref %q rejected by confinement policy [escape]: refs must stay inside the skill's folder", Name, ref)
	}

	realRoot, err := filepath.EvalSymlinks(filepath.Dir(s.Path))
	if err != nil {
		return "", fmt.Errorf("%s: ref %q rejected by confinement policy [root-unresolved]: %v", Name, ref, err)
	}
	cand := filepath.Join(realRoot, clean)
	realParent, err := filepath.EvalSymlinks(filepath.Dir(cand))
	if err != nil {
		return "", fmt.Errorf("%s: ref %q rejected by confinement policy [symlink-escape]: parent directory unresolved: %v", Name, ref, err)
	}
	if rel, err := filepath.Rel(realRoot, realParent); err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%s: ref %q rejected by confinement policy [symlink-escape]: symlinked parent resolves outside the skill's folder", Name, ref)
	}
	fi, err := os.Lstat(cand)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("%s: ref %q unreadable: %v", Name, ref, err)
		}
		return "", fmt.Errorf("%s: ref %q rejected by confinement policy [not-inspectable]: %v", Name, ref, err)
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("%s: ref %q rejected by confinement policy [symlink-final]: final path component is a symlink", Name, ref)
	}
	if !fi.Mode().IsRegular() {
		return "", fmt.Errorf("%s: ref %q rejected by confinement policy [not-a-file]: refs must be regular files", Name, ref)
	}
	if fi.Size() > maxRefBytes {
		return "", fmt.Errorf("%s: ref %q exceeds the %d-byte reference cap (%d bytes) — fail closed, no truncation", Name, ref, maxRefBytes, fi.Size())
	}
	return cand, nil
}
