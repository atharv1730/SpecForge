package mock

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/atharv1730/SpecForge/internal/ir"
)

func TestGenerateExample(t *testing.T) {
	prim := func(name string) *ir.TypeRef {
		return &ir.TypeRef{Kind: "primitive", Name: name}
	}
	if GenerateExample(nil) != nil {
		t.Fatal("nil schema")
	}
	if GenerateExample(prim("string")) != "string" {
		t.Fatal("string")
	}
	if GenerateExample(prim("integer")) != 0 {
		t.Fatal("integer")
	}
	if GenerateExample(prim("number")) != 0.0 {
		t.Fatal("number")
	}
	if GenerateExample(prim("boolean")) != false {
		t.Fatal("boolean")
	}
	enum := &ir.TypeRef{Kind: "enum", Enum: []string{"ready", "gone"}}
	if GenerateExample(enum) != "ready" {
		t.Fatal("enum")
	}

	item := ir.TypeRef{
		Kind: "object",
		Properties: []ir.Property{
			{Name: "id", Type: ir.TypeRef{Kind: "primitive", Name: "integer"}},
		},
	}
	got, ok := GenerateExample(&ir.TypeRef{Kind: "array", Items: &item}).([]any)
	if !ok || len(got) != 1 {
		t.Fatalf("array = %#v", got)
	}
	obj, ok := got[0].(map[string]any)
	if !ok || obj["id"] != 0 {
		t.Fatalf("item = %#v", got[0])
	}
}

func TestServer(t *testing.T) {
	item := ir.TypeRef{
		Kind: "object",
		Name: "Item",
		Properties: []ir.Property{
			{Name: "id", Type: ir.TypeRef{Kind: "primitive", Name: "integer"}},
			{Name: "name", Type: ir.TypeRef{Kind: "primitive", Name: "string"}},
		},
	}
	list := ir.TypeRef{Kind: "array", Items: &item}
	api := &ir.API{Endpoints: []ir.Endpoint{
		{
			Method: http.MethodGet,
			Path:   "/items",
			Responses: []ir.Response{
				{StatusCode: http.StatusNoContent},
				{StatusCode: http.StatusOK, ContentType: "application/json", Schema: &list},
			},
		},
		{
			Method: http.MethodPost,
			Path:   "/items",
			Responses: []ir.Response{{
				StatusCode:  http.StatusCreated,
				ContentType: "application/json",
				Schema: &ir.TypeRef{
					Kind: "object",
					Properties: []ir.Property{
						{Name: "ok", Type: ir.TypeRef{Kind: "primitive", Name: "boolean"}},
						{Name: "status", Type: ir.TypeRef{Kind: "enum", Enum: []string{"new", "old"}}},
					},
				},
			}},
		},
	}}

	ts := httptest.NewServer(NewServer(api, 0).Handler)
	t.Cleanup(ts.Close)

	res := get(t, ts.URL+"/items")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", res.StatusCode)
	}
	if res.Header.Get("Content-Type") != "application/json" {
		t.Fatalf("content-type = %q", res.Header.Get("Content-Type"))
	}
	var listed []map[string]any
	decode(t, res, &listed)
	if len(listed) != 1 || listed[0]["name"] != "string" || listed[0]["id"].(float64) != 0 {
		t.Fatalf("body = %#v", listed)
	}

	req, err := http.NewRequest(http.MethodPost, ts.URL+"/items", nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d", res.StatusCode)
	}
	var created map[string]any
	decode(t, res, &created)
	if created["ok"] != false || created["status"] != "new" {
		t.Fatalf("body = %#v", created)
	}

	res = get(t, ts.URL+"/missing")
	res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d", res.StatusCode)
	}
}

func get(t *testing.T, url string) *http.Response {
	t.Helper()
	res, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func decode(t *testing.T, res *http.Response, dest any) {
	t.Helper()
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(body, dest); err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}
}
