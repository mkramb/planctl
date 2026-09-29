// Package cli connects thin Cobra commands to application dependencies.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"

	"github.com/mkramb/planctl/internal/config"
	"github.com/mkramb/planctl/internal/git"
	"github.com/mkramb/planctl/internal/github"
	"github.com/mkramb/planctl/internal/output"
	"github.com/mkramb/planctl/internal/plan"
	"github.com/mkramb/planctl/internal/process"
	"github.com/mkramb/planctl/internal/review"
	"github.com/spf13/cobra"
)

type Dependencies struct {
	Dir      string
	Env      []string
	Stdout   io.Writer
	Stderr   io.Writer
	Executor process.Executor
	Provider review.Provider
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
		Use:           "planctl",
		Short:         "Review implementation plans before implementation begins",
		Long:          "planctl connects coding agents, Git, and GitHub for collaborative plan review.",
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
	root.AddCommand(newInit(deps, opts))
	root.AddCommand(newCreate(deps, opts))
	root.AddCommand(newPublish(deps, opts))
	root.AddCommand(newStatus(deps, opts))
	root.AddCommand(newFeedback(deps, opts))
	root.AddCommand(newContext(deps, opts))
	root.AddCommand(newComplete(deps, opts))
	root.AddCommand(newSkills(deps, opts))
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
		return output.ErrorDetail{Code: planErr.Code, Message: planErr.Message}, 1
	}
	if errors.As(err, &configErr) {
		return output.ErrorDetail{Code: configErr.Code, Message: configErr.Message}, 1
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

// Cobra may reject an argument before it parses a trailing --json. Detect the
// requested format independently so usage failures still honor the JSON contract.
func wantsJSON(args []string) bool {
	json := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--":
			return json
		case "--config", "--plan":
			i++ // The following value is not an output flag.
		case "--json", "--json=true", "--json=1", "--json=t", "--json=T", "--json=TRUE", "--json=True":
			json = true
		case "--json=false", "--json=0", "--json=f", "--json=F", "--json=FALSE", "--json=False":
			json = false
		}
	}
	return json
}
