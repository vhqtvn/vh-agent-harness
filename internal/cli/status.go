package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/vhqtvn/vh-agent-harness/internal/lineage"
	"github.com/vhqtvn/vh-agent-harness/internal/manifest"
	"github.com/vhqtvn/vh-agent-harness/internal/runshape"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show harness installation info",
	Long: `Show harness installation info in two sections.

Global  - binary version and build info.
Project - installation details. When a legacy runtime manifest
          (.opencode/harness-manifest.json) is present it is shown; otherwise
          the seam install authority is read from the S1 lineage record
          (.vh-agent-harness/lineage.yml) and the S4 runtime shape
          (.vh-agent-harness/run-shape.yml).

Runtime service status is a separate command: "vh-agent-harness ps".`,
	Args: cobra.NoArgs,
	RunE: runStatus,
}

func runStatus(cmd *cobra.Command, _ []string) error {
	out := cmd.OutOrStdout()

	// --- Global -----------------------------------------------------------
	fmt.Fprintln(out, "Global")
	fmt.Fprintf(out, "  version:    %s\n", VersionString())
	fmt.Fprintf(out, "  go version: %s\n", runtime.Version())
	fmt.Fprintf(out, "  platform:   %s/%s\n", runtime.GOOS, runtime.GOARCH)
	fmt.Fprintln(out)

	// --- Project ----------------------------------------------------------
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	path, m, err := manifest.Find(cwd)
	if err != nil {
		fmt.Fprintln(out, "Project")
		fmt.Fprintf(out, "  error: %s\n", err)
		return nil
	}
	if m != nil {
		printLegacyProject(out, path, m)
		return nil
	}

	// Legacy manifest absent: fall back to the seam install authority (S1
	// lineage + S4 run-shape under .vh-agent-harness/).
	root, rs, rsErr := runshape.FindForRoot(cwd)
	if rsErr != nil {
		fmt.Fprintln(out, "Project")
		fmt.Fprintf(out, "  error: %s\n", rsErr)
		return nil
	}
	if root == "" {
		fmt.Fprintln(out, "No harness installation found in this directory (or any parent).")
		return nil
	}

	fmt.Fprintln(out, "Project")
	fmt.Fprintf(out, "  project_root:       %s\n", root)
	fmt.Fprintf(out, "  lineage:            %s\n", lineage.FilePath(root))
	if lin, lErr := lineage.Read(root); lErr == nil && lin != nil {
		fmt.Fprintf(out, "  template.source:    %s\n", lin.Template.Source)
		if lin.Template.Ref != "" {
			fmt.Fprintf(out, "  template.ref:       %s\n", lin.Template.Ref)
		}
		if lin.Render.RenderedBy != "" {
			fmt.Fprintf(out, "  render.rendered_by: %s\n", lin.Render.RenderedBy)
		}
		if lin.Render.LastSuccessfulUpdateID != "" {
			fmt.Fprintf(out, "  render.update_id:   %s\n", lin.Render.LastSuccessfulUpdateID)
		}
	}
	fmt.Fprintf(out, "  run-shape:          %s\n", filepath.Join(root, runshape.DirName, runshape.FileName))
	if rs != nil && rs.Runtime != nil && rs.Runtime.Backend != "" {
		fmt.Fprintf(out, "  runtime.backend:    %s\n", rs.Runtime.Backend)
	}
	return nil
}

func printLegacyProject(out interface{ Write([]byte) (int, error) }, path string, m *manifest.Manifest) {
	fmt.Fprintln(out, "Project")
	fmt.Fprintf(out, "  manifest:           %s\n", path)
	fmt.Fprintf(out, "  schema_version:     %s\n", m.SchemaVersion)
	fmt.Fprintf(out, "  harness_version:    %s\n", m.HarnessVersion)
	fmt.Fprintf(out, "  project.name:       %s\n", m.Project.Name)
	fmt.Fprintf(out, "  project.slug:       %s\n", m.Project.Slug)
	fmt.Fprintf(out, "  runtime.backend:    %s\n", m.Runtime.Backend)

	names := m.EnabledComponents
	fmt.Fprintf(out, "  enabled_components: %d", len(names))
	if len(names) > 0 {
		fmt.Fprintf(out, " [%s]", strings.Join(names, ", "))
	}
	fmt.Fprintln(out)

	fmt.Fprintf(out, "  files:              %d\n", len(m.Files))
}
