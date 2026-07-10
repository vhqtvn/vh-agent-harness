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
// This directory ships the embedded overlay packs. The `release` pack is the
// first shipped embedded overlay pack and the reference implementation of the
// Phase-3 capability-installer overlay integration: it carries a
// capability-manifest.yml (id: core/release, hard_dep: core/gated-commit) and is
// discovered automatically by internal/cli/profile.go discoverPackContributions.
// The web-overlay pack was relocated to a non-shipped adoption reference under
// docs/adoption-examples/web/. The internal/overlay/ infra
// (KnownPacks/OpenPack/Pack) lists any directory dropped here automatically — no
// code change. The directory retains a .gitkeep so the embed directive still
// matches at least one file even if every pack directory is later removed.
const OverlaysDir = "templates/overlays"

// OverlaysFS is the read-only embedded overlay packs tree. Callers read from
// the OverlaysDir sub-directory. internal/overlay lists/opens packs from here.
// Currently embeds the `release` pack plus the retained .gitkeep; KnownPacks()
// returns ["release"] and OpenPack("release") opens the shipped pack.
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

// OverlaySkeletonDir is the embed.FS sub-directory holding the per-unit
// skeleton BODIES written by `vh-agent-harness overlay new` (one file per unit
// type: agent.md / command.md / skill.md). These are EMBED-ONLY: they are NOT
// under templates/core, so the seam never renders them into a target repo. The
// scaffolder reads a body, substitutes its __UNIT_NAME__ placeholder with the
// caller-supplied unit name, and writes it into a project's
// .vh-agent-harness/overlays/<pack>/ tree. Only the 3 canonical identity tokens
// ({{PROJECT_NAME}} / {{PROJECT_SLUG}} / {{COORDINATOR_DIR}}) and the
// __UNIT_NAME__ placeholder may appear — no brand/domain literals (the binary
// stays domain-free).
const OverlaySkeletonDir = "templates/overlay-skeleton"

// OverlaySkeletonFS is the read-only embedded unit-skeleton tree. The
// `overlay new` command reads from the OverlaySkeletonDir sub-directory.
//
//go:embed all:templates/overlay-skeleton
var OverlaySkeletonFS embed.FS

// DocsDir is the embed.FS sub-directory holding the GENERIC agent-workflow docs
// surfaced by `vh-agent-harness docs [key]` (memory model, session workflow,
// prompt guide, skill catalog, tmp-file hygiene). These are BINARY/HELP-SURFACE
// docs, NOT consumer-corpus files: they live OUTSIDE templates/core, so the
// substrate seam never renders them into a target repo (they do not clutter an
// adopter's tree), and they carry NO ownership class in core_manifest.go. They
// document harness machinery that is byte-identical for every adopter, so they
// travel with the binary and are read on demand — mirroring the embed-only
// pattern used by ExamplesFS and MigrationsFS.
//
// The `docs` command serves the embedded copy by default. A project may point a
// key at a live on-disk file via .vh-agent-harness/docs-overrides.yml — this
// repo dogfoods that path so the installed binary reads the LIVE source under
// continuous update rather than a stale build-time snapshot.
const DocsDir = "templates/docs"

// DocsFS is the read-only embedded generic-docs tree. The `docs` command reads
// from the DocsDir sub-directory (each file's basename without .md is its key).
//
//go:embed all:templates/docs
var DocsFS embed.FS

// SysPromptsDir is the embed.FS sub-directory holding the NAMED SYSTEM PROMPTS
// surfaced by `vh-agent-harness sys-prompt [name]`. These are BINARY/HELP-SURFACE
// assets, NOT consumer-corpus files: they live OUTSIDE templates/core, so the
// substrate seam never renders them into a target repo (they do not clutter an
// adopter's tree), and they carry NO ownership class in core_manifest.go. The
// `sys-prompt` command serves the embedded copy by default; an overlay pack or
// operator may supersede a prompt by rendering a live file to
// .opencode/sys-prompts/<name>.md, which the live-tree-first resolution serves
// instead. This mirrors the embed-only pattern used by DocsFS and MigrationsFS.
const SysPromptsDir = "templates/sys-prompts"

// SysPromptsFS is the read-only embedded named-system-prompts tree. The
// `sys-prompt` command reads from the SysPromptsDir sub-directory (each file's
// basename without .md is its key).
//
//go:embed all:templates/sys-prompts
var SysPromptsFS embed.FS

// MigrationsDir is the embed.FS sub-directory holding the per-release migration
// notes (one file per release: vX.Y.Z.md). These are BINARY/HELP-SURFACE docs,
// NOT consumer-corpus files: they live OUTSIDE templates/core, so the substrate
// seam never renders them into a target repo, and they carry NO ownership class
// in core_manifest.go. The `help migrate [version]` command reads ONLY from this
// embedded copy (never the live filesystem), so a release's note travels with the
// binary that shipped it and is available offline from any CWD. This mirrors the
// existing embed-only pattern used for ExamplesFS and OverlaySkeletonFS.
const MigrationsDir = "templates/migrations"

// MigrationsFS is the read-only embedded migration-notes tree. The
// `help migrate` command reads from the MigrationsDir sub-directory.
//
//go:embed templates/migrations/*
var MigrationsFS embed.FS
