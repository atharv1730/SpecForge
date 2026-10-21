package cmd

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/atharv1730/SpecForge/internal/mock"
	"github.com/spf13/cobra"
)

func newMockCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mock",
		Short: "Serve a mock API from an OpenAPI spec",
		RunE:  runMock,
	}
	cmd.Flags().String("spec", "", "path to the OpenAPI file (required)")
	cmd.Flags().Int("port", 8080, "port to listen on")
	markRequired(cmd, "spec")
	return cmd
}

func runMock(cmd *cobra.Command, args []string) error {
	api, err := loadAPI(cmd)
	if err != nil {
		return err
	}
	port, err := cmd.Flags().GetInt("port")
	if err != nil {
		return err
	}
	srv := mock.NewServer(api, port)
	ln, err := net.Listen("tcp", srv.Addr)
	if err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Mock server running on http://localhost:%d\n", port)

	serveErr := make(chan error, 1)
	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
		}
	}()

	parent := cmd.Context()
	if parent == nil {
		parent = context.Background()
	}
	ctx, stop := signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
	defer stop()

	select {
	case err := <-serveErr:
		return err
	case <-ctx.Done():
		shut, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(shut)
	}
}
