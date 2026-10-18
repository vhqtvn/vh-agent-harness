// approval_test.go — the approval responders. Fail-closed everywhere:
// EOF, ENTER alone, and anything that is not an explicit y/yes deny.
// The --json responder parses {"id":..,"approve":bool} lines; malformed
// input is rejected honestly (never applied to an approval) and the
// approval then denies at EOF. These tests drive the RESPONDER seam
// through the single-owner stdin hub (input.go) — the grant/deny pair
// over a REAL connection lives at the library-server seam in
// driver_test.go / concurrent_approval_test.go.
package main

import (
	"bufio"
	"bytes"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/vhqtvn/vh-agent-harness/internal/session"
)

func interactiveWith(input string) (ApproverFunc, *bytes.Buffer) {
	var errbuf bytes.Buffer
	hub := newStdinHub(bufio.NewReader(strings.NewReader(input)), &errbuf, false)
	hub.start()
	return interactiveApprover(hub, &errbuf), &errbuf
}

func jsonWith(input string) (ApproverFunc, *bytes.Buffer) {
	var errbuf bytes.Buffer
	hub := newStdinHub(bufio.NewReader(strings.NewReader(input)), &errbuf, true)
	hub.start()
	return jsonApprover(hub), &errbuf
}

func TestInteractiveYGrants(t *testing.T) {
	approve, errbuf := interactiveWith("y\n")
	ans := approve("approval-1", session.ToolCall{ID: "c1", Name: "write_file"}, "mutates workspace")
	if !ans.Allow {
		t.Fatalf("y must grant, got %+v (prompt was %q)", ans, errbuf.String())
	}
	if !strings.Contains(errbuf.String(), "write_file") || !strings.Contains(errbuf.String(), "mutates workspace") {
		t.Fatalf("prompt must name the tool and reason, got %q", errbuf.String())
	}
	if !strings.Contains(errbuf.String(), "[y/N]") {
		t.Fatalf("prompt must be a y/N prompt, got %q", errbuf.String())
	}
}

func TestInteractiveYesCaseInsensitiveGrants(t *testing.T) {
	for _, in := range []string{"Y\n", "yes\n", "YES\n", " y \n"} {
		approve, _ := interactiveWith(in)
		if ans := approve("a", session.ToolCall{ID: "c", Name: "n"}, "r"); !ans.Allow {
			t.Fatalf("input %q must grant, got %+v", in, ans)
		}
	}
}

func TestInteractiveNDenies(t *testing.T) {
	approve, _ := interactiveWith("n\n")
	ans := approve("a", session.ToolCall{ID: "c", Name: "n"}, "r")
	if ans.Allow {
		t.Fatal("n must deny")
	}
	if ans.Reason == "" {
		t.Fatal("deny must carry an honest reason")
	}
}

func TestInteractiveEnterAloneDeniesFailClosed(t *testing.T) {
	approve, _ := interactiveWith("\n")
	ans := approve("a", session.ToolCall{ID: "c", Name: "n"}, "r")
	if ans.Allow {
		t.Fatal("ENTER alone must deny (default n — fail-closed)")
	}
}

func TestInteractiveEOFDeniesFailClosed(t *testing.T) {
	approve, _ := interactiveWith("")
	ans := approve("a", session.ToolCall{ID: "c", Name: "n"}, "r")
	if ans.Allow {
		t.Fatal("EOF must deny (fail-closed)")
	}
	if ans.Reason == "" {
		t.Fatal("EOF deny must carry a reason")
	}
}

func TestInteractiveGarbageDenies(t *testing.T) {
	approve, _ := interactiveWith("probably not\n")
	if ans := approve("a", session.ToolCall{ID: "c", Name: "n"}, "r"); ans.Allow {
		t.Fatal("non-affirmative input must deny")
	}
}

func TestJSONResponderGrantsAndDenies(t *testing.T) {
	approve, _ := jsonWith("{\"id\":\"approval-1\",\"approve\":true}\n{\"id\":\"approval-2\",\"approve\":false}\n")
	if ans := approve("approval-1", session.ToolCall{ID: "c", Name: "n"}, "r"); !ans.Allow {
		t.Fatalf("approve:true must grant, got %+v", ans)
	}
	if ans := approve("approval-2", session.ToolCall{ID: "c", Name: "n"}, "r"); ans.Allow {
		t.Fatalf("approve:false must deny, got %+v", ans)
	}
}

func TestJSONResponderMalformedLineRejectedNeverApplied(t *testing.T) {
	// A malformed line is UNATTRIBUTABLE: it is rejected with an
	// honest stderr line and applied to NO approval (hotfix b-F1: an
	// answer must never land on the wrong approvalId); the pending
	// approval then denies fail-closed at EOF.
	approve, errbuf := jsonWith("this is not json\n")
	ans := approve("a", session.ToolCall{ID: "c", Name: "n"}, "r")
	if ans.Allow {
		t.Fatal("malformed response line must not grant (EOF deny, fail-closed)")
	}
	if !strings.Contains(errbuf.String(), "not json") && !strings.Contains(errbuf.String(), "malformed") {
		t.Fatalf("reject should explain the malformed line, got %q", errbuf.String())
	}
}

