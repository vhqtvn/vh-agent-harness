package schema

import (
	"strings"
	"testing"
)

// validSkillFrontmatter is a known-good frontmatter block matching the dir name
// "repo-recon" (one of the 10 real core skills). Tests mutate it to produce
// failure cases.
const validSkillFrontmatter = `---
name: repo-recon
description: >
  Maps the repository structure: entrypoints, packages, tests, and hotspots.
  Use this to build or refresh the repo-recon data file.
compatibility: opencode
---

# repo-recon skill body
`

func TestValidateSkillFrontmatter(t *testing.T) {
	cases := []struct {
		name    string
		content string
		dirName string
		wantErr string // substring expected in the error; empty => expect nil
	}{
		{
			name:    "valid",
			content: validSkillFrontmatter,
			dirName: "repo-recon",
			wantErr: "",
		},
		{
			name: "bad name regex (uppercase + space)",
			content: `---
name: Bad Name
description: a valid description here
---
x
`,
			dirName: "bad-name",
			wantErr: "must be lowercase alphanumeric",
		},
		{
			name:    "name does not match directory",
			content: strings.Replace(validSkillFrontmatter, "name: repo-recon", "name: other-name", 1),
			dirName: "repo-recon",
			wantErr: "must match the skill directory name",
		},
		{
			name:    "description too long",
			content: "---\nname: repo-recon\ndescription: " + strings.Repeat("a", 1025) + "\n---\n",
			dirName: "repo-recon",
			wantErr: "'description' is too long",
		},
		{
			name: "description angle brackets",
			content: `---
name: repo-recon
description: uses <tags> and <more>
---
x
`,
			dirName: "repo-recon",
			wantErr: "cannot contain angle brackets",
		},
		{
			name:    "bad compatibility value",
			content: strings.Replace(validSkillFrontmatter, "compatibility: opencode", "compatibility: vscode", 1),
			dirName: "repo-recon",
			wantErr: "'compatibility' must be 'opencode'",
		},
		{
			name:    "missing SKILL.md (empty content)",
			content: "",
			dirName: "repo-recon",
			wantErr: "empty or missing",
		},
		{
			name:    "no frontmatter delimiter",
			content: "# just a heading, no frontmatter",
			dirName: "repo-recon",
			wantErr: "no YAML frontmatter",
		},
		{
			// Regression: the opener must be an EXACT "---" match, not a prefix.
			// quick_validate.py's regex `^---\n(.*?)\n---` rejects a "----"
			// opener (the char after the first --- must be \n). The previous Go
			// port used strings.HasPrefix, which wrongly accepted "----" (and
			// "---foo"); this pins the exact-match fix.
			name:    "opener not exact (---- rejected)",
			content: "----\nname: x\ndescription: y\n---\n",
			dirName: "x",
			wantErr: "no YAML frontmatter",
		},
		{
			// Same divergence class: an opener like "---foo" must be rejected
			// even though it HasPrefix "---".
			name:    "opener not exact (---foo rejected)",
			content: "---foo\nname: x\ndescription: y\n---\n",
			dirName: "x",
			wantErr: "no YAML frontmatter",
		},
		{
			name:    "name missing entirely",
			content: "---\ndescription: a description\n---\n",
			dirName: "repo-recon",
			wantErr: "missing or empty 'name'",
		},
		{
			name:    "description missing",
			content: "---\nname: repo-recon\n---\n",
			dirName: "repo-recon",
			wantErr: "missing or empty 'description'",
		},
		{
			name:    "name too long",
			content: "---\nname: " + strings.Repeat("a", 65) + "\ndescription: ok\n---\n",
			dirName: "repo-recon",
			wantErr: "'name' is too long",
		},
		{
			name:    "compatibility absent is allowed",
			content: strings.Replace(validSkillFrontmatter, "compatibility: opencode\n", "", 1),
			dirName: "repo-recon",
			wantErr: "",
		},
		// b-F1 regression: unsupported frontmatter keys must be rejected,
		// matching quick_validate.py's ALLOWED_PROPERTIES check. Previously the
		// Go port unmarshalled straight into the struct and silently dropped
		// unknown keys, so these wrongly passed.
		{
			name:    "b-F1 unsupported key (version) rejected",
			content: "---\nname: x\ndescription: y\nversion: 2\n---\n",
			dirName: "x",
			wantErr: "unexpected frontmatter key(s): version",
		},
		{
			name:    "b-F1 unsupported key (unsupported) rejected",
			content: "---\nname: x\ndescription: y\nunsupported: true\n---\n",
			dirName: "x",
			wantErr: "unexpected frontmatter key(s): unsupported",
		},
		{
			// license is in the allow-list even though the Go struct does not
			// model it; allowed keys must not be rejected.
			name:    "b-F1 allowed key (license) passes",
			content: "---\nname: x\ndescription: y\nlicense: mit\n---\n",
			dirName: "x",
			wantErr: "",
		},
		{
			// metadata is in the allow-list; a nested map value is fine since
			// the allow-list only inspects top-level keys.
			name:    "b-F1 allowed key (metadata) passes",
			content: "---\nname: x\ndescription: y\nmetadata:\n  foo: bar\n---\n",
			dirName: "x",
			wantErr: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateSkillFrontmatter([]byte(tc.content), tc.dirName)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("expected nil error, got: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error %q does not contain expected substring %q", err.Error(), tc.wantErr)
			}
		})
	}
}
