package prompt

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadCompiledRoundTrip(t *testing.T) {
	dir := t.TempDir()
	a, cat := baseAssembler(t), baseCatalog(t)
	art, err := Compile(context.Background(), a, nil, cat, identityVersioned(), InvariantsContract{}, dir)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	loaded, err := LoadCompiled(dir, art.Hash)
	if err != nil {
		t.Fatalf("LoadCompiled: %v", err)
	}
	if !bytes.Equal(loaded.Bytes, art.Bytes) {
		t.Fatal("loaded bytes must equal compiled bytes")
	}
	if loaded.Hash != art.Hash {
		t.Fatalf("hash mismatch: %s vs %s", loaded.Hash, art.Hash)
	}
}

func TestLoadCompiledMissing(t *testing.T) {
	if _, err := LoadCompiled(t.TempDir(), "deadbeef"); !errors.Is(err, ErrArtifactNotFound) {
		t.Fatalf("missing artifact must return ErrArtifactNotFound; got %v", err)
	}
}

func TestLoadCompiledTamperedBytesDetected(t *testing.T) {
	dir := t.TempDir()
	a, cat := baseAssembler(t), baseCatalog(t)
	art, err := Compile(context.Background(), a, nil, cat, identityVersioned(), InvariantsContract{}, dir)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	path := filepath.Join(dir, "prompt-"+art.Hash+".json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// Bytes is base64 inside the artifact JSON — tamper via decode,
	// modify, re-marshal so only the payload changes.
	var asMap map[string]any
	if err := json.Unmarshal(raw, &asMap); err != nil {
		t.Fatal(err)
	}
	tamperedBytes := append([]byte("TAMPERED "), art.Bytes...)
	asMap["bytes"] = tamperedBytes // []byte marshals back to base64
	blob, err := json.Marshal(asMap)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, blob, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadCompiled(dir, art.Hash); !errors.Is(err, ErrArtifactCorrupt) {
		t.Fatalf("tampered artifact must be ErrArtifactCorrupt; got %v", err)
	}
}

func TestServeCompiledUsesCompiledBytes(t *testing.T) {
	dir := t.TempDir()
	a, cat := baseAssembler(t), baseCatalog(t)
	art, err := Compile(context.Background(), a, nil, cat, identityVersioned(), InvariantsContract{}, dir)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	got, res, err := ServeCompiled(dir, art.Hash, a, nil)
	if err != nil {
		t.Fatalf("ServeCompiled: %v", err)
	}
	if res.Source != ServeSourceCompiled {
		t.Fatalf("source must be compiled; got %+v", res)
	}
	if !bytes.Equal(got, art.Bytes) {
		t.Fatal("served bytes must equal artifact bytes")
	}
}

func TestServeCompiledFallsBackWhenMissing(t *testing.T) {
	dir := t.TempDir()
	a, cat := baseAssembler(t), baseCatalog(t)
	raw, err := a.Render(nil)
	if err != nil {
		t.Fatal(err)
	}
	got, res, err := ServeCompiled(dir, "nosuchhash", a, nil)
	if err != nil {
		t.Fatalf("ServeCompiled fallback must not error: %v", err)
	}
	if res.Source != ServeSourceRawAssembly {
		t.Fatalf("source must be raw-assembly; got %+v", res)
	}
	if res.Reason != ServeReasonNotFound {
		t.Fatalf("fallback must be explicit, not silent; got reason %q", res.Reason)
	}
	if !bytes.Equal(got, raw) {
		t.Fatal("fallback bytes must equal raw assembly render")
	}
	if bytes.Contains(got, []byte("identity-fake")) {
		t.Fatal("fallback must never contain optimizer metadata")
	}
	_ = cat
}

func TestServeCompiledFallsBackWhenCorrupt(t *testing.T) {
	dir := t.TempDir()
	a, cat := baseAssembler(t), baseCatalog(t)
	art, err := Compile(context.Background(), a, nil, cat, identityVersioned(), InvariantsContract{}, dir)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "prompt-"+art.Hash+".json"), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	raw, err := a.Render(nil)
	if err != nil {
		t.Fatal(err)
	}
	got, res, err := ServeCompiled(dir, art.Hash, a, nil)
	if err != nil {
		t.Fatalf("ServeCompiled corrupt fallback must not error: %v", err)
	}
	if res.Source != ServeSourceRawAssembly || res.Reason != ServeReasonCorrupt {
		t.Fatalf("corrupt artifact must fall back explicitly; got %+v", res)
	}
	if !bytes.Equal(got, raw) {
		t.Fatal("corrupt fallback bytes must equal raw assembly render")
	}
}

