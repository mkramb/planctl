package git

import (
	"context"
	"errors"
	"strings"
)

var ErrPushRejected = errors.New("remote rejected the plan branch update")

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

func (c *Client) ChangedFiles(ctx context.Context, base, head string) ([]string, error) {
	result, err := c.Run(ctx, "diff", "--name-only", "--no-renames", "-z", base+"..."+head, "--")
	return splitPaths(result.Stdout), err
}

func (c *Client) StagedFiles(ctx context.Context) ([]string, error) {
	result, err := c.Run(ctx, "diff", "--cached", "--name-only", "--no-renames", "-z", "--")
	return splitPaths(result.Stdout), err
}

// StatusFiles lists staged, modified, untracked, and deleted paths, optionally
// limited to a pathspec. Paths are relative to the repository root and contain
// no rename pairs.
func (c *Client) StatusFiles(ctx context.Context, pathspec ...string) ([]string, error) {
	args := append([]string{"status", "--porcelain=v1", "-z", "--no-renames", "--untracked-files=all", "--"}, pathspec...)
	result, err := c.Run(ctx, args...)
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, entry := range strings.Split(result.Stdout, "\x00") {
		if len(entry) < 4 {
			continue
		}
		paths = append(paths, entry[3:])
	}
	return paths, nil
}

// ListFiles lists tracked and untracked (but not ignored) files under a
// pathspec, relative to the repository root.
func (c *Client) ListFiles(ctx context.Context, pathspec string) ([]string, error) {
	result, err := c.Run(ctx, "ls-files", "--cached", "--others", "--exclude-standard", "-z", "--", pathspec)
	return splitPaths(result.Stdout), err
}

// DiffersFrom reports whether any of the given paths' working-tree content
// differs from their blob in commit, including files that are untracked here
// but were committed to the review branch, and files deleted locally.
func (c *Client) DiffersFrom(ctx context.Context, commit string, files []string) (bool, error) {
	for _, file := range files {
		working, wExists, err := c.workingBlob(ctx, file)
		if err != nil {
			return false, err
		}
		committed, cExists, err := c.treeBlob(ctx, commit, file)
		if err != nil {
			return false, err
		}
		if wExists != cExists || working != committed {
			return true, nil
		}
	}
	return false, nil
}

// workingBlob returns the hash of the file's working-tree content, reporting
// whether the file exists.
func (c *Client) workingBlob(ctx context.Context, path string) (string, bool, error) {
	result, err := c.Run(ctx, "hash-object", "--", path)
	if err != nil {
		if exitedWith(err, 128) {
			return "", false, nil
		}
		return "", false, err
	}
	return strings.TrimSpace(result.Stdout), true, nil
}

// treeBlob returns the hash of the file's blob in commit, reporting whether the
// path exists at that commit.
func (c *Client) treeBlob(ctx context.Context, commit, path string) (string, bool, error) {
	result, err := c.Run(ctx, "rev-parse", "--verify", "--quiet", "--end-of-options", commit+":"+path)
	if err != nil {
		if exitedWith(err, 1) {
			return "", false, nil
		}
		return "", false, err
	}
	return strings.TrimSpace(result.Stdout), true, nil
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
