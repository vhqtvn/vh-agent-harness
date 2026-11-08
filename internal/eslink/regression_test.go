package eslink

// Gate-2 regression suite (staged-restart spec §B gate 2): retained G32
// regressions ported from the BLOCKED archive, plus the five corpus-level
// negative mutations named by the F5 restart path (removed-export,
// late-import, decoy, unsupported-suffix, slash-update).
//
// Port provenance: every TestG32_* function ports a same-named function from
// the archived G32 suite (archive .local/coordinator/archives/eslink-blocked/
// eslink-g32-blocked-2026-11-07.tar.gz, sha256 f45d2fc1…e750; extracted test
// file digest ea22d628…01cd per MANIFEST.md). Ports keep the archive's input
// bytes and expected verdicts where the frozen grammar still pins them;
// adaptations and supersessions are marked per test and inventoried in the
// S1′ closeout report. G32 tests whose pinned verdicts the ratified freeze
// deliberately changed (await-as-identifier, class fields/expressions,
// tagged templates, ASI lexer-state machinery, do-while/debugger) are NOT
// ported — the freeze record supersedes them; the inventory records each.

import (
	"strings"
	"testing"
)

func treeOf(files map[string]string) map[string][]byte {
	tree := map[string][]byte{}
	for p, src := range files {
		tree[p] = []byte(src)
	}
	return tree
}

func findBroken(r *Report, importer, symbol string) *BrokenEdge {
	for i := range r.Broken {
		b := &r.Broken[i]
		if b.Importer == importer && b.Symbol == symbol {
			return b
		}
	}
	return nil
}

func findUnknownContaining(r *Report, path, substr string) *UnknownFinding {
	for i := range r.Unknown {
		u := &r.Unknown[i]
		if u.Path == path && strings.Contains(u.Reason, substr) {
			return u
		}
	}
	return nil
}

// --- tri-state basics (G32 ports) --------------------------------------------

func TestG32_Scan_Ok_NamedExportPresent(t *testing.T) {
	r := Scan(treeOf(map[string]string{
		".opencode/scripts/provider.js": "export function alpha() {}\nexport const beta = 1;\n",
		".opencode/scripts/consumer.js": "import { alpha, beta } from \"./provider.js\";\n",
	}))
	if r.HasBroken() {
		t.Fatalf("expected ok, got broken: %+v", r.Broken)
	}
	if len(r.Unknown) != 0 {
		t.Fatalf("expected no unknown, got: %+v", r.Unknown)
	}
	if r.Edges != 1 {
		t.Fatalf("Edges = %d, want 1", r.Edges)
	}
}

func TestG32_Scan_Broken_NamedExportAbsent(t *testing.T) {
	r := Scan(treeOf(map[string]string{
		".opencode/scripts/provider.js": "export const other = 1;\n",
		".opencode/scripts/consumer.js": "import { alpha } from \"./provider.js\";\n",
	}))
	if b := findBroken(r, ".opencode/scripts/consumer.js", "alpha"); b == nil {
		t.Fatalf("absent named export must be proven broken: %+v / %+v", r.Broken, r.Unknown)
	}
	if !strings.Contains(findBroken(r, ".opencode/scripts/consumer.js", "alpha").Reason, "surface closed, so the absence is proven") {
		t.Fatalf("broken verdict must cite proven absence: %+v", r.Broken)
	}
}

func TestG32_Scan_Unknown_ProviderAbsentFromTree(t *testing.T) {
	r := Scan(treeOf(map[string]string{
		".opencode/scripts/consumer.js": "import { alpha } from \"./provider.js\";\n",
	}))
	if r.HasBroken() {
		t.Fatalf("missing provider must never be broken: %+v", r.Broken)
	}
	if findUnknownContaining(r, ".opencode/scripts/consumer.js", "not present in the scanned tree") == nil {
		t.Fatalf("expected provider-absent unknown: %+v", r.Unknown)
	}
}

func TestG32_Scan_Unknown_ProviderUnparseable(t *testing.T) {
	r := Scan(treeOf(map[string]string{
		".opencode/scripts/provider.js": "export const x = 1;\nlet i = 0; i++;\n",
		".opencode/scripts/consumer.js": "import { x } from \"./provider.js\";\n",
	}))
	if r.HasBroken() {
		t.Fatalf("unparseable provider must never be broken: %+v", r.Broken)
	}
	if findUnknownContaining(r, ".opencode/scripts/consumer.js", "open export surface") == nil {
		t.Fatalf("expected open-surface unknown: %+v", r.Unknown)
	}
}

// --- hazard class (G32 ports) --------------------------------------------------

func TestG32_Scan_Hazard_RenamedImport(t *testing.T) {
	r := Scan(treeOf(map[string]string{
		".opencode/repo-configs/core.js": "export const FORBIDDEN_PATTERNS = [];\nexport const GIT_MUTATION_VERBS = [];\n",
		".opencode/repo-configs/patterns.js": "import { FORBIDDEN_PATTERNS as _CORE } from \"./core.js\";\n" +
			"use(_CORE);\n",
	}))
	if r.HasBroken() {
		t.Fatalf("renamed import must resolve: %+v", r.Broken)
	}
	if len(r.Unknown) != 0 {
		t.Fatalf("unexpected unknown: %+v", r.Unknown)
	}
}

func TestG32_Scan_Hazard_ReexportFrom(t *testing.T) {
	r := Scan(treeOf(map[string]string{
		".opencode/repo-configs/core.js":     "export const GIT_MUTATION_VERBS = [];\nexport const OTHER = 1;\n",
		".opencode/repo-configs/patterns.js": "export { GIT_MUTATION_VERBS } from \"./core.js\";\n",
		".opencode/repo-configs/consumer.js": "import { GIT_MUTATION_VERBS } from \"./patterns.js\";\n",
	}))
	if r.HasBroken() {
		t.Fatalf("re-export must resolve transitively: %+v", r.Broken)
	}
	if r.Edges != 2 {
		t.Fatalf("Edges = %d, want 2 (consumer import + re-export edge)", r.Edges)
	}
}

func TestG32_Scan_Hazard_DynamicImportOptionalFile(t *testing.T) {
	r := Scan(treeOf(map[string]string{
		".opencode/plugins/core.js": "const mod = await import(\"./optional-panel.js\");\nexport const ready = mod;\n",
	}))
	if r.HasBroken() {
		t.Fatalf("dynamic import of an absent optional file must never be broken: %+v", r.Broken)
	}
	if findUnknownContaining(r, ".opencode/plugins/core.js", "dynamic import") == nil {
		t.Fatalf("expected dynamic-import unknown: %+v", r.Unknown)
	}
}

