// Package gen is the contract every language generator implements.
// The CLI looks up a generator by name. A language registers itself from init.
package gen

import (
	"fmt"

	"github.com/atharv1730/SpecForge/internal/ir"
)

// Generator writes one language's client for api into outputDir.
type Generator interface {
	Generate(api *ir.API, outputDir string) error
}

var registry = map[string]Generator{}

// Register adds g under language, replacing any generator already stored there.
func Register(language string, g Generator) {
	registry[language] = g
}

// Get returns the generator registered for language.
func Get(language string) (Generator, error) {
	g, ok := registry[language]
	if !ok {
		return nil, fmt.Errorf("unsupported language %q", language)
	}
	return g, nil
}
