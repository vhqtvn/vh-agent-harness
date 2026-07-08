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
	"gopkg.in/yaml.v3"

	corpus "github.com/vhqtvn/vh-agent-harness"
	"github.com/vhqtvn/vh-agent-harness/internal/runshape"
)

// docsCmd prints a generic agent-workflow doc surfaced from the embedded doc
// library (templates/docs). These docs describe harness machinery that is
// identical for every adopter, so they travel with the binary instead of being
// rendered into a consuming repo's tree.
//
// Resolution is override-first, embed-fallback: if
// .vh-agent-harness/docs-overrides.yml maps the requested key to a repo-relative
// file, that LIVE file's content is served; otherwise the binary's embedded
// snapshot is served. The override path is how this repo dogfoods the library
// under continuous update — the installed binary reads live source rather than a
// stale build-time embed. An adopter, with no overrides file, always gets the
// embedded copy.
var docsCmd = &cobra.Command{
	Use:           "docs [key]",
	Short:         "Print a generic agent-workflow doc (memory model, session workflow, prompt guide, …)",
	SilenceUsage:  true,
	SilenceErrors: true,
	Long: `Print an embedded generic agent-workflow doc. These docs describe harness
machinery (the session/memory/prompt/skill model) that is identical for every
adopter, so they ship inside the binary instead of being installed into your
repo.

With NO argument, lists every available doc key. With a KEY, prints that doc to
stdout:

   vh-agent-harness docs                       # list keys
   vh-agent-harness docs opencode-memory-model # print one

By default the embedded copy is served. To serve a LIVE on-disk file instead
(e.g. while editing the source under continuous update), map the key to a
repo-relative path in .vh-agent-harness/docs-overrides.yml:

   overrides:
     opencode-memory-model: templates/docs/opencode-memory-model.md`,
	Args: cobra.MaximumNArgs(1),
	RunE: runDocs,
}

// docsTargetFlag lets tests/CI point docs at a target other than cwd (mirrors
// updateTargetFlag / doctorTargetFlag).
var docsTargetFlag string

func init() {
	docsCmd.Flags().StringVar(&docsTargetFlag, "target", "", "target repo root (default: cwd); overrides are read from <target>/.vh-agent-harness/docs-overrides.yml")
	_ = docsCmd.Flags().MarkHidden("target")
}

func runDocs(cmd *cobra.Command, args []string) error {
	out := cmd.OutOrStdout()
	errOut := cmd.ErrOrStderr()

	index, keys, err := docsIndex()
	if err != nil {
		fmt.Fprintln(errOut, "error:", err)
		return err
	}

	target := docsTargetFlag
	if target == "" {
		cwd, werr := os.Getwd()
		if werr != nil {
			return fmt.Errorf("getcwd: %w", werr)
		}
		target = cwd
	}
	overrides := readDocsOverrides(target)

	if len(args) == 0 {
		printDocsList(out, index, keys, overrides)
		return nil
	}

	want := docsKey(args[0])
	if _, ok := index[want]; !ok {
		fmt.Fprintf(errOut, "error: no doc for %q\n\n", want)
		printDocsList(errOut, index, keys, overrides)
		return errSilent{}
	}

	body, err := resolveDocBody(target, want, index, overrides)
	if err != nil {
		return err
	}
	if _, err := out.Write(body); err != nil {
		return err
	}
	return nil
}

// resolveDocBody returns the content for a doc key using override-first,
// embed-fallback resolution. A configured override pointing at a
// missing/unreadable file is a loud error, not a silent fall-back — the
// operator asked for that specific file. index/overrides are passed in so
// callers that resolve several keys load them once.
func resolveDocBody(target, key string, index map[string][]byte, overrides map[string]string) ([]byte, error) {
	if rel, ok := overrides[key]; ok {
		body, err := os.ReadFile(filepath.Join(target, filepath.FromSlash(rel)))
		if err != nil {
			return nil, fmt.Errorf("docs override for %q points at %q, which is unreadable: %w", key, rel, err)
		}
		return body, nil
	}
	body, ok := index[key]
	if !ok {
		return nil, fmt.Errorf("no doc for %q", key)
	}
	return body, nil
}

