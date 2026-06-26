package schema

import (
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// RunShape is the S4 runtime execution shape schema
// (.vh-agent-harness/run-shape.yml). Ownership class: project_owned (platform
// seeds once, never clobbers on update).
//
// Because run-shape is project_owned, Reconcile is seed-only: on update it
// returns OutcomeNoop (the project's instance is never overwritten). doctor still
// validates the shape so a malformed run-shape is caught. This file declares the
// v1 shape contract (runtime/services/lifecycle/runners/verbs/env); the
// authoritative runtime loader is internal/runshape (carried from prototype,
// migrated in Slice 2).
type RunShape struct{}

// runShapeAllowedTopLevel is the exhaustive set of top-level keys a run-shape.yml
// may carry (S4 authority). Anything else is an envelope violation.
// run_shape_version is the optional schema-version stamp (the run-shape spec §3);
// it is parseable metadata, not a runtime field, and is allowed alongside the
// seven authority blocks.
var runShapeAllowedTopLevel = map[string]bool{
	"run_shape_version": true,
	"runtime":           true,
	"services":          true,
	"lifecycle":         true,
	"runners":           true,
	"verbs":             true,
	"env":               true,
	"proxies":           true,
}

// allowedBackend enumerates the valid runtime backend values for v1.
var allowedBackend = map[string]bool{
	"bare":           true,
	"docker-compose": true,
	"host-shell":     true,
	"proxy":          true,
}

// fixedLifecycleHooks is the exhaustive set of lifecycle hook names (mirrors
// internal/runshape). Each value under lifecycle.hooks MUST be a string pointing
// at a scripts/ path, not inline shell.
var fixedLifecycleHooks = map[string]bool{
	"on_first_install": true,
	"on_update":        true,
	"pre_up":           true,
	"post_up":          true,
	"pre_down":         true,
	"post_down":        true,
	"pre_exec":         true,
	"post_exec":        true,
	"on_uninstall":     true,
}

// Validate reports structural problems in a run-shape.yml instance. It checks the
// top-level envelope, the runtime.backend enum, and that every lifecycle.hooks
// entry is a known hook name pointing at a scripts/ path (pointer, not inline
// shell). It does NOT execute the hooks or resolve the scripts.
func (RunShape) Validate(raw []byte) []FieldError {
	if len(strings.TrimSpace(string(raw))) == 0 {
		return []FieldError{{Field: "<root>", Message: "file is empty"}}
	}
	var root map[string]any
	if err := yaml.Unmarshal(raw, &root); err != nil {
		return []FieldError{{Field: "<root>", Message: fmt.Sprintf("not valid YAML: %v", err)}}
	}
	var errs []FieldError

	keys := make([]string, 0, len(root))
	for k := range root {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if !runShapeAllowedTopLevel[k] {
			errs = append(errs, FieldError{
				Field:   k,
				Message: fmt.Sprintf("unknown top-level key %q; allowed: runtime, services, lifecycle, runners, verbs, env, proxies", k),
			})
		}
	}

	// runtime.backend enum (only if runtime is a map with a backend scalar).
	if rt, ok := root["runtime"].(map[string]any); ok {
		if be, ok := rt["backend"]; ok {
			if s, ok := be.(string); ok {
				if !allowedBackend[s] {
					errs = append(errs, FieldError{
						Field:   "runtime.backend",
						Message: fmt.Sprintf("invalid backend %q; enum: bare | docker-compose | host-shell | proxy", s),
					})
				}
			} else {
				errs = append(errs, FieldError{Field: "runtime.backend", Message: "must be a string"})
			}
		}
	}

	// lifecycle.hooks: each entry must be a known hook -> scripts/ path string.
	if lc, ok := root["lifecycle"].(map[string]any); ok {
		if hooks, ok := lc["hooks"].(map[string]any); ok {
			hk := make([]string, 0, len(hooks))
			for k := range hooks {
				hk = append(hk, k)
			}
			sort.Strings(hk)
			for _, name := range hk {
				if !fixedLifecycleHooks[name] {
					errs = append(errs, FieldError{
						Field:   "lifecycle.hooks." + name,
						Message: "unknown lifecycle hook name; allowed: on_first_install, on_update, pre_up, post_up, pre_down, post_down, pre_exec, post_exec, on_uninstall",
					})
					continue
				}
				path, ok := hooks[name].(string)
				if !ok {
					errs = append(errs, FieldError{
						Field:   "lifecycle.hooks." + name,
						Message: "must be a scripts/ path pointer, not inline shell or a map",
					})
					continue
				}
				if !strings.HasPrefix(path, "scripts/") {
					errs = append(errs, FieldError{
						Field:   "lifecycle.hooks." + name,
						Message: fmt.Sprintf("must point under scripts/ (got %q); inline shell is forbidden", path),
					})
				}
			}
		}
	}

	return errs
}

// Reconcile is seed-only for run-shape (project_owned). On update it NEVER
// overwrites the project's instance. It returns OutcomeNoop. (First-install
// seeding is handled by the substrate's project_owned path, which copies the
// platform default only when the project file is absent.)
func (RunShape) Reconcile(project, platformDefault []byte) (ReconcileResult, error) {
	if len(strings.TrimSpace(string(project))) == 0 {
		// First install: the substrate seeds from platformDefault. We surface that
		// as an Apply with Merged == platformDefault so the substrate can seed
		// uniformly; the project_owned classification (skip-if-present) is the
		// substrate's responsibility, NOT the reconciler's. This keeps the
		// reconciler a pure (project, default) -> result function.
		return ReconcileResult{
			Outcome: OutcomeApply,
			Merged:  platformDefault,
			Applied: []string{"run-shape: seed-only (project_owned); substrate seeds when project instance absent"},
		}, nil
	}
	return ReconcileResult{
		Outcome: OutcomeNoop,
		Skipped: []string{"run-shape: project_owned; project instance preserved (never clobbered on update)"},
	}, nil
}
