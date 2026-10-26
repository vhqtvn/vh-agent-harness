// Package filetools implements the MODEL-FACING file tool family —
// read, write, edit, glob, search — the daily coding loop's gap-table
// rows 1-4 of the native-engine parity program. Every tool rides the
// existing pipeline unchanged (ToolDefinition contract, pre-execution
// tool/call logging via ExecuteLogged, deny-only guards, the approval
// waterfall, per-tool timeouts) and confines every user-supplied path
// to the configured workdir-root set with symlink-safe fail-closed
// checks BEFORE any filesystem effect (see confine.go).
//
// Placement (import-cycle note, mirroring internal/tools/subagenttools):
// a leaf package under internal/tools importing internal/tools and
// stdlib only. It deliberately does NOT import internal/protocol or
// internal/tools/shell — the confinement semantics are re-implemented
// here against those proven shapes (see confine.go's dedup note).
//
// Concurrency policy: read/glob/search are read-only and
// IsConcurrencySafe=true (they join the parallel pool); write/edit
// mutate the tree and run as exclusive barriers (the slice-3
// scheduler drains the pool around them). Oversize results are NOT
// special-cased here: the logResult commit seam already applies the
// session SpillPolicy, and every tool keeps its default result under
// a sane size via its own bounded output (byte cap or bounded counts
// with explicit overflow markers).
package filetools

import (
	"fmt"

	"github.com/vhqtvn/vh-agent-harness/internal/tools"
)

// Output bounds (Config zero values normalize to these).
const (
	// DefaultMaxReadBytes caps the read tool's rendered output
	// (numbered lines included), mirroring run_shell's per-stream
	// capture cap.
	DefaultMaxReadBytes = 64 * 1024
	// DefaultMaxGlobResults caps the glob tool's result count.
	DefaultMaxGlobResults = 500
	// DefaultMaxSearchMatches caps the search tool's match count.
	DefaultMaxSearchMatches = 200
)

// Config configures the family. The zero value is the production
// default EXCEPT Roots, which must be a non-empty set of absolute,
// existing directories (NewRoots validates; Definitions panics on an
// invalid set — a wiring bug, fail loudly at startup like the daemon's
// sandbox construction).
type Config struct {
	// Roots is the confinement set. Relative tool paths resolve
	// against Roots[0]; absolute paths must sit under some root.
	Roots []string

	// MaxReadBytes caps read's rendered output; <=0 ⇒ 64 KiB.
	MaxReadBytes int64
	// MaxGlobResults caps glob's listed paths; <=0 ⇒ 500.
	MaxGlobResults int
	// MaxSearchMatches caps search's reported matches; <=0 ⇒ 200.
	MaxSearchMatches int
}

// normalize fills the output bounds.
func (c *Config) normalize() {
	if c.MaxReadBytes <= 0 {
		c.MaxReadBytes = DefaultMaxReadBytes
	}
	if c.MaxGlobResults <= 0 {
		c.MaxGlobResults = DefaultMaxGlobResults
	}
	if c.MaxSearchMatches <= 0 {
		c.MaxSearchMatches = DefaultMaxSearchMatches
	}
}

// Definitions returns the file tool family for cfg (zero value =
// production bounds). Registrating on a Pipeline gives the tools their
// guards/approval/timeout choreography; nothing here bypasses it.
func Definitions(cfg Config) []tools.ToolDefinition {
	cfg.normalize()
	roots, err := NewRoots(cfg.Roots)
	if err != nil {
		panic(fmt.Sprintf("filetools: invalid workdir roots: %v", err))
	}
	return []tools.ToolDefinition{
		readDefinition(&cfg, roots),
		writeDefinition(&cfg, roots),
		editDefinition(&cfg, roots),
		globDefinition(&cfg, roots),
		searchDefinition(&cfg, roots),
	}
}
