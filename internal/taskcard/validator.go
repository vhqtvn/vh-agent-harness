// Package taskcard implements the FULL task-card contract validator (defer-018):
// a bounded, dependency-free draft-07 subset engine plus the cross-field
// acknowledgement-pair guard that draft-07 cannot express. It is the Go port of
// the retired templates/core/.opencode/scripts/verify-task-card-schema.py.
//
// Architectural correction: the harness's scripting runtime is the
// `vh-agent-harness` Go binary; JavaScript exists only for opencode integration.
// A standalone Python script carrying a `pip install jsonschema` dependency was
// an outlier that must not live in a Go repo's gate. The validator now ships in
// the binary and runs in `go test ./...` with no Python and no pip.
//
// Relationship to internal/memory/recurrence.MalformedBlock: that function is a
// load-bearing SUBSET (recurrence block identity/count fields) consumed by
// doctor's runtime advisory. This package is the FULL contract validator
// (nested evidence[]/aliases[] item shapes, all task-card fields, the relational
// ack guard). They are complementary; MalformedBlock is intentionally retained.
package taskcard

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math/big"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	corpus "github.com/vhqtvn/vh-agent-harness"
)

// SchemaRelPath is the live (corpus-relative) path of the task-card JSON Schema
// inside the curated core corpus. Reading from the embed (not the live
// filesystem) keeps the validator self-contained and drift-free: the binary
// always validates against the exact schema it ships, and `go test` validates
// against the working-tree schema captured by go:embed at compile time.
//
// The source schema's title carries the {{PROJECT_NAME}} token; it is an inert
// metadata string (no validation keyword references it), so the token does not
// affect validation.
const SchemaRelPath = "docs/coordination/schemas/task-card.schema.json"

// SchemaBytes returns the embedded task-card JSON Schema bytes from the curated
// core corpus.
func SchemaBytes() ([]byte, error) {
	sub, err := fs.Sub(corpus.CoreFS, corpus.CoreDir)
	if err != nil {
		return nil, fmt.Errorf("task-card schema: open corpus: %w", err)
	}
	b, err := fs.ReadFile(sub, SchemaRelPath)
	if err != nil {
		return nil, fmt.Errorf("task-card schema: read %s: %w", SchemaRelPath, err)
	}
	return b, nil
}

func parseSchema(raw []byte) (map[string]interface{}, error) {
	var s map[string]interface{}
	// UseNumber so schema numbers decode to json.Number (not float64). This
	// keeps schema values (minimum, const) in the SAME numeric representation
	// as the instance (ValidateCard decodes with UseNumber too), so const/enum
	// equality and minimum comparison stay type-consistent.
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&s); err != nil {
		return nil, err
	}
	return s, nil
}

// VError is one validation defect: a dot/bracket instance path ("" at root) and
// a human-readable message. Path uses JSON Pointer-ish segments ("recurrence",
// "recurrence.evidence[0]", "history[0].status").
type VError struct {
	Path    string
	Message string
}

// String renders an error as "<path>: <message>" (or just the message at root).
func (e VError) String() string {
	if e.Path == "" {
		return e.Message
	}
	return e.Path + ": " + e.Message
}

// Validate runs the bounded draft-07 subset validator and returns all collected
// errors (nil/empty if the instance is valid). Pure: no I/O, no side effects.
//
// Covered draft-07 keywords (exactly the set the task-card schema uses):
// type (single string or string-array union), enum, const, required,
// properties, additionalProperties (boolean), pattern, minimum, minLength,
// minItems, format ("date-time" via RFC3339), items, allOf, anyOf, and
// if/then/else. Keywords NOT covered (none are used by the schema): maximum,
// exclusiveMinimum/Maximum, maxLength, maxItems, uniqueItems, multipleOf,
// patternProperties, dependencies, oneOf, not, minProperties/maxProperties,
// contains, propertyNames, $ref/$id/definitions. If the schema grows a keyword
// outside this subset, add a row here — do not silently drop it.
func Validate(schema map[string]interface{}, inst interface{}) []VError {
	var errs []VError
	validateNode(schema, inst, "", &errs)
	return errs
}

