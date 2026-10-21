package mock

import "github.com/atharv1730/SpecForge/internal/ir"

// GenerateExample returns a JSON-ready value that satisfies schema.
// A nil schema has no body, so the example is nil.
func GenerateExample(schema *ir.TypeRef) any {
	if schema == nil {
		return nil
	}
	return example(*schema, map[string]bool{})
}

func example(t ir.TypeRef, seen map[string]bool) any {
	switch t.Kind {
	case "enum":
		if len(t.Enum) > 0 {
			return t.Enum[0]
		}
		return nil
	case "array":
		if t.Items == nil {
			return []any{nil}
		}
		return []any{example(*t.Items, seen)}
	case "object":
		if t.Name != "" {
			if seen[t.Name] {
				return map[string]any{}
			}
			seen[t.Name] = true
			defer delete(seen, t.Name)
		}
		out := make(map[string]any, len(t.Properties))
		for _, prop := range t.Properties {
			out[prop.Name] = example(prop.Type, seen)
		}
		return out
	default:
		switch t.Name {
		case "integer":
			return 0
		case "number":
			return 0.0
		case "boolean":
			return false
		default:
			return "string"
		}
	}
}
