package loader

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestLoadPetstore(t *testing.T) {
	t.Chdir(repoRoot(t))

	doc, err := Load("testdata/petstore.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if doc == nil {
		t.Fatal("expected document")
	}
	if doc.Info == nil || doc.Info.Title != "Swagger Petstore" {
		t.Fatalf("info = %#v", doc.Info)
	}
	if doc.Paths == nil || doc.Paths.Find("/pets") == nil {
		t.Fatal("expected /pets")
	}
}

func TestLoadNonexistent(t *testing.T) {
	doc, err := Load("nonexistent.yaml")
	if err == nil {
		t.Fatal("expected error")
	}
	if doc != nil {
		t.Fatal("expected nil document")
	}
	if !strings.Contains(err.Error(), "nonexistent.yaml") || !strings.Contains(err.Error(), "Check that --spec") {
		t.Fatalf("error = %v", err)
	}
}

func TestLoadInvalidSpec(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.yaml")
	// YAML parses, but the operation has no responses, so validation fails.
	const spec = `
openapi: "3.0.3"
info:
  title: Bad
  version: "0.0.1"
paths:
  /x:
    get:
      operationId: bad
`
	if err := os.WriteFile(path, []byte(spec), 0o644); err != nil {
		t.Fatal(err)
	}

	doc, err := Load(path)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if doc != nil {
		t.Fatal("expected nil document")
	}
	if !strings.Contains(err.Error(), "validation") || !strings.Contains(err.Error(), "Fix the document") {
		t.Fatalf("error = %v", err)
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Join(filepath.Dir(file), "..", "..")
}