// Amendment-note: this port's input (module-top `await import("web-tree-sitter")`)
// is parseable ONLY under the family-5 amendment (module-top [+Await]); under
// the pre-amendment grammar it was module-UNKNOWN.
func TestG32_Scan_Hazard_DynamicImportBareSpecifier(t *testing.T) {
	r := Scan(treeOf(map[string]string{
		".opencode/plugins/core.js": "const { Parser } = await import(\"web-tree-sitter\");\n",
	}))
	if r.HasBroken() {
		t.Fatalf("bare dynamic import must never be broken: %+v", r.Broken)
	}
	if len(r.Unknown) != 0 {
		t.Fatalf("bare dynamic import must not even yield an unknown finding: %+v", r.Unknown)
	}
}

func TestG32_Scan_Hazard_DynamicImportComputed(t *testing.T) {
	r := Scan(treeOf(map[string]string{
		".opencode/plugins/core.js": "const name = \"panel\";\nconst mod = await import(\"./\" + name + \".js\");\nexport const ready = mod;\n",
	}))
	if r.HasBroken() {
		t.Fatalf("computed dynamic import must never be broken: %+v", r.Broken)
	}
	if findUnknownContaining(r, ".opencode/plugins/core.js", "computed") == nil {
		t.Fatalf("expected computed-specifier unknown: %+v", r.Unknown)
	}
}

func TestG32_Scan_Hazard_CommentEmbeddedImportText(t *testing.T) {
	r := Scan(treeOf(map[string]string{
		".opencode/repo-configs/patterns.js": "// extend instead via `forbidden-patterns.project.js` (project_owned).\n" +
			"// import { thing } from \"./does-not-exist.js\";\n" +
			"/*\n * import { more } from \"../also-missing.js\";\n */\n" +
			"export const OK = 1;\n",
	}))
	if r.HasBroken() {
		t.Fatalf("comment-embedded import text must never be an edge: %+v", r.Broken)
	}
	if len(r.Unknown) != 0 {
		t.Fatalf("comment text must not even yield unknown findings: %+v", r.Unknown)
	}
}

func TestG32_Scan_Hazard_MultiLineImportBlock(t *testing.T) {
	r := Scan(treeOf(map[string]string{
		".opencode/scripts/state-lib.js": "export class StateError extends Error {}\n" +
			"export const collectDecisionItems = () => [];\n",
		".opencode/scripts/verify.js": "import fs from \"fs\";\n" +
			"import {\n    StateError,\n    collectDecisionItems,\n} from \"./state-lib.js\";\n",
	}))
	if r.HasBroken() {
		t.Fatalf("multi-line import block must resolve: %+v", r.Broken)
	}
}

func TestG32_Scan_Hazard_ExportBlockWithCommentsAndDefault(t *testing.T) {
	r := Scan(treeOf(map[string]string{
		".opencode/scripts/state-lib.js": "const a = 1, b = 2;\n" +
			"export {\n    a,\n    b,\n    // Exported as a TEST SEAM so tests can exercise\n    // git-failure mapping directly.\n    a as renamedA,\n};\n" +
			"export default { a, b };\n",
		".opencode/scripts/consumer.js": "import def, { a, renamedA } from \"./state-lib.js\";\n",
	}))
	if r.HasBroken() {
		t.Fatalf("comment-interleaved export block must resolve: %+v", r.Broken)
	}
	// renamedA missing must be provable when the block drops it.
	r2 := Scan(treeOf(map[string]string{
		".opencode/scripts/state-lib.js": "const a = 1;\nexport {\n    a,\n    // renamedA was removed by a consumer edit\n};\n",
		".opencode/scripts/consumer.js":  "import { renamedA } from \"./state-lib.js\";\n",
	}))
	if b := findBroken(r2, ".opencode/scripts/consumer.js", "renamedA"); b == nil {
		t.Fatalf("removed renamed export must be proven broken: %+v / %+v", r2.Broken, r2.Unknown)
	}
}

func TestG32_Scan_Hazard_CrossDirectoryEdges(t *testing.T) {
	r := Scan(treeOf(map[string]string{
		".opencode/scripts/state-lib.js":  "export const repoRoot = 1;\n",
		".opencode/plugins/pause.js":      "import { repoRoot } from \"../scripts/state-lib.js\";\n",
		".opencode/repo-configs/gates.js": "import { repoRoot } from \"../scripts/state-lib.js\";\nexport { repoRoot };\n",
	}))
	if r.HasBroken() {
		t.Fatalf("cross-dir edges must resolve: %+v", r.Broken)
	}
	// A removed cross-dir export must be provable.
	r2 := Scan(treeOf(map[string]string{
		".opencode/scripts/state-lib.js": "export const other = 1;\n",
		".opencode/plugins/pause.js":     "import { repoRoot } from \"../scripts/state-lib.js\";\n",
	}))
	if !r2.HasBroken() {
		t.Fatalf("missing cross-dir export must be proven broken: %+v / %+v", r2.Broken, r2.Unknown)
	}
}

// --- specifier / name forms (G32 ports) ---------------------------------------

func TestG32_Scan_BareAndNodeSpecifiersExcluded(t *testing.T) {
	r := Scan(treeOf(map[string]string{
		".opencode/scripts/consumer.js": "import fs from \"fs\";\n" +
			"import path from \"node:path\";\n" +
			"import { strict as assert } from \"node:assert\";\n" +
			"import { spawn } from \"node:child_process\";\n" +
			"import something from \"some-npm-package\";\n",
	}))
	if r.HasBroken() || len(r.Unknown) != 0 {
		t.Fatalf("bare/builtin specifiers must be out of graph; broken=%+v unknown=%+v", r.Broken, r.Unknown)
	}
	if r.Edges != 0 {
		t.Fatalf("Edges = %d, want 0", r.Edges)
	}
}

// Adapted-port note: the archive's form list included `export function* gen`.
// Generators are EXCLUDED by the frozen grammar (D-2 adjacent grammar text);
// the admitted forms are pinned FIT here and the generator form is pinned to
// its frozen-grammar verdict (UNKNOWN, reason-coded) instead.
func TestG32_Scan_ExportForms(t *testing.T) {
	r := Scan(treeOf(map[string]string{
		".opencode/scripts/p.js": "export class Klass {}\n" +
			"export function fn() {}\n" +
			"export async function afn() {}\n" +
			"export const c1 = 1;\n" +
			"export const c2 = 2, c3 = 3;\n" +
			"export let l1 = (a, b) => a + b;\n" +
			"export var v1 = { x: 1 };\n" +
			"export default function () {}\n",
		".opencode/scripts/c.js": "import { Klass, fn, afn, c1, c2, c3, l1, v1 } from \"./p.js\";\n",
	}))
	if r.HasBroken() {
		t.Fatalf("all admitted export forms must be recognized: %+v", r.Broken)
	}
	if len(r.Unknown) != 0 {
		t.Fatalf("unexpected unknown: %+v", r.Unknown)
	}
	// Each form's absence must be provable.
	r2 := Scan(treeOf(map[string]string{
		".opencode/scripts/c.js":     "import { afn, c3 } from \"./empty.js\";\n",
		".opencode/scripts/empty.js": "export function other() {}\n",
	}))
	if findBroken(r2, ".opencode/scripts/c.js", "afn") == nil || findBroken(r2, ".opencode/scripts/c.js", "c3") == nil {
		t.Fatalf("absences must be proven: %+v / %+v", r2.Broken, r2.Unknown)
	}
	// Frozen-grammar addition: the generator form degrades reason-coded.
	m := Parse(".opencode/scripts/gen.js", []byte("export function* gen() {}\n"))
	if !m.Unparseable || !strings.Contains(m.UnparseableWhy, ReasonGenFunc) {
		t.Fatalf("generator export must degrade with %q, got %q", ReasonGenFunc, m.UnparseableWhy)
	}
}

