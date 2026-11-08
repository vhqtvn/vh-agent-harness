package eslink

import (
	"fmt"
	"path"
	"sort"
	"strings"
)

// The graph layer REPRODUCES the reference closure semantics the XS-1…XS-8
// fixture designs pin (variant-map PART 2, grounded in the G32-era
// graph.go): star-merge with openness propagation, cycles → open,
// ambiguity → unknown, non-cascading re-export attribution, namespace
// re-exports as presence-only edges, and the tri-state Scan. The parser
// underneath is the successor; the closure semantics are deliberately
// identical.

// BrokenEdge is one PROVEN link incompatibility: a required static edge
// whose provider is present in the scanned tree, FIT under the frozen
// grammar, with a CLOSED export surface, and the required name is absent.
type BrokenEdge struct {
	Importer  string
	Specifier string
	Provider  string
	Symbol    string
	Reason    string
}

// UnknownFinding is one link the scanner could NOT prove either way.
// Unknown findings NEVER justify a refusal (U6); they are surfaced so
// operators know what was not checked.
type UnknownFinding struct {
	Path      string
	Specifier string
	Reason    string
}

// Report is the scan result over one effective tree.
type Report struct {
	Modules int
	Edges   int
	Broken  []BrokenEdge
	Unknown []UnknownFinding
}

// HasBroken reports whether any proven-broken edge exists.
func (r *Report) HasBroken() bool { return r != nil && len(r.Broken) > 0 }

// surface is a module's fully resolved export surface.
type surface struct {
	names      map[string]bool
	hasDefault bool
	ambiguous  map[string]bool
	open       bool
}

func (s *surface) namePresent(n string) bool {
	return s != nil && s.names[n] && !(s.ambiguous[n])
}

func (s *surface) nameAmbiguous(n string) bool {
	return s != nil && s.ambiguous[n]
}

// IsRelativeSpecifier reports whether an import specifier is relative
// (./ or ../). Everything else is outside the tree's link graph by design.
func IsRelativeSpecifier(spec string) bool {
	return strings.HasPrefix(spec, "./") || strings.HasPrefix(spec, "../")
}

// specifierNotAssessable reports relative specifiers this scanner cannot
// map to a tree key (ESM resolves specifiers as URLs; percent-encoding,
// queries, fragments and URL normalization change resolution in ways a raw
// path join does not reproduce). Such edges are UNKNOWN, never broken.
func specifierNotAssessable(spec string) bool {
	if strings.ContainsAny(spec, "%?#") {
		return true
	}
	if strings.ContainsAny(spec, "\t\n\r") {
		return true
	}
	return strings.TrimFunc(spec, func(r rune) bool { return r <= ' ' }) != spec
}

// ResolveSpecifier resolves spec as written inside importer and returns the
// provider's tree key (which may not exist in the tree).
func ResolveSpecifier(importer, spec string) string {
	dir := path.Dir(importer)
	return path.Clean(path.Join(dir, spec))
}

// Scan parses every module in tree and checks every required static edge
// with relative specifiers. Findings are sorted deterministically.
func Scan(tree map[string][]byte) *Report {
	parsed := make(map[string]*Module, len(tree))
	for _, p := range sortedKeys(tree) {
		parsed[p] = Parse(p, tree[p])
	}
	return scanParsed(tree, parsed)
}

