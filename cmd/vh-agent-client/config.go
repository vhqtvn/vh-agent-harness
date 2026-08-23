// config.go — the reference CLI client's flag surface, validation, mode
// resolution, and daemon-argv assembly.
//
// UX contract (README.agent.md → "vh-agent-client"):
//
//		vh-agent-client [client flags] [--exec <daemon argv...>]
//
//	  - `--exec` takes the REST of the command line as the daemon launch
//	    spec (Go's flag parser stops at the first non-flag argument, so
//	    everything after the binary name — including flags — passes
//	    through to the daemon verbatim). A literal `--` right after the
//	    binary name is stripped; a `--` elsewhere in the spec is the
//	    daemon's problem.
//	  - without `--exec` the client launches `vh-agentd` from PATH with
//	    sane args: just the forwarded `--session-dir` (the daemon's own
//	    fail-closed validation supplies the loud missing-flag messages).
//	  - `--session-dir` is forwarded to a spawned daemon exactly once
//	    (injected only when the exec spec does not carry its own).
package main

import (
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"strings"
)

// Mode is the client's input mode.
type Mode int

const (
	// ModeOneShot sends one prompt, streams the turn, prints the final
	// assistant text on stdout, exits.
	ModeOneShot Mode = iota
	// ModeRepl reads lines from stdin (a line = one user message) until
	// exit/quit/EOF.
	ModeRepl
)

// defaultSessionDir is the client's session dir default (relative;
// parseArgs resolves it — like any user-supplied relative dir — to an
// absolute, cleaned path from the client's cwd before the daemon argv
// is assembled; the daemon gets it forwarded and the client also keeps
// its last-session pointer there).
const defaultSessionDir = ".vh-agent-sessions"

// Config is the parsed, validated client configuration.
type Config struct {
	// Exec is the daemon launch spec (argv[0] + args). nil = default
	// (`vh-agentd` + forwarded --session-dir).
	Exec []string
	// SessionDir is the client-side session dir (forwarded to a spawned
	// daemon when the spec lacks its own; holds the client's
	// last-session pointer file).
	SessionDir string
	// JSON selects machine mode: NDJSON events verbatim on stdout, no
	// rendering; approval answers arrive as JSON lines on stdin.
	JSON bool
	// Prompt is the one-shot prompt text (Mode == ModeOneShot only).
	Prompt string
	// Repl forced REPL mode even without a TTY.
	Repl bool
	// Resume asks to resume the prior session (REFUSED in this slice —
	// see driver.go; session/resume does not exist on the wire yet).
	Resume bool
	// New forces a fresh session even when a prior pointer exists (the
	// default posture; the flag exists to make the choice explicit).
	New bool
	// PolicyPath is the P3 auto-approver policy file (--policy). Empty
	// = no policy engine: the interactive/--json responder answers
	// exactly as before. A present-but-broken file is a usage error
	// (exit 2) at startup — never a silently-absent policy.
	PolicyPath string
	// Mode is the resolved input mode.
	Mode Mode
}

// usageError marks a usage/validation failure (exit code 2).
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

func usagef(format string, args ...any) error {
	return &usageError{msg: fmt.Sprintf(format, args...)}
}

