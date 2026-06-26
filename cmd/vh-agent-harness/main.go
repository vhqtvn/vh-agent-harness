// Command vh-agent-harness is the single static binary for the agent-harness
// project. It acts as installer, manager, and executor for a repo-resident AI
// agent harness. See the README for the full term contract and architecture.
package main

import "github.com/vhqtvn/vh-agent-harness/internal/cli"

func main() {
	cli.Execute()
}
