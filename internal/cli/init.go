package cli

import (
	"github.com/mkramb/planctl/internal/config"
	"github.com/mkramb/planctl/internal/output"
	"github.com/spf13/cobra"
)

func newInit(deps Dependencies, opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Create configuration in an existing Git repository",
		Args: func(cmd *cobra.Command, args []string) error {
			if err := cobra.NoArgs(cmd, args); err != nil {
				return usageError{err}
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			loader := config.Loader{Executor: deps.Executor, Env: deps.Env}
			location, err := loader.Init(cmd.Context(), deps.Dir, opts.config)
			if err != nil {
				return err
			}
			return (output.Renderer{Stdout: deps.Stdout, Stderr: deps.Stderr, JSON: opts.json}).Init(
				output.InitResult{Version: output.Version, Config: location},
			)
		},
	}
}
