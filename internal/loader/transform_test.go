package loader

import (
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/atharv1730/SpecForge/internal/ir"
)

func TestTransformPetstore(t *testing.T) {
	t.Chdir(repoRoot(t))

	doc, err := Load("testdata/petstore.yaml")
	if err != nil {
		t.Fatal(err)
	}
	api, err := Transform(doc)
	if err != nil {
		t.Fatal(err)
	}
	if api.Name != "Swagger Petstore" || api.Version != "1.0.0" || api.BaseURL != "http://petstore.swagger.io/v1" {
		t.Fatalf("api meta = %+v", api)
	}
	if len(api.Endpoints) != 3 {
		t.Fatalf("endpoints = %d, want 3", len(api.Endpoints))
	}

	list := findEndpoint(t, api, "listPets")
	if list.Method != "GET" || list.Path != "/pets" || list.OperationID != "listPets" {
		t.Fatalf("listPets = %+v", list)
	}
	if len(list.Parameters) != 1 {
		t.Fatalf("listPets parameters = %+v", list.Parameters)
	}
	limit := list.Parameters[0]
	wantLimit := ir.TypeRef{Name: "integer", Kind: "primitive"}
	if limit.Name != "limit" || limit.In != "query" || limit.Required || !reflect.DeepEqual(limit.Schema, wantLimit) {
		t.Fatalf("limit = %+v", limit)
	}

	got := responseSchema(t, list, 200)
	id := ir.Property{Name: "id", Required: true, Type: ir.TypeRef{Name: "integer", Kind: "primitive"}}
	name := ir.Property{Name: "name", Required: true, Type: ir.TypeRef{Name: "string", Kind: "primitive"}}
	tag := ir.Property{Name: "tag", Type: ir.TypeRef{Name: "string", Kind: "primitive"}}
	pet := ir.TypeRef{Name: "Pet", Kind: "object", Properties: []ir.Property{id, name, tag}}
	want := ir.TypeRef{Name: "Pets", Kind: "array", Items: &pet}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("listPets 200 schema = %#v\nwant %#v", got, want)
	}

	show := findEndpoint(t, api, "showPetById")
	if len(show.Parameters) != 1 || show.Parameters[0].Name != "petId" || show.Parameters[0].In != "path" || !show.Parameters[0].Required {
		t.Fatalf("showPetById parameters = %+v", show.Parameters)
	}
	if show.Parameters[0].Schema.Kind != "primitive" || show.Parameters[0].Schema.Name != "string" {
		t.Fatalf("petId schema = %+v", show.Parameters[0].Schema)
	}
}

func TestTransformRequestBodyAndEnum(t *testing.T) {
	path := filepath.Join(t.TempDir(), "body.yaml")
	const spec = `
openapi: "3.0.3"
info:
  title: Body
  version: "1.0.0"
paths:
  /things:
    post:
      operationId: createThing
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: "#/components/schemas/Thing"
      responses:
        "200":
          description: ok
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/Status"
components:
  schemas:
    Thing:
      type: object
      required: [name]
      properties:
        name:
          type: string
        status:
          type: string
          enum: [new, old]
    Status:
      type: boolean
`
	if err := os.WriteFile(path, []byte(spec), 0o644); err != nil {
		t.Fatal(err)
	}
	doc, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	api, err := Transform(doc)
	if err != nil {
		t.Fatal(err)
	}
	if api.BaseURL != "" {
		t.Fatalf("base URL = %q", api.BaseURL)
	}
	ep := findEndpoint(t, api, "createThing")
	if ep.RequestBody == nil || !ep.RequestBody.Required || ep.RequestBody.ContentType != "application/json" {
		t.Fatalf("request body = %+v", ep.RequestBody)
	}
	status := ir.Property{Name: "status", Type: ir.TypeRef{Name: "string", Kind: "enum", Enum: []string{"new", "old"}}}
	name := ir.Property{Name: "name", Required: true, Type: ir.TypeRef{Name: "string", Kind: "primitive"}}
	want := ir.TypeRef{Name: "Thing", Kind: "object", Properties: []ir.Property{name, status}}
	if !reflect.DeepEqual(ep.RequestBody.Schema, want) {
		t.Fatalf("request schema = %#v\nwant %#v", ep.RequestBody.Schema, want)
	}
	got := responseSchema(t, ep, 200)
	if got.Kind != "primitive" || got.Name != "boolean" {
		t.Fatalf("status schema = %#v", got)
	}
}

func TestTransformSkipsMissingOperationID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "skip.yaml")
	const spec = `
openapi: "3.0.3"
info:
  title: Skip
  version: "1.0.0"
paths:
  /kept:
    get:
      operationId: kept
      responses:
        "204":
          description: empty
  /dropped:
    post:
      summary: no id
      responses:
        "204":
          description: empty
`
	if err := os.WriteFile(path, []byte(spec), 0o644); err != nil {
		t.Fatal(err)
	}
	doc, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w
	api, transformErr := Transform(doc)
	w.Close()
	os.Stderr = old
	warning, _ := io.ReadAll(r)
	if transformErr != nil {
		t.Fatal(transformErr)
	}
	if len(api.Endpoints) != 1 || api.Endpoints[0].OperationID != "kept" {
		t.Fatalf("endpoints = %+v", api.Endpoints)
	}
	if !strings.Contains(string(warning), "no operationId") || !strings.Contains(string(warning), "/dropped") {
		t.Fatalf("warning = %q", warning)
	}
	for _, ep := range api.Endpoints {
		if ep.Path == "/dropped" {
			t.Fatal("operation without operationId was kept")
		}
	}
}

func findEndpoint(t *testing.T, api *ir.API, operationID string) ir.Endpoint {
	t.Helper()
	for _, ep := range api.Endpoints {
		if ep.OperationID == operationID {
			return ep
		}
	}
	t.Fatalf("missing operation %s", operationID)
	return ir.Endpoint{}
}

func responseSchema(t *testing.T, ep ir.Endpoint, status int) ir.TypeRef {
	t.Helper()
	for _, resp := range ep.Responses {
		if resp.StatusCode == status {
			if resp.Schema == nil {
				t.Fatalf("status %d has no schema", status)
			}
			return *resp.Schema
		}
	}
	t.Fatalf("missing status %d in %+v", status, ep.Responses)
	return ir.TypeRef{}
}
