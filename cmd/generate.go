package cmd

import (
	"fmt"

	"github.com/atharv1730/SpecForge/internal/gen"
	_ "github.com/atharv1730/SpecForge/internal/gen/python"
	_ "github.com/atharv1730/SpecForge/internal/gen/typescript"
	"github.com/spf13/cobra"
)

func newGenerateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "generate",
		Short: "Generate a typed API client from an OpenAPI spec",
		RunE:  runGenerate,
	}
	cmd.Flags().String("spec", "", "path to the OpenAPI file (required)")
	cmd.Flags().String("output", "", "output directory (required)")
	cmd.Flags().String("language", "", "client language: typescript or python (required)")
	markRequired(cmd, "spec", "output", "language")
	return cmd
}

func runGenerate(cmd *cobra.Command, args []string) error {
	api, err := loadAPI(cmd)
	if err != nil {
		return err
	}
	language, err := cmd.Flags().GetString("language")
	if err != nil {
		return err
	}
	outputDir, err := cmd.Flags().GetString("output")
	if err != nil {
		return err
	}
	generator, err := gen.Get(language)
	if err != nil {
		return err
	}
	if err := generator.Generate(api, outputDir); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Generated %s client in %s\n", languageName(language), outputDir)
	return nil
}

func languageName(language string) string {
	switch language {
	case "typescript":
		return "TypeScript"
	case "python":
		return "Python"
	default:
		return language
	}
}
