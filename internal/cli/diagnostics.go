package cli

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// diagnostics-export bundles selected harness state into a single tar.gz
// archive with field-aware secret redaction, written to repo-scoped tmp/. It is
// operator debugging/support tooling: it NEVER auto-uploads. The redaction
// engine is the safety-critical core and is unit-tested separately (see
// diagnostics_test.go).
//
// Design: the redaction is deterministic and lives in the Go binary (not in
// shell scripts or LLM-followed instructions) so it can be tested. Two
// strategies compose for defense in depth:
//   - structured redaction for JSON / YAML / JSON-lines: parse → walk →
//     redact by field name / whole section / value pattern → re-serialize.
//   - line-based redaction for everything else (.env, .md, .log, unknown
//     text): scrub obvious secrets in-place.
//
// Binary/unparseable files are included as-is and flagged in the manifest so
// the operator reviews them before sharing.

// -----------------------------------------------------------------------------
// Redaction engine
// -----------------------------------------------------------------------------

// sensitiveTokens are secret-bearing name fragments. A key is sensitive if its
// separator-/case-normalized form contains any of these. Normalization strips
// '_', '-', and spaces so "api_key", "api-key", and "apiKey" all reduce to
// "apikey".
//
// Matching is intentionally substring-based and safety-leaning: it accepts a
// few false positives such as "author"/"authenticate" (which contain "auth")
// rather than risk missing a genuinely sensitive key. This tradeoff is pinned
// by TestIsSensitiveKeyKnownFalsePositiveAuthor.
var sensitiveTokens = []string{
	"apikey",
	"token",
	"secret",
	"password",
	"passwd",
	"credential",
	"auth",
	"bearer",
	"privatekey",
	"accesskey",
	"clientsecret",
}

func normalizeKey(key string) string {
	s := strings.ToLower(key)
	s = strings.ReplaceAll(s, "_", "")
	s = strings.ReplaceAll(s, "-", "")
	s = strings.ReplaceAll(s, " ", "")
	s = strings.ReplaceAll(s, ".", "")
	return s
}

// isSensitiveKey reports whether key looks secret-sensitive.
func isSensitiveKey(key string) bool {
	k := normalizeKey(key)
	for _, t := range sensitiveTokens {
		if strings.Contains(k, t) {
			return true
		}
	}
	return false
}

// sensitiveSectionNames name whole blocks whose every value is redacted
// regardless of the child key names. Matched exactly (after normalization) at
// ANY depth.
var sensitiveSectionNames = map[string]bool{
	"secrets":     true,
	"env":         true,
	"environment": true,
	"credentials": true,
}

// modelsAsSectionName is redacted as a whole section ONLY at the document root
// (depth 0): in harness/agent config a top-level "models" block usually holds
// provider config with API keys, but a nested "models" key can be a data model
// and is preserved.
const modelsAsSectionName = "models"

// isWholeSectionKey reports whether a key names a block whose values are all
// redacted. depth is the depth of the map that owns key (0 = document root).
func isWholeSectionKey(key string, depth int) bool {
	n := normalizeKey(key)
	if sensitiveSectionNames[n] {
		return true
	}
	if depth == 0 && n == modelsAsSectionName {
		return true
	}
	return false
}

// redactStringValue returns the canonical redaction placeholder carrying the
// original rune count (an aid for review). Empty strings are left empty.
func redactStringValue(s string) string {
	if s == "" {
		return ""
	}
	return fmt.Sprintf("***REDACTED(%dchars)***", utf8.RuneCountInString(s))
}

// Anchored patterns applied to whole discrete values in parsed JSON/YAML.
var (
	bearerValuePattern  = regexp.MustCompile(`(?i)^Bearer\s+\S.*$`)
	awsKeyValuePattern  = regexp.MustCompile(`^AKIA[0-9A-Z]{16}$`)
	connStrValuePattern = regexp.MustCompile(`^([a-zA-Z][a-zA-Z0-9+.\-]*://)([^\s:/@]+):([^\s:/@]+)@`)
)

