package git

import (
	"context"
	"strings"
)

type Worktree struct {
	Path     string
	Branch   string
	Prunable bool
}

func (c *Client) Worktrees(ctx context.Context) ([]Worktree, error) {
	result, err := c.Run(ctx, "worktree", "list", "--porcelain", "-z")
	if err != nil {
		return nil, err
	}
	var worktrees []Worktree
	var current Worktree
	for _, field := range strings.Split(result.Stdout, "\x00") {
		key, value, _ := strings.Cut(field, " ")
		switch key {
		case "worktree":
			current = Worktree{Path: value}
		case "branch":
			current.Branch = strings.TrimPrefix(value, "refs/heads/")
		case "prunable":
			current.Prunable = true
		case "":
			if current.Path != "" {
				worktrees = append(worktrees, current)
				current = Worktree{}
			}
		}
	}
	return worktrees, nil
}

func (c *Client) AddWorktree(ctx context.Context, path, branch, start string, newBranch bool) error {
	args := []string{"worktree", "add"}
	if newBranch {
		args = append(args, "--no-track", "-b", branch)
	} else {
		start = branch
	}
	args = append(args, "--", path, start)
	_, err := c.Run(ctx, args...)
	return err
}