// validateNode recursively validates inst against schema, appending every defect
// to *errs. It applies every supported keyword present in schema regardless of
// inst's type; per-type keywords (pattern, minimum, ...) self-guard on the
// instance type so they are inert for mismatched types.
func validateNode(schema map[string]interface{}, inst interface{}, path string, errs *[]VError) {
	if t, ok := schema["type"]; ok {
		if !typeMatches(t, inst) {
			*errs = append(*errs, VError{path, fmt.Sprintf("expected type %s, got %s", typeRepr(t), jsonTypeName(inst))})
		}
	}
	if c, ok := schema["const"]; ok {
		if !equalJSON(inst, c) {
			*errs = append(*errs, VError{path, fmt.Sprintf("expected const %v", c)})
		}
	}
	if e, ok := schema["enum"]; ok {
		if !enumMatches(e, inst) {
			*errs = append(*errs, VError{path, fmt.Sprintf("value not in enum %v", e)})
		}
	}

	// String-only keywords.
	if s, ok := inst.(string); ok {
		if p, ok := schema["pattern"].(string); ok {
			if re, err := regexp.Compile(p); err != nil {
				*errs = append(*errs, VError{path, fmt.Sprintf("schema pattern invalid: %v", err)})
			} else if !re.MatchString(s) {
				*errs = append(*errs, VError{path, fmt.Sprintf("string %q does not match pattern %q", s, p)})
			}
		}
		if ml, ok := schema["minLength"]; ok {
			if min := toInt(ml); len(s) < min {
				*errs = append(*errs, VError{path, fmt.Sprintf("string length %d < minLength %d", len(s), min)})
			}
		}
		if f, ok := schema["format"].(string); ok && f == "date-time" {
			if !validRFC3339DateTime(s) {
				*errs = append(*errs, VError{path, fmt.Sprintf("not a valid RFC3339 date-time: %q", s)})
			}
		}
	}

	// Number-only keyword (minimum). Applies to both integer and number types.
	// Compared EXACTLY via math/big.Rat (cmpNumber) so an integer literal whose
	// magnitude overflows float64 (e.g. schema_version or a recurrence count set
	// to a huge-magnitude value) is still constrained against the minimum rather
	// than being silently skipped by a float64 range error (the asymmetry that a
	// float64-based comparison would reintroduce vs the ack-pair guard's exact
	// big.Int comparison). Non-numeric instances return ok=false and are skipped
	// (they fail the type check elsewhere).
	if m, ok := schema["minimum"]; ok {
		if c, ok := cmpNumber(inst, m); ok && c < 0 {
			*errs = append(*errs, VError{path, fmt.Sprintf("%v < minimum %v", numRepr(inst), numRepr(m))})
		}
	}

	// Object-only keywords.
	if obj, ok := inst.(map[string]interface{}); ok {
		validateObject(schema, obj, path, errs)
	}

	// Array-only keywords.
	if arr, ok := inst.([]interface{}); ok {
		if mi, ok := schema["minItems"]; ok {
			if min := toInt(mi); len(arr) < min {
				*errs = append(*errs, VError{path, fmt.Sprintf("array length %d < minItems %d", len(arr), min)})
			}
		}
		if items, ok := schema["items"].(map[string]interface{}); ok {
			for i, el := range arr {
				validateNode(items, el, fmt.Sprintf("%s[%d]", path, i), errs)
			}
		}
	}

	// Combinators (draft-07 if/then/else, allOf, anyOf).
	if ifs, ok := schema["if"].(map[string]interface{}); ok {
		var cond []VError
		validateNode(ifs, inst, path, &cond) // `if` errors never surface.
		if len(cond) == 0 {
			if then, ok := schema["then"].(map[string]interface{}); ok {
				validateNode(then, inst, path, errs)
			}
		} else {
			if els, ok := schema["else"].(map[string]interface{}); ok {
				validateNode(els, inst, path, errs)
			}
		}
	}
	if all, ok := schema["allOf"].([]interface{}); ok {
		for _, sub := range all {
			if sm, ok := sub.(map[string]interface{}); ok {
				validateNode(sm, inst, path, errs)
			}
		}
	}
	if any, ok := schema["anyOf"].([]interface{}); ok {
		matched := false
		for _, sub := range any {
			if sm, ok := sub.(map[string]interface{}); ok {
				var subErrs []VError
				validateNode(sm, inst, path, &subErrs)
				if len(subErrs) == 0 {
					matched = true
					break
				}
			}
		}
		if !matched {
			*errs = append(*errs, VError{path, "does not match any anyOf option"})
		}
	}
}

