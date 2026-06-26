package schema

import "testing"

// TestForbiddenPatterns_JSModuleWithRegex confirms the validator accepts the
// REAL deny-rule format: a JS module whose rules carry regex literals and
// unquoted keys ({ id, re: /.../, allowIf: /.../, why: "..." }). Regex
// char-classes ([...]) and braces must not be miscounted as array/object syntax.
func TestForbiddenPatterns_JSModuleWithRegex(t *testing.T) {
	src := []byte(`import { ALLOW_IF_INSPECTOR_FULL } from "./forbidden-patterns.core.js";
export const FORBIDDEN_PATTERNS = [
    {
        id: "aws-iam-mutate",
        re: /\baws\s+iam\s+(create|delete|attach|detach)-/,
        allowIf: ALLOW_IF_INSPECTOR_FULL,
        why: "IAM is Terraform-managed; do not mutate via the CLI.",
    },
    {
        id: "psql-pii-read",
        re: /\bpsql\b[^|;&\n]+-c\s+["'][^"']*\bSELECT\b[^"']*\bFROM\s+(users|sessions)\b/i,
        allowIf: ALLOW_IF_INSPECTOR_FULL,
        why: "Do not enumerate identity tables via psql.",
    },
];
`)
	if errs := (ForbiddenPatternsProject{}).Validate(src); len(errs) != 0 {
		t.Fatalf("valid JS-module deny-rules should pass, got: %v", errs)
	}
}

// TestForbiddenPatterns_MissingWhy confirms the invariant survives the JS path:
// a rule object without a `why` is reported.
func TestForbiddenPatterns_MissingWhy(t *testing.T) {
	src := []byte(`export const FORBIDDEN_PATTERNS = [
    { id: "no-why", re: /\bfoo\b/ },
];
`)
	errs := (ForbiddenPatternsProject{}).Validate(src)
	if len(errs) == 0 {
		t.Fatal("expected a missing-why error, got none")
	}
}

// TestForbiddenPatterns_EmptyArray confirms the seeded blank file passes.
func TestForbiddenPatterns_EmptyArray(t *testing.T) {
	if errs := (ForbiddenPatternsProject{}).Validate([]byte("export const FORBIDDEN_PATTERNS = [];")); len(errs) != 0 {
		t.Fatalf("empty array should pass, got: %v", errs)
	}
}

// TestForbiddenPatterns_PureJSON confirms the original JSON {pattern,why} form
// still validates (and still flags an empty why).
func TestForbiddenPatterns_PureJSON(t *testing.T) {
	ok := []byte(`[ { "pattern": "rm -rf /", "why": "nope" } ]`)
	if errs := (ForbiddenPatternsProject{}).Validate(ok); len(errs) != 0 {
		t.Fatalf("valid JSON deny-rules should pass, got: %v", errs)
	}
	bad := []byte(`[ { "pattern": "rm -rf /", "why": "" } ]`)
	if errs := (ForbiddenPatternsProject{}).Validate(bad); len(errs) == 0 {
		t.Fatal("empty why in JSON form should fail")
	}
}