func scanParsed(tree map[string][]byte, parsed map[string]*Module) *Report {
	r := &Report{Modules: len(tree)}
	surfaces := make(map[string]*surface, len(parsed))
	var resolve func(m string, visiting map[string]bool) *surface
	resolve = func(m string, visiting map[string]bool) *surface {
		if s, ok := surfaces[m]; ok {
			return s
		}
		if visiting[m] {
			// Re-export/star cycle: cannot compute the surface with
			// confidence — open (the honest direction).
			return &surface{open: true}
		}
		visiting[m] = true
		s := resolveSurface(parsed[m], tree, resolve, visiting)
		delete(visiting, m)
		surfaces[m] = s
		return s
	}

	// Phase 1: resolve surfaces; re-export edges are checked here because
	// they are required edges of the re-exporting module.
	for _, m := range sortedModuleKeys(parsed) {
		mod := parsed[m]
		resolve(m, map[string]bool{})
		if mod.Unparseable {
			r.Unknown = append(r.Unknown, UnknownFinding{
				Path: m,
				Reason: fmt.Sprintf("module UNKNOWN (%s); its export surface and import edges were not assessed",
					mod.UnparseableWhy),
			})
			continue
		}
		for _, e := range mod.ReexportEdges {
			if !IsRelativeSpecifier(e.Specifier) {
				continue
			}
			r.Edges++
			if specifierNotAssessable(e.Specifier) {
				r.Unknown = append(r.Unknown, UnknownFinding{
					Path: m, Specifier: e.Specifier,
					Reason: "specifier is not a plain relative path (percent-encoding, query/fragment, or URL-normalized whitespace/control characters); link cannot be assessed",
				})
				continue
			}
			r.checkRequirement(m, e.Specifier, e.ImportName, tree, parsed, resolve)
		}
		for _, spec := range mod.NamespaceReexports {
			if !IsRelativeSpecifier(spec) {
				continue
			}
			r.Edges++
			if specifierNotAssessable(spec) {
				r.Unknown = append(r.Unknown, UnknownFinding{
					Path: m, Specifier: spec,
					Reason: "specifier is not a plain relative path (percent-encoding, query/fragment, or URL-normalized whitespace/control characters); link cannot be assessed",
				})
				continue
			}
			// `export * as ns from "m"`: presence-only edge. A missing
			// target cannot be proven broken — unknown.
			provider := ResolveSpecifier(m, spec)
			if _, present := tree[provider]; !present {
				r.Unknown = append(r.Unknown, UnknownFinding{
					Path: m, Specifier: spec,
					Reason: fmt.Sprintf("namespace re-export target %s is not present in the scanned tree (cannot assess module presence)", provider),
				})
			}
		}
		for _, spec := range mod.StarReexports {
			if !IsRelativeSpecifier(spec) {
				continue
			}
			r.Edges++
			if specifierNotAssessable(spec) {
				r.Unknown = append(r.Unknown, UnknownFinding{
					Path: m, Specifier: spec,
					Reason: "star re-export target is not a plain relative path (percent-encoding, query/fragment, or URL-normalized whitespace/control characters); merged surface cannot be assessed",
				})
				continue
			}
			provider := ResolveSpecifier(m, spec)
			if _, present := tree[provider]; !present {
				r.Unknown = append(r.Unknown, UnknownFinding{
					Path: m, Specifier: spec,
					Reason: fmt.Sprintf("star re-export target %s is not present in the scanned tree (cannot assess merged surface)", provider),
				})
				continue
			}
			if resolve(provider, map[string]bool{}).open {
				r.Unknown = append(r.Unknown, UnknownFinding{
					Path: m, Specifier: spec,
					Reason: fmt.Sprintf("star re-export target %s has an open surface (unparseable, unassessable, or cyclic); merged surface cannot be assessed", provider),
				})
			}
		}
	}

	// Phase 2: check import edges.
	for _, m := range sortedModuleKeys(parsed) {
		mod := parsed[m]
		if mod.Unparseable {
			continue // already reported in phase 1
		}
		for _, d := range mod.Imports {
			if !IsRelativeSpecifier(d.Specifier) {
				continue
			}
			r.Edges++
			if specifierNotAssessable(d.Specifier) {
				r.Unknown = append(r.Unknown, UnknownFinding{
					Path: m, Specifier: d.Specifier,
					Reason: "specifier is not a plain relative path (percent-encoding, query/fragment, or URL-normalized whitespace/control characters); link cannot be assessed",
				})
				continue
			}
			provider := ResolveSpecifier(m, d.Specifier)
			if _, present := tree[provider]; !present {
				r.Unknown = append(r.Unknown, UnknownFinding{
					Path: m, Specifier: d.Specifier,
					Reason: fmt.Sprintf("provider %s is not present in the scanned tree (cannot assess the link)", provider),
				})
				continue
			}
			ps := resolve(provider, map[string]bool{})
			if ps.open {
				r.Unknown = append(r.Unknown, UnknownFinding{
					Path: m, Specifier: d.Specifier,
					Reason: fmt.Sprintf("provider %s has an open export surface (unparseable or unresolvable export-* chain); link cannot be proven either way", provider),
				})
				continue
			}
			if d.Default && !ps.hasDefault {
				r.Broken = append(r.Broken, BrokenEdge{
					Importer: m, Specifier: d.Specifier, Provider: provider, Symbol: "default",
					Reason: fmt.Sprintf("importer requires a default export; provider %s declares none (surface fully resolved and closed, so the absence is proven)", provider),
				})
			}
			if d.Namespace || d.SideEffect {
				continue // module presence only
			}
			for _, n := range d.Named {
				if n.ImportName == "default" {
					if !ps.hasDefault {
						r.Broken = append(r.Broken, BrokenEdge{
							Importer: m, Specifier: d.Specifier, Provider: provider, Symbol: "default",
							Reason: fmt.Sprintf("importer requires the default export under a local alias; provider %s declares none (surface fully resolved and closed, so the absence is proven)", provider),
						})
					}
					continue
				}
				if ps.nameAmbiguous(n.ImportName) {
					r.Unknown = append(r.Unknown, UnknownFinding{
						Path: m, Specifier: d.Specifier,
						Reason: fmt.Sprintf("required name %q is provided ambiguously through multiple export-* targets of %s (full ESM ResolveExport semantics not implemented); link cannot be proven either way", n.ImportName, provider),
					})
					continue
				}
				if !ps.namePresent(n.ImportName) {
					r.Broken = append(r.Broken, BrokenEdge{
						Importer: m, Specifier: d.Specifier, Provider: provider, Symbol: n.ImportName,
						Reason: fmt.Sprintf("importer requires named export %q; provider %s fully resolved export surface does not contain it (surface closed, so the absence is proven)", n.ImportName, provider),
					})
				}
			}
		}
		// Dynamic imports are never required edges; surface as unknown.
		for _, di := range mod.DynamicImports {
			if di.Computed {
				r.Unknown = append(r.Unknown, UnknownFinding{
					Path: m, Specifier: "(computed)",
					Reason: fmt.Sprintf("dynamic import with a computed (non-literal) specifier at line %d; excluded from the required-edge set", di.Line),
				})
			} else if hasEscape(di.Specifier) {
				// Consuming-side mirror of the parser's not-decodable
				// guard (parseDynamicImport): a literal specifier that
				// carries escape sequences cannot be produced by Parse
				// anymore (the module degrades first), but if one ever
				// reaches here the raw bytes must still never be treated
				// as a decodable literal — an escape in the relative
				// prefix can make IsRelativeSpecifier misjudge the raw
				// bytes (silent vanish); later escapes keep the prefix
				// but stay undecodable. Surface reason-coded instead.
				// (Exercised by the synthetic scanParsed fixture — see
				// TestF5_DynImport_ConsumerGuardSynthetic.)
				r.Unknown = append(r.Unknown, UnknownFinding{
					Path: m, Specifier: di.Specifier,
					Reason: fmt.Sprintf("dynamic import specifier at line %d carries escape sequences (not decodable); excluded from the required-edge set", di.Line),
				})
			} else if IsRelativeSpecifier(di.Specifier) {
				r.Unknown = append(r.Unknown, UnknownFinding{
					Path: m, Specifier: di.Specifier,
					Reason: fmt.Sprintf("dynamic import at line %d; dynamic imports are excluded from the required-edge set (optional/project targets may legitimately be absent)", di.Line),
				})
			}
		}
	}

	sortFindings(r)
	return r
}

