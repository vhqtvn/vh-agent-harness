// Package overlay implements the Slice-4 overlay pack mechanism: opt-in packs
// selected via vh-harness-profile.yml (overlays:[...]) that layer additively on top
// of the curated core corpus. Each pack contributes unit files (agents/, skills/,
// commands/ mirroring the .opencode/ subtree) plus two merge-content files:
//
//   - opencode-append.jsonc   — deep-merged into the rendered opencode.jsonc
//   - callable-graph-snippet.md — appended to callable-graph.md
//
// The deep-merge is dependency-free (stdlib encoding/json) and string-aware
// comment-stripping so it works on JSONC sources. Overlay units are ownership
// class overlay_extension (auto-overwritten while the pack stays active).
package overlay

import (
	"encoding/json"
	"fmt"

	"github.com/vhqtvn/vh-agent-harness/internal/jsonc"
)

// MergeJSONC deep-merges one or more JSONC append documents into a base JSONC
// document. Comments are stripped before parsing (JSONC -> JSON); the merged
// result is re-serialized as indented JSON (comments are not preserved in merged
// output — acceptable since opencode reads JSONC tolerantly).
//
// Merge semantics: for each key in an append, if both base and append values are
// JSON objects, recurse; otherwise the append value wins (scalars, arrays, and
// type-mismatches overwrite the base). This lets an append BOTH introduce new
// top-level entries (e.g. a brand-new agent block) AND inject keys into existing
// nested maps (e.g. add "browser-qa": "allow" into an orchestrator agent's
// permission.task map) without disturbing sibling fields.
func MergeJSONC(base []byte, appends ...[]byte) ([]byte, error) {
	dst, err := parseJSONC(base)
	if err != nil {
		return nil, fmt.Errorf("merge base: %w", err)
	}
	for i, a := range appends {
		src, err := parseJSONC(a)
		if err != nil {
			return nil, fmt.Errorf("merge append[%d]: %w", i, err)
		}
		deepMerge(dst, src)
	}
	out, err := json.MarshalIndent(dst, "", "    ")
	if err != nil {
		return nil, fmt.Errorf("merge marshal: %w", err)
	}
	return append(out, '\n'), nil
}

// parseJSONC strips JSONC comments and unmarshals into a map. A null/empty
// document yields an empty map (never nil) so callers can merge safely.
//
// Thin wrapper over internal/jsonc.Parse — the string-aware comment/comma
// stripping is shared with internal/permconfig so there is exactly one
// implementation in the binary.
func parseJSONC(b []byte) (map[string]any, error) {
	return jsonc.Parse(b)
}

// deepMerge recursively merges src into dst in place. Nested maps recurse; all
// other value kinds are overwritten by src (append wins).
func deepMerge(dst, src map[string]any) {
	for k, sv := range src {
		dv, ok := dst[k]
		if !ok {
			dst[k] = sv
			continue
		}
		dm, dIsMap := dv.(map[string]any)
		sm, sIsMap := sv.(map[string]any)
		if dIsMap && sIsMap {
			deepMerge(dm, sm)
		} else {
			dst[k] = sv
		}
	}
}
