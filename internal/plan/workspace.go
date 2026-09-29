package plan

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mkramb/planctl/internal/git"
)

func (s Service) workspaceRoot(ctx context.Context, client *git.Client) (string, error) {
	common, err := client.CommonDirectory(ctx)
	if err != nil {
		return "", err
	}
	common, err = filepath.EvalSymlinks(common)
	if err != nil {
		return "", err
	}
	cache, err := s.cacheDir()
	if err != nil {
		return "", err
	}
	cache, err = canonicalFuturePath(cache)
	if err != nil {
		return "", err
	}
	repositoryName := strings.TrimSuffix(filepath.Base(common), ".git")
	if filepath.Base(common) == ".git" {
		repositoryName = filepath.Base(filepath.Dir(common))
	}
	name, err := Slug(repositoryName)
	if err != nil {
		name = "repository"
	}
	key := sha256.Sum256([]byte(common))
	return filepath.Join(cache, "planctl", "worktrees", fmt.Sprintf("%s-%x", name, key[:6])), nil
}

// prepareWorktree creates or reuses the isolated review worktree at the base
// commit. Reuse preserves prior published history so republish updates the same
// review.
func (s Service) prepareWorktree(ctx context.Context, source *git.Client, p Plan, baseCommit string) error {
	worktrees, err := source.Worktrees(ctx)
	if err != nil {
		return err
	}
	for _, worktree := range worktrees {
		sameBranch := worktree.Branch == p.Branch
		samePath := filepath.Clean(worktree.Path) == p.WorkspacePath
		if !sameBranch && !samePath {
			continue
		}
		if !sameBranch || !samePath {
			return &Error{Code: "workspace_conflict", Message: fmt.Sprintf("review branch or workspace is already in use at %s", worktree.Path)}
		}
		if worktree.Prunable {
			return &Error{Code: "workspace_missing", Message: "Git still registers a missing review worktree; repair or remove that registration before retrying"}
		}
		return s.verifyWorktree(ctx, source, p)
	}
	if _, err := os.Lstat(p.WorkspacePath); err == nil {
		return &Error{Code: "workspace_conflict", Message: fmt.Sprintf("workspace path already exists outside Git's worktree registry: %s", p.WorkspacePath)}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	_, exists, err := source.ResolveCommit(ctx, "refs/heads/"+p.Branch)
	if err != nil {
		return err
	}
	if err := source.AddWorktree(ctx, p.WorkspacePath, p.Branch, baseCommit, !exists); err != nil {
		return fmt.Errorf("create review worktree: %w", err)
	}
	return s.verifyWorktree(ctx, source, p)
}

func (s Service) verifyWorktree(ctx context.Context, source *git.Client, p Plan) error {
	info, err := os.Lstat(p.WorkspacePath)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return &Error{Code: "workspace_conflict", Message: "managed workspace must be a directory, not a symlink"}
	}
	client := git.New(s.Executor, p.WorkspacePath, s.Env)
	common, err := client.CommonDirectory(ctx)
	if err != nil {
		return err
	}
	want, err := source.CommonDirectory(ctx)
	if err != nil {
		return err
	}
	branch, err := client.CurrentBranch(ctx)
	if err != nil {
		return err
	}
	if common != want || branch != p.Branch {
		return &Error{Code: "workspace_conflict", Message: "managed workspace belongs to a different repository or branch"}
	}
	return nil
}

// syncFiles copies the selected files' current content into the worktree,
// removes files published earlier but no longer selected, and stages the result.
func (s Service) syncFiles(ctx context.Context, workspacePath, sourceRoot string, files []string, baseCommit string) error {
	ws := git.New(s.Executor, workspacePath, s.Env)
	for _, rel := range files {
		src := filepath.Join(sourceRoot, filepath.FromSlash(rel))
		dst := filepath.Join(workspacePath, filepath.FromSlash(rel))
		info, err := os.Stat(src)
		if errors.Is(err, os.ErrNotExist) {
			if err := os.Remove(dst); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
		data, err := os.ReadFile(src)
		if err != nil {
			return err
		}
		if err := makeDirectories(filepath.Dir(dst)); err != nil {
			return err
		}
		if err := os.WriteFile(dst, data, info.Mode().Perm()); err != nil {
			return err
		}
	}
	head, _, err := ws.ResolveCommit(ctx, "HEAD")
	if err != nil {
		return err
	}
	changed, err := ws.ChangedFiles(ctx, baseCommit, head)
	if err != nil {
		return err
	}
	selected := make(map[string]bool, len(files))
	for _, rel := range files {
		selected[rel] = true
	}
	for _, rel := range changed {
		if selected[rel] {
			continue
		}
		if err := os.Remove(filepath.Join(workspacePath, filepath.FromSlash(rel))); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	_, err = ws.Run(ctx, "add", "-A")
	return err
}

func inside(root, path string) bool {
	relative, err := filepath.Rel(root, path)
	return err == nil && filepath.IsLocal(relative)
}

// Resolve symlinks in the existing part of the cache path, including macOS's
// /var -> /private/var, without creating any directories yet.
func canonicalFuturePath(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err == nil {
		return resolved, nil
	}
	if !errors.Is(err, os.ErrNotExist) || filepath.Dir(absolute) == absolute {
		return "", err
	}
	parent, err := canonicalFuturePath(filepath.Dir(absolute))
	if err != nil {
		return "", err
	}
	return filepath.Join(parent, filepath.Base(absolute)), nil
}

// Refuse symlinks in managed paths, even when their targets happen to exist.
func makeDirectories(path string) error {
	info, err := os.Lstat(path)
	if err == nil {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return &Error{Code: "unsafe_path", Message: fmt.Sprintf("expected a directory, not a file or symlink: %s", path)}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	parent := filepath.Dir(path)
	if parent != path {
		if parentErr := makeDirectories(parent); parentErr != nil {
			return parentErr
		}
	}
	if err == nil {
		return nil
	}
	if err := os.Mkdir(path, 0o755); err != nil {
		if errors.Is(err, os.ErrExist) {
			return makeDirectories(path)
		}
		return err
	}
	return nil
}

// cacheDir returns the injectable cache directory or the user cache, resolving
// symlinks (macOS /var -> /private/var) so path traversal never touches one.
func (s Service) cacheDir() (string, error) {
	cache := s.CacheDir
	if cache == "" {
		var err error
		cache, err = os.UserCacheDir()
		if err != nil {
			return "", err
		}
	}
	return canonicalFuturePath(cache)
}
