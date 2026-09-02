package session

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// UnknownEventError is returned by the fail-closed replay reader when a
// log record carries an event type outside the known set without the
// ignorable marker. Replay refuses reconstruction rather than skipping.
type UnknownEventError struct {
	Seq  int64
	Type string
}

func (e *UnknownEventError) Error() string {
	return fmt.Sprintf("session: unknown event type %q at seq %d without ignorable marker: refusing reconstruction", e.Type, e.Seq)
}

// maxRecordBytes caps one JSONL record during replay (oversized tool
// results are a later spill concern; slice 1 fails closed beyond the cap).
const maxRecordBytes = 32 << 20

// Replay reads a JSONL session log and returns its events, enforcing the
// slice-1 fail-closed contract:
//   - every record must be valid JSON;
//   - an unknown event type without ignorable:true is an UnknownEventError;
//   - the first record must be session/header;
//   - the header format version must equal SESSION_FORMAT_VERSION;
//   - seq must be contiguous and 1-based;
//   - session/header may appear only as the first record.
func Replay(r io.Reader) ([]Event, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), maxRecordBytes)
	var events []Event
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var ev Event
		if err := json.Unmarshal(line, &ev); err != nil {
			return nil, fmt.Errorf("session: malformed record %d: %w", len(events)+1, err)
		}
		if !knownTypes[ev.Type] && !ev.Ignorable {
			return nil, &UnknownEventError{Seq: ev.Seq, Type: ev.Type}
		}
		events = append(events, ev)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("session: read log: %w", err)
	}
	if len(events) == 0 {
		return events, nil
	}
	if err := validateStructure(events); err != nil {
		return nil, err
	}
	return events, nil
}

// ReplayFile replays the JSONL session log at path.
func ReplayFile(path string) ([]Event, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("session: open %s: %w", path, err)
	}
	defer f.Close()
	return Replay(f)
}

// RecoverTail is Replay with crash tolerance for the FINAL record: if the
// log ends with a non-blank tail that is not newline-terminated, that
// tail is a torn write from a crashed process and is dropped. The writer
// commits each record as ONE write of marshaled-JSON + '\n', so a proper
// prefix of a committed record is never newline-terminated — the missing
// newline IS the torn signature. validBytes is the byte offset of the end
// of the last committed record (the truncation point for resuming the
// file); torn reports whether a tail was dropped. A malformed
// newline-terminated record anywhere, or any other structural violation,
// still fails closed exactly like Replay: torn-tail tolerance applies
// only to the uncommitted final fragment.
func RecoverTail(r io.Reader) ([]Event, int64, bool, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, 0, false, fmt.Errorf("session: read log: %w", err)
	}
	var events []Event
	var valid int64
	for len(data) > 0 {
		i := bytes.IndexByte(data, '\n')
		if i < 0 {
			break // uncommitted tail
		}
		line := bytes.TrimSpace(data[:i])
		if len(line) > 0 {
			ev, err := parseRecord(line)
			if err != nil {
				return nil, 0, false, fmt.Errorf("session: malformed record %d: %w", len(events)+1, err)
			}
			if !knownTypes[ev.Type] && !ev.Ignorable {
				return nil, 0, false, &UnknownEventError{Seq: ev.Seq, Type: ev.Type}
			}
			events = append(events, ev)
		}
		valid += int64(i) + 1
		data = data[i+1:]
	}
	torn := len(bytes.TrimSpace(data)) > 0
	if len(events) > 0 {
		if err := validateStructure(events); err != nil {
			return nil, 0, false, err
		}
	}
	return events, valid, torn, nil
}

// parseRecord decodes one JSONL record.
func parseRecord(line []byte) (Event, error) {
	var ev Event
	if err := json.Unmarshal(line, &ev); err != nil {
		return Event{}, err
	}
	return ev, nil
}

// validateStructure enforces the header/version/seq invariants over a
// non-empty event list.
func validateStructure(events []Event) error {
	if events[0].Type != TypeSessionHeader {
		return fmt.Errorf("session: first record must be %s, got %q", TypeSessionHeader, events[0].Type)
	}
	var hp HeaderPayload
	if err := json.Unmarshal(events[0].Payload, &hp); err != nil {
		return fmt.Errorf("session: malformed header payload: %w", err)
	}
	if hp.FormatVersion != SESSION_FORMAT_VERSION {
		return fmt.Errorf("session: unsupported format version %d (want %d)", hp.FormatVersion, SESSION_FORMAT_VERSION)
	}
	for i, ev := range events {
		if ev.Seq != int64(i+1) {
			return fmt.Errorf("session: record %d has non-contiguous seq %d", i+1, ev.Seq)
		}
		if i > 0 && ev.Type == TypeSessionHeader {
			return fmt.Errorf("session: %s may appear only as the first record (seq %d)", TypeSessionHeader, ev.Seq)
		}
	}
	return nil
}