// redactByValuePattern redacts a discrete string value that matches a known
// secret format. Returns the (possibly redacted) value and whether it changed.
//   - AWS access-key id (AKIA...): fully redacted.
//   - Bearer token value: fully redacted (with char count).
//   - connection string with embedded password: the password is scrubbed to
//     "***" while scheme/user/host are preserved (most useful for debugging).
func redactByValuePattern(s string) (string, bool) {
	if awsKeyValuePattern.MatchString(s) {
		return "***REDACTED***", true
	}
	if bearerValuePattern.MatchString(s) {
		return redactStringValue(s), true
	}
	if connStrValuePattern.MatchString(s) {
		red := connStrValuePattern.ReplaceAllString(s, "$1$2:***@")
		if red != s {
			return red, true
		}
	}
	return s, false
}

// Non-anchored patterns applied anywhere within a line of unstructured text.
var (
	bearerTextPattern  = regexp.MustCompile(`(?i)Bearer\s+[A-Za-z0-9._~+/=\-]+`)
	awsKeyTextPattern  = regexp.MustCompile(`AKIA[0-9A-Z]{16}`)
	connStrTextPattern = regexp.MustCompile(`([a-zA-Z][a-zA-Z0-9+.\-]*://)([^\s:/@]+):([^\s:/@]+)@`)
)

// RedactionStats counts fields redacted by category.
type RedactionStats struct {
	FieldName    int `json:"field_name"`
	Section      int `json:"section"`
	ValuePattern int `json:"value_pattern"`
}

// redactValue walks a parsed JSON/YAML value and returns a redacted copy.
// depth is the depth of the document (0 = root). Maps at depth 0 see the
// top-level-only "models" section rule; the env/secrets/credentials rules apply
// at every depth.
func redactValue(v interface{}, depth int, stats *RedactionStats) interface{} {
	switch t := v.(type) {
	case map[string]interface{}:
		res := make(map[string]interface{}, len(t))
		for k, val := range t {
			switch {
			case isWholeSectionKey(k, depth):
				res[k] = redactAllScalars(val, stats)
			case isSensitiveKey(k):
				res[k] = redactSensitiveField(val, depth, stats)
			default:
				res[k] = redactValue(val, depth+1, stats)
			}
		}
		return res
	case []interface{}:
		arr := make([]interface{}, len(t))
		for i, item := range t {
			arr[i] = redactValue(item, depth+1, stats)
		}
		return arr
	case string:
		if r, ok := redactByValuePattern(t); ok {
			stats.ValuePattern++
			return r
		}
		return t
	default:
		// numbers, bools, nulls, json.Number: preserved unchanged.
		return v
	}
}

// redactSensitiveField redacts a value whose KEY name is sensitive. A scalar
// string is fully redacted. A container (map/array) is treated as suspect in
// its entirety: every string descendant is redacted so that secret material
// cannot hide behind a benign child key name that does not itself trip the
// sensitive matcher (e.g. {"auth":{"header":"Basic dXNlcjpwYXNz"}} or
// {"secret":{"value":"not-pattern-shaped"}}). Non-string scalars (numbers/bools)
// are preserved.
//
// The depth argument is intentionally unused: the subtree is blanket-redacted
// rather than recursively judged by child key names, which was the prior leak.
func redactSensitiveField(val interface{}, _ int, stats *RedactionStats) interface{} {
	if s, ok := val.(string); ok {
		stats.FieldName++
		return redactStringValue(s)
	}
	return redactAllScalarsUnderSensitive(val, stats)
}

// redactAllScalarsUnderSensitive redacts every string descendant of a value
// whose owning key was sensitive, counting each under field_name (the trigger
// was a sensitive key name, not a whole-section block like env/secrets). It is
// the nested-container analogue of redactAllScalars and, unlike the old
// recursive path, never preserves a child string under a sensitive key.
func redactAllScalarsUnderSensitive(v interface{}, stats *RedactionStats) interface{} {
	switch t := v.(type) {
	case string:
		stats.FieldName++
		return redactStringValue(t)
	case map[string]interface{}:
		res := make(map[string]interface{}, len(t))
		for k, val := range t {
			res[k] = redactAllScalarsUnderSensitive(val, stats)
		}
		return res
	case []interface{}:
		arr := make([]interface{}, len(t))
		for i, item := range t {
			arr[i] = redactAllScalarsUnderSensitive(item, stats)
		}
		return arr
	default:
		return v
	}
}

