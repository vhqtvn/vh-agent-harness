// engine_test.go — test engine fixtures: a stub Engine for handshake
// and error-path tests (it never reaches the real seams), plus
// compile-time proofs that the real engine pieces satisfy the narrow
// protocol seams.
package protocol

import (
	"errors"
	"io"

	"github.com/vhqtvn/vh-agent-harness/internal/adapters"
	"github.com/vhqtvn/vh-agent-harness/internal/jobs"
	"github.com/vhqtvn/vh-agent-harness/internal/tools"
)

// stubEngine satisfies Engine without doing anything: handshake and
// error-path tests use it because they never reach the engine seams.
type stubEngine struct{}

func (stubEngine) NewSession(path, sessionID string, sink io.Writer) (*EngineSession, error) {
	return nil, errors.New("stubEngine: no session")
}

func (stubEngine) Adapter() adapters.Adapter      { return nil }
func (stubEngine) TurnRunner() TurnRunner         { return nil }
func (stubEngine) TurnOptions() tools.TurnOptions { return tools.TurnOptions{} }

// Compile-time proofs: the REAL engine seams satisfy the protocol's
// narrow consumer-side interfaces (telegraf-style composition).
var (
	_ TurnRunner    = (*tools.Pipeline)(nil)
	_ JobDispatcher = (*jobs.Manager)(nil)
)
