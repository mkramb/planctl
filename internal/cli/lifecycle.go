package cli

import (
	"errors"

	"github.com/mkramb/planctl/internal/output"
	"github.com/mkramb/planctl/internal/plan"
	"github.com/spf13/cobra"
)

// planFlag adds the shared --plan selector and validates it.
func planFlag(cmd *cobra.Command, id *string) {
	cmd.Flags().StringVar(id, "plan", "", "Plan slug (required when selection is ambiguous)")
	existing := cmd.Args
	cmd.Args = func(cmd *cobra.Command, args []string) error {
		if err := existing(cmd, args); err != nil {
			return err
		}
		if cmd.Flags().Changed("plan") && *id == "" {
			return usageError{errors.New("--plan requires a nonempty slug")}
		}
		return nil
	}
}

func inspect(cmd *cobra.Command, deps Dependencies, opts *options, planID string) (plan.Evaluation, error) {
	service := plan.Service{Executor: deps.Executor, Env: deps.Env, CacheDir: deps.CacheDir, Provider: deps.Provider}
	return service.Inspect(cmd.Context(), plan.PublishRequest{Dir: deps.Dir, ConfigPath: opts.config, PlanID: planID})
}

func newStatus(deps Dependencies, opts *options) *cobra.Command {
	var planID string
	cmd := &cobra.Command{
		Use: "status", Short: "Show plan review status", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			eval, err := inspect(cmd, deps, opts, planID)
			if err != nil {
				return err
			}
			return (output.Renderer{Stdout: deps.Stdout, Stderr: deps.Stderr, JSON: opts.json}).Status(output.NewStatusResult(eval))
		},
	}
	cmd.Args = func(cmd *cobra.Command, args []string) error {
		if err := cobra.NoArgs(cmd, args); err != nil {
			return usageError{err}
		}
		return nil
	}
	planFlag(cmd, &planID)
	return cmd
}

func newFeedback(deps Dependencies, opts *options) *cobra.Command {
	var planID string
	cmd := &cobra.Command{
		Use: "feedback", Short: "Show review feedback for a plan", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			eval, err := inspect(cmd, deps, opts, planID)
			if err != nil {
				return err
			}
			return (output.Renderer{Stdout: deps.Stdout, Stderr: deps.Stderr, JSON: opts.json}).Feedback(output.NewFeedbackResult(eval))
		},
	}
	cmd.Args = func(cmd *cobra.Command, args []string) error {
		if err := cobra.NoArgs(cmd, args); err != nil {
			return usageError{err}
		}
		return nil
	}
	planFlag(cmd, &planID)
	return cmd
}

func newContext(deps Dependencies, opts *options) *cobra.Command {
	var planID string
	cmd := &cobra.Command{
		Use: "context", Short: "Show plan context for coding agents", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			eval, err := inspect(cmd, deps, opts, planID)
			if err != nil {
				return err
			}
			return (output.Renderer{Stdout: deps.Stdout, Stderr: deps.Stderr, JSON: opts.json}).Context(output.NewContextResult(eval))
		},
	}
	cmd.Args = func(cmd *cobra.Command, args []string) error {
		if err := cobra.NoArgs(cmd, args); err != nil {
			return usageError{err}
		}
		return nil
	}
	planFlag(cmd, &planID)
	return cmd
}
