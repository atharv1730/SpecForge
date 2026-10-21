package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newMockCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mock",
		Short: "Serve a mock API from an OpenAPI spec",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintln(cmd.OutOrStdout(), "mock: not implemented")
			return nil
		},
	}
	cmd.Flags().String("spec", "", "path to the OpenAPI file (required)")
	cmd.Flags().Int("port", 8080, "port to listen on")
	markRequired(cmd, "spec")
	return cmd
}
