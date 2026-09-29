package cli

import (
	"errors"

	"github.com/mkramb/planctl/internal/output"
	"github.com/mkramb/planctl/internal/plan"
	"github.com/spf13/cobra"
)

func newPublish(deps Dependencies, opts *options) *cobra.Command {
	var planID string
	cmd := &cobra.Command{
		Use: "publish", Short: "Commit and push a plan for review (GitHub adapter pending)",
		Args: func(cmd *cobra.Command, args []string) error {
			if err := cobra.NoArgs(cmd, args); err != nil {
				return usageError{err}
			}
			if cmd.Flags().Changed("plan") && planID == "" {
				return usageError{errors.New("--plan requires a nonempty slug")}
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			service := plan.Service{Executor: deps.Executor, Env: deps.Env, CacheDir: deps.CacheDir, Provider: deps.Provider}
			published, err := service.Publish(cmd.Context(), plan.PublishRequest{Dir: deps.Dir, ConfigPath: opts.config, PlanID: planID})
			if err != nil {
				return err
			}
			return (output.Renderer{Stdout: deps.Stdout, Stderr: deps.Stderr, JSON: opts.json}).Publish(output.PublishResult{
				Version: output.Version, Plan: published.Plan, Commit: published.Commit,
				Review: output.ReviewResult{
					ID: published.Review.Ref.ID, Provider: published.Review.Ref.Provider, URL: published.Review.URL,
					State: published.Review.State, Draft: published.Review.Draft,
				},
			})
		},
	}
	cmd.Flags().StringVar(&planID, "plan", "", "Plan slug (required when selection is ambiguous)")
	return cmd
}
