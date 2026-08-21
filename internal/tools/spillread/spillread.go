// Package spillread implements the spill_read tool: the model-facing
// path back to spilled oversize tool results (dsh spill retrievalHint,
// realized as a first-class tool so retrieval goes through the normal
// pipeline — the same waterfall, guards, and approval lattice as any
// other call). Input is the opaque SpillLocator carried by the spilled
// tool/result event and its notice, plus an optional window:
//
//   - offset (int64, default 0) — the window's start byte;
//   - length  (int, default 0)  — the window's size; 0 means "a
//     sensible default window" = the ACTIVE spill policy's
//     MaxInlineBytes, resolved server-side. Explicit lengths clamp to
//     the same cap.
//
// Retrieval is WINDOWED so that a result ALWAYS fits inside the inline
// cap by construction (the window bytes plus the in-band paging notice
// are reserved within the cap, the same discipline as the spill
// preview). The pre-windowing full-content return was a model-visible
// NO-OP: the commit-time SpillPolicy re-spilled the oversized
// retrieval, content addressing deduped it to the SAME locator, and
// the model saw a byte-identical preview — spilled bytes could never
// reach context. Windowed retrieval pages REAL bytes in: each result
// is the [offset, offset+length) slice of the stored content, followed
// (unless the window covers the whole content) by a trailing
// `[window offset=O length=L of SIZE bytes — adjust offset/length to
// page]` notice. Reads are full-file hash-validated and fail closed on
// any mismatch — a tampered or truncated store file never serves
// bytes, even when the requested window itself is healthy.
package spillread

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/vhqtvn/vh-agent-harness/internal/session"
	"github.com/vhqtvn/vh-agent-harness/internal/tools"
)

// Name is the registered tool name.
const Name = "spill_read"

// parametersSchema is the adapter-facing argument description.
const parametersSchema = `{"type":"object","properties":{` +
	`"locator":{"type":"object","description":"the spill locator object exactly as it appeared in the spilled result's notice or spillLocator field: {file, sha256, size}"},` +
	`"offset":{"type":"integer","description":"window start byte offset within the spilled content (default 0)"},` +
	`"length":{"type":"integer","description":"window length in bytes; 0 (default) = the inline-cap-sized default window; explicit values are clamped to the cap so every result fits inline"}}` +
	`,"required":["locator"],"additionalProperties":false}`

const description = "Retrieves a bounded window of an oversize tool result that was spilled to durable storage. " +
	"Pass the locator object exactly as it appeared in the spilled result (the {\"file\",\"sha256\",\"size\"} JSON from the notice line or the spillLocator field), plus optional offset/length to page: " +
	"each call returns the bytes of one window (default: the first inline-cap-sized window) with a trailing [window offset=O length=L of SIZE bytes] notice telling you where you are — advance offset by length to page through the content. " +
	"Windows are clamped so results always fit inline. Reads are hash-validated against the full content and fail closed on any mismatch (tampered, truncated, or missing content returns an error, never wrong bytes). " +
	"Read-only and concurrency-safe."

// Args is the typed tool argument surface.
type Args struct {
	Locator session.SpillLocator `json:"locator"`
	Offset  int64                `json:"offset"`
	Length  int                  `json:"length"`
}

// windowNotice is the in-band paging notice appended to any window
// that does not cover the whole content.
func windowNotice(offset, length int64, size int64) string {
	return fmt.Sprintf("[window offset=%d length=%d of %d bytes — adjust offset/length to page]", offset, length, size)
}

// Definition returns the spill_read ToolDefinition rooted at root (the
// daemon's session dir — retrieval walks the per-session `*.spill`
// stores beneath it; see session.ReadSpillWindowUnder). maxInline is
// the ACTIVE spill policy's inline cap and bounds every window
// (defaulting the length and clamping explicit ones) so a retrieval
// result always fits inline by construction; maxInline <= 0 ⇒
// session.DefaultSpillMaxInline (there is no armed policy to consult —
// spill disabled or a spec-only wiring — and the default cap is still a
// sensible window).
func Definition(root string, maxInline int64) tools.ToolDefinition {
	windowCap := maxInline
	if windowCap <= 0 {
		windowCap = session.DefaultSpillMaxInline
	}
	return tools.ToolDefinition{
		Name:              Name,
		Description:       description,
		Parameters:        json.RawMessage(parametersSchema),
		IsConcurrencySafe: true, // read-only, hash-checked retrieval
		TimeoutMs:         10000,
		Execute: func(ctx context.Context, raw json.RawMessage) (string, error) {
			if len(raw) == 0 {
				return "", fmt.Errorf("%s: args.locator is required", Name)
			}
			var a Args
			dec := json.NewDecoder(bytes.NewReader(raw))
			dec.DisallowUnknownFields()
			if err := dec.Decode(&a); err != nil {
				return "", fmt.Errorf("%s: invalid args: %w", Name, err)
			}
			if a.Locator.File == "" || a.Locator.SHA256 == "" {
				return "", fmt.Errorf("%s: args.locator must carry the full spilled locator (file, sha256, size) exactly as it appeared in the spilled result", Name)
			}
			if a.Offset < 0 {
				return "", fmt.Errorf("%s: offset must be >= 0 (got %d)", Name, a.Offset)
			}
			if a.Length < 0 {
				return "", fmt.Errorf("%s: length must be >= 0 (got %d)", Name, a.Length)
			}

			// Resolve the window: default = the cap; explicit values
			// clamp to the cap. A retrieval result must ALWAYS fit
			// inline by construction.
			winLen := int64(a.Length)
			if winLen == 0 {
				winLen = windowCap
			}
			if winLen > windowCap {
				winLen = windowCap
			}
			// When a paging notice will ride in-band, reserve its bytes
			// INSIDE the cap (the same discipline as the spill
			// preview): shrink the window until window + "\n" + notice
			// fits. The sum strictly decreases with winLen, so the loop
			// terminates; for real caps (KiB-scale) it settles within a
			// few bytes.
			size := a.Locator.Size
			if a.Offset > 0 || a.Offset+winLen < size {
				for winLen > 0 && winLen+int64(len("\n"+windowNotice(a.Offset, winLen, size))) > windowCap {
					winLen--
				}
			}

			window, err := session.ReadSpillWindowUnder(root, a.Locator, a.Offset, winLen)
			if err != nil {
				return "", fmt.Errorf("%s: %w", Name, err)
			}
			// A window that covers the WHOLE content needs no paging
			// notice (only reachable for content that fits one window).
			if a.Offset == 0 && int64(len(window)) >= size {
				return string(window), nil
			}
			return string(window) + "\n" + windowNotice(a.Offset, winLen, size), nil
		},
	}
}
