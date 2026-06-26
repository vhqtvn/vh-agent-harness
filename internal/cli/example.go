package cli

import (
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	corpus "github.com/vhqtvn/vh-agent-harness"
)

// exampleCmd prints the embedded configuration doc/template for a project file.
// The harness no longer renders *.example scaffolds into the target tree; their
// content lives embedded under templates/examples/ (mirroring each real target
// path) and is printed on demand here. This keeps a consuming repo free of
// scaffold clutter while making the binary the single source of config docs.
var exampleCmd = &cobra.Command{
	Use:           "example [path]",
	Short:         "Print the config doc/template for a project file (no shipped *.example scaffolds)",
	SilenceUsage:  true,
	SilenceErrors: true,
	Long: `Print the embedded documentation + starter template for a configurable
harness file. The harness does NOT scatter *.example files into your repo; run
this when you want to create or configure one.

With NO argument, lists every configurable file. With a PATH (the real target
location), prints that file's doc to stdout — redirect it to create the file,
then edit:

   vh-agent-harness example .vh-agent-harness/AGENTS.mission.md > .vh-agent-harness/AGENTS.mission.md
   vh-agent-harness example .vh-agent-harness/project.config.json
   vh-agent-harness example .opencode/repo-configs/forbidden-patterns.project.js

Covered: the project-owned seeds (mission, project.config, forbidden-patterns,
compaction-primitives, cleared-assumptions, LANES, ROLES) AND the schema'd
authorities (vh-harness-profile.yml, run-shape.yml, harness-ownership.yml).`,
	Args: cobra.MaximumNArgs(1),
	RunE: runExample,
}

func runExample(cmd *cobra.Command, args []string) error {
	out := cmd.OutOrStdout()
	errOut := cmd.ErrOrStderr()

	index, paths, err := exampleIndex()
	if err != nil {
		fmt.Fprintln(errOut, "error:", err)
		return err
	}
	if len(args) == 0 {
		printExampleList(out, index, paths)
		return nil
	}

	want := strings.TrimPrefix(filepath.ToSlash(args[0]), "./")
	body, ok := index[want]
	if !ok {
		fmt.Fprintf(errOut, "error: no example for %q\n\n", want)
		printExampleList(errOut, index, paths)
		return errSilent{}
	}
	if _, err := out.Write(body); err != nil {
		return err
	}
	return nil
}

// exampleIndex reads the embedded templates/examples tree and returns a map of
// real-target-path -> file body, plus the sorted list of paths. Each example
// file's path under ExamplesDir IS the real target path it documents.
func exampleIndex() (map[string][]byte, []string, error) {
	sub, err := fs.Sub(corpus.ExamplesFS, corpus.ExamplesDir)
	if err != nil {
		return nil, nil, fmt.Errorf("open embedded examples: %w", err)
	}
	index := map[string][]byte{}
	var paths []string
	err = fs.WalkDir(sub, ".", func(rel string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		body, rerr := fs.ReadFile(sub, rel)
		if rerr != nil {
			return rerr
		}
		index[rel] = body
		paths = append(paths, rel)
		return nil
	})
	if err != nil {
		return nil, nil, fmt.Errorf("walk embedded examples: %w", err)
	}
	sort.Strings(paths)
	return index, paths, nil
}

// printExampleList prints each configurable path with a one-line purpose pulled
// from the file's first doc line.
func printExampleList(w io.Writer, index map[string][]byte, paths []string) {
	fmt.Fprintln(w, "Configurable files (vh-agent-harness example <path>):")
	width := 0
	for _, p := range paths {
		if len(p) > width {
			width = len(p)
		}
	}
	for _, p := range paths {
		fmt.Fprintf(w, "  %-*s  %s\n", width, p, firstDocLine(index[p]))
	}
	fmt.Fprintln(w, "\nPrint one and redirect it to create the file, e.g.:")
	fmt.Fprintln(w, "  vh-agent-harness example .vh-agent-harness/AGENTS.mission.md > .vh-agent-harness/AGENTS.mission.md")
}

// firstDocLine returns a short purpose string: the first non-empty content line
// with a leading comment marker (#, //, --) stripped. Falls back to "".
func firstDocLine(body []byte) string {
	for _, raw := range strings.Split(string(body), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		for _, m := range []string{"#", "//", "--", "/*", "*"} {
			if strings.HasPrefix(line, m) {
				line = strings.TrimSpace(strings.TrimPrefix(line, m))
				break
			}
		}
		// Skip JSON noise like `{` or a bare key.
		if line == "" || line == "{" {
			continue
		}
		if len(line) > 70 {
			line = line[:67] + "…"
		}
		return line
	}
	return ""
}
