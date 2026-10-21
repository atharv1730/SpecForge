package loader

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/atharv1730/SpecForge/internal/ir"
	"github.com/getkin/kin-openapi/openapi3"
)

// Transform walks a loaded, validated OpenAPI document and returns the IR.
// kin-openapi resolves $ref during validation, so schemas are read from Value.
func Transform(doc *openapi3.T) (*ir.API, error) {
	if doc == nil {
		return nil, fmt.Errorf("cannot transform a nil OpenAPI document. Load a spec before transforming")
	}
	api := &ir.API{}
	if doc.Info != nil {
		api.Name = doc.Info.Title
		api.Version = doc.Info.Version
	}
	if len(doc.Servers) > 0 && doc.Servers[0] != nil {
		api.BaseURL = doc.Servers[0].URL
	}
	if doc.Paths == nil {
		return api, nil
	}

	paths := doc.Paths.Keys()
	sort.Strings(paths)
	for _, path := range paths {
		item := doc.Paths.Value(path)
		if item == nil {
			continue
		}
		ops := item.Operations()
		methods := make([]string, 0, len(ops))
		for method := range ops {
			methods = append(methods, method)
		}
		sort.Strings(methods)
		for _, method := range methods {
			op := ops[method]
			if op == nil {
				continue
			}
			if op.OperationID == "" {
				fmt.Fprintf(os.Stderr, "warning: skipping %s %s: no operationId. Add an operationId to include this operation\n", method, path)
				continue
			}
			ep, err := toEndpoint(method, path, item, op)
			if err != nil {
				return nil, fmt.Errorf("cannot build the IR for %s %s: %w", method, path, err)
			}
			api.Endpoints = append(api.Endpoints, ep)
		}
	}
	return api, nil
}

func toEndpoint(method, path string, item *openapi3.PathItem, op *openapi3.Operation) (ir.Endpoint, error) {
	params, err := toParameters(item, op)
	if err != nil {
		return ir.Endpoint{}, err
	}
	body, err := toRequestBody(op.RequestBody)
	if err != nil {
		return ir.Endpoint{}, err
	}
	responses, err := toResponses(op.Responses)
	if err != nil {
		return ir.Endpoint{}, err
	}
	return ir.Endpoint{
		Method:      method,
		Path:        path,
		OperationID: op.OperationID,
		Summary:     op.Summary,
		Parameters:  params,
		RequestBody: body,
		Responses:   responses,
	}, nil
}

func toParameters(item *openapi3.PathItem, op *openapi3.Operation) ([]ir.Parameter, error) {
	var src openapi3.Parameters
	if item != nil {
		src = append(src, item.Parameters...)
	}
	if op != nil {
		src = append(src, op.Parameters...)
	}
	index := make(map[string]int)
	var out []ir.Parameter
	for _, pref := range src {
		if pref == nil || pref.Value == nil {
			if pref != nil && pref.Ref != "" {
				return nil, fmt.Errorf("unresolved parameter $ref %q. Define that component in the spec or replace the $ref with an inline schema", pref.Ref)
			}
			continue
		}
		p := pref.Value
		schema, err := toTypeRef(p.Schema, map[string]bool{})
		if err != nil {
			return nil, fmt.Errorf("parameter %q: %w", p.Name, err)
		}
		param := ir.Parameter{Name: p.Name, In: p.In, Required: p.Required, Schema: schema}
		key := p.In + "\x00" + p.Name
		if i, ok := index[key]; ok {
			out[i] = param
			continue
		}
		index[key] = len(out)
		out = append(out, param)
	}
	return out, nil
}

func toRequestBody(ref *openapi3.RequestBodyRef) (*ir.RequestBody, error) {
	if ref == nil {
		return nil, nil
	}
	if ref.Value == nil {
		if ref.Ref != "" {
			return nil, fmt.Errorf("unresolved request body $ref %q. Define that component in the spec or inline the body", ref.Ref)
		}
		return nil, nil
	}
	ct, schema, err := contentSchema(ref.Value.Content)
	if err != nil {
		return nil, fmt.Errorf("request body: %w", err)
	}
	body := &ir.RequestBody{Required: ref.Value.Required, ContentType: ct}
	if schema != nil {
		body.Schema = *schema
	}
	return body, nil
}

