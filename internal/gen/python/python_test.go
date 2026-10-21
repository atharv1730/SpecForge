package python

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/atharv1730/SpecForge/internal/gen"
	"github.com/atharv1730/SpecForge/internal/ir"
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

	g, err := gen.Get("python")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := g.Generate(api, dir); err != nil {
		t.Fatal(err)
	}

	models := string(mustRead(t, filepath.Join(dir, "models.py")))
	client := string(mustRead(t, filepath.Join(dir, "client.py")))

	for _, want := range []string{
		"@dataclass\nclass Pet:",
		"id: int",
		"name: str",
		"tag: str | None = None",
		"@dataclass\nclass Error:",
		"code: int",
		"message: str",
		"Pets = list[Pet]",
	} {
		if !strings.Contains(models, want) {
			t.Fatalf("models.py missing %q\n%s", want, models)
		}
	}

	for _, want := range []string{
		"class Client:",
		"def list_pets(self, limit: int | None = None) -> Pets:",
		"def create_pets(self) -> None:",
		"def show_pet_by_id(self, pet_id: str) -> Pet:",
		`f'{root}/pets/{quote(_param(pet_id), safe="")}'`,
		`params["limit"] = _param(limit)`,
		`method="GET"`,
		`method="POST"`,
		`from urllib.request import Request, urlopen`,
	} {
		if !strings.Contains(client, want) {
			t.Fatalf("client.py missing %q\n%s", want, client)
		}
	}

	checkPy(t, dir)
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
        - name: X-Trace
          in: header
          required: true
          schema:
            type: string
      responses:
        "200":
          description: ok
          content:
            application/json:
              schema:
                type: string
                enum: [ready, gone]
    post:
      operationId: create_item
      parameters:
        - name: item_id
          in: path
          required: true
          schema:
            type: string
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: "#/components/schemas/Item"
      responses:
        "200":
          description: ok
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/Item"
components:
  schemas:
    Item:
      type: object
      required: [z_name]
      properties:
        a_nick:
          type: string
        count:
          type: integer
        flag:
          type: boolean
        score:
          type: number
        status:
          type: string
          enum: [ready, gone]
        z_name:
          type: string
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
	models := string(mustRead(t, filepath.Join(dir, "models.py")))
	client := string(mustRead(t, filepath.Join(dir, "client.py")))
	for _, want := range []string{
		"@dataclass\nclass Item:",
		"z_name: str\n    a_nick: str | None = None",
		"count: int | None = None",
		"flag: bool | None = None",
		"score: float | None = None",
		`status: Literal["ready", "gone"] | None = None`,
	} {
		if !strings.Contains(models, want) {
			t.Fatalf("models.py missing %q\n%s", want, models)
		}
	}
	for _, want := range []string{
		`def get_item(self, item_id: str, x_trace: str, page_size: int | None = None) -> Literal["ready", "gone"]:`,
		`f'{root}/items/{quote(_param(item_id), safe="")}'`,
		`params["page-size"] = _param(page_size)`,
		`headers["X-Trace"] = _param(x_trace)`,
		"def create_item(self, item_id: str, body: Item) -> Item:",
		"data = _body(body)",
		`headers["content-type"] = "application/json"`,
	} {
		if !strings.Contains(client, want) {
			t.Fatalf("client.py missing %q\n%s", want, client)
		}
	}
	checkPy(t, dir)
}

func TestEmptyObject(t *testing.T) {
	api := &ir.API{Endpoints: []ir.Endpoint{{
		Method:      "GET",
		Path:        "/empty",
		OperationID: "getEmpty",
		Responses: []ir.Response{{
			StatusCode: 200,
			Schema:     &ir.TypeRef{Name: "Empty", Kind: "object"},
		}},
	}}}
	dir := t.TempDir()
	if err := (Generator{}).Generate(api, dir); err != nil {
		t.Fatal(err)
	}
	models := string(mustRead(t, filepath.Join(dir, "models.py")))
	if !strings.Contains(models, "class Empty:\n    pass") {
		t.Fatalf("models.py = %s", models)
	}
	checkPy(t, dir)
}

func TestGenerateRejectsBadInput(t *testing.T) {
	if err := (Generator{}).Generate(nil, t.TempDir()); err == nil {
		t.Fatal("expected nil api to fail")
	}
	if err := (Generator{}).Generate(&ir.API{}, ""); err == nil {
		t.Fatal("expected empty output directory to fail")
	}
}

func TestSnakeCase(t *testing.T) {
	cases := map[string]string{
		"listPets":    "list_pets",
		"showPetById": "show_pet_by_id",
		"get_item":    "get_item",
		"page-size":   "page_size",
		"X-Trace":     "x_trace",
		"class":       "class_",
		"HTTPServer":  "http_server",
	}
	for in, want := range cases {
		if got := snakeCase(in); got != want {
			t.Errorf("snakeCase(%q) = %q, want %q", in, got, want)
		}
	}
}

func checkPy(t *testing.T, dir string) {
	t.Helper()
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Log("python3 not installed; skipped syntax check")
		return
	}
	cmd := exec.Command(python, "-c", "import client")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("python: %v\n%s", err, out)
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
