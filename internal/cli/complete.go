package cli

import (
	"github.com/mkramb/planctl/internal/output"
	"github.com/mkramb/planctl/internal/plan"
	"github.com/spf13/cobra"
)

func newComplete(deps Dependencies, opts *options) *cobra.Command {
	var planID string
	var pruneRemote bool
	cmd := &cobra.Command{
		Use: "complete", Short: "Complete the plan lifecycle (close or merge, optionally prune)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			service := plan.Service{Executor: deps.Executor, Env: deps.Env, CacheDir: deps.CacheDir, Provider: deps.Provider}
			completion, err := service.Complete(cmd.Context(), plan.PublishRequest{
				Dir: deps.Dir, ConfigPath: opts.config, PlanID: planID,
			}, pruneRemote)
			if err != nil {
				return err
			}
			return (output.Renderer{Stdout: deps.Stdout, Stderr: deps.Stderr, JSON: opts.json}).Complete(
				output.NewCompleteResult(completion),
			)
		},
	}
	cmd.Args = func(cmd *cobra.Command, args []string) error {
		if err := cobra.NoArgs(cmd, args); err != nil {
			return usageError{err}
		}
		return nil
	}
	cmd.Flags().StringVar(&planID, "plan", "", "Plan slug (required when selection is ambiguous)")
	cmd.Flags().BoolVar(&pruneRemote, "prune-remote", false, "Delete the remote plan branch after finalizing")
	return cmd
}
