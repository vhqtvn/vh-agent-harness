package prompt

import (
	"bytes"
	"strings"
	"testing"
)

func regMust(t *testing.T, a *Assembler, s Section) {
	t.Helper()
	if err := a.Register(s); err != nil {
		t.Fatalf("Register(%q): %v", s.Key, err)
	}
}

func TestAssemblerOrdersByNumberWithGaps(t *testing.T) {
	a := NewAssembler()
	regMust(t, a, Section{Number: 100, Key: "tools.read", Owner: "core", Body: "READ"})
	regMust(t, a, Section{Number: -100, Key: "identity", Owner: "core", Body: "IDENTITY"})
	regMust(t, a, Section{Number: 150, Key: "tools.write", Owner: "core", Body: "WRITE"})
	regMust(t, a, Section{Number: 0, Key: "persona", Owner: "core", Body: "PERSONA"})

	out, err := a.Render(nil)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	idx := func(sub string) int { return bytes.Index(out, []byte(sub)) }
	for _, pair := range [][2]string{
		{"IDENTITY", "PERSONA"},
		{"PERSONA", "READ"},
		{"READ", "WRITE"},
	} {
		if idx(pair[0]) < 0 {
			t.Fatalf("missing %q in output:\n%s", pair[0], out)
		}
		if idx(pair[0]) >= idx(pair[1]) {
			t.Fatalf("%q must precede %q; got order:\n%s", pair[0], pair[1], out)
		}
	}
}

func TestAssemblerSameNumberStableRegistrationOrder(t *testing.T) {
	a := NewAssembler()
	regMust(t, a, Section{Number: 100, Key: "first", Body: "FIRST"})
	regMust(t, a, Section{Number: 100, Key: "second", Body: "SECOND"})
	regMust(t, a, Section{Number: 100, Key: "third", Body: "THIRD"})

	out, err := a.Render(nil)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	// Blocks carry headers between bodies, so assert position order.
	pos := map[string]int{"FIRST": -1, "SECOND": -1, "THIRD": -1}
	for body := range pos {
		i := bytes.Index(out, []byte(body))
		if i < 0 {
			t.Fatalf("missing %q in output:\n%s", body, out)
		}
		pos[body] = i
	}
	if !(pos["FIRST"] < pos["SECOND"] && pos["SECOND"] < pos["THIRD"]) {
		t.Fatalf("same-number sections must keep registration order; got positions %v:\n%s", pos, out)
	}
}

func TestAssemblerInterpolation(t *testing.T) {
	a := NewAssembler()
	regMust(t, a, Section{Number: 0, Key: "persona", Body: "Hello {{name}}, mode={{mode}}."})

	out, err := a.Render(map[string]string{"name": "vh", "mode": "strict"})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(string(out), "Hello vh, mode=strict.") {
		t.Fatalf("interpolation failed:\n%s", out)
	}
}

func TestAssemblerInterpolationUnknownVarFails(t *testing.T) {
	a := NewAssembler()
	regMust(t, a, Section{Number: 0, Key: "persona", Body: "Value: {{missing}}"})
	if _, err := a.Render(map[string]string{"known": "x"}); err == nil {
		t.Fatal("unknown variable must be an error (strict interpolation)")
	} else if !strings.Contains(err.Error(), "missing") || !strings.Contains(err.Error(), "persona") {
		t.Fatalf("error must name variable and section; got: %v", err)
	}
}

func TestAssemblerInterpolationUnterminatedFails(t *testing.T) {
	a := NewAssembler()
	regMust(t, a, Section{Number: 0, Key: "k", Body: "broken {{oops"})
	if _, err := a.Render(nil); err == nil {
		t.Fatal("unterminated {{ must be an error")
	}
}

func TestAssemblerDeterministicRender(t *testing.T) {
	build := func() []byte {
		a := NewAssembler()
		regMust(t, a, Section{Number: 100, Key: "b", Body: "B {{v}}"})
		regMust(t, a, Section{Number: 0, Key: "a", Body: "A {{v}}"})
		out, err := a.Render(map[string]string{"v": "x"})
		if err != nil {
			t.Fatalf("Render: %v", err)
		}
		return out
	}
	if !bytes.Equal(build(), build()) {
		t.Fatal("render must be byte-deterministic across identical assemblers")
	}
}

func TestAssemblerDuplicateKeyRejected(t *testing.T) {
	a := NewAssembler()
	regMust(t, a, Section{Number: 0, Key: "dup", Body: "x"})
	if err := a.Register(Section{Number: 5, Key: "dup", Body: "y"}); err == nil {
		t.Fatal("duplicate section key must be rejected")
	}
	if err := a.Register(Section{Number: 5, Key: "", Body: "y"}); err == nil {
		t.Fatal("empty section key must be rejected")
	}
}

func TestAssemblerBlankSectionsSkipped(t *testing.T) {
	a := NewAssembler()
	regMust(t, a, Section{Number: 0, Key: "blank", Body: "   "})
	regMust(t, a, Section{Number: 10, Key: "real", Body: "REAL"})
	out, err := a.Render(nil)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if bytes.Contains(out, []byte("blank")) {
		t.Fatalf("blank section must not emit a block:\n%s", out)
	}
	if !bytes.Contains(out, []byte("REAL")) {
		t.Fatalf("real section missing:\n%s", out)
	}
}

func TestEstimateTokensMatchesSessionHeuristic(t *testing.T) {
	// Session package heuristic: total chars / 4 (internal/session/compaction.go).
	if got := EstimateTokens([]byte("0123456789")); got != 2 {
		t.Fatalf("EstimateTokens(10 chars) = %d, want 2 (chars/4)", got)
	}
}