// validateObject applies required / properties / additionalProperties for an
// object instance. Declared properties validate only when present (draft-07
// `properties` constrains values, not presence — `required` does that).
func validateObject(schema map[string]interface{}, obj map[string]interface{}, path string, errs *[]VError) {
	if req, ok := schema["required"].([]interface{}); ok {
		// Deterministic order by required-list order.
		for _, r := range req {
			if name, ok := r.(string); ok {
				if _, present := obj[name]; !present {
					*errs = append(*errs, VError{path, fmt.Sprintf("missing required property %q", name)})
				}
			}
		}
	}

	props, _ := schema["properties"].(map[string]interface{})
	// Deterministic order by property name so diagnostics read predictably.
	names := make([]string, 0, len(props))
	for k := range props {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, name := range names {
		subSchema, ok := props[name].(map[string]interface{})
		if !ok {
			continue
		}
		if val, present := obj[name]; present {
			validateNode(subSchema, val, joinPath(path, name), errs)
		}
	}

	if ap, ok := schema["additionalProperties"]; ok {
		if allow, ok := ap.(bool); ok && !allow {
			// No patternProperties in this schema, so "additional" = any key not
			// declared in properties.
			var extra []string
			for k := range obj {
				if _, declared := props[k]; !declared {
					extra = append(extra, k)
				}
			}
			sort.Strings(extra)
			for _, k := range extra {
				*errs = append(*errs, VError{path, fmt.Sprintf("additional property %q not allowed", k)})
			}
		}
	}
}

// AckPairError enforces the cross-field invariant
// recurrence.recurrence_count >= recurrence.last_acknowledged_count that
// draft-07 cannot express. It mirrors assert_ack_pair from the Python validator:
// a no-op when no recurrence block is present, and — once the schema has
// guaranteed both counts are present non-negative integers — a single relational
// check. Returns nil for a malformed/missing pair (the schema owns that error);
// this guard only rejects the impossible-state where both are individually
// well-formed but count < ack.
//
// Precision: the comparison is EXACT. The retired Python compared arbitrary-
// precision ints; a float64 comparison would round both counts above 2^53 to
// the same value and slip the invariant (count=2^53, ack=2^53+1 → both 2^53 →
// not-less-than). ValidateCard decodes counts as json.Number, and
// isIntegerValue accepts every draft-07 integer form (plain, decimal-zero,
// exponent); the comparison therefore goes through cmpInteger → cmpNumber →
// math/big.Rat.Cmp, which is exact at any magnitude and across all those forms
// (a float64 ladder would collapse adjacent values >= 2^53 for the ".0"/exponent
// forms the type check now accepts).
func AckPairError(card map[string]interface{}) error {
	rec, ok := card["recurrence"].(map[string]interface{})
	if !ok {
		return nil // no block (or non-object) → no-op; legacy cards pass.
	}
	count, ack := rec["recurrence_count"], rec["last_acknowledged_count"]
	if !isIntegerValue(count) || !isIntegerValue(ack) {
		return nil // missing/non-integer — the schema's required+type owns this.
	}
	if cmpInteger(count, ack) < 0 {
		return fmt.Errorf("recurrence_count (%s) must be >= last_acknowledged_count (%s)",
			numRepr(count), numRepr(ack))
	}
	return nil
}

// Result is the verdict of ValidateCard.
type Result struct {
	Valid        bool     // true iff the schema passes AND the ack-pair guard holds.
	SchemaErrors []VError // schema-level defects (empty when the schema passes).
	AckPairError string   // non-empty when the cross-field ack guard rejects (schema had passed).
}

// ValidateCard decodes raw task-card bytes, validates against the task-card
// schema (the bounded draft-07 subset) and then — only when the schema passes —
// applies the cross-field ack-pair guard. It returns the verdict. A non-nil
// error means the input was not valid JSON (a precondition failure, not a
// validation verdict).
func ValidateCard(raw []byte) (*Result, error) {
	schemaBytes, err := SchemaBytes()
	if err != nil {
		return nil, err
	}
	schema, err := parseSchema(schemaBytes)
	if err != nil {
		return nil, fmt.Errorf("parse schema: %w", err)
	}
	// UseNumber keeps instance numbers as json.Number so the `integer` type
	// check is lexical (rejects fractional/exponent forms a float64 decode
	// would round to an integral value at magnitude >= 2^53 — see
	// jsonTypeMatches). A non-JSON input is a precondition error, not a verdict.
	var inst interface{}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&inst); err != nil {
		return nil, fmt.Errorf("parse task card: %w", err)
	}
	// Reject trailing data: a JSON document is ONE value (RFC 8259). A second
	// decode must reach EOF; otherwise the input is a multi-document / trailing-
	// garbage stream, which json.Decoder would otherwise silently accept.
	var trailing interface{}
	if err := dec.Decode(&trailing); err == nil {
		return nil, fmt.Errorf("parse task card: trailing data after top-level JSON value")
	} else if !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("parse task card: %w", err)
	}
	res := &Result{SchemaErrors: Validate(schema, inst)}
	if len(res.SchemaErrors) == 0 {
		if card, ok := inst.(map[string]interface{}); ok {
			if e := AckPairError(card); e != nil {
				res.AckPairError = e.Error()
			}
		}
	}
	res.Valid = len(res.SchemaErrors) == 0 && res.AckPairError == ""
	return res, nil
}