// redactAllScalars blanket-redacts every string descendant of a section block
// (used for env/secrets/credentials and top-level models). Numbers/bools/nulls
// are preserved.
func redactAllScalars(v interface{}, stats *RedactionStats) interface{} {
	switch t := v.(type) {
	case string:
		stats.Section++
		return redactStringValue(t)
	case map[string]interface{}:
		res := make(map[string]interface{}, len(t))
		for k, val := range t {
			res[k] = redactAllScalars(val, stats)
		}
		return res
	case []interface{}:
		arr := make([]interface{}, len(t))
		for i, item := range t {
			arr[i] = redactAllScalars(item, stats)
		}
		return arr
	default:
		return v
	}
}

// redactJSON parses, redacts, and re-serializes a JSON document. Numbers are
// decoded with UseNumber so large/precise integers survive the round trip.
func redactJSON(data []byte, stats *RedactionStats) ([]byte, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var v interface{}
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	// A trailing non-whitespace tail means this is not a single JSON document.
	if dec.More() {
		return nil, fmt.Errorf("unexpected trailing content")
	}
	red := redactValue(v, 0, stats)
	out, err := json.MarshalIndent(red, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(out, '\n'), nil
}

// redactYAML parses, redacts, and re-serializes a YAML document.
func redactYAML(data []byte, stats *RedactionStats) ([]byte, error) {
	var v interface{}
	if err := yaml.Unmarshal(data, &v); err != nil {
		return nil, err
	}
	red := redactValue(v, 0, stats)
	out, err := yaml.Marshal(red)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// redactJSONLines processes a JSON-lines file: each non-blank line is parsed as
// a standalone JSON document, redacted structurally, and re-marshaled. Lines
// that fail to parse fall back to full line-based text redaction — sensitive
// key=value pairs AND value-pattern scrubbing (Bearer/AKIA/connection strings)
// — applied per-line so that already-redacted JSON lines are never re-scanned.
// Without the value-pattern pass a secret-shaped value on a non-JSON line would
// leak even though the file is labeled structured_jsonl in the manifest.
// Returns the redacted bytes and whether at least one line parsed as JSON.
func redactJSONLines(data []byte, stats *RedactionStats) ([]byte, bool) {
	lines := bytes.Split(data, []byte("\n"))
	out := make([][]byte, len(lines))
	anyParsed := false
	for i, line := range lines {
		trimmed := bytes.TrimSpace(line)
		if len(trimmed) == 0 {
			out[i] = line
			continue
		}
		var v interface{}
		if err := json.Unmarshal(trimmed, &v); err != nil {
			out[i] = redactValuePatternsInText(redactKVLine(line, stats), stats)
			continue
		}
		anyParsed = true
		red := redactValue(v, 0, stats)
		b, err := json.Marshal(red)
		if err != nil {
			out[i] = redactValuePatternsInText(redactKVLine(line, stats), stats)
			continue
		}
		out[i] = b
	}
	return bytes.Join(out, []byte("\n")), anyParsed
}

// lineKVPattern matches a leading "key<sep>value" pair in a single line, where
// sep is ':' or '='. Used to redact .env / INI / simple key-value files.
var lineKVPattern = regexp.MustCompile(`^(\s*)([A-Za-z_][A-Za-z0-9_.\-]*)(\s*[:=]\s*)(.*)$`)

// redactText applies line-based redaction to unstructured text: first key=value
// pairs whose key is sensitive, then value-pattern scrubbing anywhere in the
// line. KV-first ordering avoids double-counting a value redacted by both
// passes.
func redactText(data []byte, stats *RedactionStats) []byte {
	lines := bytes.Split(data, []byte("\n"))
	for i, line := range lines {
		lines[i] = redactKVLine(line, stats)
	}
	joined := bytes.Join(lines, []byte("\n"))
	return redactValuePatternsInText(joined, stats)
}

// redactKVLine redacts a single line that is a sensitive key=value pair.
func redactKVLine(line []byte, stats *RedactionStats) []byte {
	m := lineKVPattern.FindSubmatch(line)
	if m == nil {
		return line
	}
	key := string(m[2])
	if !isSensitiveKey(key) {
		return line
	}
	val := string(m[4])
	if val == "" {
		return line
	}
	stats.FieldName++
	return []byte(string(m[1]) + key + string(m[3]) + redactStringValue(val))
}

// redactValuePatternsInText scrubs bearer tokens, AWS keys, and connection-
// string passwords anywhere they appear in a byte slice (counts each match).
func redactValuePatternsInText(data []byte, stats *RedactionStats) []byte {
	s := string(data)
	s = connStrTextPattern.ReplaceAllStringFunc(s, func(m string) string {
		sub := connStrTextPattern.FindStringSubmatch(m)
		stats.ValuePattern++
		return sub[1] + sub[2] + ":***@"
	})
	s = bearerTextPattern.ReplaceAllStringFunc(s, func(m string) string {
		stats.ValuePattern++
		return redactStringValue(m)
	})
	s = awsKeyTextPattern.ReplaceAllStringFunc(s, func(m string) string {
		stats.ValuePattern++
		return "***REDACTED***"
	})
	return []byte(s)
}

// fileClass labels how a file was processed for the manifest.
type fileClass string

const (
	classJSON     fileClass = "structured_json"
	classYAML     fileClass = "structured_yaml"
	classJSONL    fileClass = "structured_jsonl"
	classText     fileClass = "text"
	classBinary   fileClass = "binary_as_is"
	classUnparsed fileClass = "unparsed_as_is"
)

// isBinary reports whether data looks like a binary file (NUL byte or invalid
// UTF-8 in the first 8 KiB).
func isBinary(data []byte) bool {
	n := len(data)
	if n > 8192 {
		n = 8192
	}
	chunk := data[:n]
	if bytes.IndexByte(chunk, 0) >= 0 {
		return true
	}
	return !utf8.Valid(chunk)
}

// processFileContent dispatches a file to the right redaction strategy based on
// its name/extension and content. Returns the (possibly redacted) bytes and the
// processing class for the manifest.
func processFileContent(name string, data []byte, stats *RedactionStats) ([]byte, fileClass) {
	if isBinary(data) {
		return data, classBinary
	}
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".json":
		if out, err := redactJSON(data, stats); err == nil {
			return out, classJSON
		}
		// Malformed JSON: fall through to line-based text redaction.
		return redactText(data, stats), classUnparsed
	case ".yaml", ".yml":
		if out, err := redactYAML(data, stats); err == nil {
			return out, classYAML
		}
		return redactText(data, stats), classUnparsed
	case ".jsonl", ".ndjson":
		if out, ok := redactJSONLines(data, stats); ok {
			return out, classJSONL
		}
		return redactText(data, stats), classText
	default:
		// .env, .md, .log, .txt, unknown text: line-based redaction.
		return redactText(data, stats), classText
	}
}

// -----------------------------------------------------------------------------
// Bundling
// -----------------------------------------------------------------------------

// defaultBundleSources are the repo-relative roots bundled by default. They are
// harness machinery (not project-specific), so this list is domain-free.
func defaultBundleSources() []string {
	return []string{
		".opencode/state",
		".local/coordinator",
		".local/config",
		"docs/checkpoints",
	}
}

// shouldExcludePath reports whether a repo-relative path should be skipped
// (build artifacts, VCS internals, external study clones, the output dir).
func shouldExcludePath(rel string) bool {
	for _, seg := range strings.Split(rel, "/") {
		switch seg {
		case "refs", ".git", ".hg", ".svn", "node_modules", "tmp",
			"__pycache__", ".DS_Store", "dist", "build", "target":
			return true
		}
	}
	return false
}

// bundleFile is one redacted file staged for the archive.
type bundleFile struct {
	rel      string // repo-relative, forward slashes
	content  []byte // redacted
	original int64  // original size in bytes
	class    fileClass
}

// collectBundleFiles walks the default sources under repoRoot, redacts each
// regular file in memory, and returns the staged files, excluded paths, and
// accumulated redaction stats.
func collectBundleFiles(repoRoot string) (files []bundleFile, excluded []string, stats RedactionStats, err error) {
	seen := map[string]bool{}
	for _, src := range defaultBundleSources() {
		root := filepath.Join(repoRoot, filepath.FromSlash(src))
		fi, statErr := os.Stat(root)
		if statErr != nil {
			continue // source absent — fine.
		}
		if !fi.IsDir() {
			continue
		}
		werr := filepath.WalkDir(root, func(path string, d fs.DirEntry, werr error) error {
			if werr != nil {
				return werr
			}
			rel, rerr := filepath.Rel(repoRoot, path)
			if rerr != nil {
				return nil
			}
			rel = filepath.ToSlash(rel)
			if d.IsDir() {
				if shouldExcludePath(rel) {
					excluded = append(excluded, rel+"/")
					return filepath.SkipDir
				}
				return nil
			}
			if !d.Type().IsRegular() {
				return nil // skip symlinks, sockets, devices.
			}
			if shouldExcludePath(rel) {
				excluded = append(excluded, rel)
				return nil
			}
			if seen[rel] {
				return nil
			}
			seen[rel] = true
			data, rerr := os.ReadFile(path)
			if rerr != nil {
				excluded = append(excluded, rel)
				return nil
			}
			red, class := processFileContent(rel, data, &stats)
			files = append(files, bundleFile{
				rel:      rel,
				content:  red,
				original: int64(len(data)),
				class:    class,
			})
			return nil
		})
		if werr != nil {
			return nil, nil, stats, werr
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].rel < files[j].rel })
	sort.Strings(excluded)
	return files, excluded, stats, nil
}

