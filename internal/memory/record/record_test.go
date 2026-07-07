package record

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// strPtr is a small helper for building optional pointer fields in tests.
func strPtr(s string) *string { return &s }

// sampleRecord returns a valid Record of the given Type, with stable fields
// so round-trip comparisons are deterministic.
func sampleRecord(t Type) Record {
	now := time.Date(2026, 7, 7, 12, 0, 0, 0, time.UTC)
	return Record{
		ID:         "01HTEST0000000000000000001",
		Type:       t,
		Priority:   PriorityNormal,
		Scope:      ScopeSession,
		Scene:      strPtr("slice-1-2"),
		Workstream: strPtr("typed-memory-layer"),
		SourceRef:  strPtr("docs/checkpoints/2026-07-07-memory.md"),
		SessionKey: strPtr("build-memory"),
		CreatedAt:  now,
		UpdatedAt:  now,
		Body:       "memory atom body for " + string(t),
	}
}

// TestRecordJSONRoundTripPerType verifies that a representative record of each
// Type survives an encode → decode round trip with all fields intact,
// including optional pointer fields.
func TestRecordJSONRoundTripPerType(t *testing.T) {
	for _, tc := range []struct {
		name string
		typ  Type
	}{
		{"persona", TypePersona},
		{"episodic", TypeEpisodic},
		{"instruction", TypeInstruction},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := sampleRecord(tc.typ)
			data, err := json.Marshal(in)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			var out Record
			if err := json.Unmarshal(data, &out); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if !recordsEqual(in, out) {
				t.Fatalf("round trip mismatch\nin:  %+v\nout: %+v", in, out)
			}
		})
	}
}

// TestRecordJSONFieldTags asserts the exact snake_case JSON keys the locked
// contract requires. If a tag is accidentally renamed, downstream JSONL files
// become unreadable by other consumers.
func TestRecordJSONFieldTags(t *testing.T) {
	rec := sampleRecord(TypeEpisodic)
	data, err := json.Marshal(rec)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	wantKeys := []string{
		`"id"`, `"type"`, `"priority"`, `"scope"`, `"scene"`,
		`"workstream"`, `"source_ref"`, `"session_key"`,
		`"created_at"`, `"updated_at"`, `"body"`,
	}
	got := string(data)
	for _, k := range wantKeys {
		if !strings.Contains(got, k) {
			t.Errorf("missing JSON key %s in %s", k, got)
		}
	}
}

// TestRecordOmitsNilOptionalFields verifies that nil optional pointers are
// dropped via omitempty rather than serialized as null — keeping JSONL lines
// compact and forward-compatible.
func TestRecordOmitsNilOptionalFields(t *testing.T) {
	rec := Record{
		ID:        "x",
		Type:      TypePersona,
		Priority:  PriorityLow,
		Scope:     ScopeSession,
		CreatedAt: time.Date(2026, 7, 7, 0, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 7, 7, 0, 0, 0, 0, time.UTC),
		Body:      "no optionals",
	}
	data, err := json.Marshal(rec)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, banned := range []string{`"scene":`, `"workstream":`, `"source_ref":`, `"session_key":`} {
		if strings.Contains(string(data), banned) {
			t.Errorf("expected nil-optional to be omitted, but %s appears in %s", banned, data)
		}
	}
}

