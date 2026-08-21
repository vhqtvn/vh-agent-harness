// Package spillread implements the spill_read tool: the model-facing
// path back to spilled oversize tool results (dsh spill retrievalHint,
// realized as a first-class tool so retrieval goes through the normal
// pipeline — the same waterfall, guards, and approval lattice as any
// other call). Input is the opaque SpillLocator carried by the spilled
// tool/result event and its notice; output is the FULL content,
// hash-validated fail-closed and capped at a large-but-bounded read.
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

// DefaultMaxReadBytes caps the CONTENT of one retrieval (1 MiB — large
// enough for the typical spilled run_shell outcome, bounded enough that
// a hostile or accidental huge spill cannot flood the context through
// retrieval). A truncation notice rides on top of the cap in-band (the
// same posture as run_shell's capture marker).
const DefaultMaxReadBytes = 1 << 20

// parametersSchema is the adapter-facing argument description.
const parametersSchema = `{"type":"object","properties":{` +
	`"locator":{"type":"object","description":"the spill locator object exactly as it appeared in the spilled result's notice or spillLocator field: {file, sha256, size}"}}` +
	`,"required":["locator"],"additionalProperties":false}`

const description = "Retrieves the full bytes of an oversize tool result that was spilled to durable storage. " +
	"Pass the locator object exactly as it appeared in the spilled result (the {\"file\",\"sha256\",\"size\"} JSON from the notice line or the spillLocator field). " +
	"Reads are hash-validated and fail closed on any mismatch (tampered, truncated, or missing content returns an error, never wrong bytes). " +
	"Output is capped at 1 MiB per call with a trailing truncation notice. Read-only and concurrency-safe."

// Args is the typed tool argument surface.
type Args struct {
	Locator session.SpillLocator `json:"locator"`
}

// Definition returns the spill_read ToolDefinition rooted at root (the
// daemon's session dir — retrieval walks the per-session `*.spill`
// stores beneath it; see session.ReadSpillUnder). maxRead <= 0 ⇒
// DefaultMaxReadBytes.
func Definition(root string, maxRead int64) tools.ToolDefinition {
	if maxRead <= 0 {
		maxRead = DefaultMaxReadBytes
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
			content, err := session.ReadSpillUnder(root, a.Locator)
			if err != nil {
				return "", fmt.Errorf("%s: %w", Name, err)
			}
			if int64(len(content)) > maxRead {
				dropped := int64(len(content)) - maxRead
				return string(content[:maxRead]) + fmt.Sprintf("\n[%s: output truncated, %d bytes dropped (cap %dB)]", Name, dropped, maxRead), nil
			}
			return string(content), nil
		},
	}
}
