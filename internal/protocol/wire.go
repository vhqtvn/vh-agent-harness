// wire.go — the host-protocol wire codec: versioned JSON-RPC over
// newline-delimited JSON (NDJSON) on stdio.
//
// Grammar (dsh SDK transport pattern, see
// researches/sources/deepseek-harness/llm-protocols-tools.md §2.8 and
// F-SDK-1; tmp/solution-brief/02-solution-brief.md v3 Architecture Map
// "Frontend" row — JSON-RPC/NDJSON over stdio first):
//
//	{"jsonrpc":"2.0","id":N,"method":"...","params":{...}}   request
//	{"jsonrpc":"2.0","id":N,"result":{...}}                  response (result)
//	{"jsonrpc":"2.0","id":N,"error":{"code":C,"message":"..."}}  response (error)
//	{"jsonrpc":"2.0","method":"...","params":{...}}          notification
//
// Inbound strictness is fail-closed (the engine never guesses):
//   - a line that is not a JSON object is MALFORMED — the server skips it
//     and emits a protocol/error notification (dsh malformed-line skip);
//   - an envelope field outside the grammar, a non-"2.0" jsonrpc value,
//     a non-integer or negative id, or a contradictory shape (id+method
//     carrying result/error, etc.) is INVALID — attributable (id present
//     and readable) messages get a -32600 error response; the rest are
//     skipped with a protocol/error notification;
//   - ids are integers >= 0 in v1 (the sentinel -1 marks an unreadable
//     id internally).
package protocol

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// JSONRPCVersion is the framing dialect of every message on the wire.
const JSONRPCVersion = "2.0"

// Kind classifies one decoded wire line.
type Kind string

const (
	KindRequest      Kind = "request"
	KindResponse     Kind = "response"
	KindNotification Kind = "notification"
	KindInvalid      Kind = "invalid"
)

// Request is an id+method message (expects a response).
type Request struct {
	ID     int64           `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

// Response is an id-only message carrying exactly one of result or error.
type Response struct {
	ID     int64           `json:"id"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *Error          `json:"error,omitempty"`
}

// Notification is a method-only message (no response expected).
type Notification struct {
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

// Message is the decoded union of one wire line. Exactly the struct
// selected by Kind is populated; an invalid message keeps the readable id
// (InvalidID, with HasID) so the server can attribute its rejection.
type Message struct {
	Kind          Kind
	Request       *Request
	Response      *Response
	Notification  *Notification
	HasID         bool
	InvalidID     int64
	InvalidReason string
}

// Error is the JSON-RPC error envelope carried by error responses. See
// docs/native-engine/host-protocol.md for the closed code table.
type Error struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *Error) Error() string {
	if len(e.Data) > 0 {
		return fmt.Sprintf("protocol error %d: %s (%s)", e.Code, e.Message, e.Data)
	}
	return fmt.Sprintf("protocol error %d: %s", e.Code, e.Message)
}

