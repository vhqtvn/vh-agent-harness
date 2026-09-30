// skills.go — the daemon's P7 native-skills wiring: --skills-dir flag
// semantics (catalog source, fail-closed vs honest-absent postures),
// the session→log registry the skill_load provenance sink resolves
// through (the jobsRegistry pattern re-scoped from jobs.Manager to
// session.Log), and the startup honesty lines.
//
// OPERATOR POSTURE (verbatim invariants this wiring carries):
//
//   - A loaded SKILL.md is UNTRUSTED candidate-instruction data, never
//     system authority; nothing it says relaxes allow/deny/ask anywhere
//     in the engine.
//   - `allowed-tools` is a CEILING intersected with the registry —
//     narrow-never-widen, never a grant. Nothing here consumes it to
//     ALLOW anything (audit-only: result footer + skill/loaded event).
//     The documented enforcement seam for running risky skills scoped
//     is per-spawn tool scoping on the durable-subagent path — a later
//     knob, deliberately NOT built in this slice.
//   - Bundled scripts never auto-execute (files under a skill folder
//     are inert; running one goes through run_shell + the approval
//     waterfall).
//
// The catalog is read ONCE at startup — no per-turn hot-reload
// (documented non-goal; a skills-dir edit requires a daemon restart).
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"

	"github.com/vhqtvn/vh-agent-harness/internal/protocol"
	"github.com/vhqtvn/vh-agent-harness/internal/session"
	"github.com/vhqtvn/vh-agent-harness/internal/skills"
	"github.com/vhqtvn/vh-agent-harness/internal/tools"
	"github.com/vhqtvn/vh-agent-harness/internal/tools/skillload"
)

// defaultSkillsDir is the catalog location when --skills-dir is not
// passed: ./.opencode/skills resolved against the daemon's working
// directory (the harness-standard rendered catalog home).
func defaultSkillsDir() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("resolve daemon cwd for the default skills dir: %w", err)
	}
	return filepath.Join(cwd, ".opencode", "skills"), nil
}

// loadSkillsCatalog implements the --skills-dir flag semantics:
//
//   - flag UNSET: the default dir (./.opencode/skills against the
//     daemon cwd). Absent default = honest startup line ("skills:
//     none (no catalog at <path>)"), empty catalog, daemon runs
//     normally — the harness-standard catalog is optional.
//   - flag SET: the dir must exist — an explicitly-passed-but-missing
//     dir is a fail-closed exit-2 usage error (never a silent empty
//     catalog the operator believes is loaded).
//   - flag SET or default present: skills.Load scans, parses, and
//     validates; each fail-closed skill exclusion is ONE stderr
//     warning line; the count line reports what the daemon actually
//     loaded.
//
// Returns (nil catalog, nil error) for the honest-absent default.
func loadSkillsCatalog(flagValue string, lg *log.Logger) (*skills.Catalog, error) {
	dir := flagValue
	explicit := dir != ""
	if !explicit {
		var err error
		dir, err = defaultSkillsDir()
		if err != nil {
			return nil, err
		}
	}
	if !filepath.IsAbs(dir) {
		if cwd, err := os.Getwd(); err == nil {
			dir = filepath.Join(cwd, dir)
		}
	}
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		if explicit {
			return nil, fmt.Errorf("skills dir %s does not exist (--skills-dir was passed explicitly — refusing to start with a silently-empty catalog)", dir)
		}
		// Honest absent default: zero skills, engine runs normally.
		lg.Printf("skills: none (no catalog at %s)", dir)
		return nil, nil
	}
	cat, warns := skills.Load(dir)
	for _, w := range warns {
		lg.Printf("%s", w)
	}
	lg.Printf("skills: %d loaded from %s", cat.Len(), dir)
	return cat, nil
}

// sessionLogRegistry maps session id → that session's *session.Log —
// the seam the skill_load provenance sink resolves the EXECUTING
// session's log through (tools.WithExecutingSession in the turn
// context, the jobsRegistry pattern re-scoped). Root logs bind at
// create/resume via the sessionTracker. Thread-safe; entries live as
// long as the daemon.
type sessionLogRegistry struct {
	mu   sync.Mutex
	logs map[string]*session.Log
}

func newSessionLogRegistry() *sessionLogRegistry {
	return &sessionLogRegistry{logs: map[string]*session.Log{}}
}

// bind registers (or replaces) the session's log — called by the
// sessionTracker on NewSession/ResumeSession.
func (r *sessionLogRegistry) bind(es *protocol.EngineSession, sessionID string) {
	if es == nil || es.Log == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.logs[sessionID] = es.Log
}

// provenance returns the skillload.Provenance closure appending the
// log-only skill/loaded event {name, ref?, sha256} to the EXECUTING
// session's durable log. Best-effort (the spill-sidecar discipline):
// no executing session in the context or no bound log (e.g. a child
// session on a path that did not bind) is a silent skip — the loaded
// body already rides the logged tool/result of the same turn, so the
// event is additive audit, never load-bearing. Event ordering within
// the turn: tool/call (pre-exec) → skill/loaded (during exec) →
// tool/result — provenance lands between intent and result.
func (r *sessionLogRegistry) provenance() skillload.Provenance {
	return func(ctx context.Context, name, ref, sha256 string) {
		sessionID := tools.ExecutingSessionFrom(ctx)
		if sessionID == "" {
			return
		}
		r.mu.Lock()
		lg := r.logs[sessionID]
		r.mu.Unlock()
		if lg == nil {
			return
		}
		if _, err := lg.AppendSkillLoaded(name, ref, sha256); err != nil {
			// Best-effort: never fail the load on provenance.
			return
		}
	}
}