func TestJSONResponderEOFDenies(t *testing.T) {
	approve, _ := jsonWith("")
	ans := approve("a", session.ToolCall{ID: "c", Name: "n"}, "r")
	if ans.Allow || ans.Reason == "" {
		t.Fatalf("EOF must deny with a reason, got %+v", ans)
	}
}

func TestJSONResponderIgnoresIDMismatchDeniesByDefault(t *testing.T) {
	// A response line for a DIFFERENT id must not grant the current
	// one: it is parked for that id (never applied here), and the
	// current approval denies at EOF (fail-closed, no cross-talk).
	approve, _ := jsonWith("{\"id\":\"approval-other\",\"approve\":true}\n")
	ans := approve("approval-1", session.ToolCall{ID: "c", Name: "n"}, "r")
	if ans.Allow {
		t.Fatal("a response for another id must not grant this approval")
	}
}

func TestJSONResponderSettledIDIsNeverReApplied(t *testing.T) {
	// b-F1 regression (re-application): after an id settles, a
	// duplicate answer for it is rejected honestly and grants
	// nothing; a pre-scripted answer for a LATER id parks and
	// delivers exactly once when that id asks.
	var errbuf bytes.Buffer
	pr, pw := io.Pipe()
	hub := newStdinHub(bufio.NewReader(pr), &errbuf, true)
	hub.start()
	defer pr.Close()
	approve := jsonApprover(hub)

	res := make(chan ApprovalAnswer, 1)
	go func() { res <- approve("a1", session.ToolCall{ID: "c", Name: "n"}, "r") }()
	waitFor(t, 2*time.Second, func() bool { return hub.isWaiting("a1") })
	if _, err := pw.Write([]byte("{\"id\":\"a1\",\"approve\":true}\n")); err != nil {
		t.Fatalf("write a1 answer: %v", err)
	}
	if ans := <-res; !ans.Allow {
		t.Fatalf("first answer for a1 must grant, got %+v", ans)
	}
	waitFor(t, 2*time.Second, func() bool { return hub.isSettled("a1") })

	// Duplicate for the settled id: rejected, applied to no one.
	if _, err := pw.Write([]byte("{\"id\":\"a1\",\"approve\":true}\n")); err != nil {
		t.Fatalf("write duplicate: %v", err)
	}
	// Answer for a not-yet-asked id: parks.
	if _, err := pw.Write([]byte("{\"id\":\"a2\",\"approve\":true}\n")); err != nil {
		t.Fatalf("write a2 answer: %v", err)
	}
	_ = pw.Close() // EOF: a2's parked answer survives

	if ans := approve("a2", session.ToolCall{ID: "c", Name: "n"}, "r"); !ans.Allow {
		t.Fatalf("parked answer for a2 must deliver on ask (after EOF), got %+v (stderr:\n%s)", ans, errbuf.String())
	}
	if !strings.Contains(errbuf.String(), "already-settled id") {
		t.Fatalf("duplicate settled answer must be rejected honestly:\n%s", errbuf.String())
	}
}

// waitFor polls cond until true or the deadline fails the test.
func waitFor(t *testing.T, d time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("condition not met within %s", d)
}

func TestInteractiveConcurrentAskFIFORoutesByPromptOrder(t *testing.T) {
	// b-F1 regression (interactive): two concurrently-pending approvals
	// get lines routed in prompt order — the first granted line answers
	// the approval that prompted first, never its sibling.
	var errbuf bytes.Buffer
	hub := newStdinHub(bufio.NewReader(strings.NewReader("y\nn\n")), &errbuf, false)
	hub.start()
	approve := interactiveApprover(hub, &errbuf)
	a1 := make(chan ApprovalAnswer, 1)
	a2 := make(chan ApprovalAnswer, 1)
	go func() { a1 <- approve("approval-1", session.ToolCall{ID: "c1", Name: "t1"}, "r") }()
	// Ensure approval-1's prompt+registration land first (the property
	// under test is ordering, not goroutine racing).
	time.Sleep(20 * time.Millisecond)
	go func() { a2 <- approve("approval-2", session.ToolCall{ID: "c2", Name: "t2"}, "r") }()
	if ans := <-a1; !ans.Allow {
		t.Fatalf("first y must answer the FIRST prompt (approval-1), got %+v (stderr:\n%s)", ans, errbuf.String())
	}
	if ans := <-a2; ans.Allow {
		t.Fatalf("n must answer the second prompt (approval-2), got %+v", ans)
	}
}
