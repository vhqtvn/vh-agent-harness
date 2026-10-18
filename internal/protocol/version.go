// version.go — protocolVersion negotiation (R5′ mitigation: the wire
// contract is versioned and compat-tested; see
// tmp/solution-brief/02-solution-brief.md v3 risk register and
// compat_test.go). The constant and the error-code table are the whole
// versioning surface; bumping ProtocolVersion is the REQUIRED ritual for
// any breaking wire change.
package protocol

import "encoding/json"

// ProtocolVersion is the host-protocol version served by this engine.
const ProtocolVersion = 1

// Error codes (closed table; see docs/native-engine/host-protocol.md).
const (
	ErrParse              = -32700 // malformed line (skip + protocol/error)
	ErrInvalidRequest     = -32600 // shape violation / closing
	ErrMethodNotFound     = -32601
	ErrInvalidParams      = -32602
	ErrEngine             = -32000 // internal handler failure
	ErrInitializeRequired = -32001 // request before a successful initialize
	ErrVersionMismatch    = -32002 // initialize protocolVersion mismatch
	ErrNoSession          = -32003 // method needs an active session
	ErrUnknownApproval    = -32004 // approval/respond target unknown/expired
)

// decodeParams strictly decodes req params into v (unknown fields are
// rejected — fail-closed inbound discipline). Absent params are an error
// unless allowMissing.
func decodeParams(req *Request, v any, allowMissing bool) *Error {
	if len(req.Params) == 0 {
		if allowMissing {
			return nil
		}
		return &Error{Code: ErrInvalidParams, Message: "params are required"}
	}
	dec := jsonDecoder(req.Params, v)
	if err := dec.Decode(v); err != nil {
		return &Error{Code: ErrInvalidParams, Message: "invalid params: " + err.Error()}
	}
	return nil
}

// initializeParams is the initialize request body.
type initializeParams struct {
	ProtocolVersion int `json:"protocolVersion"`
}

// initializeResult is the initialize response body.
type initializeResult struct {
	ProtocolVersion int          `json:"protocolVersion"`
	ServerInfo      serverInfo   `json:"serverInfo"`
	Capabilities    capabilities `json:"capabilities"`
}

type serverInfo struct {
	Name string `json:"name"`
}

type capabilities struct {
	Approval           bool `json:"approval"`
	Jobs               bool `json:"jobs"`
	EventNotifications bool `json:"eventNotifications"`
}

// handleInitialize performs the one-shot version handshake. A mismatch
// is a hard error that leaves NO partial state: the server stays
// uninitialized (a mismatch is attributable and reversible — the client
// may retry with the right version, as the dsh gap analysis recommends:
// "no protocol-version negotiation" was a stated dsh SDK gap we close).
func handleInitialize(s *Server, req *Request) (json.RawMessage, *Error) {
	var p initializeParams
	if perr := decodeParams(req, &p, false); perr != nil {
		return nil, perr
	}
	if p.ProtocolVersion != ProtocolVersion {
		data, _ := json.Marshal(struct {
			Server int `json:"server"`
			Client int `json:"client"`
		}{ProtocolVersion, p.ProtocolVersion})
		return nil, &Error{
			Code:    ErrVersionMismatch,
			Message: "protocol version mismatch",
			Data:    data,
		}
	}
	s.mu.Lock()
	s.initialized = true
	s.mu.Unlock()

	result, err := json.Marshal(initializeResult{
		ProtocolVersion: ProtocolVersion,
		ServerInfo:      serverInfo{Name: "vh-agent-harness"},
		Capabilities:    capabilities{Approval: true, Jobs: true, EventNotifications: true},
	})
	if err != nil {
		return nil, &Error{Code: ErrEngine, Message: err.Error()}
	}
	return result, nil
}
