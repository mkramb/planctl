package git

import (
	"context"
	"errors"
	"strings"

	"github.com/mkramb/planctl/internal/process"
)

// ResolveCommit distinguishes an absent ref from failures to execute Git.
func (c *Client) ResolveCommit(ctx context.Context, ref string) (string, bool, error) {
	result, err := c.Run(ctx, "rev-parse", "--verify", "--quiet", "--end-of-options", ref+"^{commit}")
	if exitedWith(err, 1) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return strings.TrimSpace(result.Stdout), true, nil
}

func (c *Client) DefaultRemoteBranch(ctx context.Context) (string, error) {
	result, err := c.Run(ctx, "symbolic-ref", "--quiet", "refs/remotes/origin/HEAD")
	if exitedWith(err, 1) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return strings.TrimPrefix(strings.TrimSpace(result.Stdout), "refs/remotes/origin/"), nil
}

func (c *Client) CheckBranch(ctx context.Context, branch string) error {
	_, err := c.Run(ctx, "check-ref-format", "--branch", branch)
	return err
}

func exitedWith(err error, code int) bool {
	var commandErr *process.Error
	return errors.As(err, &commandErr) && commandErr.ExitCode == code &&
		!errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded)
}
