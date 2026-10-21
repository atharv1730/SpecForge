package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

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
	for _, want := range []string{"generate", "mock", "conformance", "--version", "--verbose"} {
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

func TestVerboseFlag(t *testing.T) {
	t.Chdir(repoRoot(t))

	dir := t.TempDir()
	out, err := run(t, "generate", "--spec", "testdata/petstore.yaml", "--output", dir, "--language", "typescript", "--verbose")
	if err != nil {
		t.Fatalf("generate --verbose: %v\n%s", err, out)
	}
	for _, want := range []string{
		"verbose: loading spec testdata/petstore.yaml",
		"verbose: parsed \"Swagger Petstore\" version 1.0.0",
		"verbose: transforming testdata/petstore.yaml into the IR",
		"verbose: base URL http://petstore.swagger.io/v1",
		"verbose: endpoint GET /pets listPets",
		"verbose: endpoint POST /pets createPets",
		"verbose: endpoint GET /pets/{petId} showPetById",
		"verbose: generating typescript client in " + dir,
		"verbose: wrote " + filepath.Join(dir, "types.ts"),
		"verbose: wrote " + filepath.Join(dir, "client.ts"),
		"Generated TypeScript client in " + dir,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("verbose output missing %q\n%s", want, out)
		}
	}

	out, err = run(t, "generate", "--spec", "testdata/petstore.yaml", "--output", t.TempDir(), "--language", "python")
	if err != nil {
		t.Fatalf("generate: %v\n%s", err, out)
	}
	if strings.Contains(out, "verbose:") {
		t.Fatalf("verbose logs without --verbose:\n%s", out)
	}
}

func TestMissingRequiredFlags(t *testing.T) {
	if _, err := run(t, "generate"); err == nil {
		t.Fatal("expected missing required flags to fail")
	}
	if _, err := run(t, "mock"); err == nil {
		t.Fatal("expected missing required flags to fail")
	}
	if _, err := run(t, "conformance"); err == nil {
		t.Fatal("expected missing required flags to fail")
	}
}

func TestConformanceHelpMarksRequiredFlags(t *testing.T) {
	out, err := run(t, "conformance", "--help")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"--spec", "--language", "--port", "8080", "required"} {
		if !strings.Contains(out, want) {
			t.Fatalf("conformance help missing %q\n%s", want, out)
		}
	}
	conf := newConformanceCmd()
	for _, name := range []string{"spec", "language"} {
		if !isRequired(conf, name) {
			t.Fatalf("%s is not marked required", name)
		}
	}
}