func TestServeCompiledByteStableAcrossRepeatedLoads(t *testing.T) {
	dir := t.TempDir()
	a, cat := baseAssembler(t), baseCatalog(t)
	art, err := Compile(context.Background(), a, nil, cat, identityVersioned(), InvariantsContract{}, dir)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	var first []byte
	for i := 0; i < 3; i++ {
		loaded, err := LoadCompiled(dir, art.Hash)
		if err != nil {
			t.Fatalf("load %d: %v", i, err)
		}
		if i == 0 {
			first = loaded.Bytes
			continue
		}
		if !bytes.Equal(loaded.Bytes, first) {
			t.Fatalf("load %d differs from load 0", i)
		}
	}
	for i := 0; i < 3; i++ {
		got, res, err := ServeCompiled(dir, art.Hash, a, nil)
		if err != nil || res.Source != ServeSourceCompiled {
			t.Fatalf("serve %d: err=%v res=%+v", i, err, res)
		}
		if !bytes.Equal(got, first) {
			t.Fatalf("serve %d is not byte-stable", i)
		}
	}
}

// TestPipelineDedupReducesRedundantInput is the end-to-end pipeline:
// assemble -> compile (dedup fake) -> load -> serve, with the redundant
// two-section input measurably shrunk and the merge surfaced for
// source-level deletion.
func TestPipelineDedupReducesRedundantInput(t *testing.T) {
	dir := t.TempDir()
	body := "Tool guidance: use read to inspect and write to edit repository files."
	a := NewAssembler()
	if err := a.Register(Section{Number: 100, Key: "tools.guidance.core", Owner: "core", Body: body}); err != nil {
		t.Fatal(err)
	}
	if err := a.Register(Section{Number: 110, Key: "tools.guidance.overlay", Owner: "overlay", Body: body, Required: true}); err != nil {
		t.Fatal(err)
	}
	cat := baseCatalog(t)

	art, err := Compile(context.Background(), a, nil, cat, Dedup, InvariantsContract{}, dir)
	if err != nil {
		t.Fatalf("Compile with Dedup: %v", err)
	}
	if art.Tokens.OutputTokens >= art.Tokens.InputTokens {
		t.Fatalf("dedup pipeline must shrink tokens: in=%d out=%d", art.Tokens.InputTokens, art.Tokens.OutputTokens)
	}
	if strings.Count(string(art.Bytes), "repository files") != 1 {
		t.Fatalf("output must contain exactly one copy of the duplicated body:\n%s", art.Bytes)
	}

	// Leak detector: the merged section is surfaced in the artifact table.
	merged := 0
	for _, s := range art.Sections {
		if s.Action == string(ActionMerged) {
			merged++
			if s.Rationale == "" {
				t.Fatalf("merged section %q must carry rationale in artifact", s.Key)
			}
		}
	}
	if merged != 1 {
		t.Fatalf("exactly one merged section expected; got %d", merged)
	}

	// Serve path: byte-stable compiled serving.
	served, res, err := ServeCompiled(dir, art.Hash, a, nil)
	if err != nil || res.Source != ServeSourceCompiled {
		t.Fatalf("ServeCompiled: err=%v res=%+v", err, res)
	}
	if !bytes.Equal(served, art.Bytes) {
		t.Fatal("served bytes must equal artifact bytes in the pipeline")
	}
}
