// e2e_policy_test.go — the P3 policy surface at the REAL-binary seam
// (e2e_client_test.go's battery style). The ask-path policy behavior
// lives at the library seam (policy_seam_test.go) because the
// daemon's shipped tools never ask; the binary seam proves the
// STARTUP posture: bad/unreadable policy = exit 2 BEFORE the daemon
// spawns, and a loaded policy does not disturb a normal run.
package main

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestClientBinaryBadPolicyFileExitsTwo(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and spawns real binaries")
	}
	clientBin, _ := buildBinaries(t)

	dir := t.TempDir()
	bad := filepath.Join(dir, "broken.policy")
	if err := os.WriteFile(bad, []byte("[[allow]]\ntool = \"read\"\nmode = \"auto\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	code, out, errbuf := runClient(t, clientBin, []string{
		"--policy", bad,
		"--session-dir", filepath.Join(dir, "s"),
		"--prompt", "hi",
		"--exec", filepath.Join(dir, "no-such-daemon"),
	}, "")
	if code != 2 {
		t.Fatalf("exit = %d, want 2 (stderr:\n%s)", code, errbuf)
	}
	if !strings.Contains(errbuf, "broken.policy:3") {
		t.Fatalf("the parse error must name file:line:\n%s", errbuf)
	}
	// The load happens BEFORE the daemon spawn: no spawn line, and a
	// nonexistent daemon binary would have failed with exit 1 instead.
	if strings.Contains(errbuf, "daemon") && strings.Contains(errbuf, "pid") {
		t.Fatalf("no daemon may be spawned for a broken policy:\n%s", errbuf)
	}
	if out != "" {
		t.Fatalf("stdout must stay empty on a usage error, got %q", out)
	}
}

func TestClientBinaryUnreadablePolicyFileExitsTwo(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and spawns real binaries")
	}
	clientBin, _ := buildBinaries(t)

	// A directory is unreadable-as-a-file on every platform/user.
	dir := t.TempDir()
	code, _, errbuf := runClient(t, clientBin, []string{
		"--policy", dir,
		"--session-dir", filepath.Join(dir, "s"),
		"--prompt", "hi",
		"--exec", filepath.Join(dir, "no-such-daemon"),
	}, "")
	if code != 2 {
		t.Fatalf("exit = %d, want 2 (stderr:\n%s)", code, errbuf)
	}
	if !strings.Contains(errbuf, "cannot read policy file") {
		t.Fatalf("the error must name the read failure:\n%s", errbuf)
	}
}

// TestClientBinaryPolicyLoadedRunsCleanly: a valid policy file loads
// (one stderr line), the engine composes in front of the responder,
// and a normal never-asking turn (echo) completes exactly as without
// --policy.
func TestClientBinaryPolicyLoadedRunsCleanly(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and spawns real binaries")
	}
	clientBin, daemonBin := buildBinaries(t)

	var mu sync.Mutex
	calls := 0
	llm := startHTTPStub(t, func(w *jsonEncoder) {
		mu.Lock()
		calls++
		n := calls
		mu.Unlock()
		if n == 1 {
			w.toolCall("call-p", "echo", `{"text":"policy-loaded"}`)
			return
		}
		w.content("policy run ok")
	})
	defer llm.Close()

	dir := t.TempDir()
	pol := filepath.Join(dir, "echo.policy")
	if err := os.WriteFile(pol, []byte("[[allow]]\ntool = \"echo\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	code, out, errbuf := runClient(t, clientBin, []string{
		"--policy", pol,
		"--session-dir", filepath.Join(dir, "s"),
		"--prompt", "echo something",
		"--exec", daemonBin,
		"--adapter", "openai", "--model", "fake-model",
		"--base-url", llm.URL,
		"--api-key-env", "VH_AGENTD_TEST_KEY",
	}, "")
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stdout:\n%s\nstderr:\n%s)", code, out, errbuf)
	}
	if out != "policy run ok\n" {
		t.Fatalf("stdout = %q (stderr:\n%s)", out, errbuf)
	}
	if !strings.Contains(errbuf, "policy loaded (1 rules) from ") {
		t.Fatalf("the policy-load note is missing:\n%s", errbuf)
	}
	// echo never asks: the policy engine is loaded but never consulted
	// (no decision lines) — the wiring changed nothing about the turn.
	if strings.Contains(errbuf, "policy: ") {
		t.Fatalf("no policy decision line may appear when nothing asks:\n%s", errbuf)
	}
}