func TestG32_Scan_StringLiteralImportNames(t *testing.T) {
	r := Scan(treeOf(map[string]string{
		".opencode/scripts/p.js": "const val = 1;\nexport { val as \"weird-name\" };\n",
		".opencode/scripts/c.js": "import { \"weird-name\" as z } from \"./p.js\";\n",
	}))
	if r.HasBroken() {
		t.Fatalf("string-literal names must resolve: %+v", r.Broken)
	}
	r2 := Scan(treeOf(map[string]string{
		".opencode/scripts/p.js": "export const other = 1;\n",
		".opencode/scripts/c.js": "import { \"weird-name\" as z } from \"./p.js\";\n",
	}))
	if !r2.HasBroken() {
		t.Fatalf("absent string-literal name must be proven broken")
	}
}

func TestG32_Scan_DefaultImportBroken(t *testing.T) {
	r := Scan(treeOf(map[string]string{
		".opencode/scripts/p.js": "export const named = 1;\n",
		".opencode/scripts/c.js": "import dd from \"./p.js\";\n",
	}))
	if b := findBroken(r, ".opencode/scripts/c.js", "default"); b == nil {
		t.Fatalf("absent default must be proven broken: %+v / %+v", r.Broken, r.Unknown)
	}
}

func TestG32_Scan_NamespaceAndSideEffectImports(t *testing.T) {
	r := Scan(treeOf(map[string]string{
		".opencode/scripts/p.js": "export const named = 1;\n",
		".opencode/scripts/c.js": "import * as ns from \"./p.js\";\nimport \"./p.js\";\n",
	}))
	if r.HasBroken() || len(r.Unknown) != 0 {
		t.Fatalf("namespace/side-effect imports impose no name checks: %+v / %+v", r.Broken, r.Unknown)
	}
}

func TestG32_Scan_DefaultCanonicalization(t *testing.T) {
	r := Scan(treeOf(map[string]string{
		".opencode/scripts/p.js": "export default 1;\nexport const named = 2;\n",
		".opencode/scripts/c.js": "import { default as d, named } from \"./p.js\";\nimport dd from \"./p.js\";\n",
	}))
	if r.HasBroken() {
		t.Fatalf("default-as-named over an export default must not be broken: %+v", r.Broken)
	}

	r2 := Scan(treeOf(map[string]string{
		".opencode/scripts/p.js": "const value = 1;\nexport { value as default };\n",
		".opencode/scripts/c.js": "import d from \"./p.js\";\nimport { default as d2 } from \"./p.js\";\n",
	}))
	if r2.HasBroken() {
		t.Fatalf("plain default import over `export { value as default }` must not be broken: %+v", r2.Broken)
	}

	// And absence stays provable in BOTH spellings.
	r3 := Scan(treeOf(map[string]string{
		".opencode/scripts/p.js": "export const other = 1;\n",
		".opencode/scripts/c.js": "import { default as d } from \"./p.js\";\nimport dd from \"./p.js\";\n",
	}))
	if findBroken(r3, ".opencode/scripts/c.js", "default") == nil {
		t.Fatalf("absent default must be proven broken in both spellings: %+v", r3.Broken)
	}
}

// Adapted-port note (assertion only): the archive matched "unparseable" in
// the finding text; the successor's module-UNKNOWN finding says "module
// UNKNOWN (…)". Input and verdict class unchanged.
func TestG32_Scan_EscapedStringNameDegradesToUnknown(t *testing.T) {
	r := Scan(treeOf(map[string]string{
		".opencode/scripts/p.js": "const value = 1;\nexport { value as \"\\u0078\" };\n",
		".opencode/scripts/c.js": "import { x } from \"./p.js\";\n",
	}))
	if r.HasBroken() {
		t.Fatalf("escaped export name must never ground a broken verdict: %+v", r.Broken)
	}
	if findUnknownContaining(r, ".opencode/scripts/p.js", "module UNKNOWN") == nil {
		t.Fatalf("expected module-UNKNOWN finding: %+v", r.Unknown)
	}
}

func TestG32_Scan_URLishSpecifiersAreUnknown(t *testing.T) {
	r := Scan(treeOf(map[string]string{
		".opencode/scripts/p.js":   "export const actual = 1;\n",
		".opencode/scripts/p%2Ejs": "export const other = 1;\n",
		".opencode/scripts/c.js":   "import { actual } from \"./p%2Ejs\";\n",
		".opencode/scripts/c2.js":  "import { x } from \"./p.js?v=1\";\n",
	}))
	if r.HasBroken() {
		t.Fatalf("URL-ish specifier must never be broken: %+v", r.Broken)
	}
	if findUnknownContaining(r, ".opencode/scripts/c.js", "percent-encoding") == nil {
		t.Fatalf("expected unknown for percent specifier: %+v", r.Unknown)
	}
	if findUnknownContaining(r, ".opencode/scripts/c2.js", "percent-encoding") == nil {
		t.Fatalf("expected unknown for query specifier: %+v", r.Unknown)
	}
}

func TestG32_Scan_MultibyteIdentifierExports(t *testing.T) {
	r := Scan(treeOf(map[string]string{
		".opencode/scripts/p.js": "export const π = 1, x = 2;\nexport const é = 3;\n",
		".opencode/scripts/c.js": "import { π, x, é } from \"./p.js\";\n",
	}))
	if r.HasBroken() {
		t.Fatalf("multibyte names must resolve whole: broken=%+v", r.Broken)
	}
	if len(r.Unknown) != 0 {
		t.Fatalf("expected clean scan, got unknown: %+v", r.Unknown)
	}

	r2 := Scan(treeOf(map[string]string{
		".opencode/scripts/p.js": "export const x\\u0079 = 1;\n",
		".opencode/scripts/c.js": "import { xy } from \"./p.js\";\n",
	}))
	if r2.HasBroken() {
		t.Fatalf("escaped identifier must be unknown, never broken: %+v", r2.Broken)
	}
	if findUnknownContaining(r2, ".opencode/scripts/p.js", "module UNKNOWN") == nil {
		t.Fatalf("expected unparseable finding for escaped identifier: %+v", r2.Unknown)
	}
}

func TestG32_Scan_ImportEmptyNamedList(t *testing.T) {
	r := Scan(treeOf(map[string]string{
		".opencode/scripts/p.js": "import {} from \"./m.js\";\nimport { actual } from \"./m.js\";\n",
		".opencode/scripts/m.js": "export const actual = 1;\n",
	}))
	if len(r.Broken) != 0 || len(r.Unknown) != 0 {
		t.Fatalf("empty named import must be clean: broken=%+v unknown=%+v", r.Broken, r.Unknown)
	}
	if r.Edges != 2 {
		t.Fatalf("edges=%d (want 2: presence + named)", r.Edges)
	}
}