// checkRequirement verifies one re-export requirement.
func (r *Report) checkRequirement(importer, spec, name string, tree map[string][]byte, parsed map[string]*Module, resolve func(string, map[string]bool) *surface) {
	if !IsRelativeSpecifier(spec) {
		return
	}
	provider := ResolveSpecifier(importer, spec)
	if _, present := tree[provider]; !present {
		r.Unknown = append(r.Unknown, UnknownFinding{
			Path: importer, Specifier: spec,
			Reason: fmt.Sprintf("re-export target %s is not present in the scanned tree (cannot assess the link)", provider),
		})
		return
	}
	ps := resolve(provider, map[string]bool{})
	if ps.open {
		r.Unknown = append(r.Unknown, UnknownFinding{
			Path: importer, Specifier: spec,
			Reason: fmt.Sprintf("re-export target %s has an open export surface; link cannot be proven either way", provider),
		})
		return
	}
	if name == "default" {
		if !ps.hasDefault {
			r.Broken = append(r.Broken, BrokenEdge{
				Importer: importer, Specifier: spec, Provider: provider, Symbol: "default",
				Reason: fmt.Sprintf("module re-exports a default export from %s, which declares none (surface fully resolved and closed, so the absence is proven)", provider),
			})
		}
		return
	}
	if ps.nameAmbiguous(name) {
		r.Unknown = append(r.Unknown, UnknownFinding{
			Path: importer, Specifier: spec,
			Reason: fmt.Sprintf("re-exported name %q is provided ambiguously through multiple export-* targets of %s; link cannot be proven either way", name, provider),
		})
		return
	}
	if !ps.namePresent(name) {
		r.Broken = append(r.Broken, BrokenEdge{
			Importer: importer, Specifier: spec, Provider: provider, Symbol: name,
			Reason: fmt.Sprintf("module re-exports %q from %s, whose fully resolved export surface does not contain it (surface closed, so the absence is proven)", name, provider),
		})
	}
}