// findRepoRoot walks up from cwd looking for a directory containing .git.
// Falls back to cwd if none is found.
func findRepoRoot(cwd string) string {
	dir := cwd
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return cwd
}

// resolveOutputPath resolves the archive output path relative to repoRoot,
// defaulting to tmp/diagnostics-<timestamp>.tar.gz. The resolved path must stay
// inside the repo root; an absolute --output that escapes the repo is refused
// (the tool never writes outside the repo).
//
// Symlinks are resolved BEFORE the containment check: both the repo root and
// the output path's existing prefix are evaluated with filepath.EvalSymlinks so
// an in-repo symlinked directory cannot route --output outside the repo. The
// returned path is the real on-disk location the write will land at.
func resolveOutputPath(repoRoot, flag string) (string, error) {
	var p string
	if flag == "" {
		ts := time.Now().UTC().Format("20060102-150405")
		p = filepath.Join(repoRoot, "tmp", "diagnostics-"+ts+".tar.gz")
	} else if filepath.IsAbs(flag) {
		p = flag
	} else {
		p = filepath.Join(repoRoot, flag)
	}
	abs, aerr := filepath.Abs(p)
	if aerr != nil {
		return "", aerr
	}
	rootAbs, _ := filepath.Abs(repoRoot)
	rootResolved, rerr := filepath.EvalSymlinks(rootAbs)
	if rerr != nil {
		// Root missing or unresolvable: fail closed rather than run a
		// containment check against a possibly-spoofed lexical path.
		return "", fmt.Errorf("cannot resolve repo root %s: %w", rootAbs, rerr)
	}
	outResolved, oerr := resolveSymlinkSafe(abs)
	if oerr != nil {
		return "", oerr
	}
	if err := assertRepoContained(rootResolved, outResolved); err != nil {
		return "", err
	}
	return outResolved, nil
}

