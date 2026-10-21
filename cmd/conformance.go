package cmd

import (
	"fmt"
	"os"

	"github.com/atharv1730/SpecForge/internal/conformance"
	"github.com/atharv1730/SpecForge/internal/gen"
	_ "github.com/atharv1730/SpecForge/internal/gen/python"
	_ "github.com/atharv1730/SpecForge/internal/gen/typescript"
	"github.com/spf13/cobra"
)

func newConformanceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "conformance",
		Short: "Check every endpoint against the mock server",
		RunE:  runConformance,
	}
	cmd.Flags().String("spec", "", "path to the OpenAPI file (required)")
	cmd.Flags().String("language", "", "client language: typescript or python (required)")
	cmd.Flags().Int("port", 8080, "port to listen on")
	markRequired(cmd, "spec", "language")
	return cmd
}

func runConformance(cmd *cobra.Command, args []string) error {
	api, err := loadAPI(cmd)
	if err != nil {
		return err
	}
	language, err := cmd.Flags().GetString("language")
	if err != nil {
		return err
	}
	port, err := cmd.Flags().GetInt("port")
	if err != nil {
		return err
	}
	generator, err := gen.Get(language)
	if err != nil {
		return err
	}
	dir, err := os.MkdirTemp("", "specforge-conformance-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	if err := generator.Generate(api, dir); err != nil {
		return err
	}
	report, err := conformance.Run(api, dir, port)
	if err != nil {
		return err
	}
	out := cmd.OutOrStdout()
	for _, f := range report.Failures {
		fmt.Fprintf(out, "FAIL %s %s %s: %s\n", f.Method, f.Path, f.OperationID, f.Message)
	}
	fmt.Fprintf(out, "%d passed, %d failed\n", report.Passed, report.Failed)
	if report.Failed > 0 {
		cmd.SilenceUsage = true
		cmd.SilenceErrors = true
		return fmt.Errorf("conformance: %d failed", report.Failed)
	}
	return nil
}
