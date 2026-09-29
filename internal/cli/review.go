package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/mkramb/planctl/internal/output"
	"github.com/mkramb/planctl/internal/plan"
	"github.com/spf13/cobra"
)

func newReview(deps Dependencies, opts *options) *cobra.Command {
	var planID string
	var timeout time.Duration
	var poll time.Duration
	cmd := &cobra.Command{
		Use:   "review",
		Short: "Publish the plan and block until it is approved or changes are requested",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			if timeout > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, timeout)
				defer cancel()
			}
			service := plan.Service{Executor: deps.Executor, Env: deps.Env, CacheDir: deps.CacheDir, Provider: deps.Provider}
			_, _ = fmt.Fprintln(deps.Stderr, "Publishing plan and waiting for review...")
			eval, err := service.Review(ctx, plan.PublishRequest{Dir: deps.Dir, ConfigPath: opts.config, PlanID: planID}, poll, func(e plan.Evaluation) {
				_, _ = fmt.Fprintf(deps.Stderr, "Status %s, approvals %d/%d\n", e.Status, e.Approvals, e.RequiredApprovals)
			})
			if err != nil {
				return err
			}
			return (output.Renderer{Stdout: deps.Stdout, Stderr: deps.Stderr, JSON: opts.json}).Context(output.NewContextResult(eval))
		},
	}
	cmd.Flags().StringVar(&planID, "plan", "", "Plan slug (required when selection is ambiguous)")
	cmd.Flags().DurationVar(&timeout, "timeout", 0, "Stop waiting after this duration (0 means wait forever)")
	cmd.Flags().DurationVar(&poll, "poll", 5*time.Second, "How often to check the review")
	return cmd
}
