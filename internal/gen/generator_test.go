package gen

import (
	"strings"
	"testing"

	"github.com/atharv1730/SpecForge/internal/ir"
)

type stubGen struct{}

func (stubGen) Generate(*ir.API, string) error { return nil }

func TestRegistry(t *testing.T) {
	const language = "test-lang"
	t.Cleanup(func() { delete(registry, language) })

	stub := stubGen{}
	Register(language, stub)
	got, err := Get(language)
	if err != nil {
		t.Fatal(err)
	}
	if got != stub {
		t.Fatalf("got %#v", got)
	}

	_, err = Get("nope")
	if err == nil || !strings.Contains(err.Error(), "unsupported language") || !strings.Contains(err.Error(), "nope") {
		t.Fatalf("err = %v", err)
	}
}
