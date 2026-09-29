// Package main wires thin Cobra commands to application dependencies.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/mkramb/planctl/internal/config"
	"github.com/mkramb/planctl/internal/git"
	"github.com/mkramb/planctl/internal/github"
	"github.com/mkramb/planctl/internal/output"
	"github.com/mkramb/planctl/internal/plan"
	"github.com/mkramb/planctl/internal/process"
	"github.com/spf13/cobra"
)

type Dependencies struct {
	Dir      string
	Env      []string
	Stdout   io.Writer
	Stderr   io.Writer
	Executor process.Executor
	Provider *github.Provider
	Version  string
	CacheDir string
}

type options struct {
	json    bool
	config  string
	verbose bool
}

func newRoot(deps Dependencies, opts *options) *cobra.Command {
	root := &cobra.Command{
		Use:           "planctl [files...]",
		Short:         "Publish files for review as a GitHub pull request and wait for a decision",
		Long:          "planctl sends files (or a folder, or all changed files) to a GitHub pull request for review and waits for approval.",
		Version:       deps.Version,
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	root.SetOut(deps.Stdout)
	root.SetErr(deps.Stderr)
	root.CompletionOptions.DisableDefaultCmd = true
	root.PersistentFlags().BoolVar(&opts.json, "json", false, "Output versioned JSON (help and --version remain text)")
	root.PersistentFlags().StringVar(&opts.config, "config", "", "Path to .planctl.yaml")
	root.PersistentFlags().BoolVar(&opts.verbose, "verbose", false, "Include diagnostic details on stderr")
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { return usageError{err} })
	root.PersistentPreRunE = func(_ *cobra.Command, _ []string) error {
		exec := deps.Executor
		if opts.verbose {
			exec = process.Logging(exec, deps.Stderr)
		}
		deps.Executor = exec
		if deps.Provider == nil {
			deps.Provider = github.NewProvider(exec)
		}
		return nil
	}

	var rf reviewFlags
	var timeout time.Duration
	var poll time.Duration
	addReviewFlags(root, &rf)
	root.Flags().DurationVar(&timeout, "timeout", 0, "Stop waiting after this duration (0 means wait forever)")
	root.Flags().DurationVar(&poll, "poll", 5*time.Second, "How often to check the review")
	root.Args = cobra.ArbitraryArgs
	root.RunE = func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		if timeout > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, timeout)
			defer cancel()
		}
		req := rf.request(deps, opts, args)
		service := plan.Service{Executor: deps.Executor, Env: deps.Env, CacheDir: deps.CacheDir, Provider: deps.Provider}
		renderer := output.Renderer{Stdout: deps.Stdout, Stderr: deps.Stderr, JSON: opts.json}

		published, err := service.Publish(ctx, req)
		if err != nil {
			return err
		}
		// Emit the pull request URL immediately so the session has it while waiting.
		_, _ = fmt.Fprintf(deps.Stderr, "Review: %s\n", published.URL)

		eval, err := service.Wait(ctx, req, poll, func(e plan.Evaluation) {
			_, _ = fmt.Fprintf(deps.Stderr, "Status %s, approvals %d/%d\n", e.Status, e.Approvals, e.RequiredApprovals)
		})
		if err != nil {
			return err
		}
		return renderer.Outcome(output.NewOutcomeResult(eval))
	}

	root.AddCommand(newInit(deps, opts))
	root.AddCommand(newInstall(deps, opts))
	root.AddCommand(newUninstall(deps, opts))
	root.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Print the planctl build version",
		Args: func(cmd *cobra.Command, args []string) error {
			if err := cobra.NoArgs(cmd, args); err != nil {
				return usageError{err}
			}
			return nil
		},
		RunE: func(_ *cobra.Command, _ []string) error {
			return (output.Renderer{Stdout: deps.Stdout, Stderr: deps.Stderr, JSON: opts.json}).Version(
				output.VersionResult{Version: output.Version, Build: deps.Version},
			)
		},
	})
	return root
}

// Run creates a fresh command tree for each invocation. It never changes process
// working directory or environment, making in-process integration tests faithful
// and parallelizable. Only main calls os.Exit.
func Run(ctx context.Context, args []string, deps Dependencies) int {
	if deps.Stdout == nil {
		deps.Stdout = io.Discard
	}
	if deps.Stderr == nil {
		deps.Stderr = io.Discard
	}
	if deps.Executor == nil {
		deps.Executor = process.Runner{}
	}
	if deps.Provider == nil {
		deps.Provider = github.NewProvider(deps.Executor)
	}
	if deps.Version == "" {
		deps.Version = "dev"
	}
	opts := &options{}
	root := newRoot(deps, opts)
	root.SetArgs(args)
	err := ctx.Err()
	if err == nil {
		_, err = root.ExecuteContextC(ctx)
	}
	if err == nil {
		return 0
	}
	detail, exitCode := classifyError(err)
	renderer := output.Renderer{Stdout: deps.Stdout, Stderr: deps.Stderr, JSON: wantsJSON(args)}
	if renderErr := renderer.Error(output.ErrorResult{Version: output.Version, Error: detail}); renderErr != nil {
		// An output failure must not trigger another write to stdout or leak details.
		_, _ = fmt.Fprintln(deps.Stderr, "error: could not write command output")
		return 1
	}
	return exitCode
}

type usageError struct{ error }

func classifyError(err error) (output.ErrorDetail, int) {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return output.ErrorDetail{Code: "canceled", Message: "operation was canceled"}, 130
	}
	var usage usageError
	if errors.As(err, &usage) || strings.HasPrefix(err.Error(), "unknown command ") {
		return output.ErrorDetail{Code: "invalid_arguments", Message: err.Error()}, 2
	}
	var commandErr *process.Error
	var configErr *config.Error
	var planErr *plan.Error
	if errors.As(err, &planErr) {
		return output.ErrorDetail{Code: planErr.Code, Message: withCause(planErr.Message, planErr.Cause)}, 1
	}
	if errors.As(err, &configErr) {
		return output.ErrorDetail{Code: configErr.Code, Message: withCause(configErr.Message, configErr.Cause)}, 1
	}
	if errors.Is(err, git.ErrNotRepository) {
		return output.ErrorDetail{Code: "not_git_repository", Message: "not inside a Git working tree; initialize a repository with git init first"}, 1
	}
	if errors.As(err, &commandErr) && errors.Is(err, exec.ErrNotFound) {
		switch commandErr.Executable {
		case "git":
			return output.ErrorDetail{Code: "git_not_found", Message: "git was not found in PATH; install Git and try again"}, 1
		case "gh":
			return output.ErrorDetail{Code: "gh_not_found", Message: "gh was not found in PATH; install the GitHub CLI and try again"}, 1
		default:
			return output.ErrorDetail{Code: "executable_not_found", Message: commandErr.Error()}, 1
		}
	}
	return output.ErrorDetail{Code: "operation_failed", Message: err.Error()}, 1
}

// withCause appends an underlying error's detail so failures are diagnosable
// without swallowing the higher-level context.
func withCause(message string, cause error) string {
	if cause == nil {
		return message
	}
	return message + ": " + cause.Error()
}

// Cobra may reject an argument before it parses a trailing --json. Detect the
// requested format independently so usage failures still honor the JSON contract.
func wantsJSON(args []string) bool {
	json := false
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--":
			return json
		case args[i] == "--config":
			i++ // skip the following value
		case args[i] == "--json":
			json = true
		case strings.HasPrefix(args[i], "--json="):
			if value, err := strconv.ParseBool(strings.TrimPrefix(args[i], "--json=")); err == nil {
				json = value
			}
		}
	}
	return json
}