// resolveSymlinkSafe returns abs with symlinks resolved for the portion of the
// path that already exists on disk. Non-existent trailing components (the
// archive file itself, plus any parent directories that will be created by
// MkdirAll) are left literal — they cannot be symlinks yet, so they cannot
// reroute the eventual write. This lets resolveOutputPath see the REAL location
// an attacker could reach via a pre-existing symlinked directory inside the
// repo, defeating a lexical-only containment check.
func resolveSymlinkSafe(abs string) (string, error) {
	resolved, err := filepath.EvalSymlinks(abs)
	if err == nil {
		return resolved, nil
	}
	if !os.IsNotExist(err) {
		return "", err
	}
	// Tail does not exist yet: resolve its parent (recursing up to the first
	// existing ancestor — the filesystem root always exists, so this
	// terminates) and re-attach the missing suffix.
	parent, perr := resolveSymlinkSafe(filepath.Dir(abs))
	if perr != nil {
		return "", perr
	}
	return filepath.Join(parent, filepath.Base(abs)), nil
}

// assertRepoContained returns an error if resolvedPath — already symlink-
// resolved on disk — escapes rootResolved. The check is lexical (filepath.Rel
// with a leading ".."), which is correct only when both arguments are fully
// resolved; callers MUST pass EvalSymlinks results. resolveOutputPath (pre-
// write) and writeArchive (post-MkdirAll) both route through here so the gate
// logic lives in exactly one place.
func assertRepoContained(rootResolved, resolvedPath string) error {
	rel, err := filepath.Rel(rootResolved, resolvedPath)
	if err != nil {
		return fmt.Errorf("cannot relate %s to repo root %s: %w", resolvedPath, rootResolved, err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("path %s resolves outside the repo root %s", resolvedPath, rootResolved)
	}
	return nil
}

// resolveArchiveParent creates the output parent directory (if needed), then
// re-resolves it now that it exists on disk and re-checks that it still sits
// inside rootResolved. It returns the freshly-resolved parent directory and the
// archive base name; the caller stages the archive via a temp file + os.Rename
// (see writeArchive) so the FINAL component — the archive filename itself —
// cannot be a pre-planted symlink escape.
//
// This closes the parent-swap TOCTOU window (F-B1): resolveOutputPath inspects
// outPath's PARENT before it exists, so resolveSymlinkSafe treats the missing
// tail as a literal suffix. A concurrent attacker who swaps that parent for a
// symlink pointing outside the repo after MkdirAll materializes it would route
// the write out of the repo. Re-running EvalSymlinks + the containment check on
// the materialized parent (and writing from the resolved path, not the original
// flag path) defeats that swap.
func resolveArchiveParent(outPath, rootResolved string) (resolvedParent, base string, err error) {
	parent := filepath.Dir(outPath)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return "", "", err
	}
	// The parent now exists; re-resolve so a symlink introduced since
	// resolveOutputPath's check is followed rather than hidden behind a
	// literal missing-tail suffix.
	resolvedParent, err = filepath.EvalSymlinks(parent)
	if err != nil {
		return "", "", fmt.Errorf("resolve output parent %s: %w", parent, err)
	}
	if err := assertRepoContained(rootResolved, resolvedParent); err != nil {
		return "", "", err
	}
	return resolvedParent, filepath.Base(outPath), nil
}

