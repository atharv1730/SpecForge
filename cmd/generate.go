package cmd

import "github.com/spf13/cobra"

func newGenerateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "generate",
		Short: "Generate a typed API client from an OpenAPI spec",
		RunE: func(cmd *cobra.Command, args []string) error {
			return loadSpec(cmd)
		},
	}
	cmd.Flags().String("spec", "", "path to the OpenAPI file (required)")
	cmd.Flags().String("output", "", "output directory (required)")
	cmd.Flags().String("language", "", "client language: typescript or python (required)")
	markRequired(cmd, "spec", "output", "language")
	return cmd
}
