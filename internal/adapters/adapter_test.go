package adapters

import (
	"context"
	"testing"
)

type fakeAdapter struct{ name string }

func (f fakeAdapter) Name() string { return f.name }

func (f fakeAdapter) Call(_ context.Context, _ *Request) (*Response, error) {
	return &Response{}, nil
}

func TestRegistryRegisterGetAndDuplicate(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(fakeAdapter{name: "alpha"}); err != nil {
		t.Fatalf("Register alpha: %v", err)
	}
	if err := r.Register(fakeAdapter{name: "beta"}); err != nil {
		t.Fatalf("Register beta: %v", err)
	}
	if err := r.Register(fakeAdapter{name: "alpha"}); err == nil {
		t.Fatal("expected duplicate registration to be rejected")
	}
	got, ok := r.Get("alpha")
	if !ok || got.Name() != "alpha" {
		t.Fatalf("Get(alpha) = %v, %v", got, ok)
	}
	if _, ok := r.Get("gamma"); ok {
		t.Fatal("Get(gamma) should miss")
	}
	names := r.Names()
	if len(names) != 2 || names[0] != "alpha" || names[1] != "beta" {
		t.Fatalf("Names() = %v, want [alpha beta]", names)
	}
}
