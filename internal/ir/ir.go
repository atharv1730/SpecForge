// Package ir is the language-agnostic model produced by one spec load.
// Generators and the mock server walk these types. This package does not
// import kin-openapi; the loader is the only place that type is allowed.
package ir

// API is the top-level container for one parsed specification.
// Name, Version, and BaseURL come from the document info and servers.
// Endpoints is every operation the generators and the mock server emit.
type API struct {
	Name      string
	Version   string
	BaseURL   string
	Endpoints []Endpoint
}

// Endpoint is one API operation: a method and path plus the inputs and outputs it declares.
type Endpoint struct {
	Method      string
	Path        string
	OperationID string
	Summary     string
	Parameters  []Parameter
	RequestBody *RequestBody
	Responses   []Response
}

// Parameter is one named input on an endpoint.
// In is where the value is sent: "query", "path", or "header".
type Parameter struct {
	Name     string
	In       string
	Required bool
	Schema   TypeRef
}

// RequestBody is the body an endpoint accepts, when it accepts one.
// ContentType is the media type the schema describes, such as "application/json".
type RequestBody struct {
	Required    bool
	ContentType string
	Schema      TypeRef
}

// Response is one status an endpoint may return.
// Schema is nil when that status has no body.
type Response struct {
	StatusCode  int
	ContentType string
	Schema      *TypeRef
}

// TypeRef is a resolved type. The loader follows $ref before building one,
// so a TypeRef never points back at the OpenAPI document.
// Kind is "primitive", "object", "array", or "enum".
// Properties is set for objects, Items for arrays, and Enum for enums.
type TypeRef struct {
	Name       string
	Kind       string
	Properties []Property
	Items      *TypeRef
	Enum       []string
	Required   bool
}

// Property is one field of an object TypeRef.
type Property struct {
	Name     string
	Type     TypeRef
	Required bool
}