// usageDoc is printed on usage errors (stderr).
const usageDoc = `vh-agent-client — reference CLI client for the vh-agentd host protocol

usage:
  vh-agent-client [flags] [--exec <daemon-argv...>]

flags:
  --exec <argv...>   daemon launch spec; the REST of the command line
                     (default: vh-agentd from PATH; flags after the
                     binary name or a leading -- pass through)
  --session-dir DIR  session dir (default .vh-agent-sessions; resolved
                      to an absolute path from the client's cwd and
                      forwarded to a spawned daemon when the spec lacks
                      its own — the daemon rejects relative dirs)
  --json             machine mode: NDJSON events verbatim on stdout,
                     approval answers as JSON lines on stdin
  --prompt TEXT      one-shot: send TEXT, stream the turn, print the
                     final assistant text on stdout, exit
  --repl             force interactive REPL even when stdin is not a TTY
  --resume           resume the prior session (REFUSED for now: the
                      session/resume wire method arrives in P4)
  --new              force a fresh session even when a prior one exists
                      (the default posture; explicit form)
  --policy FILE      auto-approver policy: [[allow]] rules evaluated
                      AFTER fixed hard-deny classes (secret env, git
                      mutation, path traversal, sandbox escalation);
                      unmatched calls still ask the human/--json
                      responder. Bad file = exit 2. Absent = unchanged
                      behavior (no policy engine)

output discipline (mirrors the daemon): rendered events and prompts go
to STDERR; stdout carries machine-readable content only (the final
assistant text in one-shot mode; NDJSON events in --json mode).

exit codes: 0 clean · 1 protocol/engine error · 2 usage/validation
`

// parseArgs parses and validates the client command line. isTTY decides
// the default REPL posture when neither --prompt nor --repl is given.
func parseArgs(args []string, isTTY func() bool, stderrw io.Writer) (*Config, error) {
	var (
		execFlag   = new(bool)
		sessionDir = new(string)
		jsonMode   = new(bool)
		prompt     = new(string)
		repl       = new(bool)
		resume     = new(bool)
		forceNew   = new(bool)
		policyPath = new(string)
	)
	fs := flag.NewFlagSet("vh-agent-client", flag.ContinueOnError)
	fs.SetOutput(stderrw)
	fs.Usage = func() { fmt.Fprint(stderrw, usageDoc) }
	fs.BoolVar(execFlag, "exec", false, "daemon launch spec (rest of the command line)")
	fs.StringVar(sessionDir, "session-dir", defaultSessionDir, "session dir (resolved to an absolute path from the client's cwd; forwarded to a spawned daemon)")
	fs.BoolVar(jsonMode, "json", false, "machine mode: NDJSON events on stdout")
	fs.StringVar(prompt, "prompt", "", "one-shot prompt text")
	fs.BoolVar(repl, "repl", false, "force interactive REPL")
	fs.BoolVar(resume, "resume", false, "resume the prior session (refused until P4)")
	fs.BoolVar(forceNew, "new", false, "force a fresh session (the default posture)")
	fs.StringVar(policyPath, "policy", "", "auto-approver policy file (fail-closed parse; absent = no policy engine)")

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			// --help prints the doc and exits clean (run maps a nil
			// error with helpRequested via errHelp sentinel).
			return nil, errHelp
		}
		return nil, usagef("flag parsing failed: %v (see usage above)", err)
	}

	cfg := &Config{
		Exec:       nil,
		SessionDir: *sessionDir,
		JSON:       *jsonMode,
		Prompt:     *prompt,
		Repl:       *repl,
		Resume:     *resume,
		New:        *forceNew,
		PolicyPath: *policyPath,
	}
	if *execFlag {
		rest := fs.Args()
		if len(rest) == 0 {
			return nil, usagef("--exec needs the daemon command (everything after it passes through)")
		}
		// A literal `--` separating the binary name from daemon flags is
		// stripped (`--exec myd -- --flag v` → `myd --flag v`); a `--`
		// elsewhere in the spec passes through to the daemon.
		if rest[0] == "--" {
			rest = rest[1:]
			if len(rest) == 0 {
				return nil, usagef("--exec needs the daemon command after --")
			}
		} else if len(rest) > 1 && rest[1] == "--" {
			rest = append([]string{rest[0]}, rest[2:]...)
		}
		cfg.Exec = rest
	} else if len(fs.Args()) > 0 {
		return nil, usagef("unexpected argument %q (daemon argv requires --exec; see usage)", fs.Args()[0])
	}

	// Validation (fail-closed, exit 2).
	if cfg.Prompt != "" && cfg.Repl {
		return nil, usagef("--prompt and --repl are mutually exclusive")
	}
	if cfg.Resume && cfg.New {
		return nil, usagef("--resume and --new are mutually exclusive")
	}
	if cfg.SessionDir == "" {
		return nil, usagef("--session-dir must not be empty")
	}
	// --policy distinguishes "absent" (no engine, unchanged behavior)
	// from "explicitly empty" (a usage error): the flag VALUE is "" in
	// both cases, so the raw args decide.
	for i, a := range args {
		if (a == "--policy" || a == "-policy") && i+1 < len(args) && args[i+1] == "" {
			return nil, usagef("--policy must not be empty (omit the flag to run without a policy engine)")
		}
		if a == "--policy=" || a == "-policy=" {
			return nil, usagef("--policy must not be empty (omit the flag to run without a policy engine)")
		}
	}
	// Hotfix b-F2: resolve the session dir to an ABSOLUTE, cleaned
	// path BEFORE daemon argv assembly — both the documented default
	// (`.vh-agent-sessions`, relative) and any user-supplied relative
	// --session-dir. The daemon hard-rejects relative session dirs
	// (under --sandbox workspace-write the dir becomes a Landlock
	// RWDir resolved against the sandboxed child's cwd, not the
	// daemon's startup cwd), so forwarding the relative default
	// verbatim made the documented default invocation exit 2 before
	// protocol init. The daemon's contract is unchanged: it still
	// receives (and validates) an absolute path, and creating the dir
	// stays its job — the client only resolves.
	abs, aerr := filepath.Abs(cfg.SessionDir)
	if aerr != nil {
		return nil, usagef("--session-dir %q cannot be resolved to an absolute path: %v", cfg.SessionDir, aerr)
	}
	cfg.SessionDir = abs

	// Mode resolution: explicit --prompt wins, then --repl, then TTY.
	switch {
	case cfg.Prompt != "":
		cfg.Mode = ModeOneShot
	case cfg.Repl:
		cfg.Mode = ModeRepl
	case isTTY != nil && isTTY():
		cfg.Mode = ModeRepl
	default:
		return nil, usagef("no input mode: pass --prompt TEXT, --repl, or run on a TTY (stdin is not a terminal)")
	}
	if cfg.Prompt == "" && cfg.Mode == ModeOneShot {
		return nil, usagef("--prompt must not be empty")
	}
	return cfg, nil
}

