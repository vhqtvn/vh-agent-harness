// e2e_jobtailing_eviction_test.go — review hotfix F2 (red first): the
// retention-eviction re-sync at the REAL binary seam (real
// vh-agent-client spawning the real vh-agentd via --exec, scripted
// LLM). The producer parks the drain's cursor on a tiny prefix
// ("pre\n"), stalls 1s (the drain settles into its 150ms empty-poll
// cadence at that cursor), then bursts 32 MiB — far past the 256 KiB
// retention ring. The drain reader is capped at 64×16 KiB pages per
// 150ms poll (~1 MiB/s) while the burst writes 32 MiB in well under a
// second, so the ring wrapping past the cursor is deterministic, not a
// timing hope.
//
// Post-fix contract under test: on the typed output-evicted -32602
// the client re-syncs forward to data.evictedBase and keeps paging, a
// settled job behind retention COMPLETES its drain (retained tail
// rendered, job/settled + job/report flushed, exit 0), and the
// HONEST-ABSENCE posture holds: the evicted prefix is asserted ABSENT
// (final re-sync lands at evictedBytes == written − retention; the
// rendered total is strictly less than written), never reassembled as
// full content. Pre-fix (the F2 wedge) the client spins on the
// evicted error until bgWaitMax and this test fails the 60s exit
// bound.
package main

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// TestClientBinaryJobTailingEvictionResync is the F2 e2e (red first).
func TestClientBinaryJobTailingEvictionResync(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and spawns real binaries")
	}
	clientBin, daemonBin := buildBinaries(t)

	// Producer: tiny prefix → 1s stall → 32 MiB burst (raw /dev/zero
	// is the fastest possible producer; the ring wraps past any cursor
	// the capped reader could reach mid-burst, so re-syncs may fire
	// more than once — every gap must still land FORWARD).
	command := "printf 'pre\\n'; sleep 1; head -c 33554432 /dev/zero"
	args, _ := json.Marshal(map[string]any{
		"command": command, "background": true, "timeout_ms": 30000,
	})

	var mu sync.Mutex
	calls := 0
	llm := startHTTPStub(t, func(w *jsonEncoder) {
		mu.Lock()
		calls++
		n := calls
		mu.Unlock()
		if n == 1 {
			w.toolCall("call-evict", "run_shell", string(args))
			return
		}
		w.content("eviction drain ok")
	})
	defer llm.Close()

	sessDir := filepath.Join(t.TempDir(), "sessions")
	code, out, errbuf := runClient(t, clientBin, []string{
		"--session-dir", sessDir,
		"--json",
		"--prompt", "start the eviction producer in background",
		"--exec", daemonBin,
		"--adapter", "openai", "--model", "fake-model",
		"--base-url", llm.URL,
		"--api-key-env", "VH_AGENTD_TEST_KEY",
	}, "")

	if code != 0 {
		t.Fatalf("client exit = %d, want 0 (stderr tail:\n%s)", code, tailText(errbuf))
	}
	if n := strings.Count(errbuf, "warning: jobs/output"); n != 0 {
		t.Fatalf("expected zero jobs/output warnings after the re-sync, got %d (stderr tail:\n%s)", n, tailText(errbuf))
	}
	if !strings.Contains(errbuf, "oldest retained byte") {
		t.Fatalf("the one-time re-sync note (honest absence) missing from stderr:\n%s", tailText(errbuf))
	}

	// Parse the machine stream: client-synthesized job-output records
	// + verbatim session events + the final prompt-result line.
	type tailRec struct {
		JobID      string `json:"jobId"`
		State      string `json:"state"`
		Offset     int64  `json:"offset"`
		NextOffset int64  `json:"nextOffset"`
		Chunk      string `json:"chunk"`
		HasMore    bool   `json:"hasMore"`
		Written    int64  `json:"written"`
		Evicted    int64  `json:"evictedBytes"`
	}
	var recs []tailRec
	sawRunning, sawSettledEvent, sawReport := false, false, false
	var lastLine map[string]json.RawMessage
	for _, ln := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		if ln == "" {
			continue
		}
		var obj map[string]json.RawMessage
		if err := json.Unmarshal([]byte(ln), &obj); err != nil {
			t.Fatalf("stdout line is not valid NDJSON: %q (%v)", ln, err)
		}
		if k, ok := obj["kind"]; ok && string(k) == `"job-output"` {
			var r tailRec
			if err := json.Unmarshal([]byte(ln), &r); err != nil {
				t.Fatalf("job-output record not decodable: %q (%v)", ln, err)
			}
			if r.JobID == "shell-1" {
				if r.State == "running" {
					sawRunning = true
				}
				recs = append(recs, r)
			}
		}
		if ty, ok := obj["type"]; ok {
			switch string(ty) {
			case `"job/settled"`:
				sawSettledEvent = true
			case `"job/report"`:
				sawReport = true
			}
		}
		lastLine = obj
	}
	if len(recs) < 17 {
		t.Fatalf("tail records = %d, want ≥17 (mid-flight prefix + 16 retained-window pages + terminal shape) — stream:\n%s", len(recs), tailText(out))
	}
	if !sawRunning {
		t.Fatalf("no running-state tail record — the mid-flight prefix read is missing (stream:\n%s)", tailText(out))
	}
	if !sawSettledEvent || !sawReport {
		t.Fatalf("drain did not flush settlement/report: settled=%v report=%v (stream:\n%s)", sawSettledEvent, sawReport, tailText(out))
	}
	if string(lastLine["kind"]) != `"prompt-result"` || string(lastLine["content"]) != `"eviction drain ok"` {
		t.Fatalf("last stdout line = %v, want the prompt-result object", lastLine)
	}

	// Cursor chain: contiguous within segments, and every gap is a
	// FORWARD re-sync jump (offset > previous nextOffset). The FINAL
	// gap must land exactly at evictedBase == written − retention.
	const retention = int64(256 * 1024)
	gaps := 0
	lastGap := -1
	for i := 1; i < len(recs); i++ {
		if recs[i].Offset > recs[i-1].NextOffset {
			gaps++
			lastGap = i
		} else if recs[i].Offset != recs[i-1].NextOffset {
			t.Fatalf("cursor chain broken backwards at record %d: offset %d != previous nextOffset %d", i, recs[i].Offset, recs[i-1].NextOffset)
		}
	}
	if gaps == 0 {
		t.Fatalf("no re-sync gap in the tail records — the eviction never re-synced (stream:\n%s)", tailText(out))
	}
	last := recs[len(recs)-1]
	written, evicted := last.Written, last.Evicted
	if evicted <= 0 || written-evicted != retention {
		t.Fatalf("final record evictedBytes=%d written=%d — want evicted == written−%d (ring math)", evicted, written, retention)
	}
	if recs[lastGap].Offset != evicted {
		t.Fatalf("final re-sync landed at %d, want evictedBase %d", recs[lastGap].Offset, evicted)
	}
	if last.Chunk != "" || last.Offset != written || last.HasMore || last.NextOffset != written || last.State != "settled" {
		t.Fatalf("terminal record = %+v — want the one-time empty end-of-tail marker (B-F1): chunk empty, offset == nextOffset == written (%d), settled, hasMore=false", last, written)
	}

	// Honest absence: the retained window is fully rendered (the final
	// segment sums to exactly retention bytes) while the evicted prefix
	// is honestly ABSENT — the rendered total is strictly below the
	// produced total. Full-content reassembly is explicitly NOT
	// asserted: those bytes are gone.
	var finalSeg, total int64
	for i, r := range recs {
		total += int64(len(r.Chunk))
		if i >= lastGap {
			finalSeg += int64(len(r.Chunk))
		}
	}
	if finalSeg != retention {
		t.Fatalf("final retained segment rendered %d bytes, want the full window %d", finalSeg, retention)
	}
	if total >= written {
		t.Fatalf("rendered total %d >= written %d — the evicted prefix must be honestly absent, not reassembled", total, written)
	}
}

// tailText clips long streams to their tail for failure output.
func tailText(s string) string {
	const max = 2500
	if len(s) <= max {
		return s
	}
	return "…" + s[len(s)-max:]
}