// --- surface-extraction hazards (G32 ports) ------------------------------------

func TestG32_Scan_MultilineExportedInitializers(t *testing.T) {
	r := Scan(treeOf(map[string]string{
		".opencode/scripts/p.js": "export const state = {\n" +
			"    one: 1,\n" +
			"    two: function () {\n" +
			"        return 2;\n" +
			"    },\n" +
			"    three: () => ({\n" +
			"        nested: true,\n" +
			"    }),\n" +
			"};\n" +
			"export const after = 42;\n" +
			"export function later() {}\n",
		".opencode/scripts/c.js": "import { state, after, later } from \"./p.js\";\n",
	}))
	if r.HasBroken() {
		t.Fatalf("multiline initializer must not hide exports: %+v", r.Broken)
	}
	if len(r.Unknown) != 0 {
		t.Fatalf("multiline initializer must not degrade the module: %+v", r.Unknown)
	}
	// Absence after a giant initializer must remain provable.
	r2 := Scan(treeOf(map[string]string{
		".opencode/scripts/p.js": "export const state = {\n    one: 1,\n};\nexport const other = 2;\n",
		".opencode/scripts/c.js": "import { doesNotExist } from \"./p.js\";\n",
	}))
	if findBroken(r2, ".opencode/scripts/c.js", "doesNotExist") == nil {
		t.Fatalf("absence after multiline initializer must be proven: %+v / %+v", r2.Broken, r2.Unknown)
	}
}

func TestG32_Scan_ImportMetaNotDynamic(t *testing.T) {
	r := Scan(treeOf(map[string]string{
		".opencode/scripts/p.js": "const here = import.meta.url;\nexport const ok = here;\n",
	}))
	for _, u := range r.Unknown {
		if strings.Contains(u.Reason, "dynamic") {
			t.Fatalf("import.meta misclassified as dynamic import: %+v", u)
		}
	}
}

// Adapted-port note (assertion only): module-UNKNOWN wording (see
// TestG32_Scan_EscapedStringNameDegradesToUnknown).
func TestG32_Scan_UnparseableImporter(t *testing.T) {
	r := Scan(treeOf(map[string]string{
		".opencode/scripts/bad.js": "import { x } from \"./p.js\";  // fine\nconst oops = {\n",
		".opencode/scripts/p.js":   "export const x = 1;\n",
	}))
	if r.HasBroken() {
		t.Fatalf("unparseable importer must not produce broken verdicts: %+v", r.Broken)
	}
	if findUnknownContaining(r, ".opencode/scripts/bad.js", "module UNKNOWN") == nil {
		t.Fatalf("expected importer-unparseable finding: %+v", r.Unknown)
	}
}

func TestG32_Scan_ImportCyclesOk(t *testing.T) {
	r := Scan(treeOf(map[string]string{
		".opencode/scripts/a.js": "import { b } from \"./b.js\";\nexport const a = 1;\n",
		".opencode/scripts/b.js": "import { a } from \"./a.js\";\nexport const b = 2;\n",
	}))
	if r.HasBroken() || len(r.Unknown) != 0 {
		t.Fatalf("import cycles must resolve ok: broken=%+v unknown=%+v", r.Broken, r.Unknown)
	}
}

func TestG32_Scan_EmptyProviderSurface(t *testing.T) {
	r := Scan(treeOf(map[string]string{
		".opencode/scripts/blank.js": "// intentionally empty scaffold\n",
		".opencode/scripts/named.js": "import { x } from \"./blank.js\";\n",
		".opencode/scripts/def.js":   "import d from \"./blank.js\";\n",
		".opencode/scripts/ns.js":    "import * as n from \"./blank.js\";\n",
		".opencode/scripts/se.js":    "import \"./blank.js\";\n",
	}))
	if findBroken(r, ".opencode/scripts/named.js", "x") == nil {
		t.Fatalf("named import of empty module must be proven broken")
	}
	if findBroken(r, ".opencode/scripts/def.js", "default") == nil {
		t.Fatalf("default import of empty module must be proven broken")
	}
	if findBroken(r, ".opencode/scripts/ns.js", "") != nil || findBroken(r, ".opencode/scripts/se.js", "") != nil {
		t.Fatalf("namespace/side-effect of empty module must be ok: %+v", r.Broken)
	}
}

func TestG32_Scan_DefaultReexports(t *testing.T) {
	r := Scan(treeOf(map[string]string{
		".opencode/scripts/base.js":     "export default 1;\nexport const x = 2;\n",
		".opencode/scripts/mid1.js":     "export { default } from \"./base.js\";\n",
		".opencode/scripts/mid2.js":     "export { x as default } from \"./base.js\";\n",
		".opencode/scripts/consumer.js": "import d1 from \"./mid1.js\";\nimport d2 from \"./mid2.js\";\n",
	}))
	if r.HasBroken() {
		t.Fatalf("default re-exports must resolve: %+v", r.Broken)
	}
	// Re-exporting a default from a module that has none is proven broken.
	r2 := Scan(treeOf(map[string]string{
		".opencode/scripts/base.js": "export const x = 1;\n",
		".opencode/scripts/mid.js":  "export { default } from \"./base.js\";\n",
	}))
	if b := findBroken(r2, ".opencode/scripts/mid.js", "default"); b == nil {
		t.Fatalf("default re-export absence must be proven: %+v / %+v", r2.Broken, r2.Unknown)
	}
}

func TestG32_Scan_BrokenReexportEdgeAttribution(t *testing.T) {
	r := Scan(treeOf(map[string]string{
		".opencode/scripts/base.js":     "export const one = 1;\n",
		".opencode/scripts/mid.js":      "export { one, two } from \"./base.js\";\n",
		".opencode/scripts/consumer.js": "import { one } from \"./mid.js\";\n",
	}))
	if b := findBroken(r, ".opencode/scripts/mid.js", "two"); b == nil {
		t.Fatalf("re-export edge must be proven broken on mid.js: %+v / %+v", r.Broken, r.Unknown)
	}
	if r.HasBroken() && findBroken(r, ".opencode/scripts/consumer.js", "one") != nil {
		t.Fatalf("consumer import of a declared re-export name is not itself broken")
	}
}