// errHelp is the --help sentinel: clean exit after the usage doc.
var errHelp = fmt.Errorf("help requested")

// daemonArgv assembles the daemon launch argv: the exec spec when
// present (with --session-dir injected iff the spec lacks one),
// otherwise `vh-agentd --session-dir <dir>`.
func (c *Config) daemonArgv() []string {
	if len(c.Exec) == 0 {
		return []string{"vh-agentd", "--session-dir", c.SessionDir}
	}
	for _, a := range c.Exec {
		if a == "--session-dir" || a == "-session-dir" {
			return append([]string(nil), c.Exec...)
		}
	}
	argv := append([]string(nil), c.Exec...)
	// Prepend-style injection would let a user --session-dir later in
	// the spec override the client's; appending makes the CLIENT's dir
	// authoritative (its pointer file lives there) — documented.
	return append(argv, "--session-dir", c.SessionDir)
}

// exitCodeFor maps a run() error to the process exit code:
// 0 clean (nil / --help), 2 usage/validation, 1 everything else.
func exitCodeFor(err error) int {
	switch {
	case err == nil, err == errHelp:
		return 0
	case isUsageError(err):
		return 2
	default:
		return 1
	}
}

func isUsageError(err error) bool {
	_, ok := err.(*usageError)
	return ok
}

// compactOneLine collapses whitespace and truncates to max runes for
// compact rendering ("… " prefix markers are the caller's).
func compactOneLine(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}
