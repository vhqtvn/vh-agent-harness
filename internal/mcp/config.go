// Package mcp implements the native engine's MCP HOST surface: parsing
// the operator's opencode-style MCP config (stdio local servers +
// Streamable-HTTP remote servers), speaking JSON-RPC 2.0 over both
// transport shapes by hand (stdlib only — no SDK), discovering each
// server's tools, and exposing them to the daemon's guarded tool
// registry under namespaced names.
//
// # SECURITY POSTURE (operator invariants, verbatim intent)
//
//   - MCP tools are EXTERNAL CANDIDATE INPUT. Every call rides the
//     engine's FULL approval/guard waterfall exactly like run_shell —
//     by construction: the registry only ever returns
//     tools.ToolDefinition values, and the pipeline applies the
//     waterfall, guards, approval bridge, and P3 policy classes to
//     every registered tool. No MCP result is trusted authority.
//   - URL path segments may embed tokens; url, headers, and env VALUES
//     are CREDENTIALS. They are never logged (structural never-log on
//     startup/diagnostic lines) and redacted out of every error or
//     captured-diagnostic surface via adapters.RedactSecret (the same
//     discipline as provider keys).
//   - A remote server going away, returning garbage, or hanging fails
//     CLOSED: per-call deadlines bound every exchange, a server-level
//     failure degrades that server (typed error at startup + at call
//     time), and the daemon never hangs a turn on an MCP exchange.
//   - Local (stdio) subprocesses start with a CLEAN, explicit env; the
//     configured env map merges in AFTER the daemon's scrub discipline
//     (secret-named vars — KEY/SECRET/TOKEN/PASSWORD, case-insensitive,
//     and the engine credential prefix — are dropped per the shell env
//     policy; the scrub wins).
//   - UNMAPPABLE input schemas skip THAT tool with a warning — the
//     server stays up (fail-closed per-tool, never per-daemon).
package mcp

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Transport kinds (the opencode config's "type" values).
const (
	TransportLocal  = "local"  // stdio subprocess
	TransportRemote = "remote" // Streamable-HTTP
)

// knownServerKeys are the config keys this host consumes; anything else
// on a server entry is ignored and recorded for the startup note.
var knownServerKeys = map[string]bool{
	"type": true, "command": true, "env": true, "url": true, "headers": true,
}

// ServerConfig is one configured MCP server (defensively parsed:
// standard shape; unknown keys ignored + surfaced for a startup note).
// URL, header values, and env values are CREDENTIALS — they reach no
// log line and are redacted from every error surface (see redactor).
type ServerConfig struct {
	Type    string
	Command []string          // local: the subprocess argv
	Env     map[string]string // local: merged into the child env AFTER the scrub
	URL     string            // remote: the endpoint (may embed tokens in the path)
	Headers map[string]string // remote: sent on every request (credentials)

	// UnknownKeys are the ignored keys, sorted — surfaced as ONE
	// startup note so an operator learns their "enabled": true was not
	// consumed.
	UnknownKeys []string
}

// Config is a parsed MCP config file.
type Config struct {
	Path    string
	Servers map[string]*ServerConfig
	// Names is the deterministic (sorted) server-name iteration order —
	// collision-safe namespacing and stable startup lines depend on it.
	Names []string
}

// DefaultConfigPath is the config used when --mcp-config is unset:
// the opencode default location.
func DefaultConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ".config/opencode/opencode.json"
	}
	return filepath.Join(home, ".config", "opencode", "opencode.json")
}

