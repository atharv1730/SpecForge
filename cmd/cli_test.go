package cmd

import (
	"bytes"
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

func TestGenerateStub(t *testing.T) {
	if _, err := run(t, "generate"); err == nil {
		t.Fatal("expected missing required flags to fail")
	}
	out, err := run(t, "generate", "--spec", "api.yaml", "--output", "out", "--language", "typescript")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "generate: not implemented") {
		t.Fatalf("stdout = %q", out)
	}
}

func TestMockStub(t *testing.T) {
	if _, err := run(t, "mock"); err == nil {
		t.Fatal("expected missing required flags to fail")
	}
	out, err := run(t, "mock", "--spec", "api.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "mock: not implemented") {
		t.Fatalf("stdout = %q", out)
	}
}

func isRequired(cmd *cobra.Command, name string) bool {
	ann := cmd.Flags().Lookup(name).Annotations
	return len(ann[cobra.BashCompOneRequiredFlag]) > 0
}
