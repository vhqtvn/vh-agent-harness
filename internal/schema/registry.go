package schema

import (
	"path/filepath"
	"strings"
)

// registry is the table of schema'd file families. Slice 1 registers the four
// armed-file types. Full corpus wiring (mapping every known template path to its
// schema) lands in Slice 5.
var registry = map[Type]Schema{
	TypeHarnessProfile: {
		Type:       TypeHarnessProfile,
		Validator:  HarnessProfile{},
		Reconciler: HarnessProfile{},
	},
	TypeRunShape: {
		Type:       TypeRunShape,
		Validator:  RunShape{},
		Reconciler: RunShape{},
	},
	TypeForbiddenPatternsProject: {
		Type:       TypeForbiddenPatternsProject,
		Validator:  ForbiddenPatternsProject{},
		Reconciler: ForbiddenPatternsProject{},
	},
	TypeRepoRecon: {
		Type:       TypeRepoRecon,
		Validator:  RepoRecon{},
		Reconciler: RepoRecon{},
	},
}

// Lookup returns the Schema descriptor for a Type, or (zero, false) if the type
// is not registered.
func Lookup(t Type) (Schema, bool) {
	s, ok := registry[t]
	return s, ok
}

// SchemaForPath maps a repo-relative path to its registered Schema by file name
// convention. It returns (zero, false) for paths that are not schema'd (the
// common case: most files are platform_managed or project_owned without a
// schema). The substrate uses this to decide whether a platform_armed target
// needs schema validation + reconcile during an apply.
//
// Path conventions (v1):
//   - .vh-agent-harness/vh-harness-profile.yml -> TypeHarnessProfile
//   - .vh-agent-harness/run-shape.yml   -> TypeRunShape
//   - forbidden-patterns.project.js     -> TypeForbiddenPatternsProject
//   - repo-recon-<name>.yml             -> TypeRepoRecon  (any repo-recon-* or
//     repo-recon.* named recon data file)
func SchemaForPath(path string) (Schema, bool) {
	clean := filepath.ToSlash(path)
	base := filepath.Base(clean)
	switch {
	case base == "vh-harness-profile.yml" && strings.HasPrefix(clean, ".vh-agent-harness/"):
		return lookup(TypeHarnessProfile)
	case base == "run-shape.yml" && strings.HasPrefix(clean, ".vh-agent-harness/"):
		return lookup(TypeRunShape)
	case base == "forbidden-patterns.project.js":
		return lookup(TypeForbiddenPatternsProject)
	case (strings.HasPrefix(base, "repo-recon-") || strings.HasPrefix(base, "repo-recon.")) && strings.HasSuffix(base, ".yml"):
		return lookup(TypeRepoRecon)
	}
	return Schema{}, false
}

func lookup(t Type) (Schema, bool) {
	s, ok := registry[t]
	return s, ok
}

// All returns every registered Schema in canonical Type order. Useful for doctor
// (validate every armed instance in the tree) and for tests.
func All() []Schema {
	order := []Type{TypeHarnessProfile, TypeRunShape, TypeForbiddenPatternsProject, TypeRepoRecon}
	out := make([]Schema, 0, len(order))
	for _, t := range order {
		if s, ok := registry[t]; ok {
			out = append(out, s)
		}
	}
	return out
}
