package eslink

// Gate-2 negative mutations (staged-restart spec §B gate 2; the set named by
// the F5 restart path): removed-export, late-import, decoy,
// unsupported-suffix, slash-update. Each mutates REAL corpus bytes with a
// targeted string surgery and pins the exact FINDING DELTA versus the
// unmutated baseline scan — a scanner that misses the change, fabricates a
// verdict, or disturbs unrelated findings fails here.

import (
	"fmt"
	"sort"
	"strings"
	"testing"
)

// mutationBase modules: real, currently-clean corpus surfaces (FIT modules,
// L3 edges) used as mutation substrates.
const (
	negImporter = ".opencode/plugins/coordination-hints.js"
	negProvider = ".opencode/scripts/coordination-hints-lib.js"
	negSentinel = ".opencode/scripts/verify-skill-sentinel.js"

	negSentinelImport = "import {\n    computeSkillFingerprint,\n    compareSkillFingerprint,\n    hashSkillAggregate,\n    server,\n} from \"../plugins/session-state.js\";\n"

	// The provider's named-export-list entry. The export-default OBJECT
	// mirrors the list, so the anchor carries the `export {` header to be
	// unique; only the LIST entry is the export.
	negDropExport = "export {\n    COMMAND_REPETITION_THRESHOLD,\n    DEFAULT_PRODUCT_PREFIXES,\n    LARGE_FILE_LINE_THRESHOLD,\n    buildCoordinationHintMessages,\n"
	negDropInto   = "export {\n    COMMAND_REPETITION_THRESHOLD,\n    DEFAULT_PRODUCT_PREFIXES,\n    LARGE_FILE_LINE_THRESHOLD,\n"
)

func mutatedTree(t *testing.T, mutate func(tree map[string][]byte)) map[string][]byte {
	t.Helper()
	tree := corpusTree(t)
	before := len(tree)
	mutate(tree)
	if len(tree) != before {
		t.Fatalf("mutation changed the module count")
	}
	return tree
}

func findingSet(r *Report) map[string]int {
	out := map[string]int{}
	for _, b := range r.Broken {
		out[fmt.Sprintf("BROKEN %s -> %s (%s)", b.Importer, b.Provider, b.Symbol)]++
	}
	for _, u := range r.Unknown {
		out[fmt.Sprintf("UNKNOWN %s :: %s :: %s", u.Path, u.Specifier, u.Reason)]++
	}
	return out
}

// assertDelta pins the exact finding delta (as a MULTISET — duplicate
// findings count): every added/removed finding is one of
// wantAdded/wantRemoved (substring match), nothing else moves.
func assertDelta(t *testing.T, base, mutated *Report, wantAdded, wantRemoved []string) {
	t.Helper()
	b, m := findingSet(base), findingSet(mutated)
	added, removed := []string{}, []string{}
	for k, n := range m {
		for i := 0; i < n; i++ {
			if bk, ok := b[k]; !ok || i >= bk {
				added = append(added, k)
			}
		}
	}
	for k, n := range b {
		for i := 0; i < n; i++ {
			if mk, ok := m[k]; !ok || i >= mk {
				removed = append(removed, k)
			}
		}
	}
	sort.Strings(added)
	sort.Strings(removed)
	keep := func(list []string, wants []string) []string {
		var rest []string
		for _, k := range list {
			matched := false
			for _, w := range wants {
				if strings.Contains(k, w) {
					matched = true
					break
				}
			}
			if !matched {
				rest = append(rest, k)
			}
		}
		return rest
	}
	if rest := keep(added, wantAdded); len(rest) != 0 {
		t.Fatalf("unexpected ADDED findings (not in expected delta):\n%s", strings.Join(rest, "\n"))
	}
	if rest := keep(removed, wantRemoved); len(rest) != 0 {
		t.Fatalf("unexpected REMOVED findings (not in expected delta):\n%s", strings.Join(rest, "\n"))
	}
	// Every expected delta member must actually appear.
	for _, w := range wantAdded {
		if !containsAny(added, w) {
			t.Fatalf("expected ADDED finding %q not present; delta was:\n%s", w, strings.Join(added, "\n"))
		}
	}
	for _, w := range wantRemoved {
		if !containsAny(removed, w) {
			t.Fatalf("expected REMOVED finding %q not present; delta was:\n%s", w, strings.Join(removed, "\n"))
		}
	}
}

