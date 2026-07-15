package schema

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// skillNamePattern is the OpenCode skill-name rule: lowercase alphanumeric
// runs joined by single hyphens (e.g. "repo-recon", "bgshell-job"). Ported from
// templates/core/.opencode/skills/skill-creator/scripts/quick_validate.py.
var skillNamePattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

const (
	skillNameMaxLen        = 64
	skillDescriptionMaxLen = 1024
)

// skillAllowedFrontmatterKeys is the allow-list of frontmatter keys accepted
// by a SKILL.md, ported verbatim from quick_validate.py's ALLOWED_PROPERTIES.
// The Go struct below models only name/description/compatibility (the
// validated fields); license and metadata are permitted keys that are not
// validated further, matching the python reference.
var skillAllowedFrontmatterKeys = map[string]bool{
	"name":          true,
	"description":   true,
	"license":       true,
	"compatibility": true,
	"metadata":      true,
}

// skillFrontmatter is the typed projection of a SKILL.md YAML frontmatter
// block. Only the validated fields are modeled (name, description,
// compatibility); license and metadata are permitted by the allow-list check
// in ValidateSkillFrontmatter but not modeled here, matching
// quick_validate.py's ALLOWED_PROPERTIES.
type skillFrontmatter struct {
	Name          string `yaml:"name"`
	Description   string `yaml:"description"`
	Compatibility string `yaml:"compatibility"`
}

// ValidateSkillFrontmatter validates the YAML frontmatter of a SKILL.md file.
// It is a Go port of the skill-creator's quick_validate.py (frontmatter-only
// checks), taking the raw SKILL.md content and the skill's directory name.
//
// dirName is the containing skill directory name and MUST equal the frontmatter
// `name` field (e.g. .opencode/skills/repo-recon/SKILL.md has dirName "repo-recon"
// and name "repo-recon").
//
// Checks:
//   - frontmatter is present and parses as YAML;
//   - frontmatter keys are limited to {name, description, license, compatibility,
//     metadata}; any other key is rejected (matches quick_validate.py's
//     ALLOWED_PROPERTIES — unknown keys are NOT silently accepted);
//   - name: matches ^[a-z0-9]+(-[a-z0-9]+)*$, <=64 chars, equals dirName;
//   - description: non-empty, <=1024 chars (after trim), no angle brackets;
//   - compatibility: if present, must equal "opencode".
//
// This is a standalone validator (skills are not schema'd files), so it does not
// implement the Validator interface. It returns nil on success or a descriptive
// error naming the first violation.
func ValidateSkillFrontmatter(content []byte, dirName string) error {
	raw, err := extractSkillFrontmatter(content)
	if err != nil {
		return err
	}

	// First pass into a generic map: collect the raw frontmatter keys and
	// reject any outside the allow-list, mirroring quick_validate.py's
	// ALLOWED_PROPERTIES check. This MUST run before struct unmarshalling,
	// which silently drops unknown keys (so an unsupported key like `version`
	// would otherwise pass the validator).
	var rawKeys map[string]any
	if err := yaml.Unmarshal(raw, &rawKeys); err != nil {
		return fmt.Errorf("invalid YAML in frontmatter: %w", err)
	}
	var unexpected []string
	for k := range rawKeys {
		if !skillAllowedFrontmatterKeys[k] {
			unexpected = append(unexpected, k)
		}
	}
	if len(unexpected) > 0 {
		sort.Strings(unexpected)
		return fmt.Errorf("frontmatter: unexpected frontmatter key(s): %s", strings.Join(unexpected, ", "))
	}

	// Second pass into the typed projection for the validated fields.
	var fm skillFrontmatter
	if err := yaml.Unmarshal(raw, &fm); err != nil {
		return fmt.Errorf("invalid YAML in frontmatter: %w", err)
	}

	// name: present, well-formed, length, and matches the directory.
	name := strings.TrimSpace(fm.Name)
	if name == "" {
		return errors.New("frontmatter: missing or empty 'name'")
	}
	if !skillNamePattern.MatchString(name) {
		return errors.New("frontmatter: 'name' must be lowercase alphanumeric with single hyphen separators (^[a-z0-9]+(-[a-z0-9]+)*$)")
	}
	if len(name) > skillNameMaxLen {
		return fmt.Errorf("frontmatter: 'name' is too long (%d chars; maximum is %d)", len(name), skillNameMaxLen)
	}
	if name != dirName {
		return fmt.Errorf("frontmatter: 'name' %q must match the skill directory name %q", name, dirName)
	}

	// description: present, length, and no angle brackets (HTML/markdown noise).
	desc := fm.Description
	if strings.TrimSpace(desc) == "" {
		return errors.New("frontmatter: missing or empty 'description'")
	}
	if len(strings.TrimSpace(desc)) > skillDescriptionMaxLen {
		return fmt.Errorf("frontmatter: 'description' is too long (%d chars; maximum is %d)", len(strings.TrimSpace(desc)), skillDescriptionMaxLen)
	}
	if strings.Contains(desc, "<") || strings.Contains(desc, ">") {
		return errors.New("frontmatter: 'description' cannot contain angle brackets ('<' or '>')")
	}

	// compatibility: optional, but when present must target opencode.
	if fm.Compatibility != "" && fm.Compatibility != "opencode" {
		return fmt.Errorf("frontmatter: 'compatibility' must be 'opencode' when provided (got %q)", fm.Compatibility)
	}

	return nil
}

// extractSkillFrontmatter returns the YAML bytes between the opening and closing
// "---" delimiters of a SKILL.md file. It mirrors quick_validate.py's regex
// extraction but is line-based so it tolerates CRLF and trailing content after
// the closing delimiter.
func extractSkillFrontmatter(content []byte) ([]byte, error) {
	if len(content) == 0 {
		return nil, errors.New("SKILL.md is empty or missing (no frontmatter)")
	}
	lines := strings.Split(string(content), "\n")
	if len(lines) == 0 {
		return nil, errors.New("SKILL.md is empty or missing (no frontmatter)")
	}
	// Exact match on the opening delimiter (not HasPrefix) so "----" or "---foo"
	// is rejected, matching quick_validate.py's regex `^---\n(.*?)\n---`. The
	// closing-delimiter side below is already an exact-equality check.
	if strings.TrimRight(lines[0], "\r") != "---" {
		return nil, errors.New("no YAML frontmatter found (SKILL.md must start with ---)")
	}
	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimRight(lines[i], "\r") == "---" {
			end = i
			break
		}
	}
	if end == -1 {
		return nil, errors.New("invalid frontmatter: missing closing '---' delimiter")
	}
	return []byte(strings.Join(lines[1:end], "\n")), nil
}