// resolveSurface computes a module's fully resolved export surface: own
// declarations plus re-exported names, with `export *` chains merging the
// targets' named surfaces. Any unresolvable star target opens the surface.
func resolveSurface(m *Module, tree map[string][]byte, resolve func(string, map[string]bool) *surface, visiting map[string]bool) *surface {
	if m == nil {
		return &surface{open: true}
	}
	s := &surface{names: map[string]bool{}, open: m.Unparseable}
	if m.Unparseable {
		return s
	}
	for n := range m.NamedExports {
		if n == "default" {
			continue
		}
		s.names[n] = true
	}
	s.hasDefault = m.HasDefault
	for _, e := range m.ReexportEdges {
		if e.ExportedName == "default" {
			s.hasDefault = true
		}
	}
	// providedBy tracks which DIRECT star target first provided each merged
	// name: two DISTINCT targets → ambiguous; duplicate stars of the SAME
	// target stay ok (ESM dedupes a single module's star re-export).
	providedBy := map[string]string{}
	for _, spec := range m.StarReexports {
		if specifierNotAssessable(spec) || !IsRelativeSpecifier(spec) {
			s.open = true
			continue
		}
		target := ResolveSpecifier(m.Path, spec)
		if _, present := tree[target]; !present {
			s.open = true
			continue
		}
		ts := resolve(target, visiting)
		if ts.open {
			s.open = true
			continue
		}
		for n := range ts.names {
			if n == "default" {
				continue
			}
			if m.NamedExports[n] {
				continue // a local declaration shadows every star source
			}
			if prevTarget, seen := providedBy[n]; seen && prevTarget != target {
				s.markAmbiguous(n)
				continue
			}
			providedBy[n] = target
			s.names[n] = true
		}
		for n := range ts.ambiguous {
			if n == "default" || m.NamedExports[n] {
				continue
			}
			s.markAmbiguous(n)
		}
	}
	return s
}

func (s *surface) markAmbiguous(n string) {
	if s.ambiguous == nil {
		s.ambiguous = map[string]bool{}
	}
	s.ambiguous[n] = true
}

// --- gate-6 disaggregated reporting -----------------------------------------

