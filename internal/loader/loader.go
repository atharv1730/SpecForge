package loader

import (
	"context"
	"fmt"

	"github.com/getkin/kin-openapi/openapi3"
)

// Load reads an OpenAPI 3.x document from path, parses it, and validates it.
func Load(path string) (*openapi3.T, error) {
	doc, err := openapi3.NewLoader().LoadFromFile(path)
	if err != nil {
		return nil, fmt.Errorf("load spec %q: %w", path, err)
	}
	if err := doc.Validate(context.Background()); err != nil {
		return nil, fmt.Errorf("validate spec %q: %w", path, err)
	}
	return doc, nil
}
