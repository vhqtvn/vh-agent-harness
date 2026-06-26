package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/vhqtvn/vh-agent-harness/internal/lineage"
	"github.com/vhqtvn/vh-agent-harness/internal/ownership"
)

// validOwnershipClassesList renders the six armed classes as a stable guidance
// string for invalid-override errors (sorted for deterministic messaging).
var validOwnershipClassesList = func() string {
	all := ownership.AllClasses()
	out := make([]string, len(all))
	for i, c := range all {
		out[i] = string(c)
	}
	return strings.Join(out, " | ")
}()

// This file is the Slice-5.1 LIVE wiring of the S2 ownership authority
// (harness-ownership.yml) into the seam apply path. The seam classifier now
// reads the project's raise-only overrides and feeds them to ownership.Resolve,
// so a downgrade override (e.g. trying to lower a project_owned path to
// platform_managed so a plain render may clobber it) is REJECTED at apply time
// (decision D2-A). Until Slice 5 the classifier resolved with nil overrides, so
// the raise-only rule was unit-tested but never enforced end-to-end.
//
// S2 authority file: <target>/.vh-agent-harness/harness-ownership.yml. It is a
// project-authored, raise-only amendment to the platform module defaults. Shape:
//
//	overrides:
//	  opencode.jsonc:
//	    class: project_owned
//	    reason: "hand-curated; do not auto-render"
//
// Each override may only RAISE protection on the lattice
// (platform_managed < platform_armed < overlay_extension < project_owned); a
// downgrade, an unknown path, an invalid class, or an off-lattice class
// (external_generated / local_only) is rejected by ownership.Resolve and aborts
// the apply before any write touches the live tree.

// ownershipOverridesFileName is the S2 authority file inside the lineage dir.
const ownershipOverridesFileName = "harness-ownership.yml"

// ownershipOverridesFile is the on-disk path to the S2 overrides under a target.
func ownershipOverridesFile(target string) string {
	return filepath.Join(target, lineage.DirName, ownershipOverridesFileName)
}

// ownershipOverridesDoc is the minimal YAML envelope of harness-ownership.yml.
type ownershipOverridesDoc struct {
	Overrides map[string]ownershipOverrideEntry `yaml:"overrides"`
}

// ownershipOverrideEntry is one project-authored amendment.
type ownershipOverrideEntry struct {
	Class  string `yaml:"class"`
	Reason string `yaml:"reason"`
}

// readOwnershipOverrides loads the project's S2 raise-only overrides. A missing
// file is the common case (the project has not amended any defaults) and returns
// nil overrides + nil error. A present-but-unparseable file, or an entry whose
// class literal is not one of the six armed classes, is a hard error: the
// harness must never silently ignore a project-authored ownership amendment, and
// an invalid class literal cannot be honored.
//
// Path-resolution / lattice reasoning is deferred to ownership.Resolve: this
// reader only turns the YAML into typed Override values. A downgrade or unknown
// path surfaces as a Resolve error at classify time (seamClassifierWithOverlays),
// which aborts the apply before any write.
func readOwnershipOverrides(target string) (ownership.Overrides, error) {
	path := ownershipOverridesFile(target)
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // common case: no project overrides
		}
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var doc ownershipOverridesDoc
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if doc.Overrides == nil {
		return nil, nil
	}
	out := make(ownership.Overrides, len(doc.Overrides))
	for rel, entry := range doc.Overrides {
		class := ownership.Class(entry.Class)
		if !class.IsValid() {
			return nil, fmt.Errorf(
				"%s: override for %q uses unknown ownership class %q (must be one of: %s)",
				path, rel, entry.Class, validOwnershipClassesList)
		}
		out[filepath.ToSlash(rel)] = ownership.Override{Class: class, Reason: entry.Reason}
	}
	return out, nil
}
