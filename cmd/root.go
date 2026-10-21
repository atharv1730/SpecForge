// Package cmd implements the specforge command line.
package cmd

import (
	"fmt"
	"os"

	"github.com/atharv1730/SpecForge/internal/ir"
	"github.com/atharv1730/SpecForge/internal/loader"
	"github.com/spf13/cobra"
)

// version is reported by --version.
const version = "0.1.0"

// Execute runs the specforge command line and exits with status 1 on error.
func Execute() {
	if err := newRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "specforge",
		Short: "Generate typed API clients and a mock server from an OpenAPI spec",
		Long: `SpecForge reads an OpenAPI 3.x spec into a language-agnostic intermediate
representation and generates typed clients and a mock server from that single IR.`,
		Version:           version,
		CompletionOptions: cobra.CompletionOptions{DisableDefaultCmd: true},
	}
	root.AddCommand(newGenerateCmd(), newMockCmd())
	return root
}

// loadAPI reads the spec, builds the IR, and prints how many endpoints were found.
func loadAPI(cmd *cobra.Command) (*ir.API, error) {
	specPath, err := cmd.Flags().GetString("spec")
	if err != nil {
		return nil, err
	}
	doc, err := loader.Load(specPath)
	if err != nil {
		return nil, err
	}
	api, err := loader.Transform(doc)
	if err != nil {
		return nil, err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Loaded %d endpoints from %s\n", len(api.Endpoints), specPath)
	return api, nil
}

func markRequired(cmd *cobra.Command, names ...string) {
	for _, name := range names {
		if err := cmd.MarkFlagRequired(name); err != nil {
			panic(err)
		}
	}
}
