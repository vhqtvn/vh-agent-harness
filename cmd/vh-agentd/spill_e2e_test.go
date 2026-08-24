// spill_e2e_test.go — the spill CRUX over the daemon's own
// composition: a real buildServer engine + a real session (FileSpillStore
// armed per session) + the REAL run_shell tool. One logged turn whose
// tool result exceeds --spill-max-inline commits a spilled event
// (bounded preview + notice + locator), the persisted log replays
// byte-identically, and follow-up turns page the spilled content back
// through the real pipeline via WINDOWED spill_read (page 1 default,
// page 2 explicit offset — different bytes, the no-op kill test — and
// an overlong length clamped to the cap), every retrieval result
// staying inline. Also pins the flag contract (default 65536;
// 0 = disabled; negative = fail-closed) and the four-tool daemon set.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vhqtvn/vh-agent-harness/internal/adapters"
	"github.com/vhqtvn/vh-agent-harness/internal/session"
	"github.com/vhqtvn/vh-agent-harness/internal/tools/spillread"
)

// scriptedAdapter serves Call responses from a script function indexed
// by call number (the two-turn crux: run_shell, then spill_read with
// the locator the first turn produced).
type scriptedAdapter struct {
	next  func(calls int) *adapters.Response
	calls int
}

func (a *scriptedAdapter) Name() string { return "scripted" }
func (a *scriptedAdapter) Call(_ context.Context, _ *adapters.Request) (*adapters.Response, error) {
	r := a.next(a.calls)
	a.calls++
	return r, nil
}

// spillTestConfig validates a config with the spill flag threaded.
func spillTestConfig(t *testing.T, dir string, spillMaxInline int64) *Config {
	t.Helper()
	cfg, err := validate("openai", "fake-model", "http://x.test", "VH_AGENTD_TEST_KEY", dir, "", 0, defaultApprovalTimeoutMs, 0, "off", spillMaxInline, "")
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	return cfg
}

func TestValidateSpillMaxInline(t *testing.T) {
	// Default 65536 matches the run_shell capture cap order.
	if cfg := spillTestConfig(t, t.TempDir(), 65536); cfg.SpillMaxInline != 65536 {
		t.Fatalf("SpillMaxInline = %d, want 65536", cfg.SpillMaxInline)
	}
	// 0 = spill disabled (the explicit off switch).
	if cfg := spillTestConfig(t, t.TempDir(), 0); cfg.SpillMaxInline != 0 {
		t.Fatalf("SpillMaxInline = %d, want 0 (disabled)", cfg.SpillMaxInline)
	}
	// Negative: fail closed.
	if _, err := validate("openai", "m", "http://x.test", "K", t.TempDir(), "", 0, defaultApprovalTimeoutMs, 0, "off", -1, ""); err == nil || !strings.Contains(err.Error(), "--spill-max-inline") {
		t.Fatalf("negative spill cap must fail closed, got %v", err)
	}
}

// TestDaemonToolSetIncludesSpillRead: the daemon tool set covers the
// dogfood probes, run_shell, spill_read, the file family, and the
// subagent family, and the engine wires the per-session spill policy.
func TestDaemonToolSetIncludesSpillRead(t *testing.T) {
	cfg := spillTestConfig(t, t.TempDir(), 65536)
	svc, cli := net.Pipe()
	defer cli.Close()
	_, eng, _, _ := buildServer(cfg, "test-key", svc)
	got := map[string]bool{}
	for _, d := range eng.Pipeline().Definitions() {
		got[d.Name] = true
	}
	if !got["echo"] || !got["clock"] || !got["run_shell"] || !got[spillread.Name] {
		t.Fatalf("daemon tools not registered: %v", got)
	}
	if !got["subagent_spawn"] || !got["subagent_send"] {
		t.Fatalf("model-facing subagent tools not registered: %v", got)
	}
	if !got["read"] || !got["write"] || !got["edit"] || !got["glob"] || !got["search"] {
		t.Fatalf("model-facing file tools not registered: %v", got)
	}
	if len(eng.TurnOptions().Tools) != 11 {
		t.Fatalf("turn options do not advertise all eleven tools: %+v", eng.TurnOptions().Tools)
	}
	// The per-session policy seam is wired: a new session's log carries
	// an armed policy with a FileSpillStore under <dir>/<id>.spill/.
	es, err := eng.NewSession("", "sess-wired", io.Discard)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	pol := es.Log.SpillPolicy()
	if pol == nil || pol.Store == nil || pol.MaxInlineBytes != 65536 {
		t.Fatalf("session spill policy not armed: %+v", pol)
	}
}

