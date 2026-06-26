package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

// DockerComposeConfig is the plain, manifest-resolved configuration for the
// docker_compose backend. It intentionally has no dependency on internal/manifest;
// the CLI layer fills it from the manifest runtime block (+ project identity).
type DockerComposeConfig struct {
	// ComposeFile is the path to docker-compose.yml (absolute or relative to
	// Dir). Empty defaults to "docker-compose.yml" resolved against Dir.
	ComposeFile string
	// ProjectName is the `docker compose -p <name>` project name. Usually the
	// manifest project slug.
	ProjectName string
	// DefaultService is the service used by Exec/Shell when ExecOpts.Service is
	// empty. May itself be empty — in that case a service-less exec errors with
	// guidance rather than guessing.
	DefaultService string
	// Dir is the working directory for compose invocations (the project root).
	Dir string
}

// DockerCompose implements Backend by shelling out to
// `docker compose -f <file> -p <project> <verb>`.
//
// Reachable is an injectable daemon-reachability probe; when nil, Up/Down/Exec/
// Logs/Ps preflight with a real `docker compose version` + `docker info` probe.
// Tests override Reachable to exercise the fail_with_guidance path without a
// real docker daemon.
type DockerCompose struct {
	Cfg       DockerComposeConfig
	Runner    Runner
	Reachable func(ctx context.Context) error
}

// NewDockerCompose builds a backend with the production OS runner. The returned
// backend uses the default daemon-reachability probe unless Reachable is
// replaced after construction.
func NewDockerCompose(cfg DockerComposeConfig) *DockerCompose {
	dc := &DockerCompose{Cfg: cfg, Runner: NewOSRunner()}
	dc.Reachable = dc.defaultProbe
	return dc
}

// Name returns the stable backend identifier.
func (dc *DockerCompose) Name() string { return "docker_compose" }

// Capabilities returns docker_compose's matrix: every core verb is Supported
// (full service-capable backend). docker_compose never returns an
// UnsupportedVerbError for a core verb.
func (dc *DockerCompose) Capabilities() CapabilityMatrix { return dockerComposeMatrix() }

// composeFile resolves the compose-file path against Dir, defaulting to
// docker-compose.yml when unset.
func (dc *DockerCompose) composeFile() string {
	if dc.Cfg.ComposeFile != "" {
		return dc.Cfg.ComposeFile
	}
	return "docker-compose.yml"
}

// baseArgs returns the shared `compose -f <file> -p <project>` argv tail. It
// deliberately does NOT include the leading "docker" binary name — every caller
// passes "docker" as the Runner command name and baseArgs() as the argv, so
// including "docker" here would double it into `docker docker compose ...` (the
// `docker: 'docker' is not a docker command` bug). defaultProbe() already
// follows the same name/args split: Run(ctx, "docker", []string{"compose", ...}).
func (dc *DockerCompose) baseArgs() []string {
	return []string{"compose", "-f", dc.composeFile(), "-p", dc.Cfg.ProjectName}
}

// upArgs builds `docker compose ... up -d`.
func (dc *DockerCompose) upArgs() []string {
	return append(dc.baseArgs(), "up", "-d")
}

// downArgs builds `docker compose ... down`.
func (dc *DockerCompose) downArgs() []string {
	return append(dc.baseArgs(), "down")
}

// execArgs builds the exec argv for the given command and opts. It returns an
// error when no service can be resolved (neither opts.Service nor
// DefaultService) — docker_compose never guesses the target container.
//
// When cmd is empty AND opts.Interactive is true, no trailing command is
// emitted: `docker compose exec <service>` opens the container's default shell
// (this is the `vh-agent-harness shell` path). A non-interactive empty-cmd exec errors.
func (dc *DockerCompose) execArgs(cmd []string, opts ExecOpts) ([]string, error) {
	service := opts.Service
	if service == "" {
		service = dc.Cfg.DefaultService
	}
	if service == "" {
		return nil, fmt.Errorf(
			"docker_compose exec requires a service: no --service given and manifest runtime.default_service is empty. " +
				"Set runtime.default_service in the manifest, or pass the service explicitly (later: `vh-agent-harness exec --service <name> ...`).",
		)
	}
	if len(cmd) == 0 && !opts.Interactive {
		return nil, fmt.Errorf("docker_compose exec requires a command to run (use `vh-agent-harness shell` for an interactive shell)")
	}
	args := append(dc.baseArgs(), "exec")
	// `docker compose exec` allocates a TTY by default; -T disables it. So:
	// Interactive=true  -> leave TTY on (pass host stdin through)
	// Interactive=false -> force -T (no TTY, clean piped output)
	if !opts.Interactive {
		args = append(args, "-T")
	}
	if opts.Workdir != "" {
		args = append(args, "-w", opts.Workdir)
	}
	args = append(args, service)
	args = append(args, cmd...)
	return args, nil
}

// logsArgs builds the logs argv. follow enables --follow; service is optional
// (empty tails all services).
func (dc *DockerCompose) logsArgs(service string, follow bool) []string {
	args := append(dc.baseArgs(), "logs")
	if follow {
		args = append(args, "--follow")
	}
	if service != "" {
		args = append(args, service)
	}
	return args
}

// psArgs builds `docker compose ... ps --format json` (one JSON object per
// service line).
func (dc *DockerCompose) psArgs() []string {
	return append(dc.baseArgs(), "ps", "--format", "json")
}