// Unknown classes (never blended — the F5 never-a-single-blended-number
// rule extended to the parser's own output).
const (
	ClassDynamicImport     = "dynamic-import"
	ClassUnsupportedSyntax = "unsupported-syntax"
	ClassOpenSurface       = "open-surface"
	ClassMissingProvider   = "missing-provider"
	ClassAmbiguous         = "ambiguous-export-star"
	ClassUnassessableSpec  = "unassessable-specifier"
	ClassOther             = "other"
)

// ClassifyUnknown buckets one unknown finding by its reason text.
func ClassifyUnknown(u UnknownFinding) string {
	switch {
	case strings.Contains(u.Reason, "dynamic import"):
		return ClassDynamicImport
	case strings.Contains(u.Reason, "module UNKNOWN"):
		return ClassUnsupportedSyntax
	case strings.Contains(u.Reason, "open export surface") ||
		strings.Contains(u.Reason, "open surface"):
		return ClassOpenSurface
	case strings.Contains(u.Reason, "not present in the scanned tree"):
		return ClassMissingProvider
	case strings.Contains(u.Reason, "ambiguously"):
		return ClassAmbiguous
	case strings.Contains(u.Reason, "not a plain relative path"):
		return ClassUnassessableSpec
	}
	return ClassOther
}

// LintSummary is the disaggregated lint report (gate 6): module-level
// grammar verdicts, per-class unknown counters, edge outcomes, and the
// L-chain split (L0 unrecognized importer / L1 recognized importer,
// provider not assessable / L3 closed obligation) — never a single blended
// number.
type LintSummary struct {
	Modules         int
	ModulesFit      int
	ModulesUnknown  int
	UnknownReasons  map[string]int // module-level reason-code histogram
	Edges           int            // ledger edges (L0+L1+L3)
	EdgesBroken     int
	UnknownByClass  map[string]int
	DynamicUnknowns int // subset of UnknownByClass (the corpus's 5-pin class)
	L0Edges         int // unrecognized-importer edges
	L1Edges         int // recognized importer, provider not assessable
	L3Edges         int // closed obligations (both ends fit, surface closed)
}

// Summarize computes the disaggregated summary over a tree scan.
func Summarize(tree map[string][]byte) *LintSummary {
	parsed, edges := EdgeLedger(tree)
	rep := scanParsed(tree, parsed)
	s := &LintSummary{
		Modules:        rep.Modules,
		EdgesBroken:    len(rep.Broken),
		UnknownReasons: map[string]int{},
		UnknownByClass: map[string]int{},
	}
	for _, e := range edges {
		switch e.LClass {
		case "L0":
			s.L0Edges++
		case "L1":
			s.L1Edges++
		case "L3":
			s.L3Edges++
		}
	}
	s.Edges = len(edges)
	for _, m := range parsed {
		if m.Unparseable {
			s.ModulesUnknown++
			s.UnknownReasons[m.UnparseableWhy]++
		} else {
			s.ModulesFit++
		}
	}
	for _, u := range rep.Unknown {
		c := ClassifyUnknown(u)
		s.UnknownByClass[c]++
		if c == ClassDynamicImport {
			s.DynamicUnknowns++
		}
	}
	return s
}

// EdgeRecord is one row of the per-edge audit ledger (gates 3–4): the
// L-classification mirrors the packet's L-chain (L0 importer unrecognized;
// L1 importer FIT, provider not assessable/open; L3 both FIT, provider
// surface closed).
type EdgeRecord struct {
	Type            string
	Importer        string
	Specifier       string
	Provider        string
	Symbols         []string
	Presence        bool
	ProviderPresent bool
	ProviderFit     bool
	ProviderClosed  bool
	LClass          string // "L0" | "L1" | "L3"
}

