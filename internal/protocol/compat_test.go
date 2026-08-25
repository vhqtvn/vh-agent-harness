// compat_test.go — slice 5 step 6 (R5′ mitigation): byte-exact golden
// wire fixtures under testdata/. Every request/response/notification
// shape the protocol emits is locked here; changing or adding a shape
// without bumping ProtocolVersion (and regenerating fixtures, by
// design a visible diff) fails this test. External frontends
// (vh-solara-class consumers) depend on this stability — see
// tmp/solution-brief/02-solution-brief.md v3 risk register R5′.
//
// Fixtures are regenerated deliberately: UPDATE_FIXTURES=1 go test
// ./internal/protocol/ (review the diff like any wire change).
package protocol

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vhqtvn/vh-agent-harness/internal/jobs"
	"github.com/vhqtvn/vh-agent-harness/internal/session"
	"github.com/vhqtvn/vh-agent-harness/internal/subagents"
)

// fixtureCase is one locked wire line, built through the public API so
// the lock covers encode paths exactly as production uses them.
type fixtureCase struct {
	name string
	line func() ([]byte, error)
}

// listResultLock is the session/list result envelope (the handler's
// anonymous shape, named once for the fixture builder).
type listResultLock struct {
	Sessions []SessionEntry `json:"sessions"`
}