// Supersession note: the archive's star/namespace-reexport family
// (StarReexportClosedChain, StarReexportOpenThroughMissingTarget,
// StarReexportNamespaceForm, NamespaceReexportPresence(+CountsEdge),
// NamespaceReexportNamedDefault, StarAmbiguityIsUnknown,
// TransitiveStarAmbiguityIsUnknown, DuplicateStarSameTargetNotAmbiguous,
// StarReexportCycleOpens, DefaultReexportOfAbsentNameIsBroken — the last is
// ported above) is retained via the gate-7 XS fixtures, which pin the SAME
// semantics from the map's post-freeze designs (XS-1..XS-7), plus these two
// small form ports the XS set does not duplicate:
func TestG32_Scan_ExportEmptyFromClausePresence(t *testing.T) {
	r := Scan(treeOf(map[string]string{
		".opencode/scripts/m.js": "export {} from \"./p.js\";\n",
		".opencode/scripts/p.js": "export const actual = 1;\n",
	}))
	if r.HasBroken() || len(r.Unknown) != 0 {
		t.Fatalf("`export {} from` is a presence-only edge: broken=%+v unknown=%+v", r.Broken, r.Unknown)
	}
	if r.Edges != 1 {
		t.Fatalf("Edges = %d, want 1", r.Edges)
	}
}

func TestG32_Scan_StringNamespaceExportName(t *testing.T) {
	r := Scan(treeOf(map[string]string{
		".opencode/scripts/ns.js": "export * as \"ns-mod\" from \"./a.js\";\n",
		".opencode/scripts/a.js":  "export const inner = 1;\n",
		".opencode/scripts/c.js":  "import { \"ns-mod\" as m } from \"./ns.js\";\n",
		".opencode/scripts/c2.js": "import { inner } from \"./ns.js\";\n",
		".opencode/scripts/c3.js": "import { nope } from \"./ns.js\";\n",
	}))
	if findBroken(r, ".opencode/scripts/c.js", "ns-mod") != nil ||
		findUnknownContaining(r, ".opencode/scripts/c.js", "open export surface") != nil {
		t.Fatalf("string namespace name must resolve: %+v / %+v", r.Broken, r.Unknown)
	}
	if findBroken(r, ".opencode/scripts/c2.js", "inner") == nil {
		t.Fatalf("inner is NOT re-exported by the ns form — proven broken")
	}
	if findBroken(r, ".opencode/scripts/c3.js", "nope") == nil {
		t.Fatalf("absent name over closed ns surface — proven broken")
	}
}

// --- slash-boundary discriminators (G32 ports; the G32 root defect family) -----

func TestG32_Scan_StatementRegexAfterBlockAndControlParen(t *testing.T) {
	r := Scan(treeOf(map[string]string{
		".opencode/scripts/p.js": "let s = 'x';\n" +
			"if (s) { s = 'y'; }\n" +
			"/[{]/.test(s);\n" + // statement-position regex after a BLOCK close
			"if (s) /[}]/.test(s);\n" + // after a control paren close
			"export const ok = 1;\n",
		".opencode/scripts/c.js": "import { ok } from \"./p.js\";\n",
	}))
	if r.HasBroken() {
		t.Fatalf("statement regexes must stay opaque: broken=%+v", r.Broken)
	}
	if len(r.Unknown) != 0 {
		t.Fatalf("expected clean scan, got unknown: %+v", r.Unknown)
	}
}

func TestG32_Scan_DivisionAfterObjectLiteralClose(t *testing.T) {
	r := Scan(treeOf(map[string]string{
		".opencode/scripts/p.js": "const n = {} / 2;\n" +
			"export const actual = 1;\n" +
			"const z = 4 / 2;\n",
		".opencode/scripts/c.js": "import { actual } from \"./p.js\";\n",
	}))
	if r.HasBroken() {
		t.Fatalf("object-literal division must not hide exports: broken=%+v", r.Broken)
	}
	if len(r.Unknown) != 0 {
		t.Fatalf("expected clean scan, got unknown: %+v", r.Unknown)
	}
}

func TestG32_Scan_KeywordPropertyValuesAreDivision(t *testing.T) {
	for _, kw := range []string{"return", "typeof", "default", "of", "extends"} {
		src := "const obj = { " + kw + ": 4 };\n" +
			"obj." + kw + " / 2;\n" +
			"export const actual = 1;\n" +
			"const z = 4 / 2;\n"
		r := Scan(treeOf(map[string]string{
			".opencode/scripts/p.js": src,
			".opencode/scripts/c.js": "import { actual } from \"./p.js\";\n",
		}))
		if r.HasBroken() || len(r.Unknown) != 0 {
			t.Fatalf("property %q must be a value (division): broken=%+v unknown=%+v", kw, r.Broken, r.Unknown)
		}
	}
}

// Adapted-port note: the archive's r3 case (`const C = class {} / 2;`) is
// NOT applicable — class EXPRESSIONS are excluded by the frozen grammar; the
// successor pins that shape to UNKNOWN(reason-coded) here instead.
func TestG32_Scan_ExpressionBodyCloseIsDivision(t *testing.T) {
	r := Scan(treeOf(map[string]string{
		".opencode/scripts/p.js": "const n = function() {} / 2;\n" +
			"export const actual = 1;\n" +
			"const z = 4 / 2;\n",
		".opencode/scripts/c.js": "import { actual } from \"./p.js\";\n",
	}))
	if r.HasBroken() || len(r.Unknown) != 0 {
		t.Fatalf("function-expression body close must be division: broken=%+v unknown=%+v", r.Broken, r.Unknown)
	}

	r2 := Scan(treeOf(map[string]string{
		".opencode/scripts/p.js": "const f = (() => {}) / 2;\n" +
			"export const hidden = 1;\n" +
			"const g = 3 / 4;\n",
		".opencode/scripts/c.js": "import { hidden } from \"./p.js\";\n",
	}))
	if r2.HasBroken() || len(r2.Unknown) != 0 {
		t.Fatalf("parenthesized arrow body close must be division: broken=%+v unknown=%+v", r2.Broken, r2.Unknown)
	}

	r4 := Scan(treeOf(map[string]string{
		".opencode/scripts/p.js": "const a = async function() {} / 2;\n" +
			"export const af = 1;\n" +
			"const y = 6 / 2;\n",
		".opencode/scripts/c.js": "import { af } from \"./p.js\";\n",
	}))
	if r4.HasBroken() || len(r4.Unknown) != 0 {
		t.Fatalf("async function-expression body close must be division: broken=%+v unknown=%+v", r4.Broken, r4.Unknown)
	}

	// Declaration bodies keep statement-position regexes lexable.
	r5 := Scan(treeOf(map[string]string{
		".opencode/scripts/p.js": "function f() {}\n/[{]/.test('x');\nexport const decl = 1;\n",
		".opencode/scripts/c.js": "import { decl } from \"./p.js\";\n",
	}))
	if r5.HasBroken() || len(r5.Unknown) != 0 {
		t.Fatalf("declaration body close keeps statement regex: broken=%+v unknown=%+v", r5.Broken, r5.Unknown)
	}

	// Frozen-grammar pin: the class-expression shape degrades reason-coded.
	m := Parse(".opencode/scripts/p.js", []byte("const C = class {} / 2;\nexport const cls = 1;\n"))
	if !m.Unparseable || !strings.Contains(m.UnparseableWhy, ReasonClassExpr) {
		t.Fatalf("class expression must degrade with %q, got %q", ReasonClassExpr, m.UnparseableWhy)
	}
}