// TestSpillCruxEndToEnd is the load-bearing behavioral closure for this
// slice: threshold-crossing run_shell result → spilled event with
// preview + locator → byte-identical replay (regardless of spill-file
// existence) → WINDOWED retrieval through spill_read over the real
// pipeline: page 1 (default window) and page 2 (explicit offset from
// page 1's notice) return DIFFERENT bytes — the no-op kill test (the
// pre-windowing tool returned the full content, which the commit-time
// policy re-spilled to a byte-identical preview, so no page of spilled
// bytes ever reached the model) — a length above the cap is clamped, and
// every retrieval result stays INLINE (never re-spilled) because it is
// <= cap by construction.
func TestSpillCruxEndToEnd(t *testing.T) {
	dir := t.TempDir()
	cfg := spillTestConfig(t, dir, 65536)
	svc, cli := net.Pipe()
	defer cli.Close()
	_, eng, _, _ := buildServer(cfg, "test-key", svc)

	es, err := eng.NewSession("", "sess-crux", io.Discard)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	// The oversized result: 40000 A's on stdout AND 40000 B's on stderr.
	// Each stream stays UNDER run_shell's 64 KiB per-stream capture cap
	// (so capture truncates nothing — Truncated stays false), while the
	// SERIALIZED outcome (~80 KB JSON) crosses the 65536 spill threshold
	// at commit time: the spill replaces LOSSY truncation with a full
	// sidecar copy (requirement 4's exact contract).
	const bigCmd = `head -c 40000 /dev/zero | tr '\0' A; head -c 40000 /dev/zero | tr '\0' B >&2`

	var loc *session.SpillLocator
	var page1 string // the first retrieval window, for page 2's offset
	readArgs := func(extra map[string]any) json.RawMessage {
		m := map[string]any{"locator": *loc}
		for k, v := range extra {
			m[k] = v
		}
		args, _ := json.Marshal(m)
		return args
	}
	ad := &scriptedAdapter{next: func(calls int) *adapters.Response {
		switch calls {
		case 0:
			return &adapters.Response{
				Model: "fake-model", FinishReason: "tool_calls",
				ToolCalls: []session.ToolCall{{
					ID: "call_big", Name: "run_shell",
					Args: json.RawMessage(fmt.Sprintf(`{"command":%q}`, bigCmd)),
				}},
			}
		case 1:
			// Page 1: the default window.
			return &adapters.Response{
				Model: "fake-model", FinishReason: "tool_calls",
				ToolCalls: []session.ToolCall{{ID: "call_read1", Name: spillread.Name, Args: readArgs(nil)}},
			}
		case 2:
			// Page 2: follow page 1's notice (next offset =
			// offset+length) — what a model paging through the spilled
			// bytes does.
			off1, len1, _ := parseWindowNotice(t, page1)
			return &adapters.Response{
				Model: "fake-model", FinishReason: "tool_calls",
				ToolCalls: []session.ToolCall{{ID: "call_read2", Name: spillread.Name, Args: readArgs(map[string]any{"offset": off1 + len1})}},
			}
		default:
			// Page 3: an absurd explicit length (1 MiB) — must clamp to
			// the cap and stay inline.
			return &adapters.Response{
				Model: "fake-model", FinishReason: "tool_calls",
				ToolCalls: []session.ToolCall{{ID: "call_read3", Name: spillread.Name, Args: readArgs(map[string]any{"length": 1 << 20})}},
			}
		}
	}}

	ctx := context.Background()
	report1, err := eng.TurnRunner().RunTurn(ctx, es.Log, ad, eng.TurnOptions(), "print 70000 A's")
	if err != nil {
		t.Fatalf("RunTurn 1: %v", err)
	}
	if len(report1.Results) != 1 || report1.Results[0].IsError {
		t.Fatalf("turn-1 results = %+v", report1.Results)
	}
	preview := report1.Results[0].Content
	if len(preview) > 65536 {
		t.Fatalf("committed preview = %d bytes, must stay <= 65536", len(preview))
	}
	if !strings.Contains(preview, "[spilled ") || !strings.Contains(preview, "read via spill_read") {
		t.Fatalf("preview missing the spill notice: %q...", preview[:120])
	}

	// The durable event carries the additive spill fields.
	var spilled *session.ToolResultPayload
	for _, ev := range es.Log.Events() {
		if ev.Type != session.TypeToolResult {
			continue
		}
		var p session.ToolResultPayload
		if err := json.Unmarshal(ev.Payload, &p); err != nil {
			t.Fatalf("payload: %v", err)
		}
		if p.Spilled {
			spilled = &p
		}
	}
	if spilled == nil || spilled.SpillLocator == nil {
		t.Fatalf("no spilled tool/result event in the log: %+v", spilled)
	}
	loc = spilled.SpillLocator

	// The spill file exists under <session-dir>/<session-id>.spill/ and
	// holds the FULL serialized outcome.
	full, err := session.NewFileSpillStore(dir, "sess-crux").Read(*loc)
	if err != nil {
		t.Fatalf("spill read: %v", err)
	}
	var outcome struct {
		Cause     string `json:"cause"`
		Stdout    string `json:"stdout"`
		Stderr    string `json:"stderr"`
		Truncated bool   `json:"truncated"`
	}
	if err := json.Unmarshal(full, &outcome); err != nil {
		t.Fatalf("spilled content is not the run_shell outcome: %v", err)
	}
	if outcome.Cause != "exit" || outcome.Truncated ||
		len(outcome.Stdout) != 40000 || strings.Trim(outcome.Stdout, "A") != "" ||
		len(outcome.Stderr) != 40000 || strings.Trim(outcome.Stderr, "B") != "" {
		t.Fatalf("spilled outcome = cause %s truncated %v stdout %dB stderr %dB (want both streams fully captured: 40000 A's + 40000 B's)",
			outcome.Cause, outcome.Truncated, len(outcome.Stdout), len(outcome.Stderr))
	}

	// Turn 2 — page 1 (default window): the FIRST bytes of the spilled
	// content, inline (never re-spilled — the no-op fix).
	report2, err := eng.TurnRunner().RunTurn(ctx, es.Log, ad, eng.TurnOptions(), "read the spilled bytes back")
	if err != nil {
		t.Fatalf("RunTurn 2: %v", err)
	}
	if len(report2.Results) != 1 || report2.Results[0].IsError {
		t.Fatalf("turn-2 results = %+v", report2.Results)
	}
	page1 = report2.Results[0].Content
	if len(page1) > 65536 {
		t.Fatalf("page 1 = %d bytes, must stay <= the 65536 inline cap", len(page1))
	}
	off1, len1, size1 := parseWindowNotice(t, page1)
	if off1 != 0 || size1 != loc.Size {
		t.Fatalf("page-1 notice = offset %d of %d, want 0 of %d", off1, size1, loc.Size)
	}
	if !strings.HasPrefix(page1, string(full[:len1])) {
		t.Fatalf("page 1 does not start with the spilled content's first %d bytes", len1)
	}
	assertResultInline(t, es.Log, "call_read1")

	// Turn 3 — page 2 (explicit offset from page 1's notice): DIFFERENT
	// bytes. This is the red-green crux: under the pre-fix behavior
	// every retrieval returned the same byte-identical re-spilled
	// preview, so no page of spilled bytes ever reached the model.
	report3, err := eng.TurnRunner().RunTurn(ctx, es.Log, ad, eng.TurnOptions(), "read the next window")
	if err != nil {
		t.Fatalf("RunTurn 3: %v", err)
	}
	if len(report3.Results) != 1 || report3.Results[0].IsError {
		t.Fatalf("turn-3 results = %+v", report3.Results)
	}
	page2 := report3.Results[0].Content
	if page2 == page1 {
		t.Fatal("page 2 returned the SAME bytes as page 1 — retrieval is a no-op (the pre-fix re-spill bug)")
	}
	off2, _, _ := parseWindowNotice(t, page2)
	if off2 != off1+len1 {
		t.Fatalf("page-2 offset = %d, want %d (following page 1's notice)", off2, off1+len1)
	}
	if len(page2) > 65536 {
		t.Fatalf("page 2 = %d bytes, must stay <= the 65536 inline cap", len(page2))
	}
	if !strings.HasPrefix(page2, string(full[off2:off2+32])) {
		t.Fatalf("page 2 does not start with the spilled content's bytes at offset %d", off2)
	}
	assertResultInline(t, es.Log, "call_read2")

	// Turn 4 — length > cap clamps: the result is the default first
	// window again (offset 0, length clamped to the cap) and inline.
	report4, err := eng.TurnRunner().RunTurn(ctx, es.Log, ad, eng.TurnOptions(), "overlong window must clamp")
	if err != nil {
		t.Fatalf("RunTurn 4: %v", err)
	}
	if len(report4.Results) != 1 || report4.Results[0].IsError {
		t.Fatalf("turn-4 results = %+v", report4.Results)
	}
	if got := report4.Results[0].Content; got != page1 {
		t.Fatalf("clamped read must equal the default first window (%d bytes), got %d bytes", len(page1), len(got))
	}
	assertResultInline(t, es.Log, "call_read3")

	// Hash validation still holds end-to-end.
	sum := sha256.Sum256(full)
	if hex.EncodeToString(sum[:]) != loc.SHA256 {
		t.Fatal("retrieved content hash does not match the locator")
	}

	// Replay: the persisted log file folds byte-identically — and the
	// spill files are sidecar state, so deleting them first changes
	// nothing about replay (loss degrades retrieval, not replay).
	if err := os.RemoveAll(filepath.Join(dir, "sess-crux.spill")); err != nil {
		t.Fatalf("remove spill dir: %v", err)
	}
	raw, err := os.ReadFile(es.Path)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	replayed, err := session.Replay(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("Replay: %v", err)
	}
	liveJSON, _ := json.Marshal(es.Log.Events())
	replayJSON, _ := json.Marshal(replayed)
	if !bytes.Equal(liveJSON, replayJSON) {
		t.Fatal("replayed events are not byte-identical to the live log")
	}
	liveMsgs, err := es.Log.Surface()
	if err != nil {
		t.Fatalf("live surface: %v", err)
	}
	replayMsgs, err := session.DeriveMessages(replayed)
	if err != nil {
		t.Fatalf("DeriveMessages: %v", err)
	}
	liveMsgsJSON, _ := json.Marshal(liveMsgs)
	replayMsgsJSON, _ := json.Marshal(replayMsgs)
	if !bytes.Equal(liveMsgsJSON, replayMsgsJSON) {
		t.Fatal("replayed surface is not byte-identical to the live surface")
	}
	// The spilled preview (with its locator) is model-visible from the
	// log alone; every retrieval WINDOW is visible inline too — real
	// spilled bytes in context, not a re-spilled preview. The expected
	// byte count is derived from THIS run's locator: the serialized
	// run_shell outcome embeds durationMs, so its exact length (and the
	// notice's count) is machine-speed-dependent — a hardcoded total
	// only holds on a machine fast enough to keep durationMs under the
	// next digit boundary.
	wantNotice := fmt.Sprintf("[spilled %d bytes:", loc.Size)
	foundSpilled, foundWindow := false, false
	for _, m := range replayMsgs {
		if strings.Contains(m.Content, wantNotice) {
			foundSpilled = true
		}
		if strings.Contains(m.Content, "[window offset=0 ") {
			foundWindow = true
		}
	}
	if !foundSpilled || !foundWindow {
		t.Fatalf("replayed surface missing spilled preview (%v) or inline window (%v)", foundSpilled, foundWindow)
	}
}