func fixtureCases() []fixtureCase {
	return []fixtureCase{
		{"request-initialize", func() ([]byte, error) {
			return MarshalRequest(1, "initialize", json.RawMessage(`{"protocolVersion":1}`))
		}},
		{"response-initialize", func() ([]byte, error) {
			return json.Marshal(struct {
				JSONRPC string           `json:"jsonrpc"`
				ID      int64            `json:"id"`
				Result  initializeResult `json:"result"`
			}{JSONRPCVersion, 1, initializeResult{
				ProtocolVersion: ProtocolVersion,
				ServerInfo:      serverInfo{Name: "vh-agent-harness"},
				Capabilities:    capabilities{Approval: true, Jobs: true, EventNotifications: true},
			}})
		}},
		{"response-error-mismatch", func() ([]byte, error) {
			data, _ := json.Marshal(struct {
				Server int `json:"server"`
				Client int `json:"client"`
			}{1, 2})
			return MarshalResponseError(1, ErrVersionMismatch, "protocol version mismatch", data)
		}},
		{"request-session_create", func() ([]byte, error) {
			return MarshalRequest(2, "session/create", json.RawMessage(`{"sessionId":"sess-example"}`))
		}},
		{"response-session_create", func() ([]byte, error) {
			return json.Marshal(struct {
				JSONRPC string       `json:"jsonrpc"`
				ID      int64        `json:"id"`
				Result  createResult `json:"result"`
			}{JSONRPCVersion, 2, createResult{SessionID: "sess-example", Path: "/tmp/sessions/sess-example.jsonl"}})
		}},
		{"request-session_resume", func() ([]byte, error) {
			return MarshalRequest(7, "session/resume", json.RawMessage(`{"sessionId":"sess-example"}`))
		}},
		{"response-session_resume", func() ([]byte, error) {
			return json.Marshal(struct {
				JSONRPC string        `json:"jsonrpc"`
				ID      int64         `json:"id"`
				Result  ResumeSummary `json:"result"`
			}{JSONRPCVersion, 7, ResumeSummary{
				SessionID: "sess-example",
				Path:      "/tmp/sessions/sess-example.jsonl",
				Events:    4,
				Messages: []session.Message{
					{Role: "user", Content: "continue this session"},
					{Role: "assistant", Content: "resumed"},
				},
				Title:         "continue this session",
				Usage:         session.Usage{PromptTokens: 12, CompletionTokens: 6, TotalTokens: 18},
				UnsettledJobs: []string{"background-2"},
			}})
		}},
		{"request-session_list", func() ([]byte, error) {
			return MarshalRequest(8, "session/list", nil)
		}},
		{"response-session_list", func() ([]byte, error) {
			return json.Marshal(struct {
				JSONRPC string         `json:"jsonrpc"`
				ID      int64          `json:"id"`
				Result  listResultLock `json:"result"`
			}{JSONRPCVersion, 8, listResultLock{Sessions: []SessionEntry{
				{SessionID: "sess-example", Title: "continue this session", Events: 4, LastActivity: time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)},
			}}})
		}},
		{"request-session_prompt", func() ([]byte, error) {
			return MarshalRequest(3, "session/prompt", json.RawMessage(`{"text":"summarize the repo"}`))
		}},
		{"response-session_prompt", func() ([]byte, error) {
			return json.Marshal(struct {
				JSONRPC string       `json:"jsonrpc"`
				ID      int64        `json:"id"`
				Result  promptResult `json:"result"`
			}{JSONRPCVersion, 3, promptResult{
				Content:   "done",
				ToolCalls: []promptToolCall{{ID: "call-1", Name: "read_file"}},
				Results:   []promptToolResult{{CallID: "call-1", Name: "read_file", Content: "42 files", IsError: false}},
			}})
		}},
		{"request-session_dispatch", func() ([]byte, error) {
			return MarshalRequest(4, "session/dispatch", json.RawMessage(`{"kind":"background","payload":{"n":1}}`))
		}},
		{"response-session_dispatch", func() ([]byte, error) {
			return json.Marshal(struct {
				JSONRPC string  `json:"jsonrpc"`
				ID      int64   `json:"id"`
				Result  Receipt `json:"result"`
			}{JSONRPCVersion, 4, Receipt{JobID: "background-1"}})
		}},
		{"request-session_subscribe", func() ([]byte, error) {
			return MarshalRequest(5, "session/subscribe", json.RawMessage(`{"types":["job/enqueued","job/started","job/settled"]}`))
		}},
		{"response-session_subscribe", func() ([]byte, error) {
			return json.Marshal(struct {
				JSONRPC string `json:"jsonrpc"`
				ID      int64  `json:"id"`
				Result  struct {
					Subscribed bool `json:"subscribed"`
				} `json:"result"`
			}{JSONRPCVersion, 5, struct {
				Subscribed bool `json:"subscribed"`
			}{true}})
		}},
		{"request-session_surface", func() ([]byte, error) {
			return MarshalRequest(6, "session/surface", nil)
		}},
		{"response-session_surface", func() ([]byte, error) {
			return json.Marshal(struct {
				JSONRPC string `json:"jsonrpc"`
				ID      int64  `json:"id"`
				Result  struct {
					Messages []session.Message `json:"messages"`
				} `json:"result"`
			}{JSONRPCVersion, 6, struct {
				Messages []session.Message `json:"messages"`
			}{[]session.Message{
				{Role: "user", Content: "run the guarded thing"},
				{Role: "assistant", Content: "", ToolCalls: []session.ToolCall{{ID: "call-1", Name: "guarded", Args: json.RawMessage(`{"x":1}`)}}},
				{Role: "tool", ToolCallID: "call-1", Name: "guarded", Content: "approved-ran"},
			}}})
		}},
		{"request-approval_respond", func() ([]byte, error) {
			return MarshalRequest(7, "approval/respond", json.RawMessage(`{"approvalId":"approval-1","allow":true}`))
		}},
		{"response-approval_respond", func() ([]byte, error) {
			return json.Marshal(struct {
				JSONRPC string `json:"jsonrpc"`
				ID      int64  `json:"id"`
				Result  struct {
					Resolved bool `json:"resolved"`
				} `json:"result"`
			}{JSONRPCVersion, 7, struct {
				Resolved bool `json:"resolved"`
			}{true}})
		}},
		{"request-jobs_status", func() ([]byte, error) {
			return MarshalRequest(8, "jobs/status", nil)
		}},
		{"response-jobs_status", func() ([]byte, error) {
			return json.Marshal(struct {
				JSONRPC string `json:"jsonrpc"`
				ID      int64  `json:"id"`
				Result  struct {
					Jobs []jobs.Status `json:"jobs"`
				} `json:"result"`
			}{JSONRPCVersion, 8, struct {
				Jobs []jobs.Status `json:"jobs"`
			}{[]jobs.Status{
				{JobID: "background-1", Kind: "background", State: "settled", Result: "completed"},
				{JobID: "ingest-2", Kind: "ingest", State: "running"},
			}}})
		}},
		{"request-subagent_spawn", func() ([]byte, error) {
			return MarshalRequest(9, "subagent/spawn", json.RawMessage(`{"role":"researcher","prompt":"study the repo","mode":"oneshot","seedFromParent":2}`))
		}},
		{"response-subagent_spawn", func() ([]byte, error) {
			return json.Marshal(struct {
				JSONRPC string              `json:"jsonrpc"`
				ID      int64               `json:"id"`
				Result  subagentSpawnResult `json:"result"`
			}{JSONRPCVersion, 9, subagentSpawnResult{ChildID: "sess-example.1"}})
		}},
		{"request-subagent_send", func() ([]byte, error) {
			return MarshalRequest(10, "subagent/send", json.RawMessage(`{"childId":"sess-example.1","message":"go deeper on the tests"}`))
		}},
		{"response-subagent_send", func() ([]byte, error) {
			return json.Marshal(struct {
				JSONRPC string             `json:"jsonrpc"`
				ID      int64              `json:"id"`
				Result  subagentSendResult `json:"result"`
			}{JSONRPCVersion, 10, subagentSendResult{Queued: true}})
		}},
		{"request-subagent_list", func() ([]byte, error) {
			return MarshalRequest(11, "subagent/list", nil)
		}},
		{"response-subagent_list", func() ([]byte, error) {
			children := []subagents.Status{
				{ChildID: "sess-example.1", Kind: "one-shot", Prompt: "study the repo", Depth: 1, State: "settled", SettledResult: "completed", ContentSeq: 7},
				{ChildID: "sess-example.2", Kind: "continuable", Depth: 1, State: "waiting"},
			}
			return json.Marshal(struct {
				JSONRPC string             `json:"jsonrpc"`
				ID      int64              `json:"id"`
				Result  subagentListResult `json:"result"`
			}{JSONRPCVersion, 11, subagentListResult{Children: children}})
		}},
		{"request-schedule_add", func() ([]byte, error) {
			return MarshalRequest(12, "schedule/add", json.RawMessage(`{"name":"nightly-digest","kind":"digest","at":"2026-08-21T00:00:00Z","every":3600000000000,"payload":{"job":"digest"}}`))
		}},
		{"response-schedule_add", func() ([]byte, error) {
			at := time.Date(2026, 8, 21, 0, 0, 0, 0, time.UTC)
			return json.Marshal(struct {
				JSONRPC string      `json:"jsonrpc"`
				ID      int64       `json:"id"`
				Result  scheduleDTO `json:"result"`
			}{JSONRPCVersion, 12, scheduleDTO{
				Name:    "nightly-digest",
				Kind:    "digest",
				At:      &at,
				Every:   int64(time.Hour),
				Payload: json.RawMessage(`{"job":"digest"}`),
				NextRun: at,
			}})
		}},
		{"request-schedule_list", func() ([]byte, error) {
			return MarshalRequest(13, "schedule/list", nil)
		}},
		{"response-schedule_list", func() ([]byte, error) {
			at := time.Date(2026, 8, 21, 0, 0, 0, 0, time.UTC)
			next := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)
			return json.Marshal(struct {
				JSONRPC string `json:"jsonrpc"`
				ID      int64  `json:"id"`
				Result  struct {
					Schedules []scheduleDTO `json:"schedules"`
				} `json:"result"`
			}{JSONRPCVersion, 13, struct {
				Schedules []scheduleDTO `json:"schedules"`
			}{[]scheduleDTO{{
				Name:    "nightly-digest",
				Kind:    "digest",
				At:      &at,
				Every:   int64(time.Hour),
				Payload: json.RawMessage(`{"job":"digest"}`),
				NextRun: next,
			}}}})
		}},
		{"request-schedule_remove", func() ([]byte, error) {
			return MarshalRequest(14, "schedule/remove", json.RawMessage(`{"name":"nightly-digest"}`))
		}},
		{"response-schedule_remove", func() ([]byte, error) {
			return json.Marshal(struct {
				JSONRPC string `json:"jsonrpc"`
				ID      int64  `json:"id"`
				Result  struct {
					Removed bool `json:"removed"`
				} `json:"result"`
			}{JSONRPCVersion, 14, struct {
				Removed bool `json:"removed"`
			}{true}})
		}},
		{"notification-session_event", func() ([]byte, error) {
			payload, _ := json.Marshal(session.JobPayload{
				JobID: "background-1", Kind: "background", Owner: "sess-example", Result: "completed",
			})
			ev := session.Event{Seq: 9, Type: "job/settled", Payload: payload}
			params, _ := json.Marshal(ev)
			return MarshalNotification("session/event", params)
		}},
		{"notification-approval_request", func() ([]byte, error) {
			params, _ := json.Marshal(approvalRequestParams{
				ApprovalID: "approval-1",
				Call:       session.ToolCall{ID: "call-1", Name: "write_file", Args: json.RawMessage(`{"path":"a.txt"}`)},
				Reason:     "mutates workspace",
			})
			return MarshalNotification("approval/request", params)
		}},
		{"notification-protocol_error", func() ([]byte, error) {
			params, _ := json.Marshal(struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			}{ErrParse, "malformed line: invalid character 'o' in literal null"})
			return MarshalNotification("protocol/error", params)
		}},
	}
}

