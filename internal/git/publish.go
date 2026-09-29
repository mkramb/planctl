package git

import (
	"context"
	"errors"
	"strings"
)

var ErrPushRejected = errors.New("remote rejected the plan branch update")

func (c *Client) Branches(ctx context.Context) ([]string, error) {
	result, err := c.Run(ctx, "for-each-ref", "--format=%(refname:strip=2)", "refs/heads/")
	return strings.Fields(result.Stdout), err
}

func (c *Client) Origin(ctx context.Context) (string, error) {
	fetch, err := c.value(ctx, "remote", "get-url", "origin")
	if err != nil {
		return "", err
	}
	push, err := c.value(ctx, "remote", "get-url", "--push", "--all", "origin")
	if err != nil {
		return "", err
	}
	if push != fetch {
		return "", errors.New("origin must use the same single URL for fetching and pushing")
	}
	return fetch, nil
}

// RemoteURL returns a named remote's URL without the fetch/push consistency check.
func (c *Client) RemoteURL(ctx context.Context, name string) (string, error) {
	return c.value(ctx, "remote", "get-url", name)
}

func (c *Client) ChangedFiles(ctx context.Context, base, head string) ([]string, error) {
	result, err := c.Run(ctx, "diff", "--name-only", "--no-renames", "-z", base+"..."+head, "--")
	return splitPaths(result.Stdout), err
}

func (c *Client) StagedFiles(ctx context.Context) ([]string, error) {
	result, err := c.Run(ctx, "diff", "--cached", "--name-only", "--no-renames", "-z", "--")
	return splitPaths(result.Stdout), err
}

// PlanDirty reports staged, modified, or untracked state for one path.
func (c *Client) PlanDirty(ctx context.Context, path string) (bool, error) {
	result, err := c.Run(ctx, "status", "--porcelain", "-z", "--", path)
	return result.Stdout != "", err
}

// Dirty reports any staged, modified, or untracked state in the worktree.
func (c *Client) Dirty(ctx context.Context) (bool, error) {
	result, err := c.Run(ctx, "status", "--porcelain", "-z")
	return result.Stdout != "", err
}

// DeleteRemoteBranch removes a remote branch; an already-absent branch is success.
func (c *Client) DeleteRemoteBranch(ctx context.Context, remote, branch string) error {
	result, err := c.Run(ctx, "push", "--porcelain", "--", remote, ":refs/heads/"+branch)
	if err != nil && strings.Contains(result.Stderr, "remote ref does not exist") {
		return nil
	}
	return err
}

func (c *Client) RemoveWorktree(ctx context.Context, path string) error {
	_, err := c.Run(ctx, "worktree", "remove", "--", path)
	return err
}

// HistoryFiles also catches unrelated work that was committed and later reverted.
// Merge commits are rejected: merging implementation work into a plan branch can
// expose that history even when the final diff only contains a plan.
func (c *Client) HistoryFiles(ctx context.Context, base, head string) ([]string, error) {
	merges, err := c.value(ctx, "rev-list", "--merges", base+".."+head)
	if err != nil {
		return nil, err
	}
	if merges != "" {
		return nil, errors.New("plan branch contains merge commits")
	}
	commits, err := c.value(ctx, "rev-list", base+".."+head)
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, commit := range strings.Fields(commits) {
		result, err := c.Run(ctx, "diff-tree", "--root", "--no-commit-id", "--name-only", "--no-renames", "-r", "-z", commit, "--")
		if err != nil {
			return nil, err
		}
		paths = append(paths, splitPaths(result.Stdout)...)
	}
	return paths, nil
}

func (c *Client) CommitOnly(ctx context.Context, message, path string) error {
	_, err := c.Run(ctx, "commit", "--only", "-m", message, "--", path)
	return err
}

func (c *Client) PushPlan(ctx context.Context, commit, branch string) error {
	result, err := c.Run(ctx, "-c", "push.followTags=false", "-c", "remote.origin.mirror=false",
		"push", "--porcelain", "--", "origin", commit+":refs/heads/"+branch)
	if err != nil && strings.Contains(result.Stdout, "[rejected]") {
		return ErrPushRejected
	}
	return err
}

func splitPaths(output string) []string {
	if output == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(output, "\x00"), "\x00")
}
