// Package cli defines the hub's command line.
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/xonoko/robust-home-automation/internal/api"
	"github.com/xonoko/robust-home-automation/internal/app"
	"github.com/xonoko/robust-home-automation/internal/config"
	"github.com/xonoko/robust-home-automation/internal/version"
)

// Execute runs the command line and returns the process exit code.
func Execute() int {
	root := newRoot(os.Stdout)
	if err := root.ExecuteContext(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	return 0
}

func newRoot(out io.Writer) *cobra.Command {
	root := &cobra.Command{
		Use:           "hub",
		Short:         "Robust Home Automation hub",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(out)
	root.PersistentFlags().String("config", "", "path to a YAML config file (optional)")
	root.AddCommand(newServe(), newOpenAPI(), newVersion())
	return root
}

func newServe() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Run the hub",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			path, _ := cmd.Flags().GetString("config")
			cfg, err := config.Load(path, cmd.Flags())
			if err != nil {
				return err
			}
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			return app.Run(ctx, cfg, cmd.OutOrStdout())
		},
	}
	config.RegisterFlags(cmd.Flags())
	return cmd
}

func newOpenAPI() *cobra.Command {
	var format string
	cmd := &cobra.Command{
		Use:   "openapi",
		Short: "Print the OpenAPI document of the HTTP API",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			doc := api.New(api.Options{}).OpenAPI()
			var (
				b   []byte
				err error
			)
			switch format {
			case "json":
				b, err = json.MarshalIndent(doc, "", "  ")
			case "yaml":
				b, err = doc.YAML()
			default:
				return fmt.Errorf("unknown format %q (json or yaml)", format)
			}
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), string(b))
			return err
		},
	}
	cmd.Flags().StringVar(&format, "format", "json", "output format: json or yaml")
	return cmd
}

func newVersion() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print build information",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			b, err := json.Marshal(version.Get())
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), string(b))
			return err
		},
	}
}