// LoadConfigFile reads and validates the MCP config at path. The file
// is EITHER a full opencode.json (a top-level object with an "mcp" key
// — that block is extracted) OR a bare {"<name>": {...}} server map.
// Every validation failure is a typed error naming the file and the
// offending server key (or, for syntax errors, the LINE — a
// file:line-ish locator, fail-closed exit 2 at the daemon).
func LoadConfigFile(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("mcp: read config %s: %v", path, err)
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(raw, &top); err != nil {
		return nil, fmt.Errorf("mcp: config %s: parse error at %s: %v", path, lineOf(raw, err), err)
	}
	if top == nil {
		return nil, fmt.Errorf("mcp: config %s: top level must be a JSON object", path)
	}

	serversRaw := top
	if mcpBlock, ok := top["mcp"]; ok {
		var block map[string]json.RawMessage
		if err := json.Unmarshal(mcpBlock, &block); err != nil || block == nil {
			return nil, fmt.Errorf("mcp: config %s: the \"mcp\" block must be an object mapping server names to server configs", path)
		}
		serversRaw = block
	}

	cfg := &Config{Path: path, Servers: make(map[string]*ServerConfig, len(serversRaw))}
	for name, entry := range serversRaw {
		sc, err := parseServer(name, entry)
		if err != nil {
			return nil, fmt.Errorf("mcp: config %s: %v", path, err)
		}
		if sc == nil {
			continue // an empty {"mcp": {}} contributes nothing
		}
		cfg.Servers[name] = sc
	}
	for name := range cfg.Servers {
		cfg.Names = append(cfg.Names, name)
	}
	sort.Strings(cfg.Names)
	return cfg, nil
}

// parseServer validates one server entry (fail-closed typed errors
// naming the server key). A nil, non-object entry errors — a full
// opencode.json WITHOUT an "mcp" key therefore fails closed on its
// first scalar top-level key with an actionable message, never a
// silently-empty server set.
func parseServer(name string, entry json.RawMessage) (*ServerConfig, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(entry, &fields); err != nil || fields == nil {
		return nil, fmt.Errorf("server %q is not a server-config object (if this file is a full opencode.json, its servers belong under a top-level \"mcp\" key)", name)
	}
	sc := &ServerConfig{}
	for k, v := range fields {
		if !knownServerKeys[k] {
			sc.UnknownKeys = append(sc.UnknownKeys, k)
			continue
		}
		switch k {
		case "type":
			if err := json.Unmarshal(v, &sc.Type); err != nil {
				return nil, fmt.Errorf("server %q: \"type\" must be a string", name)
			}
		case "command":
			if err := json.Unmarshal(v, &sc.Command); err != nil {
				return nil, fmt.Errorf("server %q: \"command\" must be an array of strings", name)
			}
		case "url":
			if err := json.Unmarshal(v, &sc.URL); err != nil {
				return nil, fmt.Errorf("server %q: \"url\" must be a string", name)
			}
		case "env":
			if err := json.Unmarshal(v, &sc.Env); err != nil || sc.Env == nil {
				return nil, fmt.Errorf("server %q: \"env\" must be an object mapping names to string values", name)
			}
		case "headers":
			if err := json.Unmarshal(v, &sc.Headers); err != nil || sc.Headers == nil {
				return nil, fmt.Errorf("server %q: \"headers\" must be an object mapping names to string values", name)
			}
		}
	}
	sort.Strings(sc.UnknownKeys)

	switch sc.Type {
	case TransportLocal:
		if len(sc.Command) == 0 {
			return nil, fmt.Errorf("server %q: type local requires a non-empty \"command\" array", name)
		}
		for _, c := range sc.Command {
			if c == "" {
				return nil, fmt.Errorf("server %q: \"command\" entries must be non-empty strings", name)
			}
		}
	case TransportRemote:
		u, err := url.Parse(sc.URL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return nil, fmt.Errorf("server %q: type remote requires \"url\" to be an absolute http(s) URL", name)
		}
	default:
		return nil, fmt.Errorf("server %q: \"type\" must be %q or %q (got %q)", name, TransportLocal, TransportRemote, sc.Type)
	}
	return sc, nil
}

// lineOf derives a "line N" locator from a json syntax error's byte
// offset (file:line-ish — the best a byte-offset API offers without a
// full position-tracking decoder). Truncated-input errors ("unexpected
// end of JSON input") can arrive without a usable offset; the end of
// the buffer is then the honest locator.
func lineOf(raw []byte, err error) string {
	off := -1
	type offsetter interface{ Offset() int64 }
	if se, ok := err.(offsetter); ok && se.Offset() > 0 {
		off = int(se.Offset())
	} else if strings.Contains(err.Error(), "unexpected end") {
		off = len(raw)
	}
	if off < 0 {
		return "line 1"
	}
	if off > len(raw) {
		off = len(raw)
	}
	return fmt.Sprintf("line %d", 1+strings.Count(string(raw[:off]), "\n"))
}
