package cli

import (
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/vhqtvn/vh-agent-harness/internal/schema"
)

// harnessProfileName is the S3 feature-surface authority file, platform_armed.
// It lives at the target root and carries the project's profile/modules/features/
// overlays/policy_packs selections.
const harnessProfileName = ".vh-agent-harness/vh-harness-profile.yml"

// readProfileAnswers reads the LIVE S3 vh-harness-profile.yml at
// <target>/vh-harness-profile.yml (the platform_armed feature-surface authority)
// and projects it onto flat render answers the GoTemplateRenderer consumes via
// buildTemplateData:
//
//   - "features.<flag>" -> "true"/"false"  (one key per bool feature; v1 = backlog)
//   - "overlays"        -> comma-joined overlay pack names (e.g. "alpha-pack")
//
// On first install the live profile is absent (it is seeded FROM the platform
// default by the apply step), so render decisions fall back to defaults
// (backlog=false, no overlays). On update the live profile drives the re-render,
// so Slice-3 backlog conditionals and Slice-4 overlay selection resolve from the
// operator's actual decisions rather than the install-time answers.
//
// A missing OR invalid profile yields an empty map (defaults). Doctor reports the
// real validation error separately; render never aborts on a malformed profile.
func readProfileAnswers(target string) map[string]string {
	out := map[string]string{}
	raw, err := os.ReadFile(filepath.Join(target, harnessProfileName))
	if err != nil {
		return out
	}
	// Lint via the schema registry before trusting the projection. A malformed
	// profile cannot drive a render; fall back to defaults and let doctor flag it.
	if errs := (schema.HarnessProfile{}).Validate(raw); len(errs) > 0 {
		return out
	}
	return projectProfileAnswers(raw)
}

// projectProfileAnswers is the pure projection of a validated vh-harness-profile.yml
// blob onto render answers. It is split from readProfileAnswers so Slice-4 overlay
// selection and tests can reuse the projection without re-reading the file.
func projectProfileAnswers(raw []byte) map[string]string {
	out := map[string]string{}
	// Reuse the schema's own struct shape (projection only; validation already ran).
	var d struct {
		Profile  string          `yaml:"profile"`
		Modules  []string        `yaml:"modules"`
		Features map[string]bool `yaml:"features"`
		Overlays []string        `yaml:"overlays"`
	}
	if err := yaml.Unmarshal(raw, &d); err != nil {
		return out
	}
	for k, v := range d.Features {
		out["features."+k] = boolStr(v)
	}
	if len(d.Overlays) > 0 {
		out["overlays"] = strings.Join(d.Overlays, ",")
	}
	return out
}

// activeOverlays returns the overlay pack names selected in the live profile, in
// declared order. It is the Slice-4 overlay-selection read site (each pack is
// rendered + merged when present here). Missing/invalid profile -> no overlays.
func activeOverlays(target string) []string {
	raw, err := os.ReadFile(filepath.Join(target, harnessProfileName))
	if err != nil {
		return nil
	}
	if errs := (schema.HarnessProfile{}).Validate(raw); len(errs) > 0 {
		return nil
	}
	var d struct {
		Overlays []string `yaml:"overlays"`
	}
	if err := yaml.Unmarshal(raw, &d); err != nil {
		return nil
	}
	return d.Overlays
}

// boolStr renders a Go bool as the canonical string form buildTemplateData's
// coerce() recognizes ("true"/"false"), so {{ if .features.backlog }} evaluates
// the boolean rather than a non-empty string.
func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
