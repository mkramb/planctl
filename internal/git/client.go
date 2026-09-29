// Package git operates on repositories using the locally installed Git executable.
package git

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/mkramb/planctl/internal/process"
)

var ErrNotRepository = errors.New("not inside a Git working tree")

type Client struct {
	executor process.Executor
	dir      string
	env      []string
}

func New(executor process.Executor, dir string, env []string) *Client {
	return &Client{executor: executor, dir: dir, env: env}
}

// Run is the Git-only escape hatch for commands without a dedicated helper yet.
// Every invocation has an explicit working directory and goes through process.
func (c *Client) Run(ctx context.Context, args ...string) (process.Result, error) {
	env := c.env
	if env == nil {
		env = os.Environ()
	}
	// Keep Git diagnostics predictable when translating known repository errors.
	env = append(slices.Clone(env), "LC_ALL=C")
	result, err := c.executor.Run(ctx, process.Request{
		Executable: "git", Args: args, Dir: c.dir, Env: env,
	})
	if err != nil {
		err = withStderr(err, result.Stderr)
	}
	return result, err
}

// withStderr appends a short sanitized snippet of git's stderr so failures are
// diagnosable. It never includes arguments or environment.
func withStderr(err error, stderr string) error {
	const limit = 300
	detail := strings.Join(strings.Fields(stderr), " ")
	if len(detail) > limit {
		detail = detail[:limit]
	}
	if detail == "" {
		return err
	}
	return fmt.Errorf("%w: %s", err, detail)
}

func (c *Client) Root(ctx context.Context) (string, error) {
	result, err := c.Run(ctx, "rev-parse", "--show-toplevel")
	if err != nil {
		if strings.Contains(result.Stderr, "not a git repository") || strings.Contains(result.Stderr, "must be run in a work tree") {
			return "", ErrNotRepository
		}
		return "", err
	}
	return strings.TrimSpace(result.Stdout), nil
}

func (c *Client) CommonDirectory(ctx context.Context) (string, error) {
	return c.value(ctx, "rev-parse", "--path-format=absolute", "--git-common-dir")
}

func (c *Client) CurrentBranch(ctx context.Context) (string, error) {
	branch, err := c.value(ctx, "symbolic-ref", "--quiet", "--short", "HEAD")
	if exitedWith(err, 1) {
		return "", nil // A detached HEAD has no current branch.
	}
	return branch, err
}

func (c *Client) Head(ctx context.Context) (string, error) {
	return c.value(ctx, "rev-parse", "HEAD")
}

func (c *Client) Add(ctx context.Context, paths ...string) error {
	args := append([]string{"add", "--"}, paths...)
	_, err := c.Run(ctx, args...)
	return err
}

func (c *Client) Commit(ctx context.Context, message string) error {
	_, err := c.Run(ctx, "commit", "-m", message)
	return err
}

func (c *Client) Push(ctx context.Context, remote, branch string) error {
	_, err := c.Run(ctx, "push", "--", remote, branch)
	return err
}

func (c *Client) value(ctx context.Context, args ...string) (string, error) {
	result, err := c.Run(ctx, args...)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(result.Stdout), nil
}