func TestConformanceCLI(t *testing.T) {
	t.Chdir(repoRoot(t))

	port := freePort(t)
	out, err := run(t, "conformance", "--spec", "testdata/petstore.yaml", "--language", "typescript", "--port", strconv.Itoa(port))
	if err != nil {
		t.Fatalf("conformance: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Loaded 3 endpoints from testdata/petstore.yaml") || !strings.Contains(out, "3 passed, 0 failed") {
		t.Fatalf("output = %q", out)
	}

	port = freePort(t)
	out, err = run(t, "conformance", "--spec", "testdata/petstore.yaml", "--language", "python", "--port", strconv.Itoa(port))
	if err != nil {
		t.Fatalf("python conformance: %v\n%s", err, out)
	}
	if !strings.Contains(out, "3 passed, 0 failed") {
		t.Fatalf("python output = %q", out)
	}

	spec := filepath.Join(t.TempDir(), "fail.yaml")
	failSpec := []byte(`openapi: "3.0.0"
info:
  title: Fail
  version: "1.0.0"
paths:
  /missing:
    get:
      operationId: noSuccess
      responses:
        "404":
          description: missing
`)
	if err := os.WriteFile(spec, failSpec, 0o644); err != nil {
		t.Fatal(err)
	}
	port = freePort(t)
	out, err = run(t, "conformance", "--spec", spec, "--language", "typescript", "--port", strconv.Itoa(port))
	if err == nil {
		t.Fatalf("expected failing conformance to return an error\n%s", out)
	}
	if !strings.Contains(out, "FAIL GET /missing noSuccess:") || !strings.Contains(out, "0 passed, 1 failed") {
		t.Fatalf("output = %q", out)
	}
	if strings.Contains(out, "Usage:") {
		t.Fatalf("usage printed on conformance failure\n%s", out)
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

	outDir := filepath.Join(t.TempDir(), "out")
	out, err := runBin(t, bin, root, "generate", "--spec", spec, "--output", outDir, "--language", "typescript")
	if err != nil {
		t.Fatalf("generate: %v\n%s", err, out)
	}
	if !strings.Contains(out, want) || !strings.Contains(out, "Generated TypeScript client in "+outDir) {
		t.Fatalf("generate output = %q", out)
	}
	types := string(mustRead(t, filepath.Join(outDir, "types.ts")))
	client := string(mustRead(t, filepath.Join(outDir, "client.ts")))
	for _, want := range []string{"export interface Pet", "export interface Error", "export type Pets"} {
		if !strings.Contains(types, want) {
			t.Fatalf("types.ts missing %q\n%s", want, types)
		}
	}
	for _, want := range []string{"async listPets(", "async showPetById(", "/pets/${encodeURIComponent(petId)}"} {
		if !strings.Contains(client, want) {
			t.Fatalf("client.ts missing %q\n%s", want, client)
		}
	}

	out, err = runBin(t, bin, root, "generate", "--spec", spec, "--output", outDir, "--language", "python")
	if err != nil {
		t.Fatalf("python: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Generated Python client in "+outDir) {
		t.Fatalf("python output = %q", out)
	}
	models := string(mustRead(t, filepath.Join(outDir, "models.py")))
	pyClient := string(mustRead(t, filepath.Join(outDir, "client.py")))
	for _, want := range []string{"class Pet", "class Error", "Pets = list[Pet]"} {
		if !strings.Contains(models, want) {
			t.Fatalf("models.py missing %q\n%s", want, models)
		}
	}
	for _, want := range []string{"def list_pets(", "def show_pet_by_id(", `/pets/{quote(_param(pet_id), safe="")}`} {
		if !strings.Contains(pyClient, want) {
			t.Fatalf("client.py missing %q\n%s", want, pyClient)
		}
	}

	out, err = runBin(t, bin, root, "generate", "--spec", spec, "--output", outDir, "--language", "nope")
	if err == nil {
		t.Fatal("expected unsupported language to exit 1")
	}
	var pyErr *exec.ExitError
	if !errors.As(err, &pyErr) || pyErr.ExitCode() != 1 || !strings.Contains(out, "unsupported language") {
		t.Fatalf("nope: %v\n%s", err, out)
	}

	out, err = runBin(t, bin, root, "generate", "--spec", "bad.yaml", "--output", "out", "--language", "typescript")
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

func TestMockSmoke(t *testing.T) {
	t.Chdir(repoRoot(t))
	port := freePort(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"mock", "--spec", "testdata/petstore.yaml", "--port", strconv.Itoa(port)})

	done := make(chan error, 1)
	go func() {
		done <- root.ExecuteContext(ctx)
	}()

	url := fmt.Sprintf("http://127.0.0.1:%d/pets", port)
	var res *http.Response
	var err error
	deadline := time.Now().Add(3 * time.Second)
	for {
		res, err = http.Get(url)
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("server did not start: %v\n%s", err, out.String())
		}
		time.Sleep(10 * time.Millisecond)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", res.StatusCode)
	}
	var body []map[string]any
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body) != 1 || body[0]["name"] != "string" {
		t.Fatalf("body = %#v", body)
	}
	got := out.String()
	running := fmt.Sprintf("Mock server running on http://localhost:%d", port)
	if !strings.Contains(got, running) || !strings.Contains(got, "Loaded 3 endpoints from testdata/petstore.yaml") {
		t.Fatalf("output = %q", got)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("shutdown timed out")
	}
}

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
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
