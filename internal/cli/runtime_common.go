package cli

import (
	"context"
	"fmt"
	"os"
	"sync"

	"github.com/vhqtvn/vh-agent-harness/internal/manifest"
	"github.com/vhqtvn/vh-agent-harness/internal/permission"
	"github.com/vhqtvn/vh-agent-harness/internal/runshape"
	"github.com/vhqtvn/vh-agent-harness/internal/runtime"
)

// runtimeConfig is the backend-selection spec, decoupled from the manifest type
// so the runtime verbs can resolve a backend from EITHER the S4 run-shape.yml
// runtime block (preferred authority) OR the legacy manifest Runtime/Project
// fields. Both populate the same shape; selectBackend treats them uniformly.
type runtimeConfig struct {
	backend        string
	composeFile    string
	defaultService string
	projectSlug    string
	proxyCommand   []string
}

// hookCache memoizes the ShellGuardHook per harness root so each exec/shell
// call does not re-probe node + eval.js availability (the probe runs `node
// --version` and an os.Stat). It is keyed by the absolute project-root
// containing .opencode/ (loadedManifest.dir). Tests reset it via
// resetHookCache (called from resetRuntimeDeps).
var (
	hookCacheMu sync.Mutex
	hookCache   = map[string]permission.Hook{}
)

// runtimeCmdDeps lets unit tests replace the backend + hook wiring without a
// real docker daemon or manifest. The production runExec/runShell use the live
// default; tests override the fields to inject doubles.
//
// NOTE (slice 4b): the package no longer holds a NoOpHook default. The wired
// default is now a real ShellGuardHook built lazily via activeHook(harnessRoot).
// runtimeCmdDeps.hook == nil means "use the lazy ShellGuardHook default".
var runtimeCmdDeps = struct {
	backendFor func(lm *loadedManifest) (runtime.Backend, error)
	hook       permission.Hook
}{
	backendFor: defaultBackendFor,
	hook:       nil, // nil => use the lazy ShellGuardHook default (activeHook)
}

// resetHookCache drops all memoized ShellGuardHooks. Called between tests so a
// hook constructed against one temp root never leaks into another.
func resetHookCache() {
	hookCacheMu.Lock()
	defer hookCacheMu.Unlock()
	hookCache = make(map[string]permission.Hook)
}

// selectBackend maps a runtimeConfig to a concrete Backend. The switch is
// STRICT: an unknown backend yields a clear error and there is NO fallback path
// between isolation models (docker_compose never silently degrades to bare).
//
// Both the S4 run-shape and legacy manifest vocabularies are accepted for the
// docker backend: the run-shape schema validates `docker-compose` (hyphen, the
// documented public enum) while the legacy manifest carried `docker_compose`
// (underscore). normalizeDockerBackend collapses both to the internal
// `docker_compose` token so a consumer's declared form always resolves.
func selectBackend(cfg runtimeConfig, projectDir string) (runtime.Backend, error) {
	backend := normalizeDockerBackend(cfg.backend)
	switch backend {
	case "docker_compose":
		dc := runtime.DockerComposeConfig{
			ComposeFile:    cfg.composeFile, // empty => default docker-compose.yml resolved in backend
			ProjectName:    cfg.projectSlug,
			DefaultService: cfg.defaultService,
			Dir:            projectDir,
		}
		return runtime.NewDockerCompose(dc), nil
	case "bare":
		return runtime.NewBare(runtime.BareConfig{Dir: projectDir}), nil
	case "host-shell":
		// D1-C: host-shell is a first-class, capability-scoped backend — a typed
		// peer of docker_compose/proxy in the backend enum, deliberately chosen
		// for web-less / docker-less repos. It is NOT a fallback for an
		// unreachable docker_compose (that path errors, never degrades here).
		return runtime.NewHostShell(runtime.HostShellConfig{Dir: projectDir}), nil
	case "proxy":
		// proxy delegates exec/shell to a project-owned wrapper command (e.g.
		// ["./dev.sh","exec"]) that carries the project's domain knowledge. The
		// shell-guard gate runs first (in runExec/runShell), so the harness is
		// still the single gated entrypoint.
		if len(cfg.proxyCommand) == 0 {
			return nil, fmt.Errorf(
				"runtime.backend=proxy requires a non-empty runtime.proxy_command in " +
					".vh-agent-harness/run-shape.yml (e.g. proxy_command: [\"./dev.sh\", \"exec\"])",
			)
		}
		return runtime.NewProxy(runtime.ProxyConfig{Dir: projectDir, Command: cfg.proxyCommand}), nil
	case "":
		// Be explicit: no backend declared anywhere is ambiguous; point the
		// operator at the canonical default rather than guessing.
		return nil, fmt.Errorf(
			"no runtime.backend set. " +
				"Re-run `vh-agent-harness install` (default backend is host-shell) or set runtime.backend in .vh-agent-harness/run-shape.yml.",
		)
	default:
		return nil, fmt.Errorf(
			"unknown runtime.backend %q (expected \"docker_compose\"/\"docker-compose\", \"host-shell\", \"proxy\", or \"bare\"); "+
				"edit .vh-agent-harness/run-shape.yml (or .opencode/harness-manifest.json) and re-run",
			cfg.backend,
		)
	}
}