func containsAny(list []string, substr string) bool {
	for _, k := range list {
		if strings.Contains(k, substr) {
			return true
		}
	}
	return false
}

// Negative 1 — removed-export: dropping a provider's named export must flip
// the consumer edge to PROVEN broken while the unaffected symbols stay ok
// (non-cascading attribution); no unknown may appear (the surface stays
// closed).
func TestNegRemovedExport(t *testing.T) {
	base := Scan(corpusTree(t))
	tree := mutatedTree(t, func(tree map[string][]byte) {
		src := string(tree[negProvider])
		if strings.Count(src, negDropExport) != 1 {
			t.Fatalf("provider export-list anchor moved; re-derive the mutation")
		}
		tree[negProvider] = []byte(strings.Replace(src, negDropExport, negDropInto, 1))
	})
	m := Parse(negProvider, tree[negProvider])
	if m.Unparseable {
		t.Fatalf("mutation must keep the provider parseable: %s", m.UnparseableWhy)
	}
	if m.NamedExports["buildCoordinationHintMessages"] {
		t.Fatalf("mutation must remove the named export")
	}
	r := Scan(tree)
	// Both importers of the removed symbol flip to PROVEN broken (the
	// golden ledger carries buildCoordinationHintMessages on TWO edges:
	// coordination-hints.js and verify-coordination-hints.js).
	assertDelta(t, base, r,
		[]string{
			fmt.Sprintf("BROKEN %s -> %s (buildCoordinationHintMessages)", negImporter, negProvider),
			fmt.Sprintf("BROKEN .opencode/scripts/verify-coordination-hints.js -> %s (buildCoordinationHintMessages)", negProvider),
		},
		nil)
}

// Negative 2 — late-import: an import declaration appearing LATE in the
// module (after all statements) must still be recognized — ZERO finding
// delta and zero ledger delta for the module.
func TestNegLateImport(t *testing.T) {
	base := Scan(corpusTree(t))
	tree := mutatedTree(t, func(tree map[string][]byte) {
		src := string(tree[negSentinel])
		if !strings.Contains(src, negSentinelImport) {
			t.Fatalf("sentinel import anchor moved; re-derive the mutation")
		}
		tree[negSentinel] = []byte(strings.Replace(src, negSentinelImport, "", 1) + "\n" + negSentinelImport)
	})
	m := Parse(negSentinel, tree[negSentinel])
	if m.Unparseable {
		t.Fatalf("late import must still parse FIT: %s", m.UnparseableWhy)
	}
	r := Scan(tree)
	assertDelta(t, base, r, nil, nil)
	// The edge and all four symbols survived the move.
	_, edges := EdgeLedger(tree)
	found := false
	for _, e := range edges {
		if e.Importer == negSentinel && e.Provider == ".opencode/plugins/session-state.js" {
			found = true
			if len(e.Symbols) != 4 {
				t.Fatalf("late import lost symbols: %v", e.Symbols)
			}
		}
	}
	if !found {
		t.Fatalf("late import edge lost from the ledger")
	}
}

// Negative 3 — decoy: import-LOOKING text inside comments and strings must
// not create edges, unknowns, or broken verdicts — ZERO finding delta.
func TestNegDecoy(t *testing.T) {
	base := Scan(corpusTree(t))
	tree := mutatedTree(t, func(tree map[string][]byte) {
		src := string(tree[negSentinel])
		decoy := "\n// import { fakeSymbol } from \"../scripts/does-not-exist.js\";\n" +
			"const note = 'import { otherFake } from \"./also-missing.js\"';\n" +
			"/* import { third } from \"../plugins/missing.js\"; */\n" +
			"export const noteOut = note;\n"
		tree[negSentinel] = []byte(src + decoy)
	})
	m := Parse(negSentinel, tree[negSentinel])
	if m.Unparseable {
		t.Fatalf("decoy text must not degrade the module: %s", m.UnparseableWhy)
	}
	r := Scan(tree)
	assertDelta(t, base, r, nil, nil)
}

