package resolver

import "path/filepath"

// CoreSelectionPlan is the per-request view of which capability-owned core
// outputs are ACTIVE (the owning capability is selected) vs ALL-KNOWN (declared
// by any capability, selected or not). It is compiled once per render request
// from the merged catalog and the resolved capability selection, and consumed
// by three distinct concerns that must agree on the SAME path sets:
//
//   - RENDERER FILTERING — InactiveLivePaths are skipped at corpus-walk time so
//     unselected capabilities do not emit their owned files.
//   - ACTIVE OWNERSHIP — ActiveLivePaths become active managed outputs in the
//     ownership defaults (doctor's "managed" count, classifier's managed set).
//   - RESIDUE RECOGNITION — InactiveLivePaths are the prior-version residue
//     exempt from managed-drift and unexpected-drift failures (a
//     selected→deselected transition leaves the file on disk untouched; it is
//     neither deleted nor flagged).
//
// All path sets key by LIVE (suffix-stripped), forward-slash, source-relative
// (to templates/core) paths — the same key form the ownership map and the live
// tree use. A capability that owns a templated output (.tmpl/.template source)
// declares the suffix-stripped LIVE form in CoreOutputs; the plan carries that
// live form unchanged; the renderer maps the live form back to its source file
// at walk time by checking the suffix-stripped walked path against the exclude
// set.
//
// This type is PURE: it carries no corpus FS reference. Declared-source
// existence (a CoreOutputs path must correspond to a real file in
// templates/core) is checked by the CLI selection-plan consumer, which has
// embedded-corpus access; the resolver is leaf-level and cannot import the
// corpus package.
type CoreSelectionPlan struct {
	// ActiveLivePaths are the LIVE core output paths owned by SELECTED
	// capabilities. These files are eligible to enter staging and are active
	// managed outputs.
	ActiveLivePaths map[string]bool

	// AllKnownLivePaths are the LIVE core output paths declared by ANY
	// capability in the merged catalog (selected or not). This is the
	// superset used for residue recognition and overlay-collision checks —
	// it lets the CLI distinguish "inactive capability residue" (a known
	// path whose capability is unselected) from "genuinely unexpected file."
	AllKnownLivePaths map[string]bool

	// InactiveLivePaths are AllKnownLivePaths minus ActiveLivePaths — the
	// LIVE core output paths owned by UNSELECTED capabilities. These are the
	// residue candidates: if such a file exists on disk (from a prior
	// selected render), it is left untouched and exempt from drift.
	InactiveLivePaths map[string]bool
}

// CompileCoreSelectionPlan computes the active/all-known/inactive live-path
// views from the merged catalog and the resolved capability selection. It is
// PURE and allocation-light: it walks the catalog's CoreOutputs once, splitting
// each declared path into active or inactive based on whether the owning
// capability is in the selected set.
//
// catalog MUST be non-nil; selected MAY be nil (treated as empty — everything
// declared is inactive). The returned plan's maps are never nil (callers may
// range over them unconditionally). A catalog with no CoreOutputs-declaring
// capabilities yields a plan with three empty maps — equivalent to the
// pre-filtering behavior (nothing is excluded, nothing is inactive).
func CompileCoreSelectionPlan(catalog *Catalog, selected *CapabilitySet) CoreSelectionPlan {
	plan := CoreSelectionPlan{
		ActiveLivePaths:   make(map[string]bool),
		AllKnownLivePaths: make(map[string]bool),
		InactiveLivePaths: make(map[string]bool),
	}
	if catalog == nil {
		return plan
	}
	for _, m := range catalog.caps {
		active := selected != nil && selected.Has(m.ID)
		for _, p := range m.CoreOutputs {
			clean := filepath.Clean(p)
			plan.AllKnownLivePaths[clean] = true
			if active {
				plan.ActiveLivePaths[clean] = true
			}
		}
	}
	// Inactive = all-known minus active. Computed in a second pass so the
	// active set is fully populated first (a path owned by two capabilities
	// is already rejected by validateOutputPaths at merge time, so there is
	// no ambiguity about a path being both active and inactive).
	for p := range plan.AllKnownLivePaths {
		if !plan.ActiveLivePaths[p] {
			plan.InactiveLivePaths[p] = true
		}
	}
	return plan
}

// Empty reports whether the plan declares any capability-owned core outputs at
// all. An empty plan means no capability participates in core-output gating —
// the renderer and ownership layer retain their unconditional (pre-filtering)
// behavior for every file. This lets callers cheaply short-circuit: if the plan
// is empty, skip the exclude-set plumbing entirely.
func (p CoreSelectionPlan) Empty() bool {
	return len(p.AllKnownLivePaths) == 0
}
