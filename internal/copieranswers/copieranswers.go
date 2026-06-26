// Package copieranswers is a read-only reader for Copier's `.copier-answers.yml`
// cache file. In the D3-B model Copier's answers file is NOT policy authority:
// it is Copier's own render/replay cache (and, per the spike finding, may be
// fragile or absent in some environments). The shim-owned lineage.yml is the
// stable authority. doctor compares the two; this package only loads the
// Copier side into a shape doctor can diff against lineage.
//
// The file is read-only by design: the shim never writes `.copier-answers.yml`.
// When Copier is wired in as the render substrate, Copier will produce it and
// this reader will reflect that.
package copieranswers

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/vhqtvn/vh-agent-harness/internal/lineage"
)

// FileName is the conventional name Copier uses for its answers cache.
const FileName = ".copier-answers.yml"

// FilePath returns the path to Copier's answers cache under targetDir.
func FilePath(targetDir string) string {
	return filepath.Join(targetDir, FileName)
}

// CopierAnswers is the read-only projection of `.copier-answers.yml` used for
// drift comparison. It carries Copier's recorded template origin plus the
// user-rendered answers (everything except the `_`-prefixed Copier meta keys).
type CopierAnswers struct {
	// SrcPath mirrors Copier's `_src_path` (the template origin it replayed).
	SrcPath string
	// Commit mirrors Copier's `_commit` (the pinned template commit; "" if unset).
	Commit string
	// Answers are the user answer pairs (Copier keys without the `_` prefix).
	// doctor feeds these to lineage.DigestOf so the digest is comparable to
	// the lineage answers digest.
	Answers map[string]string
}

// Read loads Copier's answers cache under targetDir. A missing file returns
// (nil, nil) so callers can treat "Copier cache absent" as a normal state
// (this is the spike's failure mode and exactly the gap D3-B covers). A
// present-but-unparseable file returns an error.
func Read(targetDir string) (*CopierAnswers, error) {
	data, err := os.ReadFile(FilePath(targetDir))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse %s: %w", FilePath(targetDir), err)
	}
	out := &CopierAnswers{Answers: map[string]string{}}
	for k, v := range raw {
		s, ok := stringValue(v)
		if !ok {
			continue
		}
		switch {
		case k == "_src_path":
			out.SrcPath = s
		case k == "_commit":
			out.Commit = s
		case strings.HasPrefix(k, "_"):
			// other Copier internal meta keys (e.g. _commit, _src_path already
			// handled, _answers_file, _extensions, _copy_without_render, ...)
			// are intentionally excluded from the answer digest.
		default:
			out.Answers[k] = s
		}
	}
	return out, nil
}

// AnswerDigest recomputes the lineage-style digest over the Copier answers so
// it can be compared directly to Lineage.Answers.Digest. Returns "" when there
// are no answers.
func (c *CopierAnswers) AnswerDigest() string {
	if c == nil || len(c.Answers) == 0 {
		return ""
	}
	return lineage.DigestOf(c.Answers)
}

// stringValue reports whether v is a YAML scalar that should be treated as a
// string answer, returning its canonical string form.
func stringValue(v any) (string, bool) {
	switch t := v.(type) {
	case nil:
		return "", false
	case string:
		return t, true
	case bool:
		if t {
			return "true", true
		}
		return "false", true
	case int:
		return fmt.Sprintf("%d", t), true
	case float64:
		return fmt.Sprintf("%v", t), true
	default:
		// Maps / sequences are not scalar answers; skip them.
		return "", false
	}
}