// ---- helpers ---------------------------------------------------------------

// typeMatches handles a `type` keyword that is either a single string or a
// string-array (union) — the schema uses both ("integer" and ["string","null"]).
func typeMatches(t, inst interface{}) bool {
	switch tt := t.(type) {
	case string:
		return jsonTypeMatches(tt, inst)
	case []interface{}:
		for _, opt := range tt {
			if s, ok := opt.(string); ok && jsonTypeMatches(s, inst) {
				return true
			}
		}
		return false
	}
	return true
}

// jsonTypeMatches maps a draft-07 type name to a Go JSON-decoded value.
//
// Integer precision: draft-07 defines "integer" as a number with zero fractional
// part. The check is delegated to isIntegerValue, which compares EXACTLY via
// math/big.Rat (toRat → big.Rat.IsInt, denominator == 1). This is parity-correct
// with the retired Python jsonschema (float.is_integer, accepting "1.0"/"1e2")
// and precision-exact: an integer literal whose magnitude overflows float64
// (e.g. a huge recurrence count) stays an integer without a float64 range error,
// while a fractional literal (e.g. "9007199254740993.5") is rejected regardless
// of how its float64 coercion rounds.
func jsonTypeMatches(want string, inst interface{}) bool {
	switch want {
	case "string":
		_, ok := inst.(string)
		return ok
	case "boolean":
		_, ok := inst.(bool)
		return ok
	case "object":
		_, ok := inst.(map[string]interface{})
		return ok
	case "array":
		_, ok := inst.([]interface{})
		return ok
	case "null":
		return inst == nil
	case "integer":
		return isIntegerValue(inst)
	case "number":
		switch inst.(type) {
		case json.Number, float64, float32, int, int64:
			return true
		}
		return false
	}
	return false
}

