// spill_e2e_test.go — the spill CRUX over the daemon's own
// composition: a real buildServer engine + a real session (FileSpillStore
// armed per session) + the REAL run_shell tool. One logged turn whose
// tool result exceeds --spill-max-inline commits a spilled event
// (bounded preview + notice + locator), the persisted log replays
// byte-identically, and a second turn retrieves the FULL content
// through the real pipeline via spill_read, hash-validated against the
// locator. Also pins the flag contract (default 65536; 0 = disabled;
// negative = fail-closed) and the four-tool daemon set.
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
	cfg, err := validate("openai", "fake-model", "http://x.test", "VH_AGENTD_TEST_KEY", dir, "", 0, defaultApprovalTimeoutMs, 0, "off", spillMaxInline)
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
	if _, err := validate("openai", "m", "http://x.test", "K", t.TempDir(), "", 0, defaultApprovalTimeoutMs, 0, "off", -1); err == nil || !strings.Contains(err.Error(), "--spill-max-inline") {
		t.Fatalf("negative spill cap must fail closed, got %v", err)
	}
}

// TestDaemonToolSetIncludesSpillRead: the daemon tool set grows to four
// (echo, clock, run_shell, spill_read) and the engine wires the
// per-session spill policy.
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
	if len(eng.TurnOptions().Tools) != 4 {
		t.Fatalf("turn options do not advertise all four tools: %+v", eng.TurnOptions().Tools)
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
// existence) → full-content retrieval through spill_read over the real
// pipeline → hash-validated.
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
	ad := &scriptedAdapter{next: func(calls int) *adapters.Response {
		if calls == 0 {
			return &adapters.Response{
				Model: "fake-model", FinishReason: "tool_calls",
				ToolCalls: []session.ToolCall{{
					ID: "call_big", Name: "run_shell",
					Args: json.RawMessage(fmt.Sprintf(`{"command":%q}`, bigCmd)),
				}},
			}
		}
		// Turn 2: retrieve the spilled bytes with the locator from turn 1.
		args, _ := json.Marshal(map[string]any{"locator": *loc})
		return &adapters.Response{
			Model: "fake-model", FinishReason: "tool_calls",
			ToolCalls: []session.ToolCall{{
				ID: "call_read", Name: spillread.Name, Args: args,
			}},
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
	if !strings.Contains(preview, "[spilled ") || !strings.Contains(preview, "read via spill/read") {
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

	// Turn 2: spill_read through the real pipeline retrieves the FULL
	// content. The retrieval (80213 bytes) itself crosses the threshold,
	// so its OWN result re-spills at commit — and because the store is
	// content-addressed, the re-spill's locator MUST equal the original
	// locator: byte-identical retrieval proven by dedup, plus the
	// hash check on the store read above.
	report2, err := eng.TurnRunner().RunTurn(ctx, es.Log, ad, eng.TurnOptions(), "read the spilled bytes back")
	if err != nil {
		t.Fatalf("RunTurn 2: %v", err)
	}
	if len(report2.Results) != 1 || report2.Results[0].IsError {
		t.Fatalf("turn-2 results = %+v", report2.Results)
	}
	if !strings.Contains(report2.Results[0].Content, "[spilled 80213 bytes:") {
		t.Fatalf("spill_read result should itself be a spilled preview of the full retrieval: %q...", report2.Results[0].Content[:120])
	}
	sum := sha256.Sum256(full)
	if hex.EncodeToString(sum[:]) != loc.SHA256 {
		t.Fatal("retrieved content hash does not match the locator")
	}
	var readLoc *session.SpillLocator
	for _, ev := range es.Log.Events() {
		if ev.Type != session.TypeToolResult {
			continue
		}
		var p session.ToolResultPayload
		if err := json.Unmarshal(ev.Payload, &p); err != nil {
			t.Fatalf("payload: %v", err)
		}
		if p.Spilled && p.CallID == "call_read" {
			readLoc = p.SpillLocator
		}
	}
	if readLoc == nil {
		t.Fatal("spill_read result was not itself spilled (payload missing)")
	}
	if *readLoc != *loc {
		t.Fatalf("retrieval re-spill locator %v != original %v: retrieval was not byte-identical", *readLoc, *loc)
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
	// log alone; the retrieval turn's own preview is visible too (the
	// full bytes live in the shared, content-addressed sidecar).
	foundSpilled, foundRead := false, false
	for _, m := range replayMsgs {
		if strings.Contains(m.Content, "[spilled 80213 bytes:") {
			foundSpilled = true
		}
		if strings.Contains(m.Content, "[spilled ") && m.ToolCallID == "call_read" {
			foundRead = true
		}
	}
	if !foundSpilled || !foundRead {
		t.Fatalf("replayed surface missing spilled preview (%v) or retrieval preview (%v)", foundSpilled, foundRead)
	}
}