// EdgeLedger re-derives the per-edge ledger with L-classes over a parsed
// tree (the packet harness's ledger join, on successor-parser output).
//
// L0 rows: an UNRECOGNIZED (unparseable) importer's import declarations that
// were captured BEFORE the parse failed (imports sit at file tops) still
// join the ledger as L0 rows — this is the packet's L-join semantics (the
// reference ledger recognized these edges; the census then classified them
// by importer fitness). The rows are marked by the captured surface only;
// an unparseable module's captured list may be partial where the failure
// precedes a later import, which is exactly why such edges are L0
// (unrecognized importer), never assessed.
func EdgeLedger(tree map[string][]byte) (parsed map[string]*Module, edges []EdgeRecord) {
	parsed = make(map[string]*Module, len(tree))
	for _, p := range sortedKeys(tree) {
		parsed[p] = Parse(p, tree[p])
	}
	surfaces := make(map[string]*surface, len(parsed))
	var resolve func(m string, visiting map[string]bool) *surface
	resolve = func(m string, visiting map[string]bool) *surface {
		if s, ok := surfaces[m]; ok {
			return s
		}
		if visiting[m] {
			return &surface{open: true}
		}
		visiting[m] = true
		s := resolveSurface(parsed[m], tree, resolve, visiting)
		delete(visiting, m)
		surfaces[m] = s
		return s
	}
	add := func(typ, importer, spec string, symbols []string, presence bool) {
		provider := ResolveSpecifier(importer, spec)
		_, present := tree[provider]
		ps := resolve(provider, map[string]bool{})
		pm := parsed[provider]
		providerFit := present && pm != nil && !pm.Unparseable
		lc := "L1"
		if parsed[importer].Unparseable {
			lc = "L0"
		} else if providerFit && !ps.open && !specifierNotAssessable(spec) {
			// An unassessable specifier (percent-encoding/query/fragment)
			// can path-clean onto an existing provider by accident; the
			// scanner reports such edges unknown, so the ledger must not
			// count them as closed obligations either.
			lc = "L3"
		}
		closed := !ps.open && !specifierNotAssessable(spec)
		edges = append(edges, EdgeRecord{
			Type: typ, Importer: importer, Specifier: spec, Provider: provider,
			Symbols: symbols, Presence: presence,
			ProviderPresent: present, ProviderFit: providerFit,
			ProviderClosed: closed, LClass: lc,
		})
	}
	for _, m := range sortedModuleKeys(parsed) {
		mod := parsed[m]
		// NOTE: unparseable importers are NOT skipped — their captured
		// pre-failure edges join as L0 rows (see function comment).
		for _, e := range mod.ReexportEdges {
			if IsRelativeSpecifier(e.Specifier) {
				add("reexport", m, e.Specifier, []string{e.ImportName}, false)
			}
		}
		for _, spec := range mod.NamespaceReexports {
			if IsRelativeSpecifier(spec) {
				add("ns-reexport", m, spec, nil, true)
			}
		}
		for _, spec := range mod.StarReexports {
			if IsRelativeSpecifier(spec) {
				add("star", m, spec, nil, true)
			}
		}
	}
	for _, m := range sortedModuleKeys(parsed) {
		mod := parsed[m]
		for _, d := range mod.Imports {
			if !IsRelativeSpecifier(d.Specifier) {
				continue
			}
			var syms []string
			if d.Default {
				syms = append(syms, "default")
			}
			for _, n := range d.Named {
				syms = append(syms, n.ImportName)
			}
			presence := d.Namespace || d.SideEffect || len(syms) == 0
			add("import", m, d.Specifier, syms, presence)
		}
	}
	return parsed, edges
}

func sortedKeys(m map[string][]byte) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortedModuleKeys(m map[string]*Module) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortFindings(r *Report) {
	sort.Slice(r.Broken, func(i, j int) bool {
		if r.Broken[i].Importer != r.Broken[j].Importer {
			return r.Broken[i].Importer < r.Broken[j].Importer
		}
		if r.Broken[i].Specifier != r.Broken[j].Specifier {
			return r.Broken[i].Specifier < r.Broken[j].Specifier
		}
		return r.Broken[i].Symbol < r.Broken[j].Symbol
	})
	sort.Slice(r.Unknown, func(i, j int) bool {
		if r.Unknown[i].Path != r.Unknown[j].Path {
			return r.Unknown[i].Path < r.Unknown[j].Path
		}
		return r.Unknown[i].Specifier < r.Unknown[j].Specifier
	})
}
