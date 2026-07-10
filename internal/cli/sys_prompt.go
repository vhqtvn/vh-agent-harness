package cli

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	corpus "github.com/vhqtvn/vh-agent-harness"
)

// sysPromptCmd prints a named system prompt surfaced from an embedded
// binary-only library (templates/sys-prompts). Named system prompts are assets
// the binary serves to plugins/agents on demand — they are NOT consumer-corpus
// files and are never rendered into a target repo by the substrate seam.
//
// Resolution is live-tree-first, embed-fallback: if
// <target>/.opencode/sys-prompts/<name>.md exists on disk (rendered there by an
// overlay pack via the standard .opencode/ render path, or hand-placed by the
// operator), that LIVE file's content is served; otherwise the binary's embedded
// snapshot is served. This mirrors the docs command's shape but replaces its
// bespoke docs-overrides.yml with a live-tree check — customization flows
// through the EXISTING overlay pack system (an overlay renders
// .opencode/sys-prompts/<name>.md), which the shadow system allows because
// .opencode/sys-prompts/ is not a core collision path.
var sysPromptCmd = &cobra.Command{
	Use:           "sys-prompt [name]",
	Short:         "Print a named system prompt (binary default, overridable via overlay)",
	SilenceUsage:  true,
	SilenceErrors: true,
	Long: `Print a named system prompt served from the binary's embedded library.

Named system prompts are assets the harness ships inside the binary (outside
templates/core, so they are never rendered into your repo). Plugins and agents
consume them on demand instead of carrying their own inline copy.

With NO argument, lists every available prompt key. With a NAME, prints that
prompt to stdout as raw bytes (no token substitution):

   vh-agent-harness sys-prompt                        # list keys
   vh-agent-harness sys-prompt auto-gate-classifier   # print one

By default the embedded copy is served. To serve a LIVE on-disk file instead,
render it to .opencode/sys-prompts/<name>.md (an overlay pack can do this via
the standard .opencode/ render path, or an operator can hand-place it); that
live file supersedes the embedded default.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runSysPrompt,
}

// sysPromptTargetFlag lets tests point sys-prompt at a target other than cwd
// (mirrors docsTargetFlag).
var sysPromptTargetFlag string

func init() {
	sysPromptCmd.Flags().StringVar(&sysPromptTargetFlag, "target", "", "target repo root (default: cwd); a live override is read from <target>/.opencode/sys-prompts/<name>.md")
	_ = sysPromptCmd.Flags().MarkHidden("target")
}

func runSysPrompt(cmd *cobra.Command, args []string) error {
	out := cmd.OutOrStdout()
	errOut := cmd.ErrOrStderr()

	index, keys, err := sysPromptIndex()
	if err != nil {
		fmt.Fprintln(errOut, "error:", err)
		return err
	}

	target := sysPromptTargetFlag
	if target == "" {
		cwd, werr := os.Getwd()
		if werr != nil {
			return fmt.Errorf("getcwd: %w", werr)
		}
		target = cwd
	}

	if len(args) == 0 {
		printSysPromptList(out, index, keys, target)
		return nil
	}

	want := sysPromptKey(args[0])
	if _, ok := index[want]; !ok {
		fmt.Fprintf(errOut, "error: no sys-prompt for %q\n\n", want)
		printSysPromptList(errOut, index, keys, target)
		return errSilent{}
	}

	body, err := resolveSysPromptBody(target, want, index)
	if err != nil {
		return err
	}
	if _, err := out.Write(body); err != nil {
		return err
	}
	return nil
}

// resolveSysPromptBody returns the content for a prompt key using live-tree-first,
// embed-fallback resolution. A live file at
// <target>/.opencode/sys-prompts/<key>.md (rendered by an overlay pack or
// hand-placed by the operator) supersedes the embedded copy. The embedded key
// set was already validated by the caller, so the embed-fallback never misses.
func resolveSysPromptBody(target, key string, index map[string][]byte) ([]byte, error) {
	livePath := filepath.Join(target, ".opencode", "sys-prompts", key+".md")
	if body, err := os.ReadFile(livePath); err == nil {
		return body, nil
	}
	body, ok := index[key]
	if !ok {
		return nil, fmt.Errorf("no sys-prompt for %q", key)
	}
	return body, nil
}

// sysPromptLiveOverride reports whether a live override file exists for key under
// target (used by the no-arg list to mark superseded keys).
func sysPromptLiveOverride(target, key string) bool {
	livePath := filepath.Join(target, ".opencode", "sys-prompts", key+".md")
	_, err := os.Stat(livePath)
	return err == nil
}

// sysPromptKey normalizes a user-supplied name: strips a leading ./, a trailing
// .md, and any directory prefix, so `sys-prompt foo`, `sys-prompt foo.md`, and
// `sys-prompt templates/sys-prompts/foo.md` all resolve to key "foo". Mirrors
// docsKey.
func sysPromptKey(arg string) string {
	s := strings.TrimPrefix(filepath.ToSlash(arg), "./")
	s = strings.TrimSuffix(s, ".md")
	if i := strings.LastIndex(s, "/"); i >= 0 {
		s = s[i+1:]
	}
	return s
}

// sysPromptIndex reads the embedded templates/sys-prompts tree and returns a map
// of key -> file body plus the sorted key list. A key is a prompt file's
// basename without the .md extension. Mirrors docsIndex.
func sysPromptIndex() (map[string][]byte, []string, error) {
	sub, err := fs.Sub(corpus.SysPromptsFS, corpus.SysPromptsDir)
	if err != nil {
		return nil, nil, fmt.Errorf("open embedded sys-prompts: %w", err)
	}
	index := map[string][]byte{}
	var keys []string
	err = fs.WalkDir(sub, ".", func(rel string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || !strings.HasSuffix(rel, ".md") {
			return nil
		}
		body, rerr := fs.ReadFile(sub, rel)
		if rerr != nil {
			return rerr
		}
		key := sysPromptKey(rel)
		index[key] = body
		keys = append(keys, key)
		return nil
	})
	if err != nil {
		return nil, nil, fmt.Errorf("walk embedded sys-prompts: %w", err)
	}
	sort.Strings(keys)
	return index, keys, nil
}

// printSysPromptList prints each prompt key with a one-line purpose (its first
// doc line), marking any key whose live override file supersedes the embed.
func printSysPromptList(w io.Writer, index map[string][]byte, keys []string, target string) {
	fmt.Fprintln(w, "Named system prompts (vh-agent-harness sys-prompt <name>):")
	width := 0
	for _, k := range keys {
		if len(k) > width {
			width = len(k)
		}
	}
	for _, k := range keys {
		marker := ""
		if sysPromptLiveOverride(target, k) {
			marker = "  [override]"
		}
		fmt.Fprintf(w, "  %-*s  %s%s\n", width, k, firstDocLine(index[k]), marker)
	}
}