// normalizeDockerBackend collapses the two documented spellings of the docker
// backend onto the internal token. The run-shape schema enum is `docker-compose`
// (hyphen); the legacy manifest + backend.Name() use `docker_compose`
// (underscore). Both are accepted on read; neither is ever written back.
func normalizeDockerBackend(b string) string {
	if b == "docker-compose" {
		return "docker_compose"
	}
	return b
}

// backendSelector maps the manifest runtime block to a concrete Backend. It is
// the legacy entry point (preflight uses it directly with a real manifest); it
// delegates to selectBackend after extracting the runtimeConfig from the
// manifest fields.
func backendSelector(m *manifest.Manifest, projectDir string) (runtime.Backend, error) {
	return selectBackend(runtimeConfig{
		backend:        m.Runtime.Backend,
		composeFile:    m.Runtime.ComposeFile,
		defaultService: m.Runtime.DefaultService,
		projectSlug:    m.Project.Slug,
	}, projectDir)
}

// defaultBackendFor is the production backend resolver bound into
// runtimeCmdDeps. It selects the backend from the loaded runtime authority's
// runtimeConfig (populated from S4 run-shape when present, else the legacy
// manifest).
func defaultBackendFor(lm *loadedManifest) (runtime.Backend, error) {
	return selectBackend(lm.runtimeConfig, lm.dir)
}

// loadRuntimeAuthority resolves the runtime authority for the runtime verbs
// (exec/shell/up/down/logs/ps). Per the config-authority model the S4 run-shape
// (.vh-agent-harness/run-shape.yml) is the runtime authority and is read FIRST:
// when a run-shape with a `runtime:` block declaring a backend is found by
// walking up from the cwd, it populates the loadedManifest's runtimeConfig from
// S4 (and dir = the project root containing .vh-agent-harness/). When no S4
// run-shape is found (or it declares no backend), it falls back to the legacy
// manifest (.opencode/harness-manifest.json) via loadManifest. This keeps the
// runtime verbs working post-seam-install (which seeds a host-shell run-shape)
// while preserving the legacy manifest path for older installs.
//
// The returned loadedManifest.dir is always the project root anchoring the
// shell-guard hook + lifecycle hook dispatcher, regardless of which authority
// resolved the backend.
func loadRuntimeAuthority() (*loadedManifest, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	if root, rs, err := runshape.FindForRoot(cwd); err != nil {
		// A present-but-broken run-shape is a hard error (do not silently fall
		// through to the legacy manifest and mask the malformed S4).
		return nil, fmt.Errorf("read run-shape: %w", err)
	} else if root != "" && rs != nil && rs.Runtime != nil && rs.Runtime.Backend != "" {
		return &loadedManifest{
			dir: root,
			m:   nil, // S4 authority; no legacy manifest consulted
			runtimeConfig: runtimeConfig{
				backend:        rs.Runtime.Backend,
				composeFile:    rs.Runtime.ComposeFile,
				defaultService: rs.Runtime.DefaultService,
				projectSlug:    rs.Runtime.ProjectSlug,
				proxyCommand:   rs.Runtime.ProxyCommand,
			},
			source: "run-shape",
		}, nil
	}
	// S4 absent or declares no backend → fall back to the legacy manifest.
	lm, err := loadManifest()
	if err != nil {
		return nil, err
	}
	if lm.source == "" {
		lm.source = "manifest"
	}
	return lm, nil
}

// resolveBackend loads the runtime authority (S4 run-shape preferred, legacy
// manifest fallback) and resolves the runtime backend. Shared by all runtime
// verbs.
func resolveBackend() (runtime.Backend, *loadedManifest, error) {
	lm, err := loadRuntimeAuthority()
	if err != nil {
		return nil, nil, err
	}
	be, err := runtimeCmdDeps.backendFor(lm)
	if err != nil {
		return nil, lm, err
	}
	return be, lm, nil
}

// activeHook returns the permission hook to use: the test override if set,
// otherwise the lazily-built, memoized ShellGuardHook anchored at harnessRoot.
// harnessRoot is the project root containing .opencode/ (loadedManifest.dir).
func activeHook(harnessRoot string) permission.Hook {
	if runtimeCmdDeps.hook != nil {
		return runtimeCmdDeps.hook
	}
	hookCacheMu.Lock()
	defer hookCacheMu.Unlock()
	if h, ok := hookCache[harnessRoot]; ok {
		return h
	}
	h := permission.NewShellGuardHook(harnessRoot)
	hookCache[harnessRoot] = h
	return h
}

// evaluateGate runs the permission hook against cmd and returns the verdict +
// human reason. It is called by exec/shell BEFORE the backend is touched.
// harnessRoot anchors the ShellGuardHook's node subprocess (cwd) and eval.js
// lookup; it is the project root containing .opencode/.
func evaluateGate(harnessRoot string, cmd []string) (permission.Action, string, error) {
	return activeHook(harnessRoot).Evaluate(context.Background(), cmd)
}