// rawLine is the permissive intermediate decode target: every value is
// kept raw so shape violations can be reported precisely (attributable)
// instead of collapsing into a parse error.
type rawLine struct {
	JSONRPC json.RawMessage `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  json.RawMessage `json:"method"`
	Params  json.RawMessage `json:"params"`
	Result  json.RawMessage `json:"result"`
	Error   json.RawMessage `json:"error"`
}

// envelopeKeys is the closed key set of the wire grammar.
var envelopeKeys = map[string]bool{
	"jsonrpc": true, "id": true, "method": true,
	"params": true, "result": true, "error": true,
}

// ParseLine decodes and classifies one NDJSON line. An error return means
// the line is MALFORMED (not a JSON object / unreadable): the caller skips
// it and emits a protocol/error notification. A KindInvalid message is a
// well-formed JSON object that violates the grammar; the caller answers it
// with -32600 when it carries a readable id.
func ParseLine(line []byte) (*Message, error) {
	trimmed := bytes.TrimSpace(line)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("empty line")
	}
	dec := json.NewDecoder(bytes.NewReader(trimmed))
	var obj map[string]json.RawMessage
	if err := dec.Decode(&obj); err != nil {
		return nil, fmt.Errorf("malformed line: %w", err)
	}
	// Exactly one JSON value per line (no trailing content except
	// whitespace).
	if dec.More() {
		return nil, fmt.Errorf("malformed line: trailing content after JSON value")
	}
	if trimmed[0] != '{' || obj == nil {
		return nil, fmt.Errorf("malformed line: not a JSON object")
	}

	// Fail-closed envelope strictness: any key outside the grammar
	// rejects the whole message (dsh-style strict Host contracts; the
	// engine never guesses about extra fields).
	msg := &Message{Kind: KindInvalid}
	for k := range obj {
		if !envelopeKeys[k] {
			msg.InvalidReason = fmt.Sprintf("unknown envelope field %q", k)
			msg.InvalidID, msg.HasID = rawID(obj["id"])
			return msg, nil
		}
	}
	var raw rawLine
	raw.JSONRPC, raw.ID, raw.Method = obj["jsonrpc"], obj["id"], obj["method"]
	raw.Params, raw.Result, raw.Error = obj["params"], obj["result"], obj["error"]
	if len(raw.JSONRPC) > 0 {
		var v string
		if err := json.Unmarshal(raw.JSONRPC, &v); err != nil || v != JSONRPCVersion {
			msg.InvalidReason = fmt.Sprintf("jsonrpc must be %q", JSONRPCVersion)
			msg.InvalidID, msg.HasID = rawID(raw.ID)
			return msg, nil
		}
	} else {
		msg.InvalidReason = fmt.Sprintf("jsonrpc field is required (%q)", JSONRPCVersion)
		msg.InvalidID, msg.HasID = rawID(raw.ID)
		return msg, nil
	}

	hasID := len(raw.ID) > 0
	hasMethod := len(raw.Method) > 0
	hasResult := len(raw.Result) > 0
	hasError := len(raw.Error) > 0

	switch {
	case hasID && hasMethod && !hasResult && !hasError:
		var id int64
		if err := json.Unmarshal(raw.ID, &id); err != nil || id < 0 {
			msg.InvalidReason = "id must be an integer >= 0"
			msg.InvalidID, msg.HasID = rawID(raw.ID)
			return msg, nil
		}
		var method string
		if err := json.Unmarshal(raw.Method, &method); err != nil || method == "" {
			msg.InvalidReason = "method must be a non-empty string"
			msg.InvalidID, msg.HasID = id, true
			return msg, nil
		}
		msg.Kind = KindRequest
		msg.Request = &Request{ID: id, Method: method, Params: raw.Params}
	case hasID && !hasMethod && (hasResult != hasError):
		var id int64
		if err := json.Unmarshal(raw.ID, &id); err != nil || id < 0 {
			msg.InvalidReason = "id must be an integer >= 0"
			msg.InvalidID, msg.HasID = rawID(raw.ID)
			return msg, nil
		}
		if hasError {
			var e Error
			if err := json.Unmarshal(raw.Error, &e); err != nil || e.Message == "" {
				msg.InvalidReason = "error must carry code and message"
				msg.InvalidID, msg.HasID = id, true
				return msg, nil
			}
			msg.Kind = KindResponse
			msg.Response = &Response{ID: id, Error: &e}
		} else {
			msg.Kind = KindResponse
			msg.Response = &Response{ID: id, Result: raw.Result}
		}
	case !hasID && hasMethod && !hasResult && !hasError:
		var method string
		if err := json.Unmarshal(raw.Method, &method); err != nil || method == "" {
			msg.InvalidReason = "method must be a non-empty string"
			return msg, nil
		}
		msg.Kind = KindNotification
		msg.Notification = &Notification{Method: method, Params: raw.Params}
	default:
		msg.InvalidReason = contradictionReason(hasID, hasMethod, hasResult, hasError)
		msg.InvalidID, msg.HasID = rawID(raw.ID)
	}
	return msg, nil
}

// contradictionReason names the grammar violation of an unclassifiable
// shape (request carrying a result, result+error together, bare jsonrpc,
// ...).
func contradictionReason(hasID, hasMethod, hasResult, hasError bool) string {
	switch {
	case hasID && hasMethod:
		return "a request must not carry result or error"
	case hasID && hasResult && hasError:
		return "a response must carry exactly one of result or error"
	case hasID:
		return "an id-only message must carry exactly one of result or error"
	case hasMethod && (hasResult || hasError):
		return "a notification must not carry result or error"
	default:
		return "a message needs an id, a method, or both"
	}
}

// rawID extracts the integer id of an invalid message when it is still
// readable, for attribution. Negative or non-integer ids are not
// attributable.
func rawID(raw json.RawMessage) (int64, bool) {
	if len(raw) == 0 {
		return -1, false
	}
	var id int64
	if err := json.Unmarshal(raw, &id); err != nil || id < 0 {
		return -1, false
	}
	return id, true
}

// MarshalRequest encodes one request line (no trailing newline).
func MarshalRequest(id int64, method string, params json.RawMessage) ([]byte, error) {
	return json.Marshal(struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      int64           `json:"id"`
		Method  string          `json:"method"`
		Params  json.RawMessage `json:"params,omitempty"`
	}{JSONRPCVersion, id, method, params})
}

// MarshalResponse encodes a successful response line (no trailing
// newline).
func MarshalResponse(id int64, result json.RawMessage) ([]byte, error) {
	return json.Marshal(struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      int64           `json:"id"`
		Result  json.RawMessage `json:"result,omitempty"`
	}{JSONRPCVersion, id, result})
}

// MarshalResponseError encodes an error response line (no trailing
// newline). data may be nil.
func MarshalResponseError(id int64, code int, message string, data json.RawMessage) ([]byte, error) {
	return json.Marshal(struct {
		JSONRPC string `json:"jsonrpc"`
		ID      int64  `json:"id"`
		Error   *Error `json:"error"`
	}{JSONRPCVersion, id, &Error{Code: code, Message: message, Data: data}})
}

// MarshalNotification encodes one notification line (no trailing
// newline).
func MarshalNotification(method string, params json.RawMessage) ([]byte, error) {
	return json.Marshal(struct {
		JSONRPC string          `json:"jsonrpc"`
		Method  string          `json:"method"`
		Params  json.RawMessage `json:"params,omitempty"`
	}{JSONRPCVersion, method, params})
}
