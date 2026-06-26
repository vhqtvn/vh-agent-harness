// Package corpus embeds the static agent-harness scaffold (the "corpus") that
// the substrate seam renders into a target project.
//
// The embed directives live in this module-root package on purpose: go:embed
// patterns cannot traverse parents (no ".."), and the corpora live under
// <module-root>/templates/{core,overlays}. Only a file at or above those
// directories can embed them, so the embed vars are declared here and consumed
// by the substrate seam (internal/substrate) via the cli bridge (internal/cli).
//
// The "all:" prefix is required: the corpora contain dot-directories such as
// .opencode/ and .local/. Without "all:" the Go embed walker skips any path
// element beginning with "." or "_", which would silently drop the entire
// .opencode tree.
//
// The legacy templates/harness-root corpus + its embed (corpus.FS/RootDir) and
// the generator/installer/component packages that walked it were retired: the
// seam (templates/core) is the single canonical install source.
package corpus

import "embed"

// CoreDir is the embed.FS sub-directory holding the CURATED platform-managed
// corpus (Slice 2). This is the canonical corpus the seam renders into target
// projects: every file under here is platform_managed except the documented
// armed/owned exceptions (vh-harness-profile.yml=platform_armed,
// the project-identity files=project_owned). It replaces the prototype
// harness-root corpus as the install source. The Overlay packs (Slice 4) layer
// additively on top of this base.
const CoreDir = "templates/core"

// CoreFS is the read-only embedded curated corpus. Callers read from the
// CoreDir sub-directory. The substrate's EmbedFSRenderer renders this tree
// into staging from an embed.FS (no on-disk dependency), so the harness binary
// is self-contained and install works from any CWD.
//
//go:embed all:templates/core
var CoreFS embed.FS

// OverlaysDir is the embed.FS sub-directory that holds shipped opt-in overlay
// packs (Slice 4 forward mechanism). Each immediate child directory is a
// generic pack selected via vh-harness-profile.yml: overlays:[...]; a pack mirrors
// the .opencode/ subtree it contributes (agents/, skills/, commands/) plus
// merge-content files at its root (opencode-append.jsonc deep-merged into
// opencode.jsonc; callable-graph-snippet.md appended to callable-graph.md).
// Overlay units are ownership class overlay_extension (auto-overwritten while
// the pack stays active).
//
// As of the 2026-06-25 pre-publish clearance this directory ships NO packs:
// KnownPacks returns an empty list. The web-overlay pack (the only candidate)
// was relocated to a non-shipped adoption reference under
// docs/adoption-examples/web/ (a non-shipped adoption reference). The
// internal/overlay/ infra (KnownPacks/OpenPack/Pack) stays intact as the
// forward mechanism: the day an overlay pack ships, drop its directory under
// templates/overlays/ and KnownPacks lists it automatically — no code change.
// The directory retains a .gitkeep so the embed directive still matches a file.
const OverlaysDir = "templates/overlays"

// OverlaysFS is the read-only embedded overlay packs tree. Callers read from
// the OverlaysDir sub-directory. internal/overlay lists/opens packs from here.
// Currently embeds only the .gitkeep (no shipped packs); KnownPacks() returns
// an empty slice and OpenPack(name) returns fs.ErrNotExist for any name.
//
//go:embed all:templates/overlays
var OverlaysFS embed.FS

// ExamplesDir is the embed.FS sub-directory holding configuration DOCS/TEMPLATES
// for every project-configurable file, mirroring the real target path of each
// (e.g. templates/examples/.vh-agent-harness/project.config.json documents
// .vh-agent-harness/project.config.json). These files are EMBED-ONLY: they are
// NOT under templates/core, so the seam never renders them into a target repo.
// Instead `vh-agent-harness example <path>` prints them on demand, so the target
// tree stays free of *.example scaffolds. Covers both the project_owned seeds
// (mission, forbidden-patterns.project, project.config, …) and the schema'd
// authorities (vh-harness-profile.yml, run-shape.yml, harness-ownership.yml).
const ExamplesDir = "templates/examples"

// ExamplesFS is the read-only embedded configuration-docs tree. The `example`
// command reads from the ExamplesDir sub-directory.
//
//go:embed all:templates/examples
var ExamplesFS embed.FS
