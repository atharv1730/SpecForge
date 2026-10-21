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
	root.PersistentFlags().Bool("verbose", false, "print detailed logs while loading, transforming, and generating")
	root.AddCommand(newGenerateCmd(), newMockCmd(), newConformanceCmd())
	return root
}

func logVerbose(cmd *cobra.Command, format string, args ...any) {
	if !verbose(cmd) {
		return
	}
	fmt.Fprintf(cmd.ErrOrStderr(), "verbose: "+format+"\n", args...)
}

func verbose(cmd *cobra.Command) bool {
	if cmd == nil {
		return false
	}
	on, err := cmd.Flags().GetBool("verbose")
	return err == nil && on
}

func stringFlag(cmd *cobra.Command, name string) (string, error) {
	v, err := cmd.Flags().GetString(name)
	if err != nil {
		return "", fmt.Errorf("cannot read --%s: %w. Re-run with --help", name, err)
	}
	return v, nil
}

func intFlag(cmd *cobra.Command, name string) (int, error) {
	v, err := cmd.Flags().GetInt(name)
	if err != nil {
		return 0, fmt.Errorf("cannot read --%s: %w. Re-run with --help", name, err)
	}
	return v, nil
}

// loadAPI reads the spec, builds the IR, and prints how many endpoints were found.
func loadAPI(cmd *cobra.Command) (*ir.API, error) {
	specPath, err := stringFlag(cmd, "spec")
	if err != nil {
		return nil, err
	}
	logVerbose(cmd, "loading spec %s", specPath)
	doc, err := loader.Load(specPath)
	if err != nil {
		return nil, err
	}
	title, version := "", ""
	if doc.Info != nil {
		title, version = doc.Info.Title, doc.Info.Version
	}
	logVerbose(cmd, "parsed %q version %s", title, version)
	logVerbose(cmd, "transforming %s into the IR", specPath)
	api, err := loader.Transform(doc)
	if err != nil {
		return nil, err
	}
	if api.BaseURL != "" {
		logVerbose(cmd, "base URL %s", api.BaseURL)
	}
	for _, ep := range api.Endpoints {
		logVerbose(cmd, "endpoint %s %s %s", ep.Method, ep.Path, ep.OperationID)
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