// preflight runs the reachability probe and, on failure, returns the canonical
// fail_with_guidance error. It NEVER falls back to the bare backend.
func (dc *DockerCompose) preflight(ctx context.Context) error {
	probe := dc.Reachable
	if probe == nil {
		probe = dc.defaultProbe
	}
	if err := probe(ctx); err != nil {
		return unavailableError(err)
	}
	return nil
}

// defaultProbe verifies the docker plugin and a reachable daemon by running
// `docker compose version` then `docker info` with discarded output.
func (dc *DockerCompose) defaultProbe(ctx context.Context) error {
	discard := RunOpts{Stdout: io.Discard, Stderr: io.Discard, Dir: dc.Cfg.Dir}
	if err := dc.Runner.Run(ctx, "docker", []string{"compose", "version"}, discard); err != nil {
		return fmt.Errorf("'docker compose' plugin not available: %w", err)
	}
	if err := dc.Runner.Run(ctx, "docker", []string{"info"}, discard); err != nil {
		return fmt.Errorf("docker daemon unreachable: %w", err)
	}
	return nil
}

// Up brings the compose stack up detached. It prefights the daemon first.
func (dc *DockerCompose) Up(ctx context.Context) error {
	if err := dc.preflight(ctx); err != nil {
		return err
	}
	return dc.Runner.Run(ctx, "docker", dc.upArgs(), RunOpts{Dir: dc.Cfg.Dir})
}

// Down tears the compose stack down. It prefights the daemon first so an
// unreachable daemon yields the guidance error rather than a raw exec error.
func (dc *DockerCompose) Down(ctx context.Context) error {
	if err := dc.preflight(ctx); err != nil {
		return err
	}
	return dc.Runner.Run(ctx, "docker", dc.downArgs(), RunOpts{Dir: dc.Cfg.Dir})
}

// Exec runs cmd inside the resolved service container. It prefights the daemon
// first. opts.Interactive controls the -T (no-TTY) flag.
func (dc *DockerCompose) Exec(ctx context.Context, cmd []string, opts ExecOpts) error {
	if err := dc.preflight(ctx); err != nil {
		return err
	}
	args, err := dc.execArgs(cmd, opts)
	if err != nil {
		return err
	}
	return dc.Runner.Run(ctx, "docker", args, RunOpts{Interactive: opts.Interactive, Dir: dc.Cfg.Dir})
}

// Logs tails or snapshots container logs. It prefights the daemon first.
func (dc *DockerCompose) Logs(ctx context.Context, service string, follow bool) error {
	if err := dc.preflight(ctx); err != nil {
		return err
	}
	return dc.Runner.Run(ctx, "docker", dc.logsArgs(service, follow), RunOpts{Dir: dc.Cfg.Dir})
}

// Ps lists service status, parsing `docker compose ps --format json` output into
// normalized ServiceStatus rows. It prefights the daemon first.
func (dc *DockerCompose) Ps(ctx context.Context) ([]ServiceStatus, error) {
	if err := dc.preflight(ctx); err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := dc.Runner.Run(ctx, "docker", dc.psArgs(), RunOpts{Dir: dc.Cfg.Dir, Stdout: &buf}); err != nil {
		return nil, err
	}
	return parsePsJSON(buf.Bytes()), nil
}

// Healthcheck exposes the reachability probe directly (used by a future
// `vh-agent-harness doctor`/preflight verb). It returns the same guidance error as the
// verb preflights on failure.
func (dc *DockerCompose) Healthcheck(ctx context.Context) error {
	if err := dc.preflight(ctx); err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "runtime backend docker_compose: reachable\n")
	return nil
}

// unavailableError wraps a probe failure with the canonical fail_with_guidance
// message. It is the single place that asserts NO silent fallback to bare.
func unavailableError(detail error) error {
	return fmt.Errorf(
		"runtime backend docker_compose is selected, but the docker daemon/compose is not reachable (%v). "+
			"No fallback configured. Start Docker, or regenerate the project with runtime.backend=bare "+
			"(no isolation). Refusing to silently switch isolation models.",
		detail,
	)
}

// psRow is the subset of `docker compose ps --format json` fields we capture.
type psRow struct {
	Service string `json:"Service"`
	State   string `json:"State"`
	Status  string `json:"Status"`
}

// parsePsJSON decodes newline-delimited JSON objects from `docker compose ps
// --format json`. Unparseable lines are skipped; if nothing parsed but the
// output was non-empty, a single "unknown" row carrying the raw text is
// returned so output is never silently dropped.
func parsePsJSON(raw []byte) []ServiceStatus {
	var out []ServiceStatus
	for _, line := range bytes.Split(raw, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		var r psRow
		if err := json.Unmarshal(line, &r); err != nil {
			continue
		}
		// docker emits an empty Service for header/footer noise; skip those.
		if r.Service == "" && r.State == "" && r.Status == "" {
			continue
		}
		name := r.Service
		state := strings.ToLower(strings.TrimSpace(r.State))
		if state == "" {
			state = "unknown"
		}
		out = append(out, ServiceStatus{Name: name, State: state, Status: r.Status})
	}
	if len(out) == 0 && len(bytes.TrimSpace(raw)) > 0 {
		out = append(out, ServiceStatus{Name: "", State: "unknown", Status: strings.TrimSpace(string(raw))})
	}
	return out
}
