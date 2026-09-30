// component_test.go — the canonical id-component grammar shared by the
// protocol engine boundary and the subagents FileStore.
package session

import (
	"errors"
	"testing"
)

func TestValidateIDComponent(t *testing.T) {
	valid := []string{
		"s", "s1", "S", "A-b_2.x", "sess-0123456789abcdef", "root-1.1",
		"x..", "9starts-digit", "has--under_score",
	}
	for _, id := range valid {
		if err := ValidateIDComponent(id); err != nil {
			t.Errorf("ValidateIDComponent(%q) = %v, want nil", id, err)
		}
	}

	invalid := []string{
		"",        // empty
		".", "..", // relative primitives
		"../climb", "a/b", `a\b`, // separators (both kinds)
		"/abs",                                   // absolute
		".hidden", "-leading", "_leading", ".  ", // first-char grammar
		"has space", "has\ttab", "has\nnewline",
		"unicode-é", "semi;colon", "star*",
	}
	for _, id := range invalid {
		if err := ValidateIDComponent(id); err == nil {
			t.Errorf("ValidateIDComponent(%q) = nil, want rejection", id)
		} else if !errors.Is(err, ErrInvalidIDComponent) {
			t.Errorf("ValidateIDComponent(%q) = %v, want it to wrap ErrInvalidIDComponent", id, err)
		}
	}
}
