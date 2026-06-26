// Package runtime drives the runtime backend abstraction (docker_compose,
// bare) used by the exec/shell/up/down/logs/ps subcommands.
//
// The package is intentionally independent of internal/manifest: a Backend is
// configured from plain config structs (DockerComposeConfig, BareConfig) that
// the CLI layer resolves from the manifest's runtime block. This keeps the
// command-construction logic unit-testable without a real docker daemon.
//
// Design (slice 4a):
//
//   - Backend is the lifecycle + exec surface: Up/Down/Exec/Logs/Ps/Healthcheck.
//   - A Runner abstracts os/exec so argv construction is pure and testable; the
//     default runner (NewOSRunner) is a thin wrapper over exec.CommandContext.
//   - The docker_compose backend shells out to `docker compose -f <file>
//     -p <project> <verb>`. It resolves compose file, project name, and
//     default exec service from its config. On an unreachable daemon/binary it
//     returns a clear "fail_with_guidance" error and NEVER silently falls back
//     to the bare backend.
//   - The bare backend runs commands directly on the host via os/exec with no
//     isolation. It prints a loud stderr warning on every lifecycle/exec verb;
//     it is a real fallback path but must never be silently substituted.
//
// The permission gate (internal/permission) is a separate layer that runs
// BEFORE the backend on exec/shell; slice 4a ships a fail-loud NoOp stub there.
package runtime