// writeArchive writes the redacted files plus manifest.json to a gzipped tar at
// outPath. Content files are stored at their repo-relative paths; manifest.json
// sits at the archive root.
//
// repoRoot is re-resolved here (not trusted from resolveOutputPath) and the
// materialized output parent is re-checked for containment immediately before
// the archive is written — see resolveArchiveParent for the parent-swap TOCTOU
// rationale.
//
// The archive is staged via a temp file created inside the contained parent and
// then atomically renamed onto the final path. On Linux rename(2) does NOT
// follow a symlink on the destination: it atomically replaces the destination
// directory entry itself. So if the archive filename is a pre-planted symlink
// pointing outside the repo (the c-F1 final-component escape), the rename
// replaces that symlink with the regular file and leaves the outside target
// untouched. This closes the vector with NO TOCTOU window: the temp name is
// unpredictable and O_EXCL-protected (an attacker cannot pre-plant it), and the
// rename does not evaluate the destination symlink at all — so there is no
// EvalSymlinks-then-open gap for an attacker to race. This mirrors the atomic-
// replace pattern in replaceInPlace (selfupdate.go).
func writeArchive(repoRoot, outPath string, files []bundleFile, m bundleManifest) error {
	rootAbs, _ := filepath.Abs(repoRoot)
	rootResolved, rerr := filepath.EvalSymlinks(rootAbs)
	if rerr != nil {
		return fmt.Errorf("cannot resolve repo root %s: %w", rootAbs, rerr)
	}
	resolvedParent, base, err := resolveArchiveParent(outPath, rootResolved)
	if err != nil {
		return err
	}
	// Stage inside the contained parent with an unpredictable name so an
	// attacker cannot pre-plant a symlink at it. os.CreateTemp opens with
	// O_CREAT|O_EXCL (mode 0o600), so a colliding entry — symlink or regular
	// file — makes the open fail rather than follow.
	tmp, err := os.CreateTemp(resolvedParent, ".diagnostics-export-*.tmp")
	if err != nil {
		return fmt.Errorf("create diagnostics archive temp file in %s: %w", resolvedParent, err)
	}
	tmpName := tmp.Name()
	// Best-effort cleanup: on any failure the temp file is removed; on success
	// os.Rename moves it away and this Remove becomes a no-op (the now-moved
	// path returns an error we intentionally ignore).
	defer os.Remove(tmpName)

	if err := streamArchive(tmp, files, m); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close diagnostics archive temp file: %w", err)
	}

	finalPath := filepath.Join(resolvedParent, base)
	// Atomically install the staged archive. rename(2) replaces the destination
	// directory entry (a planted symlink included) without following it, so the
	// write cannot escape via the final component.
	if err := os.Rename(tmpName, finalPath); err != nil {
		return fmt.Errorf("install diagnostics archive at %s: %w", finalPath, err)
	}
	return nil
}