func toResponses(responses *openapi3.Responses) ([]ir.Response, error) {
	if responses == nil {
		return nil, nil
	}
	keys := responses.Keys()
	sort.Strings(keys)
	out := make([]ir.Response, 0, len(keys))
	for _, key := range keys {
		ref := responses.Value(key)
		if ref == nil || ref.Value == nil {
			continue
		}
		code, err := strconv.Atoi(key)
		if err != nil {
			code = 0
		}
		ct, schema, err := contentSchema(ref.Value.Content)
		if err != nil {
			return nil, fmt.Errorf("response %s: %w", key, err)
		}
		out = append(out, ir.Response{StatusCode: code, ContentType: ct, Schema: schema})
	}
	return out, nil
}

// contentSchema prefers application/json, then the first media type by name.
// The schema is nil when that media type has no body schema.
func contentSchema(content openapi3.Content) (string, *ir.TypeRef, error) {
	if len(content) == 0 {
		return "", nil, nil
	}
	ct := "application/json"
	mt, ok := content[ct]
	if !ok {
		names := make([]string, 0, len(content))
		for name := range content {
			names = append(names, name)
		}
		sort.Strings(names)
		ct = names[0]
		mt = content[ct]
	}
	if mt == nil || mt.Schema == nil {
		return ct, nil, nil
	}
	schema, err := toTypeRef(mt.Schema, map[string]bool{})
	if err != nil {
		return "", nil, fmt.Errorf("schema for %s: %w", ct, err)
	}
	return ct, &schema, nil
}

func toTypeRef(ref *openapi3.SchemaRef, stack map[string]bool) (ir.TypeRef, error) {
	if ref == nil || ref.Value == nil {
		if ref != nil && ref.Ref != "" {
			return ir.TypeRef{}, fmt.Errorf("unresolved schema $ref %q. Define that component in the spec or inline the schema", ref.Ref)
		}
		return ir.TypeRef{}, nil
	}
	name := refName(ref.Ref)
	if ref.Ref != "" {
		if stack[ref.Ref] {
			return ir.TypeRef{Name: name, Kind: "object"}, nil
		}
		stack[ref.Ref] = true
		defer delete(stack, ref.Ref)
	}

	s := ref.Value
	if len(s.Enum) > 0 {
		vals := make([]string, len(s.Enum))
		for i, v := range s.Enum {
			vals[i] = fmt.Sprint(v)
		}
		if name == "" {
			name = schemaType(s.Type)
		}
		return ir.TypeRef{Name: name, Kind: "enum", Enum: vals}, nil
	}

	typ := schemaType(s.Type)
	switch {
	case typ == "array" || (typ == "" && s.Items != nil):
		var items *ir.TypeRef
		if s.Items != nil {
			it, err := toTypeRef(s.Items, stack)
			if err != nil {
				return ir.TypeRef{}, fmt.Errorf("array items: %w", err)
			}
			items = &it
		}
		return ir.TypeRef{Name: name, Kind: "array", Items: items}, nil
	case typ == "object" || (typ == "" && len(s.Properties) > 0):
		props, err := toProperties(s, stack)
		if err != nil {
			return ir.TypeRef{}, err
		}
		return ir.TypeRef{Name: name, Kind: "object", Properties: props}, nil
	default:
		if typ == "" {
			typ = name
		}
		return ir.TypeRef{Name: typ, Kind: "primitive"}, nil
	}
}

func toProperties(s *openapi3.Schema, stack map[string]bool) ([]ir.Property, error) {
	if len(s.Properties) == 0 {
		return nil, nil
	}
	names := make([]string, 0, len(s.Properties))
	for name := range s.Properties {
		names = append(names, name)
	}
	sort.Strings(names)
	required := make(map[string]bool, len(s.Required))
	for _, name := range s.Required {
		required[name] = true
	}
	out := make([]ir.Property, 0, len(names))
	for _, name := range names {
		t, err := toTypeRef(s.Properties[name], stack)
		if err != nil {
			return nil, fmt.Errorf("property %q: %w", name, err)
		}
		out = append(out, ir.Property{Name: name, Type: t, Required: required[name]})
	}
	return out, nil
}

func schemaType(types *openapi3.Types) string {
	if types == nil {
		return ""
	}
	for _, typ := range types.Slice() {
		if typ != "null" {
			return typ
		}
	}
	return ""
}

func refName(ref string) string {
	if ref == "" {
		return ""
	}
	if i := strings.LastIndex(ref, "/"); i >= 0 && i < len(ref)-1 {
		return ref[i+1:]
	}
	return ref
}
