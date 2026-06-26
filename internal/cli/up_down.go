package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/vhqtvn/vh-agent-harness/internal/hooks"
	"github.com/vhqtvn/vh-agent-harness/internal/runshape"
)

// upCmd brings the runtime backend up. For docker_compose this is
// `docker compose up -d` (preflighting the daemon); for bare it is a no-op with
// a no-isolation warning.
var upCmd = &cobra.Command{
	Use:   "up",
	Short: "Start the harness runtime backend",
	Long: `Start the runtime backend declared by the manifest (manifest.runtime.backend).

docker_compose: runs ` + "`docker compose up -d`" + ` after a daemon-reachability preflight.
If the daemon/compose is unavailable, the command fails with guidance and does
NOT silently fall back to the bare backend.

bare: prints a no-isolation warning and does nothing (commands always run on the host).`,
	Args: cobra.NoArgs,
	RunE: runUp,
}

// downCmd tears the runtime backend down.
var downCmd = &cobra.Command{
	Use:   "down",
	Short: "Stop the harness runtime backend",
	Long: `Stop the runtime backend declared by the manifest.

docker_compose: runs ` + "`docker compose down`" + ` after a daemon-reachability preflight.
bare: prints a no-isolation warning and does nothing.`,
	Args: cobra.NoArgs,
	RunE: runDown,
}

func runUp(cmd *cobra.Command, _ []string) error {
	out := cmd.OutOrStdout()
	be, lm, err := resolveBackend()
	if err != nil {
		return err
	}
	ctx := context.Background()
	// Hooks WRAP the verb body (the run-shape spec §5): pre_up -> backend.Up -> post_up.
	// They do NOT replace it. For host-shell/bare where Up is a no-op-with-guidance,
	// the pre/post hooks still fire around the no-op (a consumer may use pre_up to
	// provision host deps, post_up to smoke). Each hook is gate-checked through the
	// SAME shell-guard policy pack as exec (see fireHook); absent = no-op.
	if _, err := fireHook(ctx, hooks.Point(runshape.HookPreUp), be.Name(), lm.dir); err != nil {
		// pre_up is FailVerb (§4): on denial/non-zero the services stay down.
		return fmt.Errorf("pre_up hook: %w", err)
	}
	if err := be.Up(ctx); err != nil {
		return err
	}
	if _, err := fireHook(ctx, hooks.Point(runshape.HookPostUp), be.Name(), lm.dir); err != nil {
		// post_up is FailVerbServicesUp (§4): up succeeded but the post-step failed.
		return fmt.Errorf("post_up hook: %w", err)
	}
	fmt.Fprintf(out, "up: runtime backend %s started\n", be.Name())
	return nil
}

func runDown(cmd *cobra.Command, _ []string) error {
	out := cmd.OutOrStdout()
	be, lm, err := resolveBackend()
	if err != nil {
		return err
	}
	ctx := context.Background()
	// pre_down/post_down are WarnAndContinue (§4): a hook denial/non-zero is
	// logged and the verb proceeds. Hooks wrap the body, never replace it.
	if _, err := fireHook(ctx, hooks.Point(runshape.HookPreDown), be.Name(), lm.dir); err != nil {
		return fmt.Errorf("pre_down hook: %w", err)
	}
	if err := be.Down(ctx); err != nil {
		return err
	}
	if _, err := fireHook(ctx, hooks.Point(runshape.HookPostDown), be.Name(), lm.dir); err != nil {
		return fmt.Errorf("post_down hook: %w", err)
	}
	fmt.Fprintf(out, "down: runtime backend %s stopped\n", be.Name())
	return nil
}
