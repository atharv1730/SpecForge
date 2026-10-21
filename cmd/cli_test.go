package cmd

import (
	"bytes"
	"errors"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func run(t *testing.T, args ...string) (stdout string, err error) {
	t.Helper()
	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)
	err = root.Execute()
	return out.String(), err
}

func TestRootHelpListsSubcommands(t *testing.T) {
	out, err := run(t, "--help")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"generate", "mock", "--version"} {
		if !strings.Contains(out, want) {
			t.Fatalf("root help missing %q\n%s", want, out)
		}
	}
}

func TestGenerateHelpMarksRequiredFlags(t *testing.T) {
	out, err := run(t, "generate", "--help")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"--spec", "--output", "--language", "required"} {
		if !strings.Contains(out, want) {
			t.Fatalf("generate help missing %q\n%s", want, out)
		}
	}
	gen := newGenerateCmd()
	for _, name := range []string{"spec", "output", "language"} {
		if !isRequired(gen, name) {
			t.Fatalf("%s is not marked required", name)
		}
	}
}

func TestMockHelpShowsSpecAndPort(t *testing.T) {
	out, err := run(t, "mock", "--help")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"--spec", "--port", "8080", "required"} {
		if !strings.Contains(out, want) {
			t.Fatalf("mock help missing %q\n%s", want, out)
		}
	}
	if !isRequired(newMockCmd(), "spec") {
		t.Fatal("spec is not marked required")
	}
}

func TestMissingRequiredFlags(t *testing.T) {
	if _, err := run(t, "generate"); err == nil {
		t.Fatal("expected missing required flags to fail")
	}
	if _, err := run(t, "mock"); err == nil {
		t.Fatal("expected missing required flags to fail")
	}
}

func TestBinaryLoadsPetstore(t *testing.T) {
	root := repoRoot(t)
	bin := filepath.Join(t.TempDir(), "specforge")
	build := exec.Command("go", "build", "-o", bin, ".")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}

	spec := "testdata/petstore.yaml"
	want := "Loaded 3 endpoints from " + spec
	for _, args := range [][]string{
		{"generate", "--spec", spec, "--output", "out", "--language", "typescript"},
		{"mock", "--spec", spec},
	} {
		out, err := runBin(t, bin, root, args...)
		if err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
		if !strings.Contains(out, want) {
			t.Fatalf("%v output = %q", args, out)
		}
	}

	out, err := runBin(t, bin, root, "generate", "--spec", "bad.yaml", "--output", "out", "--language", "typescript")
	if err == nil {
		t.Fatal("expected exit 1")
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
		t.Fatalf("err = %v\n%s", err, out)
	}
	if !strings.Contains(out, "bad.yaml") {
		t.Fatalf("output = %q", out)
	}
}

func runBin(t *testing.T, bin, dir string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Dir(filepath.Dir(file))
}

func isRequired(cmd *cobra.Command, name string) bool {
	ann := cmd.Flags().Lookup(name).Annotations
	return len(ann[cobra.BashCompOneRequiredFlag]) > 0
}