// streamArchive writes the gzipped tar (redacted files + manifest.json) to w.
// manifest.json sits at the archive root; content files are stored at their
// repo-relative paths. The writers are closed explicitly in dependency order on
// the success path so a truncated footer is surfaced as an error (rather than
// silently renamed into place); a deferred best-effort close covers the error
// paths.
func streamArchive(w io.Writer, files []bundleFile, m bundleManifest) error {
	gz := gzip.NewWriter(w)
	tw := tar.NewWriter(gz)
	defer func() {
		tw.Close()
		gz.Close()
	}()
	for _, bf := range files {
		if err := writeTarEntry(tw, bf.rel, bf.content); err != nil {
			return err
		}
	}
	manifestBytes, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	manifestBytes = append(manifestBytes, '\n')
	if err := writeTarEntry(tw, "manifest.json", manifestBytes); err != nil {
		return err
	}
	// Explicit close in dependency order (the tar footer flushes through gzip)
	// so a flush error is returned, not swallowed. The deferred closes above
	// then run as harmless no-ops on already-closed writers.
	if err := tw.Close(); err != nil {
		return fmt.Errorf("close tar writer: %w", err)
	}
	if err := gz.Close(); err != nil {
		return fmt.Errorf("close gzip writer: %w", err)
	}
	return nil
}

