// config_test.go — the --mcp-config parsing contract, red-first:
// both file shapes (full opencode.json with a .mcp block, bare server
// map), defensive env/headers, unknown-key notes, credential posture,
// and the fail-closed typed errors with file:line-ish locators.
package mcp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadConfigFullOpencodeJSONExtractsMCPBlock(t *testing.T) {
	p := writeTemp(t, "opencode.json", `{
		"model": "glm-4.7",
		"mcp": {
			"vhmcp": {"type": "remote", "url": "https://llm.libvh.dev/chuppy-chuppa-mcp/mcp"},
			"zai": {"type": "local", "command": ["zai-mcp-server", "@z_ai/mcp-server", "start"]}
		},
		"theme": "dark"
	}`)
	cfg, err := LoadConfigFile(p)
	if err != nil {
		t.Fatalf("LoadConfigFile: %v", err)
	}
	if len(cfg.Servers) != 2 {
		t.Fatalf("servers = %d, want 2", len(cfg.Servers))
	}
	remote := cfg.Servers["vhmcp"]
	if remote == nil || remote.Type != TransportRemote {
		t.Fatalf("vhmcp = %+v", remote)
	}
	if remote.URL != "https://llm.libvh.dev/chuppy-chuppa-mcp/mcp" {
		t.Fatalf("url = %q", remote.URL)
	}
	local := cfg.Servers["zai"]
	if local == nil || local.Type != TransportLocal {
		t.Fatalf("zai = %+v", local)
	}
	if strings.Join(local.Command, " ") != "zai-mcp-server @z_ai/mcp-server start" {
		t.Fatalf("command = %v", local.Command)
	}
}

func TestLoadConfigBareServerMap(t *testing.T) {
	p := writeTemp(t, "mcp.json", `{
		"only": {"type": "local", "command": ["vh-mockmcp", "--stdio"]}
	}`)
	cfg, err := LoadConfigFile(p)
	if err != nil {
		t.Fatalf("LoadConfigFile: %v", err)
	}
	if len(cfg.Servers) != 1 || cfg.Servers["only"] == nil {
		t.Fatalf("servers = %+v", cfg.Servers)
	}
	// Sorted deterministic name order.
	if strings.Join(cfg.Names, ",") != "only" {
		t.Fatalf("names = %v", cfg.Names)
	}
}

func TestLoadConfigEnvAndHeadersDefensive(t *testing.T) {
	p := writeTemp(t, "mcp.json", `{
		"loc": {"type": "local", "command": ["x"], "env": {"FOO_CONFIG": "1", "ANOTHER": "two"}},
		"rem": {"type": "remote", "url": "https://h.example/mcp", "headers": {"X-Trace": "t1", "Authorization": "Bearer zz"}}
	}`)
	cfg, err := LoadConfigFile(p)
	if err != nil {
		t.Fatalf("LoadConfigFile: %v", err)
	}
	if cfg.Servers["loc"].Env["FOO_CONFIG"] != "1" || cfg.Servers["loc"].Env["ANOTHER"] != "two" {
		t.Fatalf("env = %v", cfg.Servers["loc"].Env)
	}
	if cfg.Servers["rem"].Headers["X-Trace"] != "t1" || cfg.Servers["rem"].Headers["Authorization"] != "Bearer zz" {
		t.Fatalf("headers = %v", cfg.Servers["rem"].Headers)
	}
}

func TestLoadConfigUnknownKeysNoted(t *testing.T) {
	p := writeTemp(t, "mcp.json", `{
		"srv": {"type": "local", "command": ["x"], "enabled": true, "cwd": "/tmp"}
	}`)
	cfg, err := LoadConfigFile(p)
	if err != nil {
		t.Fatalf("LoadConfigFile: %v", err)
	}
	uk := cfg.Servers["srv"].UnknownKeys
	if strings.Join(uk, ",") != "cwd,enabled" {
		t.Fatalf("unknown keys = %v (want sorted cwd,enabled)", uk)
	}
}

func TestLoadConfigEmptyMCPIsZeroServersNotError(t *testing.T) {
	p := writeTemp(t, "opencode.json", `{"model": "m", "mcp": {}}`)
	cfg, err := LoadConfigFile(p)
	if err != nil {
		t.Fatalf("LoadConfigFile: %v", err)
	}
	if len(cfg.Servers) != 0 {
		t.Fatalf("servers = %d, want 0", len(cfg.Servers))
	}
}

func TestLoadConfigFailClosedErrors(t *testing.T) {
	cases := []struct {
		name, body, wantErr string
	}{
		{"syntax", `{"a": `, "line 1"},
		{"non-object top level", `["x"]`, "object"},
		{"full config without mcp", `{"model": "m"}`, "server-config object"},
		{"missing type", `{"s": {"command": ["x"]}}`, `"s"`},
		{"unknown type", `{"s": {"type": "weird"}}`, `"s"`},
		{"local missing command", `{"s": {"type": "local"}}`, `"s"`},
		{"local empty command", `{"s": {"type": "local", "command": []}}`, `"s"`},
		{"local non-string command entry", `{"s": {"type": "local", "command": ["x", 3]}}`, `"s"`},
		{"remote missing url", `{"s": {"type": "remote"}}`, `"s"`},
		{"remote bad url scheme", `{"s": {"type": "remote", "url": "ftp://x/y"}}`, `"s"`},
		{"entry not an object", `{"s": "nope"}`, `"s"`},
		{"mcp block not object", `{"mcp": ["x"]}`, "mcp"},
		{"env not string map", `{"s": {"type": "local", "command": ["x"], "env": {"A": 3}}}`, `"s"`},
		{"headers not string map", `{"s": {"type": "remote", "url": "http://h/", "headers": {"A": true}}}`, `"s"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := writeTemp(t, "mcp.json", tc.body)
			_, err := LoadConfigFile(p)
			if err == nil {
				t.Fatalf("no error for %q", tc.body)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error %q does not contain %q", err, tc.wantErr)
			}
			if !strings.Contains(err.Error(), "mcp.json") {
				t.Fatalf("error %q does not name the file", err)
			}
		})
	}
}

func TestLoadConfigMissingFile(t *testing.T) {
	_, err := LoadConfigFile(filepath.Join(t.TempDir(), "absent.json"))
	if err == nil || !strings.Contains(err.Error(), "absent.json") {
		t.Fatalf("err = %v", err)
	}
}

func TestSyntaxErrorCarriesLine(t *testing.T) {
	p := writeTemp(t, "mcp.json", "{\n  \"a\": {\n  \n")
	_, err := LoadConfigFile(p)
	if err == nil {
		t.Fatal("no error")
	}
	if !strings.Contains(err.Error(), "line 4") {
		t.Fatalf("error %q lacks line locator", err)
	}
}

func TestDefaultConfigPathIsOpencodeJSON(t *testing.T) {
	p := DefaultConfigPath()
	if !strings.HasSuffix(filepath.ToSlash(p), ".config/opencode/opencode.json") {
		t.Fatalf("default path = %q", p)
	}
}