// Adapted-port note: the archive wrapped the substitution in a TAGGED
// template; tagged templates are excluded by the frozen grammar, so the port
// drops the tag (discrimination retained) and pins the tagged shape to its
// frozen verdict separately.
func TestG32_Scan_TemplateSubstitutionLeadingRegex(t *testing.T) {
	r := Scan(treeOf(map[string]string{
		".opencode/scripts/p.js": "const s = 'x';\n" +
			"const t = `${/[{]/.test(s)} text`;\n" +
			"export const actual = 1;\n" +
			"const z = 4 / 2;\n",
		".opencode/scripts/c.js": "import { actual } from \"./p.js\";\n",
	}))
	if r.HasBroken() || len(r.Unknown) != 0 {
		t.Fatalf("substitution-leading regex must lex opaquely: broken=%+v unknown=%+v", r.Broken, r.Unknown)
	}

	// Division INSIDE a substitution still works.
	r2 := Scan(treeOf(map[string]string{
		".opencode/scripts/p.js": "const d = 2;\nconst u = `${d / 2}`;\nexport const actual = 1;\n",
		".opencode/scripts/c.js": "import { actual } from \"./p.js\";\n",
	}))
	if r2.HasBroken() || len(r2.Unknown) != 0 {
		t.Fatalf("substitution division must stay division: broken=%+v unknown=%+v", r2.Broken, r2.Unknown)
	}

	// Frozen-grammar pin: the tagged shape degrades reason-coded.
	m := Parse(".opencode/scripts/p.js", []byte("const s = 'x';\nconst t = tag`${s} text`;\n"))
	if !m.Unparseable || !strings.Contains(m.UnparseableWhy, ReasonTaggedTmpl) {
		t.Fatalf("tagged template must degrade with %q, got %q", ReasonTaggedTmpl, m.UnparseableWhy)
	}
}

func TestG32_Scan_TrailingDotNumberDivision(t *testing.T) {
	r := Scan(treeOf(map[string]string{
		".opencode/scripts/p.js": "const r = 1. / 2;\n" +
			"export const hidden = 1;\n" +
			"const s = 3 / 4;\n",
		".opencode/scripts/c.js": "import { hidden } from \"./p.js\";\n",
	}))
	if r.HasBroken() || len(r.Unknown) != 0 {
		t.Fatalf("trailing-dot division must not hide exports: broken=%+v unknown=%+v", r.Broken, r.Unknown)
	}
}

func TestG32_Scan_RegexLiteralsWithQuotesAndBraces(t *testing.T) {
	r := Scan(treeOf(map[string]string{
		".opencode/scripts/p.js": "const q = /\"quoted\"/.test(\"x\");\n" +
			"const b = /[{]}/.test(\"y\");\n" +
			"const c = /a|b\\/c/.test(\"z\");\n" +
			"const d = /\\//.test(\"w\");\n" +
			"export const actual = 1;\n",
		".opencode/scripts/c.js": "import { actual } from \"./p.js\";\n",
	}))
	if r.HasBroken() || len(r.Unknown) != 0 {
		t.Fatalf("regex quotes/braces/pipes must stay opaque: broken=%+v unknown=%+v", r.Broken, r.Unknown)
	}
}

func TestG32_Scan_TemplateLiteralsWithSubstitutions(t *testing.T) {
	r := Scan(treeOf(map[string]string{
		".opencode/scripts/p.js": "const ev = [{ file: \"a\", line: 1 }];\n" +
			"const t = ev.map((r) => `${r.file}:${r.line}`).join(\", \");\n" +
			"const n = `${stderr ? `: ${stderr}` : \"\"}`;\n" +
			"export const actual = t + n;\n",
		".opencode/scripts/c.js": "import { actual } from \"./p.js\";\n",
	}))
	if r.HasBroken() || len(r.Unknown) != 0 {
		t.Fatalf("nested template substitutions must parse: broken=%+v unknown=%+v", r.Broken, r.Unknown)
	}
}

// --- async/await scope discipline (G32 ports, adapted to frozen semantics) -----

// Adapted-port note: the archive's p1 (paren async arrows whose await operand
// is a regex) ports verbatim. The archive's p2 (`async =>` param named
// "async", await-as-identifier) pinned a semantics the frozen grammar
// deliberately REJECTS (await is a keyword, never an identifier); the port
// pins the frozen verdicts instead. The archive's p3 lived in a class FIELD
// (excluded); the phantom-regex discrimination is retained without the field.
func TestG32_Scan_AsyncArrowParenAndConciseForms(t *testing.T) {
	tree := treeOf(map[string]string{
		// Paren params: [+Await] — `await /re/` is an await-expression
		// whose operand is the regex; misclassifying it as division
		// desyncs braces across the mirrored pair and hides `actual`.
		"p1.js": "function f() { const g = async (a, b) => { let v = await /[{]/.test(\"x\"); }; return g; }\n" +
			"export const actual = 1;\n" +
			"function h() { const g = async () => { let v = await /[}]/.test(\"y\"); }; return g; }\n",
		"c1.js": "import { actual } from \"./p1.js\";\n",
		// Frozen pin: `async =>` is a NON-async arrow over a parameter
		// named "async"; awaiting inside it degrades reason-coded.
		"p2.js": "function f() { const g = async => { return await 1; }; }\n" +
			"export const actual = 1;\nconst z = 4 / 2;\n",
		"c2.js": "import { actual } from \"./p2.js\";\n",
		// Concise async arrow at module top (amendment: module-top async
		// context): the body is [+Await], so the regex is the await
		// operand — `phantom` occurs only inside regex text and the
		// consumer is genuinely broken.
		"p3.js": "const f = async x => await /} export const phantom = 1; {x/.test(\"\");\n" +
			"export const real = 2;\n",
		"c3.js": "import { phantom } from \"./p3.js\";\n",
		"c4.js": "import { real } from \"./p3.js\";\n",
	})
	r := Scan(tree)
	if b := findBroken(r, "c1.js", "actual"); b != nil {
		t.Fatalf("paren async arrows are [+Await]; actual exported: %+v", b)
	}
	if findUnknownContaining(r, "p2.js", "module UNKNOWN") == nil {
		t.Fatalf("p2 must degrade: awaiting in a non-async arrow body: %+v", r.Unknown)
	}
	m2 := Parse("p2.js", tree["p2.js"])
	if !m2.Unparseable || !strings.Contains(m2.UnparseableWhy, ReasonTopAwait) {
		t.Fatalf("p2 must degrade with %q, got %q", ReasonTopAwait, m2.UnparseableWhy)
	}
	if b := findBroken(r, "c3.js", "phantom"); b == nil {
		t.Fatalf("concise-body regex text must not fabricate an export (phantom genuinely absent)")
	}
	if b := findBroken(r, "c4.js", "real"); b != nil {
		t.Fatalf("real is genuinely exported: %+v", b)
	}
}

