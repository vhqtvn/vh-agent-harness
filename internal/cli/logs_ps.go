package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

// logsFl holds the flag for `vh-agent-harness logs`.
type logsFl struct {
	follow bool
}

var logsFlags *logsFl

// logsCmd shows runtime logs. With --follow/-f it tails; otherwise it snapshots.
var logsCmd = &cobra.Command{
	Use:   "logs [service]",
	Short: "Show harness runtime logs",
	Long: `Tail or snapshot logs from the runtime backend.

docker_compose: ` + "`docker compose logs [--follow] [service]`" + `.
bare: returns an error (no managed services to log).

Without a service argument, logs for all services are shown.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runLogs,
}

func init() {
	logsFlags = &logsFl{}
	logsCmd.Flags().BoolVarP(&logsFlags.follow, "follow", "f", false, "follow log output (stream)")
}

// psCmd lists runtime service status.
var psCmd = &cobra.Command{
	Use:   "ps",
	Short: "Show harness runtime service status",
	Long: `List runtime services and their status.

docker_compose: ` + "`docker compose ps`" + `.
bare: returns an error (no managed services to list).`,
	Args: cobra.NoArgs,
	RunE: runPs,
}

func runLogs(cmd *cobra.Command, args []string) error {
	be, _, err := resolveBackend()
	if err != nil {
		return err
	}
	service := ""
	if len(args) == 1 {
		service = args[0]
	}
	return be.Logs(context.Background(), service, logsFlags.follow)
}

func runPs(cmd *cobra.Command, _ []string) error {
	out := cmd.OutOrStdout()
	be, _, err := resolveBackend()
	if err != nil {
		return err
	}
	services, err := be.Ps(context.Background())
	if err != nil {
		return err
	}
	if len(services) == 0 {
		fmt.Fprintln(out, "no running services")
		return nil
	}
	fmt.Fprintf(out, "%-20s %-12s %s\n", "SERVICE", "STATE", "STATUS")
	for _, s := range services {
		fmt.Fprintf(out, "%-20s %-12s %s\n", s.Name, s.State, s.Status)
	}
	return nil
}