// contextDocKeys are the generic docs that opencode auto-loads as always-on
// context via opencode.jsonc instructions[]. opencode reads FILES, not command
// output, so these must exist on disk in the consumer tree. install/update
// materialize them from the embedded library into .vh-agent-harness/docs/ (see
// materializeContextDocs); opencode.jsonc points instructions[] at that path.
var contextDocKeys = []string{"opencode-session-workflow"}

// contextDocsSubdir is the folder under .vh-agent-harness/ that holds the
// materialized always-on docs. It lives under the harness dir (not the
// project's own docs/ tree) so it does not clutter a consumer's documentation,
// and it is outside .opencode/ so the drift scanner never flags it.
const contextDocsSubdir = "docs"

// materializeContextDocs writes each always-on context doc (contextDocKeys)
// into <target>/.vh-agent-harness/docs/<key>.md, resolving content
// override-first (so this repo's dogfood copy tracks the live templates/docs/
// source on every update) then embed-fallback. Called by install/update after
// a real (non-dry-run) apply. Returns the relative paths written.
func materializeContextDocs(target string) ([]string, error) {
	index, _, err := docsIndex()
	if err != nil {
		return nil, err
	}
	overrides := readDocsOverrides(target)
	dir := filepath.Join(target, runshape.DirName, contextDocsSubdir)
	var written []string
	for _, key := range contextDocKeys {
		body, berr := resolveDocBody(target, key, index, overrides)
		if berr != nil {
			return nil, berr
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(filepath.Join(dir, key+".md"), body, 0o644); err != nil {
			return nil, err
		}
		written = append(written, filepath.ToSlash(filepath.Join(runshape.DirName, contextDocsSubdir, key+".md")))
	}
	return written, nil
}

// docsKey normalizes a user-supplied key: strips a leading ./, a trailing .md,
// and any directory prefix, so `docs foo`, `docs foo.md`, and
// `docs templates/docs/foo.md` all resolve to key "foo".
func docsKey(arg string) string {
	s := strings.TrimPrefix(filepath.ToSlash(arg), "./")
	s = strings.TrimSuffix(s, ".md")
	if i := strings.LastIndex(s, "/"); i >= 0 {
		s = s[i+1:]
	}
	return s
}

// docsIndex reads the embedded templates/docs tree and returns a map of
// key -> file body plus the sorted key list. A key is a doc file's basename
// without the .md extension.
func docsIndex() (map[string][]byte, []string, error) {
	sub, err := fs.Sub(corpus.DocsFS, corpus.DocsDir)
	if err != nil {
		return nil, nil, fmt.Errorf("open embedded docs: %w", err)
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
		key := docsKey(rel)
		index[key] = body
		keys = append(keys, key)
		return nil
	})
	if err != nil {
		return nil, nil, fmt.Errorf("walk embedded docs: %w", err)
	}
	sort.Strings(keys)
	return index, keys, nil
}

// docsOverridesFile is the optional per-project map of doc key -> repo-relative
// file whose live content overrides the embedded copy.
type docsOverridesFile struct {
	Overrides map[string]string `yaml:"overrides"`
}

// readDocsOverrides loads .vh-agent-harness/docs-overrides.yml under target.
// An absent file is the normal case (empty map -> embedded copies are served).
// A present-but-malformed file warns to stderr and is ignored (never fatal),
// matching the project.config.json tolerance model.
func readDocsOverrides(target string) map[string]string {
	if target == "" {
		return nil
	}
	path := filepath.Join(target, runshape.DirName, "docs-overrides.yml")
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil // absent/unreadable: no overrides
	}
	var doc docsOverridesFile
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		fmt.Fprintf(os.Stderr, "warning: ignoring unparseable %s: %v\n", path, err)
		return nil
	}
	return doc.Overrides
}

// printDocsList prints each doc key with a one-line purpose (its first doc
// line), marking any key served from a live override.
func printDocsList(w io.Writer, index map[string][]byte, keys []string, overrides map[string]string) {
	fmt.Fprintln(w, "Agent-workflow docs (vh-agent-harness docs <key>):")
	width := 0
	for _, k := range keys {
		if len(k) > width {
			width = len(k)
		}
	}
	for _, k := range keys {
		marker := ""
		if _, ok := overrides[k]; ok {
			marker = "  [override]"
		}
		fmt.Fprintf(w, "  %-*s  %s%s\n", width, k, firstDocLine(index[k]), marker)
	}
}