// TestRecordRejectsUnknownEnumOnDecode asserts that unknown Type, Priority, or
// Scope values are rejected when decoding — data corruption is surfaced
// rather than silently coerced.
func TestRecordRejectsUnknownEnumOnDecode(t *testing.T) {
	cases := []struct {
		name string
		json string
	}{
		{
			name: "unknown type",
			json: `{"id":"x","type":"bogus","priority":"normal","scope":"session","created_at":"2026-07-07T00:00:00Z","updated_at":"2026-07-07T00:00:00Z","body":"b"}`,
		},
		{
			name: "unknown priority",
			json: `{"id":"x","type":"persona","priority":"urgent","scope":"session","created_at":"2026-07-07T00:00:00Z","updated_at":"2026-07-07T00:00:00Z","body":"b"}`,
		},
		{
			name: "unknown scope",
			json: `{"id":"x","type":"persona","priority":"normal","scope":"global","created_at":"2026-07-07T00:00:00Z","updated_at":"2026-07-07T00:00:00Z","body":"b"}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var rec Record
			if err := json.Unmarshal([]byte(tc.json), &rec); err == nil {
				t.Fatalf("expected decode error, got nil; record=%+v", rec)
			}
		})
	}
}

// TestRecordAcceptsUnknownFieldsForForwardCompat verifies that a JSON object
// carrying an unknown (future) field decodes cleanly — the schema is
// forward-compatible with additive optional fields.
func TestRecordAcceptsUnknownFieldsForForwardCompat(t *testing.T) {
	raw := `{"id":"x","type":"persona","priority":"normal","scope":"session","created_at":"2026-07-07T00:00:00Z","updated_at":"2026-07-07T00:00:00Z","body":"b","future_field":{"nested":[1,2,3]},"tags":["a","b"]}`
	var rec Record
	if err := json.Unmarshal([]byte(raw), &rec); err != nil {
		t.Fatalf("expected forward-compat decode, got error: %v", err)
	}
	if rec.ID != "x" || rec.Body != "b" {
		t.Fatalf("decoded fields wrong: %+v", rec)
	}
}

// TestRecordValidate covers Validate's required-field checks. These mirror
// what the store layer relies on before writing any line.
func TestRecordValidate(t *testing.T) {
	good := sampleRecord(TypeInstruction)
	if err := good.Validate(); err != nil {
		t.Fatalf("good record should validate: %v", err)
	}

	cases := []struct {
		name string
		mut  func(Record) Record
	}{
		{"missing id", func(r Record) Record { r.ID = ""; return r }},
		{"missing body", func(r Record) Record { r.Body = ""; return r }},
		{"zero created_at", func(r Record) Record { r.CreatedAt = time.Time{}; return r }},
		{"zero updated_at", func(r Record) Record { r.UpdatedAt = time.Time{}; return r }},
		{"bad type", func(r Record) Record { r.Type = "nope"; return r }},
		{"bad priority", func(r Record) Record { r.Priority = "nope"; return r }},
		{"bad scope", func(r Record) Record { r.Scope = "nope"; return r }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.mut(good).Validate(); err == nil {
				t.Fatalf("expected validation error")
			}
		})
	}
}

// TestPriorityRank orders the tiers so the store sort can rely on Rank()
// without re-implementing the ordering.
func TestPriorityRank(t *testing.T) {
	if PriorityCritical.Rank() <= PriorityHigh.Rank() {
		t.Error("critical must outrank high")
	}
	if PriorityHigh.Rank() <= PriorityNormal.Rank() {
		t.Error("high must outrank normal")
	}
	if PriorityNormal.Rank() <= PriorityLow.Rank() {
		t.Error("normal must outrank low")
	}
	if (Priority("bogus")).Rank() >= PriorityLow.Rank() {
		t.Error("unknown priority must rank below low")
	}
}

// recordsEqual is a deep-enough equality check for round-trip tests. Pointer
// fields are compared by dereferenced value; zero pointers on one side and
// nil on the other are treated as unequal to catch real divergences.
func recordsEqual(a, b Record) bool {
	if a.ID != b.ID || a.Type != b.Type || a.Priority != b.Priority || a.Scope != b.Scope {
		return false
	}
	if a.CreatedAt != b.CreatedAt || a.UpdatedAt != b.UpdatedAt {
		return false
	}
	if a.Body != b.Body {
		return false
	}
	if !strPtrEq(a.Scene, b.Scene) || !strPtrEq(a.Workstream, b.Workstream) ||
		!strPtrEq(a.SourceRef, b.SourceRef) || !strPtrEq(a.SessionKey, b.SessionKey) {
		return false
	}
	return true
}

func strPtrEq(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}