// Adapted-port note: the archive pinned await-as-identifier FIT inside
// non-async bodies; the frozen grammar rejects await outside async contexts
// outright, so the port pins the degrade + the await-free clean case.
func TestG32_Scan_NonAsyncArrowBodiesRevokeAwait(t *testing.T) {
	tree := treeOf(map[string]string{
		"p.js": "const f = () => { return helper(1); };\n" +
			"export const actual = 1;\n" +
			"const g = function() { let q = 4 / 2; };\n",
		"c.js": "import { actual } from \"./p.js\";\n",
	})
	r := Scan(tree)
	if b := findBroken(r, "c.js", "actual"); b != nil {
		t.Fatalf("non-async arrow bodies parse cleanly: %+v", b)
	}
	if len(r.Unknown) != 0 {
		t.Fatalf("clean module must not degrade: %+v", r.Unknown)
	}
	// await inside a non-async concise arrow body degrades reason-coded.
	tree2 := treeOf(map[string]string{
		"p.js": "function f() { const g = x => await y; }\nexport const actual = 1;\n",
		"c.js": "import { actual } from \"./p.js\";\n",
	})
	m := Parse("p.js", tree2["p.js"])
	if !m.Unparseable || !strings.Contains(m.UnparseableWhy, ReasonTopAwait) {
		t.Fatalf("await in non-async concise arrow must degrade with %q, got %q", ReasonTopAwait, m.UnparseableWhy)
	}
	r2 := Scan(tree2)
	if r2.HasBroken() {
		t.Fatalf("degraded module must never ground broken: %+v", r2.Broken)
	}
	if findUnknownContaining(r2, "p.js", "module UNKNOWN") == nil {
		t.Fatalf("expected module-UNKNOWN: %+v", r2.Unknown)
	}
}

// --- for-head and case-body discriminators (G32 ports, adapted) ----------------

func TestG32_Scan_OfIsContextual(t *testing.T) {
	r := Scan(treeOf(map[string]string{
		".opencode/scripts/p.js": "const of = 4;\n" +
			"const r = of / 2;\n" +
			"export const actual = 1;\n" +
			"const z = 5 / 2;\n",
		".opencode/scripts/c.js": "import { actual } from \"./p.js\";\n",
	}))
	if r.HasBroken() || len(r.Unknown) != 0 {
		t.Fatalf("identifier `of` must be a value (division): broken=%+v unknown=%+v", r.Broken, r.Unknown)
	}

	// Adapted-port note: the archive pinned this shape UNKNOWN (its lexer
	// could not decide the slash after `of`). The frozen grammar's AMB-2
	// resolution decides it by parse structure: the for-of RHS is an
	// EXPRESSION START, so the regex is admitted and the module is FIT.
	r2 := Scan(treeOf(map[string]string{
		".opencode/scripts/p.js": "const s = 'ab';\n" +
			"for (const x of /[{]/g.exec(s)) {}\n" +
			"export const actual = 1;\n",
		".opencode/scripts/c.js": "import { actual } from \"./p.js\";\n",
	}))
	if r2.HasBroken() || len(r2.Unknown) != 0 {
		t.Fatalf("for-of RHS regex is an expression start (AMB-2): broken=%+v unknown=%+v", r2.Broken, r2.Unknown)
	}
}

// Adapted-port note: the archive pinned tree1 UNKNOWN ("uncertain body
// close"); the frozen grammar admits statement-initial regex (D-3), so the
// successor pins FIT/ok for the same bytes — the discrimination (never a
// wrong closed surface) is retained in both eras.
func TestG32_Scan_CaseClauseDeclarationBody(t *testing.T) {
	tree := treeOf(map[string]string{
		".opencode/scripts/p.js": "switch(0){case 0: function f() {}\n/[{]/.test(\"\");}\nexport const actual = 1;\nswitch(0){case 0: function g() {}\n/[}]/.test(\"\");}\n",
		".opencode/scripts/c.js": "import { actual } from \"./p.js\";\n",
	})
	r := Scan(tree)
	if b := findBroken(r, ".opencode/scripts/c.js", "actual"); b != nil {
		t.Fatalf("actual must never be proven broken: %+v", b)
	}
	if len(r.Unknown) != 0 {
		t.Fatalf("frozen grammar: statement regex after case body is admitted: %+v", r.Unknown)
	}

	// Class-declaration variant.
	tree2 := treeOf(map[string]string{
		".opencode/scripts/p.js":  "switch(0){case 0: class C {}\n/re/.test(\"\");}\nexport const actual = 1;\n",
		".opencode/scripts/c2.js": "import { actual } from \"./p.js\";\n",
	})
	r2 := Scan(tree2)
	if b := findBroken(r2, ".opencode/scripts/c2.js", "actual"); b != nil {
		t.Fatalf("class variant: actual must never be proven broken: %+v", b)
	}
	if len(r2.Unknown) != 0 {
		t.Fatalf("class variant: clean under frozen grammar: %+v", r2.Unknown)
	}

	// Without a directly following '/', the module parses cleanly.
	tree3 := treeOf(map[string]string{
		".opencode/scripts/p.js":  "switch(0){case 0: function f() { return 1; }}\nexport const actual = 1;\n",
		".opencode/scripts/c3.js": "import { actual } from \"./p.js\";\n",
	})
	r3 := Scan(tree3)
	if b := findBroken(r3, ".opencode/scripts/c3.js", "actual"); b != nil {
		t.Fatalf("no-slash variant: actual is genuinely exported: %+v", b)
	}
	if len(r3.Unknown) != 0 {
		t.Fatalf("no-slash variant: no unknown expected: %+v", r3.Unknown)
	}
}

// --- keyword/property shapes (G32 ports) ---------------------------------------

func TestG32_Scan_PropertyNamedFunctionAndClass(t *testing.T) {
	cases := map[string]string{
		"function-call": "const obj = {};\n" +
			"obj.function(1);\n" +
			"const o = { a: 1 } / 2;\n" +
			"export const actual = 1;\n" +
			"const g = 4 / 2;\n",
		"class-call": "const obj = {};\n" +
			"obj.class(1);\n" +
			"const n = {} / 2;\n" +
			"export const actual = 1;\n" +
			"const z = 4 / 2;\n",
		"class-value": "const obj2 = { value: 1 };\n" +
			"obj2.class;\n" +
			"const n2 = {} / 2;\n" +
			"export const actual = 1;\n" +
			"const z2 = 4 / 2;\n",
	}
	for name, src := range cases {
		r := Scan(treeOf(map[string]string{
			".opencode/scripts/p.js": src,
			".opencode/scripts/c.js": "import { actual } from \"./p.js\";\n",
		}))
		if r.HasBroken() || len(r.Unknown) != 0 {
			t.Fatalf("%s: property-named keyword corrupted the surface: broken=%+v unknown=%+v", name, r.Broken, r.Unknown)
		}
	}
}

