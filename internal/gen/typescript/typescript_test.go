package typescript

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/atharv1730/SpecForge/internal/gen"
	"github.com/atharv1730/SpecForge/internal/loader"
)

func TestGeneratePetstore(t *testing.T) {
	t.Chdir(repoRoot(t))
	doc, err := loader.Load("testdata/petstore.yaml")
	if err != nil {
		t.Fatal(err)
	}
	api, err := loader.Transform(doc)
	if err != nil {
		t.Fatal(err)
	}

	g, err := gen.Get("typescript")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := g.Generate(api, dir); err != nil {
		t.Fatal(err)
	}

	types := string(mustRead(t, filepath.Join(dir, "types.ts")))
	client := string(mustRead(t, filepath.Join(dir, "client.ts")))

	for _, want := range []string{
		"export interface Pet {",
		"id: number;",
		"name: string;",
		"tag?: string;",
		"export interface Error {",
		"code: number;",
		"message: string;",
		"export type Pets = Pet[];",
	} {
		if !strings.Contains(types, want) {
			t.Fatalf("types.ts missing %q\n%s", want, types)
		}
	}

	for _, want := range []string{
		"export class Client",
		"async listPets(limit?: number): Promise<Pets>",
		"async createPets(): Promise<void>",
		"async showPetById(petId: string): Promise<Pet>",
		"/pets/${encodeURIComponent(petId)}",
		`params.set("limit", String(limit))`,
		`method: "GET"`,
		`method: "POST"`,
	} {
		if !strings.Contains(client, want) {
			t.Fatalf("client.ts missing %q\n%s", want, client)
		}
	}

	checkTS(t, dir)
}

func TestGenerateSnakeCaseAndEnum(t *testing.T) {
	path := filepath.Join(t.TempDir(), "items.yaml")
	const spec = `
openapi: "3.0.3"
info:
  title: Items
  version: "1.0.0"
paths:
  /items/{item_id}:
    get:
      operationId: get_item
      parameters:
        - name: item_id
          in: path
          required: true
          schema:
            type: string
        - name: page-size
          in: query
          schema:
            type: integer
      responses:
        "200":
          description: ok
          content:
            application/json:
              schema:
                type: string
                enum: [ready, gone]
`
	if err := os.WriteFile(path, []byte(spec), 0o644); err != nil {
		t.Fatal(err)
	}
	doc, err := loader.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	api, err := loader.Transform(doc)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := (Generator{}).Generate(api, dir); err != nil {
		t.Fatal(err)
	}
	client := string(mustRead(t, filepath.Join(dir, "client.ts")))
	wantSig := `async getItem(itemId: string, pageSize?: number): Promise<"ready" | "gone">`
	for _, want := range []string{
		wantSig,
		"/items/${encodeURIComponent(itemId)}",
		`params.set("page-size", String(pageSize))`,
	} {
		if !strings.Contains(client, want) {
			t.Fatalf("client.ts missing %q\n%s", want, client)
		}
	}
	checkTS(t, dir)
}

func checkTS(t *testing.T, dir string) {
	t.Helper()
	tsc, err := exec.LookPath("tsc")
	if err != nil {
		t.Log("tsc not installed; skipped syntax check")
		return
	}
	cfg := `{
  "compilerOptions": {
    "strict": true,
    "target": "ES2020",
    "module": "ESNext",
    "moduleResolution": "bundler",
    "lib": ["ES2020", "DOM"],
    "noEmit": true,
    "skipLibCheck": true
  },
  "include": ["*.ts"]
}
`
	if err := os.WriteFile(filepath.Join(dir, "tsconfig.json"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(tsc, "--noEmit", "-p", dir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("tsc: %v\n%s", err, out)
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "..")
}