// asNumber extracts a float64 from a JSON-decoded numeric value. ValidateCard
// decodes numbers as json.Number; the int/int64/float cases defend against
// hand-built maps (the test fixtures construct some values as Go literals).
func asNumber(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	}
	return 0, false
}

func toInt(v interface{}) int {
	if n, ok := asNumber(v); ok {
		return int(n)
	}
	return 0
}

// isIntegerValue reports whether v is an integer-valued number under draft-07
// semantics ("a number with zero fractional part"), compared EXACTLY via
// math/big.Rat (toRat): a numeric lexeme is an integer iff its reduced rational
// form has denominator 1 (big.Rat.IsInt). This is parity-correct with the
// retired Python jsonschema (float.is_integer, which accepts "1.0" and "1e2")
// AND precision-exact: an integer literal whose magnitude overflows float64
// parses to a Rat with denominator 1 and remains an integer without a float64
// range error. Non-numeric values return false.
func isIntegerValue(v interface{}) bool {
	r, ok := toRat(v)
	if !ok {
		return false
	}
	return r.IsInt()
}

// validRFC3339DateTime reports whether s is a valid RFC 3339 date-time.
//
// RFC 3339 (section 5.6 note) explicitly permits the date-time separator and
// the "Zulu" zone designator to be lower case: "[Tt]" and "[Zz]". Go's
// time.Parse layout treats 'T'/'Z' as case-sensitive literals, so it would
// reject e.g. "2026-04-30t12:00:00z". The retired Python gate used
// jsonschema's FormatChecker (rfc3339-validator backend), whose regex accepts
// [Tt]/[Zz]; to preserve parity with the retired reference and the RFC, this
// helper normalizes those two case-insensitive positions before parsing. The
// original value is never mutated — this is a format-validity check only.
func validRFC3339DateTime(s string) bool {
	// Fast path: the standard upper-case form (the common, machine-generated case).
	if _, err := time.Parse(time.RFC3339, s); err == nil {
		return true
	}
	norm := s
	// The separator sits at index 10 (immediately after the fixed YYYY-MM-DD date).
	if len(norm) > 10 && norm[10] == 't' {
		b := []byte(norm)
		b[10] = 'T'
		norm = string(b)
	}
	// A trailing lower-case 'z' is unambiguously the Zulu zone designator (the
	// only legal RFC 3339 zone token that ends with that letter).
	if strings.HasSuffix(norm, "z") {
		norm = norm[:len(norm)-1] + "Z"
	}
	_, err := time.Parse(time.RFC3339, norm)
	return err == nil
}

// cmpInteger compares two integer values EXACTLY, returning -1/0/1 for
// a<b / a==b / a>b. It delegates to cmpNumber (math/big.Rat.Cmp via toRat),
// which handles every integer form isIntegerValue accepts — plain literals
// ("9007199254740993"), decimal-zero ("9007199254740993.0"), and exponent
// ("1e2") — at arbitrary magnitude, so a ".0"/exponent integer literal is
// compared by its true value rather than a float64 coercion that collapses
// adjacent values at >= 2^53. Both operands must satisfy isIntegerValue;
// behaviour is undefined otherwise.
func cmpInteger(a, b interface{}) int {
	c, ok := cmpNumber(a, b)
	if !ok {
		return 0 // unreachable for isIntegerValue operands; defensive equal.
	}
	return c
}