func TestCompatFixtures(t *testing.T) {
	update := os.Getenv("UPDATE_FIXTURES") == "1"
	for _, tc := range fixtureCases() {
		t.Run(tc.name, func(t *testing.T) {
			want, err := tc.line()
			if err != nil {
				t.Fatalf("construct: %v", err)
			}
			path := filepath.Join("testdata", tc.name+".json")
			if update {
				if werr := os.WriteFile(path, append(want, '\n'), 0o644); werr != nil {
					t.Fatalf("update fixture: %v", werr)
				}
				return
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("fixture missing (run UPDATE_FIXTURES=1 to bootstrap): %v", err)
			}
			if got := strings.TrimSuffix(string(raw), "\n"); got != string(want) {
				t.Fatalf("wire drift in %s:\n got %s\nwant %s\nBump ProtocolVersion (and review) for intentional shape changes.",
					tc.name, got, want)
			}
			// The locked line must still be a grammatical message.
			if _, err := ParseLine(raw); err != nil {
				t.Fatalf("fixture is not a valid wire line: %v", err)
			}
		})
	}
}

// TestFixtureCoverageLock guards the OTHER direction: every method in
// the closed method table must have request AND response fixtures —
// adding a method without locking its shape fails here (R5′).
func TestFixtureCoverageLock(t *testing.T) {
	have := map[string]bool{}
	for _, tc := range fixtureCases() {
		have[tc.name] = true
	}
	for method := range handlers {
		req := "request-" + strings.ReplaceAll(method, "/", "_")
		resp := "response-" + strings.ReplaceAll(method, "/", "_")
		if !have[req] {
			t.Errorf("method %s has no %s.json fixture (lock the wire shape or bump ProtocolVersion)", method, req)
		}
		if !have[resp] {
			t.Errorf("method %s has no %s.json fixture (lock the wire shape or bump ProtocolVersion)", method, resp)
		}
	}
	// Event-notification kinds are locked too.
	for _, note := range []string{"session/event", "approval/request", "protocol/error"} {
		name := "notification-" + strings.ReplaceAll(note, "/", "_")
		if !have[name] {
			t.Errorf("notification %s has no %s.json fixture", note, name)
		}
	}
	// No orphan fixtures: every testdata file must correspond to a case.
	cases, err := os.ReadDir("testdata")
	if err != nil {
		t.Fatalf("read testdata: %v", err)
	}
	for _, c := range cases {
		name := strings.TrimSuffix(c.Name(), ".json")
		if !have[name] {
			t.Errorf("orphan fixture %s (no fixture case produces it)", c.Name())
		}
	}
}

// TestProtocolVersionPinnedToFixtures makes the bump ritual explicit:
// the initialize fixtures carry the version in clear text; a constant
// change without regenerating fixtures (and reviewing the diff) cannot
// pass silently.
func TestProtocolVersionPinnedToFixtures(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "response-initialize.json"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	msg, err := ParseLine(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var res initializeResult
	if err := json.Unmarshal(msg.Response.Result, &res); err != nil {
		t.Fatalf("result: %v", err)
	}
	if res.ProtocolVersion != ProtocolVersion {
		t.Fatalf("fixture pins protocolVersion=%d but ProtocolVersion=%d (regenerate fixtures when bumping)", res.ProtocolVersion, ProtocolVersion)
	}
}
