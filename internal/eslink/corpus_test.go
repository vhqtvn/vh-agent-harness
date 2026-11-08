package eslink

// Corpus-level gates (staged-restart spec §B): gate 3 (independent audit of
// the 49-edge ledger), gate 4 (revision/hash-bound golden pins), gate 5
// (mixed preserved-tree skew, REQUIRED), gate 6 (disaggregated counters).
//
// PIN SET (the family-5 grammar-freeze AMENDMENT, operator-ratified
// 2026-11-08; measured derivation in the S1′ escalation report §2B):
//
//	modules 61 · FIT 45 · ledger edges 49 · L0/L1/L3 = 17/22/10
//	dynamic unknowns 5 · clean-tree broken 0
//	mixed-tree broken = 1: verify-task-registry.js → state-lib.js
//	missing collectDecisionItems
//
// Cross-check vs the O3 packet census (d4ae991): every pin matches the
// packet EXCEPT FIT 46 → 45; the single-module delta is
// plugins/compaction-primitives.js, which is objectively invalid JavaScript
// (`node --check` → SyntaxError: missing ) after argument list, template
// opened line 11 terminated by unescaped backticks line 18 — re-derived
// fresh this slice). The packet's reference lexer mis-lexed it as balanced
// and the census counted it FIT. The corpus defect is carded separately at
// .local/coordinator/tasks/task-2026-11-08t16-27-14-corpus-defect-…json;
// the successor pins its honest verdict: reason-coded UNKNOWN.

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path"
	"regexp"
	"sort"
	"strings"
	"testing"

	corpus "github.com/vhqtvn/vh-agent-harness"
)

