package cli

import (
	"github.com/mkramb/planctl/internal/output"
	"github.com/mkramb/planctl/internal/plan"
	"github.com/spf13/cobra"
)

func newCreate(deps Dependencies, opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "create <title>",
		Short: "Create a Markdown plan in an isolated Git worktree",
		Args: func(cmd *cobra.Command, args []string) error {
			if err := cobra.ExactArgs(1)(cmd, args); err != nil {
				return usageError{err}
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			service := plan.Service{Executor: deps.Executor, Env: deps.Env, CacheDir: deps.CacheDir, Provider: deps.Provider}
			created, err := service.Create(cmd.Context(), plan.CreateRequest{
				Dir: deps.Dir, ConfigPath: opts.config, Title: args[0],
			})
			if err != nil {
				return err
			}
			return (output.Renderer{Stdout: deps.Stdout, Stderr: deps.Stderr, JSON: opts.json}).Create(
				output.CreateResult{Version: output.Version, Plan: created},
			)
		},
	}
}
