package plan

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mkramb/planctl/internal/config"
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

// A newly created plan worktree may not contain .planctl.yaml yet (init does not
// commit it). Recover its configuration from sibling Git worktrees, not a local
// state file. Restrict this fallback to our deterministic managed workspaces.
func (s Service) loadConfiguration(ctx context.Context, dir, override string) (config.Loaded, error) {
	loader := config.Loader{Executor: s.Executor, Env: s.Env}
	loaded, originalErr := loader.Load(ctx, dir, override)
	var configErr *config.Error
	if originalErr == nil || override != "" || !errors.As(originalErr, &configErr) || configErr.Code != "config_not_found" {
		return loaded, originalErr
	}
	client := git.New(s.Executor, dir, s.Env)
	root, err := client.Root(ctx)
	if err != nil {
		return config.Loaded{}, err
	}
	managedRoot, err := s.workspaceRoot(ctx, client)
	if err != nil {
		return config.Loaded{}, err
	}
	if filepath.Dir(root) != managedRoot {
		return config.Loaded{}, originalErr
	}
	branch, err := client.CurrentBranch(ctx)
	if err != nil {
		return config.Loaded{}, err
	}
	worktrees, err := client.Worktrees(ctx)
	if err != nil {
		return config.Loaded{}, err
	}
	var selected *config.Loaded
	for _, worktree := range worktrees {
		if worktree.Prunable || worktree.Bare || inside(managedRoot, worktree.Path) {
			continue
		}
		candidate, err := loader.Load(ctx, worktree.Path, "")
		if errors.As(err, &configErr) && configErr.Code == "config_not_found" {
			continue
		}
		if err != nil {
			return config.Loaded{}, err
		}
		if strings.ReplaceAll(candidate.Config.Branch.Pattern, "{slug}", filepath.Base(root)) != branch {
			continue
		}
		if selected != nil && selected.Config != candidate.Config {
			return config.Loaded{}, &Error{Code: "ambiguous_config", Message: "related worktrees have different configuration; pass --config explicitly"}
		}
		if selected == nil {
			selected = &candidate
		}
	}
	if selected == nil {
		return config.Loaded{}, originalErr
	}
	return *selected, nil
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
// This also keeps a tracked symlink in the plan directory from redirecting writes.
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

func writePlan(p Plan) error {
	if !inside(p.WorkspacePath, p.AbsolutePath) {
		return &Error{Code: "unsafe_path", Message: "plan path escapes its workspace"}
	}
	if err := makeDirectories(filepath.Dir(p.AbsolutePath)); err != nil {
		return err
	}
	file, err := os.OpenFile(p.AbsolutePath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return &Error{Code: "plan_exists", Message: fmt.Sprintf("plan already exists at %s; choose another title or edit the existing plan", p.AbsolutePath)}
		}
		return err
	}
	_, writeErr := file.WriteString(template(p.Title))
	return errors.Join(writeErr, file.Close())
}