// corpusTree assembles the 61-module effective tree exactly as the G32
// corpus test and the packet harness did: core templates under
// templates/core/.opencode plus every overlay pack mirrored at .opencode/
// rendered-destination keys.
func corpusTree(t *testing.T) map[string][]byte {
	t.Helper()
	tree := map[string][]byte{}
	filesSeen := 0
	sub, err := fs.Sub(corpus.CoreFS, corpus.CoreDir)
	if err != nil {
		t.Fatal(err)
	}
	err = fs.WalkDir(sub, ".opencode", func(p string, d fs.DirEntry, err error) error {
		return corpusCollect(sub, p, p, d, err, tree, &filesSeen)
	})
	if err != nil {
		t.Fatal(err)
	}
	packs, err := fs.ReadDir(corpus.OverlaysFS, corpus.OverlaysDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, pack := range packs {
		if !pack.IsDir() {
			continue
		}
		packFS, err := fs.Sub(corpus.OverlaysFS, path.Join(corpus.OverlaysDir, pack.Name()))
		if err != nil {
			t.Fatal(err)
		}
		err = fs.WalkDir(packFS, ".", func(p string, d fs.DirEntry, err error) error {
			if p == "." {
				return nil
			}
			return corpusCollect(packFS, path.Join(".opencode", p), p, d, err, tree, &filesSeen)
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if filesSeen != len(tree) {
		t.Fatalf("destination collision: %d files walked but %d distinct keys", filesSeen, len(tree))
	}
	if len(tree) != 61 {
		t.Fatalf("corpus tree = %d modules, want 61", len(tree))
	}
	return tree
}

func corpusCollect(fsys fs.FS, destKey, p string, d fs.DirEntry, err error, tree map[string][]byte, filesSeen *int) error {
	if err != nil {
		return err
	}
	if d.IsDir() {
		return nil
	}
	ext := strings.ToLower(path.Ext(p))
	if ext != ".js" && ext != ".mjs" {
		return nil
	}
	b, err := fs.ReadFile(fsys, p)
	if err != nil {
		return err
	}
	(*filesSeen)++
	tree[destKey] = b
	return nil
}

// goldenEdge is one row of the frozen reference ledger (generated file
// ledger_golden_test.go). Line/SpecLine are the importer-side source span
// of the declaring import/export (decl keyword line → specifier-string
// line) — the span columns added by the cascade-3 t1c-F1 hardening so a
// span drift (e.g. an off-by-one line recorder) fails gate 3 even when
// the edge SET is unchanged.
type goldenEdge struct {
	Type     string
	Importer string
	Provider string
	Presence bool
	Symbols  []string
	Line     int
	SpecLine int
}

func edgeKey(e goldenEdge) string {
	syms := append([]string(nil), e.Symbols...)
	sort.Strings(syms)
	return fmt.Sprintf("%s|%s|%s|%v|%s|%d-%d", e.Type, e.Importer, e.Provider, e.Presence, strings.Join(syms, ","), e.Line, e.SpecLine)
}

// --- gate 3: independent audit of the 49-edge ledger ---------------------------

func TestGate3_SuccessorLedgerMatchesReferenceAudit(t *testing.T) {
	tree := corpusTree(t)
	_, edges := EdgeLedger(tree)
	if len(edges) != len(goldenEdges) {
		t.Fatalf("successor ledger has %d edges, golden has %d", len(edges), len(goldenEdges))
	}
	got := map[string]int{}
	for _, e := range edges {
		var syms []string
		syms = append(syms, e.Symbols...)
		got[edgeKey(goldenEdge{Type: e.Type, Importer: e.Importer, Provider: e.Provider, Presence: e.Presence, Symbols: syms, Line: e.Line, SpecLine: e.SpecLine})]++
	}
	want := map[string]int{}
	for _, e := range goldenEdges {
		want[edgeKey(e)]++
	}
	for k, n := range want {
		if got[k] != n {
			t.Fatalf("edge %q: successor %d×, reference audit %d×", k, got[k], n)
		}
	}
	for k, n := range got {
		if want[k] != n {
			t.Fatalf("edge %q: successor %d×, reference audit %d× (fabricated or shifted)", k, n, want[k])
		}
	}
	// The packet §C L-join on the same ledger.
	lc := map[string]int{}
	for _, e := range edges {
		lc[e.LClass]++
	}
	if lc["L0"] != 17 || lc["L1"] != 22 || lc["L3"] != 10 {
		t.Fatalf("L-join = L0:%d L1:%d L3:%d, want 17/22/10", lc["L0"], lc["L1"], lc["L3"])
	}
}

// TestGate3_IndependentRegexCrossCheck re-derives a SUBSET of the edge set
// with a deliberately dumb, line-based regex extractor (no shared code with
// the parser) and requires the successor ledger to contain every edge it
// finds — the "S1 cannot be its own correctness oracle" arm of gate 3. The
// regex only sees single-line forms, so it under-approximates by design;
// every hit must still agree.
func TestGate3_IndependentRegexCrossCheck(t *testing.T) {
	tree := corpusTree(t)
	type rgx struct {
		importer, spec string
		symbols        []string
	}
	var found []rgx
	for mod, src := range tree {
		for _, line := range strings.Split(string(src), "\n") {
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "import {") {
				continue
			}
			closeIdx := strings.Index(line, "}")
			fromIdx := strings.Index(line, "from ")
			if closeIdx < 0 || fromIdx < closeIdx {
				continue
			}
			list := line[len("import {"):closeIdx]
			rest := strings.TrimSpace(line[fromIdx+len("from"):])
			if !strings.HasPrefix(rest, "\".") || !strings.HasSuffix(rest, "\";") {
				continue
			}
			spec := rest[1 : len(rest)-2]
			var syms []string
			for _, entry := range strings.Split(list, ",") {
				entry = strings.TrimSpace(entry)
				if entry == "" {
					continue
				}
				name := strings.Fields(entry)[0]
				syms = append(syms, strings.Trim(name, `"`))
			}
			found = append(found, rgx{importer: mod, spec: spec, symbols: syms})
		}
	}
	if len(found) < 10 {
		t.Fatalf("independent extractor found only %d single-line edges — suspiciously few; check the extractor", len(found))
	}
	_, edges := EdgeLedger(tree)
	have := map[string][]string{}
	for _, e := range edges {
		if e.Type != "import" {
			continue
		}
		key := e.Importer + "|" + e.Specifier
		have[key] = append(have[key], e.Symbols...)
	}
	for _, f := range found {
		key := f.importer + "|" + f.spec
		got, ok := have[key]
		if !ok {
			t.Fatalf("independent extractor found edge %s → %q; successor ledger lacks it", f.importer, f.spec)
		}
		sort.Strings(got)
		want := append([]string(nil), f.symbols...)
		sort.Strings(want)
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("edge %s → %q: successor symbols %v, independent %v", f.importer, f.spec, got, want)
		}
	}
}

// --- gate 4: revision/hash-bound golden pins ------------------------------------

// corpusDigest binds every pin below to the exact scanned bytes: sha256 over
// sorted (key, per-file sha256) rows. Any corpus edit (rendered or overlay)
// changes the digest and fails this test, forcing an explicit re-pin.
func corpusDigest(tree map[string][]byte) string {
	h := sha256.New()
	for _, k := range sortedKeys(tree) {
		sum := sha256.Sum256(tree[k])
		fmt.Fprintf(h, "%s\x00%s\n", k, hex.EncodeToString(sum[:]))
	}
	return hex.EncodeToString(h.Sum(nil))
}

const pinnedCorpusDigest = "fea47acd172c81a28d42112135033d567f4bd0e47d30f45f13f4bc667834f095"

// updateOpDebtFiles are the packet §B per-file table's 15 modules (the only
// corpus grammar debt: `++`/`--` update operators).
var updateOpDebtFiles = []string{
	".opencode/plugins/shell-guard-core.js",
	".opencode/plugins/shell-guard.js",
	".opencode/scripts/check-defer-triggers.mjs",
	".opencode/scripts/complexity-signal-lib.js",
	".opencode/scripts/state-lib.js",
	".opencode/scripts/check-s2-holds.mjs",
	".opencode/scripts/verify-f3-authoring-surfaces.js",
	".opencode/scripts/verify-no-source-tree-only-paths.js",
	".opencode/scripts/verify-release-interaction-contract.js",
	".opencode/plugins/auto-gate-live.js",
	".opencode/plugins/auto-gate-tiered.js",
	".opencode/plugins/auto-tool-gate.js",
	".opencode/plugins/session-progress.js",
	".opencode/scripts/session-progress-judge.js",
	".opencode/scripts/session-progress-policy.js",
}

func TestGate4_CorpusGoldenPins(t *testing.T) {
	tree := corpusTree(t)
	if d := corpusDigest(tree); d != pinnedCorpusDigest {
		t.Fatalf("corpus digest %s != pinned %s — the scanned bytes moved; re-derive the pin set before re-pinning (spec: never re-pin to absorb)", d, pinnedCorpusDigest)
	}

	s := Summarize(tree)
	if s.Modules != 61 || s.ModulesFit != 45 || s.ModulesUnknown != 16 {
		t.Fatalf("modules 61/45/16 pin: got %d/%d/%d", s.Modules, s.ModulesFit, s.ModulesUnknown)
	}
	if s.Edges != 49 || s.EdgesBroken != 0 || s.DynamicUnknowns != 5 {
		t.Fatalf("edges 49 / broken 0 / dynUnknowns 5 pin: got %d/%d/%d", s.Edges, s.EdgesBroken, s.DynamicUnknowns)
	}
	if s.L0Edges != 17 || s.L1Edges != 22 || s.L3Edges != 10 {
		t.Fatalf("L-join 17/22/10 pin: got %d/%d/%d", s.L0Edges, s.L1Edges, s.L3Edges)
	}

	// Per-module verdicts: the 15 update-op debt files degrade with the
	// update-op reason; compaction-primitives.js degrades (known corpus
	// defect: invalid JS, carded separately); every other module is FIT.
	debt := map[string]bool{}
	for _, p := range updateOpDebtFiles {
		debt[p] = true
		m := Parse(p, tree[p])
		if !m.Unparseable || !strings.HasPrefix(m.UnparseableWhy, ReasonUpdateOp) {
			t.Fatalf("%s: expected UNKNOWN(%s…) at the packet-cited line, got FIT or %q", p, ReasonUpdateOp, m.UnparseableWhy)
		}
	}
	const cp = ".opencode/plugins/compaction-primitives.js"
	if m := Parse(cp, tree[cp]); !m.Unparseable {
		t.Fatalf("%s: objectively invalid JS (node --check) — must be reason-coded UNKNOWN, got FIT", cp)
	}
	fit := 0
	for p, b := range tree {
		if p == cp || debt[p] {
			continue
		}
		if m := Parse(p, b); !m.Unparseable {
			fit++
		} else {
			t.Fatalf("%s: unexpected UNKNOWN(%q) — pin set says FIT", p, m.UnparseableWhy)
		}
	}
	if fit != 45 {
		t.Fatalf("FIT modules = %d, want 45", fit)
	}
}

// --- rendered-variant corpus coverage (cascade-3 t1b-F3) ------------------------

// renderTokens mirrors the corpus's token-substitution seam, test-local:
// the real render pipeline lives in the harness binary; this test only
// needs the BYTE-LEVEL effect adopters see — every {{TOKEN}} replaced by
// a concrete value. The lint must hold identically over the rendered
// variants, not just the raw templates/ source (tokens occur only in
// comments and string literals, never in specifiers — parity is exact).
var renderTokens = map[string]string{
	"{{COORDINATOR_DIR}}": "coordinator",
	"{{PROJECT_SLUG}}":    "vh-agent-harness",
	"{{PROJECT_NAME}}":    "VH Agent Harness",
}

// tokenRe matches render-token-shaped text only ({{UPPER_CASE}}): the
// corpus legitimately carries non-token doubled braces inside strings
// (e.g. Go composite-literal examples in f3-design-readiness.js).
var tokenRe = regexp.MustCompile(`\{\{[A-Z][A-Z0-9_]*\}\}`)

func renderCorpusTree(t *testing.T, tree map[string][]byte) map[string][]byte {
	t.Helper()
	out := make(map[string][]byte, len(tree))
	replaced := 0
	for k, v := range tree {
		s := string(v)
		for tok, val := range renderTokens {
			replaced += strings.Count(s, tok)
			s = strings.ReplaceAll(s, tok, val)
		}
		if i := tokenRe.FindStringIndex(s); i != nil {
			t.Fatalf("%s: unknown render token %q — extend renderTokens", k, s[i[0]:i[1]])
		}
		out[k] = []byte(s)
	}
	if replaced == 0 {
		t.Fatalf("substitution replaced nothing — the corpus token inventory moved; re-derive renderTokens")
	}
	return out
}

// TestGate4b_RenderedVariantLintParity runs the lint over the
// token-substituted corpus (the variants adopters actually receive) and
// pins EXACT parity with the raw-template lint: identical module verdicts
// per module, identical aggregate counters (the gate-4 pin set), zero
// broken. A token ever landing inside a relative specifier or shifting
// syntax would break this parity and fail here.
func TestGate4b_RenderedVariantLintParity(t *testing.T) {
	tree := corpusTree(t)
	rendered := renderCorpusTree(t, tree)

	// Per-module verdict parity.
	for _, p := range sortedKeys(tree) {
		a := Parse(p, tree[p])
		b := Parse(p, rendered[p])
		if a.Unparseable != b.Unparseable {
			t.Fatalf("%s: verdict changed under token substitution: raw=%v rendered=%v (%s)", p, a.Unparseable, b.Unparseable, b.UnparseableWhy)
		}
	}

	// Aggregate parity: the rendered tree carries the same pins.
	raw, ren := Summarize(tree), Summarize(rendered)
	if ren.Modules != 61 || ren.ModulesFit != 45 || ren.ModulesUnknown != 16 {
		t.Fatalf("rendered modules 61/45/16 pin: got %d/%d/%d", ren.Modules, ren.ModulesFit, ren.ModulesUnknown)
	}
	if ren.Edges != 49 || ren.EdgesBroken != 0 || ren.DynamicUnknowns != 5 {
		t.Fatalf("rendered edges 49 / broken 0 / dynUnknowns 5 pin: got %d/%d/%d", ren.Edges, ren.EdgesBroken, ren.DynamicUnknowns)
	}
	if ren.L0Edges != 17 || ren.L1Edges != 22 || ren.L3Edges != 10 {
		t.Fatalf("rendered L-join 17/22/10 pin: got %d/%d/%d", ren.L0Edges, ren.L1Edges, ren.L3Edges)
	}
	if raw.Edges != ren.Edges || raw.L0Edges != ren.L0Edges || raw.L1Edges != ren.L1Edges || raw.L3Edges != ren.L3Edges {
		t.Fatalf("raw/rendered ledger divergence: %+v vs %+v", raw, ren)
	}

	// The ledger itself (with spans AND specifiers) is identical over both
	// variants — the parity key carries the raw specifier text so a token
	// landing inside a relative specifier cannot evade the check by
	// path-cleaning to the same provider (cascade-3 C5).
	_, rawEdges := EdgeLedger(tree)
	_, renEdges := EdgeLedger(rendered)
	if len(rawEdges) != len(renEdges) {
		t.Fatalf("ledger size changed under substitution: %d vs %d", len(rawEdges), len(renEdges))
	}
	edgeParityKey := func(e EdgeRecord) string {
		syms := append([]string(nil), e.Symbols...)
		sort.Strings(syms)
		return fmt.Sprintf("%s|%s|%q|%s|%v|%s|%d-%d", e.Type, e.Importer, e.Specifier, e.Provider, e.Presence, strings.Join(syms, ","), e.Line, e.SpecLine)
	}
	rawKeys := map[string]int{}
	for _, e := range rawEdges {
		rawKeys[edgeParityKey(e)]++
	}
	for _, e := range renEdges {
		k := edgeParityKey(e)
		rawKeys[k]--
		if rawKeys[k] < 0 {
			t.Fatalf("rendered ledger carries an edge the raw tree lacks: %s", k)
		}
	}
	for k, n := range rawKeys {
		if n != 0 {
			t.Fatalf("raw ledger carries an edge the rendered tree lacks: %s (%d)", k, n)
		}
	}
}

// --- gate 5: mixed preserved-tree skew (REQUIRED) -------------------------------

// TestGate5_MixedTreeSkewRequired reproduces the packet §E mixed-tree method
// (runmixed.sh: current corpus with the hub preserved at an older base) as
// the packet's demonstrated adopter-representable skew: the pre-cccf1b0
// hub lacked `collectDecisionItems` (git log -S: added by cccf1b0), and the
// pre-trilogy hub was fully conformant (0 update ops, packet §E era table),
// so an adopter preserving state-lib.js at that base presents a CLOSED
// surface missing exactly the symbol verify-task-registry.js requires. The
// successor must fire L3 enforcement: broken=1 on exactly that edge.
//
// The older hub is DERIVED from the current bytes by two era-representable
// surgeries: (1) remove the collectDecisionItems export (both export-list
// entries — the pre-cccf1b0 surface), (2) lift the current-era update-op
// debt at L5554 with the packet's sanctioned mechanical rewrite (`i++` →
// `i += 1` — restoring the pre-trilogy hub's 0-update-op conformance). The
// LITERAL b788b43^ blob (68 exports, FIT) was additionally run during
// development via the packet's runmixed.sh method and produced the
// identical broken=1 (receipt in the S1′ closeout; the blob is packet-local
// and not committable).
func TestGate5_MixedTreeSkewRequired(t *testing.T) {
	tree := corpusTree(t)
	const hub = ".opencode/scripts/state-lib.js"
	cur := string(tree[hub])
	const exportLine = "    collectDecisionItems,\n"
	if n := strings.Count(cur, exportLine); n != 2 {
		t.Fatalf("hub export-list shape moved: %d occurrences of the export line, want 2 — re-derive the skew fixture", n)
	}
	older := strings.ReplaceAll(cur, exportLine, "")
	const debtLine = "    for (let i = 0; i < s.length; i++) {\n"
	if strings.Count(older, debtLine) != 1 {
		t.Fatalf("hub update-op anchor moved; re-derive the skew fixture")
	}
	older = strings.Replace(older, debtLine, "    for (let i = 0; i < s.length; i += 1) {\n", 1)
	tree[hub] = []byte(older)

	// The preserved hub itself stays conformant with a CLOSED surface.
	m := Parse(hub, tree[hub])
	if m.Unparseable {
		t.Fatalf("preserved hub must parse FIT, got UNKNOWN(%s)", m.UnparseableWhy)
	}
	if m.NamedExports["collectDecisionItems"] {
		t.Fatalf("derived pre-cccf1b0 hub must NOT export collectDecisionItems")
	}

	r := Scan(tree)
	if len(r.Broken) != 1 {
		t.Fatalf("mixed-tree skew must produce EXACTLY broken=1, got %d: %+v / %+v", len(r.Broken), r.Broken, r.Unknown)
	}
	b := r.Broken[0]
	if b.Importer != ".opencode/scripts/verify-task-registry.js" ||
		b.Provider != hub ||
		b.Symbol != "collectDecisionItems" {
		t.Fatalf("broken edge mismatch: %+v", b)
	}
	if !strings.Contains(b.Reason, "surface closed, so the absence is proven") {
		t.Fatalf("broken verdict must cite proven absence: %+v", b)
	}
	// U6: the scan never refuses — it produced a full report.
	if r.Modules != 61 {
		t.Fatalf("mixed tree modules = %d, want 61", r.Modules)
	}
}

// TestGate5_LiteralPretrilogyBlobReceipt exercises the packet §E method on
// the LITERAL b788b43^ state-lib bytes (runmixed.sh: byte-replacement of the
// hub) whenever the conformance packet's historical blob is present — the
// dev-environment receipt backing the derived required test above. The blob
// is packet-local (tmp/eslink-o4-conformance-packet/hist/pretrilogy/) and
// not committable; in trees without the packet this test skips rather than
// fabricate coverage.
func TestGate5_LiteralPretrilogyBlobReceipt(t *testing.T) {
	const blobPath = "../../tmp/eslink-o4-conformance-packet/hist/pretrilogy/templates__core__.opencode__scripts__state-lib.js"
	blob, err := os.ReadFile(blobPath)
	if err != nil {
		t.Skipf("literal pretrilogy blob not present (%s) — derived required test above carries the gate", blobPath)
	}
	tree := corpusTree(t)
	const hub = ".opencode/scripts/state-lib.js"
	tree[hub] = blob

	m := Parse(hub, blob)
	if m.Unparseable {
		t.Fatalf("literal b788b43^ hub must parse FIT, got UNKNOWN(%s)", m.UnparseableWhy)
	}
	if len(m.NamedExports) != 68 {
		t.Fatalf("pretrilogy hub surface = %d export names, want the packet's 68", len(m.NamedExports))
	}
	r := Scan(tree)
	if len(r.Broken) != 1 {
		t.Fatalf("literal-blob mixed tree must produce EXACTLY broken=1, got %d: %+v / %+v", len(r.Broken), r.Broken, r.Unknown)
	}
	b := r.Broken[0]
	if b.Importer != ".opencode/scripts/verify-task-registry.js" ||
		b.Provider != hub ||
		b.Symbol != "collectDecisionItems" {
		t.Fatalf("broken edge mismatch: %+v", b)
	}
}

// --- gate 6: disaggregated reporting ---------------------------------------------

func TestGate6_DisaggregatedCounters(t *testing.T) {
	tree := corpusTree(t)
	s := Summarize(tree)

	// The three unknown classes are reported SEPARATELY, never blended.
	if s.UnknownByClass[ClassDynamicImport] != 5 {
		t.Fatalf("dynamic-import unknowns = %d, want 5 (the 5-pin class)", s.UnknownByClass[ClassDynamicImport])
	}
	if s.UnknownByClass[ClassUnsupportedSyntax] != 16 {
		t.Fatalf("unsupported-syntax unknowns = %d, want 16 (15 update-op + 1 invalid-JS module)", s.UnknownByClass[ClassUnsupportedSyntax])
	}
	if s.UnknownByClass[ClassOpenSurface] != 22 {
		t.Fatalf("open-surface unknowns = %d, want 22 (the L1 provider-open edges)", s.UnknownByClass[ClassOpenSurface])
	}
	if s.DynamicUnknowns != s.UnknownByClass[ClassDynamicImport] {
		t.Fatalf("DynamicUnknowns (%d) must equal the dynamic class counter (%d)", s.DynamicUnknowns, s.UnknownByClass[ClassDynamicImport])
	}
	// Closed obligations are their own counter (L3), never folded into
	// unknowns or edges totals.
	if s.L3Edges != 10 {
		t.Fatalf("closed obligations (L3) = %d, want 10", s.L3Edges)
	}
	if s.L0Edges+s.L1Edges+s.L3Edges != s.Edges {
		t.Fatalf("L0+L1+L3 (%d) must equal the ledger edge count (%d)", s.L0Edges+s.L1Edges+s.L3Edges, s.Edges)
	}
	// Module-level reason histogram: 15 update-op reasons (one per debt
	// file, line-numbered) + 1 compaction-primitives parse reason.
	updateReasons, otherReasons := 0, 0
	for why, n := range s.UnknownReasons {
		if strings.HasPrefix(why, ReasonUpdateOp) {
			updateReasons += n
		} else {
			otherReasons += n
		}
	}
	if updateReasons != 15 || otherReasons != 1 {
		t.Fatalf("module reason histogram = %d update-op + %d other, want 15 + 1", updateReasons, otherReasons)
	}
}