func TestG32_Scan_ExportImportInPropertyPosition(t *testing.T) {
	r := Scan(treeOf(map[string]string{
		".opencode/scripts/p.js": "const obj = { export: { default: 1 } };\n" +
			"obj.export.default;\n" +
			"const foo = { import: (s) => s };\n" +
			"export const actual = foo.import(\"./x.js\");\n",
		".opencode/scripts/c.js": "import d from \"./p.js\";\n",
	}))
	if findBroken(r, ".opencode/scripts/c.js", "default") == nil {
		t.Fatalf("genuinely absent default must be proven broken: %+v", r.Broken)
	}
	for _, u := range r.Unknown {
		if strings.Contains(u.Reason, "dynamic import") {
			t.Fatalf("property-position import() must not be a dynamic finding: %+v", u)
		}
	}
}

// --- lexer basics (G32 ports) ---------------------------------------------------

func TestG32_Scan_LineTerminatorsEndLineComments(t *testing.T) {
	cases := map[string]string{
		"lf":       "// c\nexport const actual = 1;\n",
		"cr":       "// c\rexport const actual = 1;\n",
		"crlf":     "// c\r\nexport const actual = 1;\n",
		"u2028":    "// c\u2028export const actual = 1;\n",
		"u2029":    "// c\u2029export const actual = 1;\n",
		"block-cr": "/* c */\rexport const actual = 1;\n",
	}
	for name, src := range cases {
		r := Scan(treeOf(map[string]string{
			".opencode/scripts/p.js": src,
			".opencode/scripts/c.js": "import { actual } from \"./p.js\";\n",
		}))
		if r.HasBroken() || len(r.Unknown) != 0 {
			t.Fatalf("%s: terminator must end comment: broken=%+v unknown=%+v", name, r.Broken, r.Unknown)
		}
	}
}

func TestG32_Scan_FormFeedVerticalTabWhitespace(t *testing.T) {
	tree := treeOf(map[string]string{
		"p.js": "const a = 4\f/ 2;\n" +
			"export const actual = 1;\n" +
			"const b = 5\v/ 2;\n",
		"c.js": "import { actual } from \"./p.js\";\n",
	})
	r := Scan(tree)
	if b := findBroken(r, "c.js", "actual"); b != nil {
		t.Fatalf("FF/VT are whitespace: divisions must stay divisions, actual is exported: %+v", b)
	}
	if len(r.Unknown) != 0 {
		t.Fatalf("FF/VT whitespace must parse cleanly: %+v", r.Unknown)
	}
}

func TestG32_Scan_BomPrefix(t *testing.T) {
	r := Scan(treeOf(map[string]string{
		".opencode/scripts/p.js": "\uFEFFexport const bom = 1;\n",
		".opencode/scripts/c.js": "import { bom } from \"./p.js\";\n",
	}))
	if r.HasBroken() || len(r.Unknown) != 0 {
		t.Fatalf("BOM-prefixed module must scan cleanly: broken=%+v unknown=%+v", r.Broken, r.Unknown)
	}
}

func TestG32_Scan_HashbangCommentIgnored(t *testing.T) {
	// Phantom-fabrication direction: hashbang text is not code.
	r := Scan(treeOf(map[string]string{
		".opencode/scripts/p.js": "#! export const phantom = 1;\n" +
			"export const real = 2;\n",
		".opencode/scripts/c.js": "import { phantom } from \"./p.js\";\n",
	}))
	if findBroken(r, ".opencode/scripts/c.js", "phantom") == nil {
		t.Fatalf("hashbang text must not fabricate exports: broken=%+v", r.Broken)
	}
	if len(r.Unknown) != 0 {
		t.Fatalf("hashbang module must stay parseable: unknown=%+v", r.Unknown)
	}

	// Real-export direction, incl. the classic slash-bearing shebang.
	for _, hb := range []string{"#!/usr/bin/env node", "#! some text"} {
		r := Scan(treeOf(map[string]string{
			".opencode/scripts/p.js": hb + "\nexport const actual = 1;\n",
			".opencode/scripts/c.js": "import { actual } from \"./p.js\";\n",
		}))
		if r.HasBroken() || len(r.Unknown) != 0 {
			t.Fatalf("hashbang %q: expected clean scan, broken=%+v unknown=%+v", hb, r.Broken, r.Unknown)
		}
	}

	// A module that is ONLY a hashbang parses to an empty surface.
	r2 := Scan(treeOf(map[string]string{
		".opencode/scripts/p.js": "#!/usr/bin/env node",
		".opencode/scripts/c.js": "import { x } from \"./p.js\";\n",
	}))
	if findBroken(r2, ".opencode/scripts/c.js", "x") == nil {
		t.Fatalf("hashbang-only module has no exports (genuinely broken): broken=%+v", r2.Broken)
	}
	if len(r2.Unknown) != 0 {
		t.Fatalf("hashbang-only module must stay parseable: unknown=%+v", r2.Unknown)
	}

	// CR-terminated hashbang line (all four line terminators honor it).
	r3 := Scan(treeOf(map[string]string{
		".opencode/scripts/p.js": "#! junk\rexport const actual = 1;\n",
		".opencode/scripts/c.js": "import { actual } from \"./p.js\";\n",
	}))
	if r3.HasBroken() || len(r3.Unknown) != 0 {
		t.Fatalf("CR-terminated hashbang: broken=%+v unknown=%+v", r3.Broken, r3.Unknown)
	}
}

func TestG32_Scan_TruncatedModuleIsUnknown(t *testing.T) {
	for i, src := range []string{
		"export const a = {\n",
		"foo(\n",
		"export const a = 1;\nif (\n",
		"const xs = [1,\n",
		"export const f = function\n",
		"export const C = class\n",
	} {
		m := Parse(".opencode/scripts/p.js", []byte(src))
		if !m.Unparseable {
			t.Fatalf("case %d: truncated module must be unparseable", i)
		}
	}
}

// --- path helpers (G32 ports) ----------------------------------------------------

func TestG32_ResolveSpecifier(t *testing.T) {
	cases := []struct{ importer, spec, want string }{
		{".opencode/scripts/a.js", "./b.js", ".opencode/scripts/b.js"},
		{".opencode/plugins/p.js", "../scripts/s.js", ".opencode/scripts/s.js"},
		{".opencode/a/b.js", "./c/../d.js", ".opencode/a/d.js"},
		{".opencode/a.js", "./sub/x.js", ".opencode/sub/x.js"},
	}
	for _, c := range cases {
		if got := ResolveSpecifier(c.importer, c.spec); got != c.want {
			t.Fatalf("ResolveSpecifier(%q, %q) = %q, want %q", c.importer, c.spec, got, c.want)
		}
	}
}

func TestG32_IsRelativeSpecifier(t *testing.T) {
	for spec, want := range map[string]bool{
		"./a.js": true, "../a.js": true, "a.js": false, "node:fs": false,
		"some-pkg": false, "/abs/path.js": false, "https://x/y.js": false,
	} {
		if got := IsRelativeSpecifier(spec); got != want {
			t.Fatalf("IsRelativeSpecifier(%q) = %v, want %v", spec, got, want)
		}
	}
}
