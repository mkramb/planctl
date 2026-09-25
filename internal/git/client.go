// Package git operates on repositories using the locally installed Git executable.
package git

import (
	"context"
	"strings"

	"github.com/mkramb/planctl/internal/process"
)

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
	return c.executor.Run(ctx, process.Request{
		Executable: "git", Args: args, Dir: c.dir, Env: c.env,
	})
}

func (c *Client) Root(ctx context.Context) (string, error) {
	return c.value(ctx, "rev-parse", "--show-toplevel")
}

func (c *Client) CommonDirectory(ctx context.Context) (string, error) {
	return c.value(ctx, "rev-parse", "--path-format=absolute", "--git-common-dir")
}

func (c *Client) CurrentBranch(ctx context.Context) (string, error) {
	return c.value(ctx, "symbolic-ref", "--quiet", "--short", "HEAD")
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