// toRat converts a numeric JSON value to an EXACT big.Rat. It accepts decimal
// literals (integer, fractional, or with exponent) as parsed from json.Number,
// and finite Go numeric literals. Returns ok=false for non-numeric values;
// JSON itself never produces NaN/Inf, and big.Rat parses arbitrarily large
// magnitudes without a range error.
func toRat(v interface{}) (*big.Rat, bool) {
	switch n := v.(type) {
	case json.Number:
		r, ok := new(big.Rat).SetString(string(n))
		return r, ok
	case float64:
		return new(big.Rat).SetFloat64(n), true
	case float32:
		return new(big.Rat).SetFloat64(float64(n)), true
	case int:
		return new(big.Rat).SetInt64(int64(n)), true
	case int64:
		return new(big.Rat).SetInt64(n), true
	}
	return nil, false
}

// cmpNumber compares two numeric values EXACTLY as rationals, returning
// -1/0/1 for a<b / a==b / a>b. It uses math/big.Rat (via toRat) so an integer
// literal whose magnitude overflows float64 — in any draft-07 integer form
// (plain, decimal-zero, exponent) — is still compared by its true value,
// instead of being silently skipped by a float64 range error or collapsed onto
// an adjacent value. The scalar minimum check and the cross-field ack-pair
// guard (via cmpInteger) both route through this exact comparison. Returns
// ok=false if either operand is not a finite numeric value.
func cmpNumber(a, b interface{}) (int, bool) {
	ra, oka := toRat(a)
	rb, okb := toRat(b)
	if !oka || !okb {
		return 0, false
	}
	return ra.Cmp(rb), true
}

// numRepr renders a numeric value for a diagnostic message, preferring the
// original lexeme for json.Number (preserves the exact authored form, e.g.
// "9007199254740993") and falling back to numText for float64/int.
func numRepr(v interface{}) string {
	if n, ok := v.(json.Number); ok {
		return string(n)
	}
	if f, ok := asNumber(v); ok {
		return numText(f)
	}
	return fmt.Sprint(v)
}

func jsonTypeName(v interface{}) string {
	switch v.(type) {
	case nil:
		return "null"
	case bool:
		return "boolean"
	case json.Number, float64:
		return "number"
	case string:
		return "string"
	case []interface{}:
		return "array"
	case map[string]interface{}:
		return "object"
	}
	return reflect.TypeOf(v).String()
}

func typeRepr(t interface{}) string {
	switch tt := t.(type) {
	case string:
		return tt
	case []interface{}:
		parts := make([]string, 0, len(tt))
		for _, p := range tt {
			parts = append(parts, fmt.Sprint(p))
		}
		return "[" + strings.Join(parts, ",") + "]"
	}
	return fmt.Sprint(t)
}

// numText renders a number minimally: strconv.FormatFloat with precision -1
// yields "2" for 2.0 and "2.5" for 2.5 (no trailing .0 on integral floats), so
// diagnostics read as authored integers rather than float64 artifacts.
func numText(n float64) string {
	return strconv.FormatFloat(n, 'f', -1, 64)
}

// enumMatches reports whether inst equals any value in the enum array. JSON null
// decodes to nil in both the enum array and the instance, so a null instance
// matches an enum that lists null.
func enumMatches(enum, inst interface{}) bool {
	list, ok := enum.([]interface{})
	if !ok {
		return false
	}
	for _, opt := range list {
		if equalJSON(opt, inst) {
			return true
		}
	}
	return false
}

// equalJSON is JSON-value equality over decoded values. Under ValidateCard's
// json.Decoder.UseNumber, numbers arrive as json.Number (lexical equality),
// not float64 — so two numeric consts/enums compare by lexeme. This is exact
// for the schema's current const/enum usage (string/bool/null only; no numeric
// const/enum). If a numeric const/enum is ever added, prefer a value-aware
// comparison (parse both sides) over this lexical DeepEqual.
func equalJSON(a, b interface{}) bool {
	return reflect.DeepEqual(a, b)
}

func joinPath(base, name string) string {
	if base == "" {
		return name
	}
	return base + "." + name
}
