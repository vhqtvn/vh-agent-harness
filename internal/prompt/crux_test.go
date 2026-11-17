package prompt

import (
	"bytes"
	"context"
	"errors"
	"os"
	"testing"
)

// TestCruxCompileServeFallbackPipeline is the load-bearing crux: compile
// offline, serve compiled bytes, lose the artifact, fall back to raw
// assembly explicitly, recompile, and observe byte-stable serving across
// repeated loads — the request path never running optimizer logic.
func TestCruxCompileServeFallbackPipeline(t *testing.T) {
	dir := t.TempDir()
	a, cat := baseAssembler(t), baseCatalog(t)
	ctx := context.Background()

	// 1. Offline compile.
	art, err := Compile(ctx, a, nil, cat, identityVersioned(), InvariantsContract{}, dir)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	// 2. Serve from the compiled artifact.
	compiled1, res, err := ServeCompiled(dir, art.Hash, a, nil)
	if err != nil || res.Source != ServeSourceCompiled || !bytes.Equal(compiled1, art.Bytes) {
		t.Fatalf("serve-compiled: err=%v res=%+v equal=%v", err, res, bytes.Equal(compiled1, art.Bytes))
	}

	// 3. Lose the artifact; the request path falls back to raw assembly,
	//    explicitly, and without invoking any optimizer.
	if err := os.Remove(dir + "/prompt-" + art.Hash + ".json"); err != nil {
		t.Fatal(err)
	}
	raw, err := a.Render(nil)
	if err != nil {
		t.Fatal(err)
	}
	fallback, res, err := ServeCompiled(dir, art.Hash, a, nil)
	if err != nil || res.Source != ServeSourceRawAssembly || res.Reason != ServeReasonNotFound {
		t.Fatalf("serve-fallback: err=%v res=%+v", err, res)
	}
	if !bytes.Equal(fallback, raw) {
		t.Fatal("fallback must serve the raw assembly bytes")
	}
	if _, err := LoadCompiled(dir, art.Hash); !errors.Is(err, ErrArtifactNotFound) {
		t.Fatalf("expected ErrArtifactNotFound; got %v", err)
	}

	// 4. Recompile offline (explicit step, not request-path magic).
	art2, err := Compile(ctx, a, nil, cat, identityVersioned(), InvariantsContract{}, dir)
	if err != nil {
		t.Fatalf("recompile: %v", err)
	}
	if art2.Hash != art.Hash || !bytes.Equal(art2.Bytes, art.Bytes) {
		t.Fatal("recompilation of identical inputs must reproduce identical bytes")
	}

	// 5. Byte-stable serving across repeated loads.
	for i := 0; i < 3; i++ {
		got, res, err := ServeCompiled(dir, art.Hash, a, nil)
		if err != nil || res.Source != ServeSourceCompiled || !bytes.Equal(got, art.Bytes) {
			t.Fatalf("stable serve %d: err=%v res=%+v", i, err, res)
		}
	}
}