// assertResultInline asserts the committed payload for callID was NOT
// re-spilled: a windowed retrieval result is <= cap by construction,
// so the commit-time policy keeps it inline (the pre-fix tool's
// full-content returns were re-spilled to byte-identical previews).
func assertResultInline(t *testing.T, lg *session.Log, callID string) {
	t.Helper()
	for _, ev := range lg.Events() {
		if ev.Type != session.TypeToolResult {
			continue
		}
		var p session.ToolResultPayload
		if err := json.Unmarshal(ev.Payload, &p); err != nil {
			t.Fatalf("payload: %v", err)
		}
		if p.CallID == callID && (p.Spilled || p.SpillLocator != nil) {
			t.Fatalf("retrieval %s was re-spilled at commit — the no-op bug: %+v", callID, p)
		}
	}
}

// parseWindowNotice extracts (offset, length, size) from a retrieval
// result's trailing [window …] notice — the arithmetic a model follows
// to page (next offset = offset+length).
func parseWindowNotice(t *testing.T, out string) (offset, length, size int64) {
	t.Helper()
	i := strings.LastIndex(out, "\n[window ")
	if i < 0 {
		t.Fatalf("result lacks the window notice: tail %q", out[min(len(out), 160):])
	}
	rest := out[i+1:]
	rest = strings.TrimSuffix(rest, "]")
	var o, l, s int64
	if n, _ := fmt.Sscanf(rest, "[window offset=%d length=%d of %d bytes", &o, &l, &s); n != 3 {
		t.Fatalf("unparsable window notice: %q", rest)
	}
	return o, l, s
}