func writeTarEntry(tw *tar.Writer, name string, data []byte) error {
	hdr := &tar.Header{
		Name:     name,
		Mode:     0o644,
		Size:     int64(len(data)),
		ModTime:  time.Now().UTC(),
		Typeflag: tar.TypeReg,
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	_, err := tw.Write(data)
	return err
}

// --- manifest types -------------------------------------------------------

type redactionReport struct {
	FieldName    int `json:"field_name"`
	Section      int `json:"section"`
	ValuePattern int `json:"value_pattern"`
}

type fileEntry struct {
	Path  string `json:"path"`
	Bytes int64  `json:"bytes"`
	Class string `json:"class"`
}

type bundleManifest struct {
	Tool       string          `json:"tool"`
	Version    string          `json:"version"`
	CreatedAt  string          `json:"created_at"`
	RepoRoot   string          `json:"repo_root"`
	DryRun     bool            `json:"dry_run"`
	Output     string          `json:"output,omitempty"`
	TotalBytes int64           `json:"total_bytes"`
	Included   []fileEntry     `json:"included"`
	Excluded   []string        `json:"excluded"`
	Redaction  redactionReport `json:"redaction"`
}

func buildManifest(repoRoot, outPath string, files []bundleFile, excluded []string, stats RedactionStats, dryRun bool, totalOriginal int64) bundleManifest {
	m := bundleManifest{
		Tool:       "vh-agent-harness diagnostics-export",
		Version:    VersionString(),
		CreatedAt:  time.Now().UTC().Format(time.RFC3339),
		RepoRoot:   repoRoot,
		DryRun:     dryRun,
		Output:     outPath,
		TotalBytes: totalOriginal,
		Excluded:   excluded,
		Redaction: redactionReport{
			FieldName:    stats.FieldName,
			Section:      stats.Section,
			ValuePattern: stats.ValuePattern,
		},
	}
	for _, f := range files {
		m.Included = append(m.Included, fileEntry{
			Path:  f.rel,
			Bytes: f.original,
			Class: string(f.class),
		})
	}
	return m
}

func printManifestSummary(out io.Writer, m bundleManifest) {
	fmt.Fprintln(out, "diagnostics-export dry-run — no archive written")
	fmt.Fprintf(out, "  repo root:   %s\n", m.RepoRoot)
	fmt.Fprintf(out, "  would write: %s\n", m.Output)
	fmt.Fprintf(out, "  included:    %d files (%d bytes original)\n", len(m.Included), m.TotalBytes)
	fmt.Fprintf(out, "  excluded:    %d paths\n", len(m.Excluded))
	fmt.Fprintf(out, "  redaction:   %d field-name, %d section, %d value-pattern\n",
		m.Redaction.FieldName, m.Redaction.Section, m.Redaction.ValuePattern)
	if len(m.Included) > 0 {
		fmt.Fprintln(out, "  files:")
		for _, e := range m.Included {
			fmt.Fprintf(out, "    %-18s %9d  %s\n", e.Class, e.Bytes, e.Path)
		}
	}
	if len(m.Excluded) > 0 {
		fmt.Fprintln(out, "  excluded paths:")
		for _, e := range m.Excluded {
			fmt.Fprintf(out, "    %s\n", e)
		}
	}
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Review the redaction counts above. Re-run without --dry-run to write the archive.")
}

// -----------------------------------------------------------------------------
// Command
// -----------------------------------------------------------------------------

var (
	diagDryRun bool
	diagOutput string
)

var diagnosticsExportCmd = &cobra.Command{
	Use:           "diagnostics-export",
	Short:         "Bundle harness state into a redacted, shareable diagnostics archive",
	SilenceUsage:  true,
	SilenceErrors: true,
	Long: `Bundle selected harness state into a single tar.gz archive with
field-aware secret redaction.

This is operator debugging/support tooling: package session memory, local
coordinator state, operator config, and recent checkpoints to share with a
maintainer or archive for a complex issue.

Bundled (repo-relative):
  .opencode/state/    session + workstream memory (the primary payload)
  .local/coordinator/ local task registry, research runs (if present)
  .local/config/      operator config (if present; high redaction priority)
  docs/checkpoints/   dated progress snapshots (if present)

Excluded: refs/, .git/, node_modules/, build artifacts, tmp/ (never itself).

Redaction (field-aware, applied before archiving):
  - field-name: keys like apiKey/token/secret/password/credential/auth are
    replaced with ***REDACTED(Nchars)***.
  - whole-section: env/secrets/credentials blocks (and top-level models config)
    have every value redacted.
  - value-pattern: Bearer tokens, AWS-style keys, and connection-string
    passwords are scrubbed.
  - non-sensitive fields (paths, timestamps, ids, statuses, body text) survive.

The archive is written to repo-scoped tmp/ and is NEVER auto-uploaded. The
operator decides if/when to share it. A manifest.json inside the archive lists
included/excluded paths and redaction counts.

Flags:
  --dry-run        print the manifest (included/excluded paths, redaction
                   counts, output path) without writing the archive.
  --output <path>  archive path (default tmp/diagnostics-<timestamp>.tar.gz).
                   Relative paths resolve against the repo root; the resolved
                   path must stay inside the repo.`,
	Args: cobra.NoArgs,
	RunE: runDiagnosticsExport,
}

func init() {
	diagnosticsExportCmd.Flags().BoolVar(&diagDryRun, "dry-run", false,
		"print the bundle manifest without writing the archive")
	diagnosticsExportCmd.Flags().StringVar(&diagOutput, "output", "",
		"output archive path (default tmp/diagnostics-<timestamp>.tar.gz)")
}

func runDiagnosticsExport(cmd *cobra.Command, _ []string) (err error) {
	defer func() { reportRunErrToStderr(cmd, err) }()
	out := cmd.OutOrStdout()

	cwd, gerr := os.Getwd()
	if gerr != nil {
		return gerr
	}
	repoRoot := findRepoRoot(cwd)

	files, excluded, stats, ferr := collectBundleFiles(repoRoot)
	if ferr != nil {
		return ferr
	}

	var totalOriginal int64
	for _, f := range files {
		totalOriginal += f.original
	}

	outPath, oerr := resolveOutputPath(repoRoot, diagOutput)
	if oerr != nil {
		return oerr
	}

	if diagDryRun {
		m := buildManifest(repoRoot, outPath, files, excluded, stats, true, totalOriginal)
		printManifestSummary(out, m)
		return nil
	}

	m := buildManifest(repoRoot, outPath, files, excluded, stats, false, totalOriginal)
	if werr := writeArchive(repoRoot, outPath, files, m); werr != nil {
		return werr
	}

	var archiveSize int64
	if fi, serr := os.Stat(outPath); serr == nil {
		archiveSize = fi.Size()
	}
	fmt.Fprintf(out, "wrote %s (%d bytes)\n", outPath, archiveSize)
	fmt.Fprintf(out, "files: %d included, %d excluded (%d bytes original)\n",
		len(files), len(excluded), totalOriginal)
	fmt.Fprintf(out, "redaction: %d field-name, %d section, %d value-pattern\n",
		stats.FieldName, stats.Section, stats.ValuePattern)
	fmt.Fprintln(out)
	fmt.Fprintln(out, "The archive is local only and was NOT uploaded.")
	fmt.Fprintln(out, "Inspect manifest.json inside the archive, verify redaction, then share manually if desired.")
	return nil
}