// Negative 4 — unsupported-suffix: a relative specifier carrying a query
// suffix cannot be path-joined soundly; the edge must degrade to a
// reason-coded UNKNOWN (unassessable specifier), never broken, never a
// fabricated ok — and the assessable-surface unknown it replaces is nothing
// (the baseline edge was clean-ok, so the delta is exactly one added
// unknown).
func TestNegUnsupportedSuffix(t *testing.T) {
	base := Scan(corpusTree(t))
	tree := mutatedTree(t, func(tree map[string][]byte) {
		src := string(tree[negSentinel])
		needle := "} from \"../plugins/session-state.js\";"
		if strings.Count(src, needle) != 1 {
			t.Fatalf("sentinel specifier anchor moved; re-derive the mutation")
		}
		tree[negSentinel] = []byte(strings.Replace(src, needle, "} from \"../plugins/session-state.js?v=2\";", 1))
	})
	r := Scan(tree)
	if r.HasBroken() {
		t.Fatalf("suffix-bearing specifier must never be broken: %+v", r.Broken)
	}
	assertDelta(t, base, r,
		[]string{"UNKNOWN " + negSentinel + " :: ../plugins/session-state.js?v=2 :: specifier is not a plain relative path"},
		nil)
	// The edge still JOINED the ledger as an L1 row — unassessable
	// specifiers never count as closed obligations even when they
	// path-clean onto an existing provider by accident.
	_, edges := EdgeLedger(tree)
	found := false
	for _, e := range edges {
		if e.Importer == negSentinel && e.Specifier == "../plugins/session-state.js?v=2" {
			found = true
			if e.LClass != "L1" {
				t.Fatalf("unassessable-specifier edge LClass = %s, want L1", e.LClass)
			}
			if e.ProviderClosed {
				t.Fatalf("unassessable-specifier edge must not count as closed")
			}
		}
	}
	if !found {
		t.Fatalf("suffix edge must still join the ledger as an L1 row")
	}
}

// Negative 5 — slash-update: the two-token G32 defect family on real bytes.
// (a) update: a single `i++` degrades the module reason-coded; its edge's
// baseline findings flip to module-UNKNOWN, never broken. (b) slash:
// statement-position regexes and a division chain in the same module stay
// FIT — zero delta.
func TestNegSlashUpdate(t *testing.T) {
	base := Scan(corpusTree(t))

	// (a) update-op insertion.
	treeA := mutatedTree(t, func(tree map[string][]byte) {
		src := string(tree[negSentinel])
		tree[negSentinel] = []byte(src + "let suIndex = 0;\nsuIndex++;\n")
	})
	m := Parse(negSentinel, treeA[negSentinel])
	if !m.Unparseable || !strings.HasPrefix(m.UnparseableWhy, ReasonUpdateOp) {
		t.Fatalf("inserted update op must degrade with %s…, got %q", ReasonUpdateOp, m.UnparseableWhy)
	}
	rA := Scan(treeA)
	if rA.HasBroken() {
		t.Fatalf("degraded module must never ground broken (U6): %+v", rA.Broken)
	}
	assertDelta(t, base, rA,
		[]string{"UNKNOWN " + negSentinel + " ::  :: module UNKNOWN (update operator"},
		nil)

	// (b) slash: statement regex + division chain keep the module FIT.
	treeB := mutatedTree(t, func(tree map[string][]byte) {
		src := string(tree[negSentinel])
		tree[negSentinel] = []byte(src + "const suS = \"x\";\n/^sentinel-/i.test(suS);\nconst suRatio = 10 / 2 / 1;\nexport const suOut = suRatio;\n")
	})
	mB := Parse(negSentinel, treeB[negSentinel])
	if mB.Unparseable {
		t.Fatalf("statement regex + division must stay FIT, got UNKNOWN(%s)", mB.UnparseableWhy)
	}
	rB := Scan(treeB)
	assertDelta(t, base, rB, nil, nil)
}
