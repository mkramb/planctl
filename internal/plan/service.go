package plan

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/mkramb/planctl/internal/git"
	"github.com/mkramb/planctl/internal/process"
	"github.com/mkramb/planctl/internal/review"
)

type Service struct {
	Executor process.Executor
	Env      []string
	// CacheDir is injectable so tests never write into the user's cache.
	CacheDir string
	Provider review.Provider
}

type CreateRequest struct {
	Dir        string
	ConfigPath string
	Title      string
}

func (s Service) Create(ctx context.Context, req CreateRequest) (Plan, error) {
	title := strings.TrimSpace(req.Title)
	if strings.ContainsFunc(title, unicode.IsControl) {
		return Plan{}, &Error{Code: "invalid_title", Message: "title must be a single line without control characters"}
	}
	slug, err := Slug(title)
	if err != nil {
		return Plan{}, err
	}
	loaded, err := s.loadConfiguration(ctx, req.Dir, req.ConfigPath)
	if err != nil {
		return Plan{}, err
	}
	if loaded.Config.Repositories.Plans != "current" {
		return Plan{}, &Error{Code: "dedicated_repository_unavailable", Message: "dedicated plans repositories are not implemented yet; use repositories.plans: current"}
	}
	client := git.New(s.Executor, loaded.RepositoryRoot, s.Env)
	branch := strings.ReplaceAll(loaded.Config.Branch.Pattern, "{slug}", slug)
	if err := client.CheckBranch(ctx, branch); err != nil {
		return Plan{}, &Error{Code: "invalid_branch", Message: "branch pattern and title do not produce a valid Git branch name", Cause: err}
	}
	workspaceRoot, err := s.workspaceRoot(ctx, client)
	if err != nil {
		return Plan{}, err
	}
	if inside(loaded.RepositoryRoot, workspaceRoot) {
		return Plan{}, &Error{Code: "invalid_workspace", Message: "the user cache must be outside the implementation checkout"}
	}
	lock, err := lockWorkspace(workspaceRoot, slug)
	if err != nil {
		return Plan{}, err
	}
	defer lock.Unlock()
	if err := ctx.Err(); err != nil {
		return Plan{}, err
	}
	base, commit, err := resolveBase(ctx, client, loaded.Config.Branch.Base)
	if err != nil {
		return Plan{}, err
	}
	p := Plan{
		ID: slug, Title: title, Branch: branch, Base: base,
		Path:          filepath.ToSlash(filepath.Join(loaded.Config.Plan.Directory, slug+".md")),
		WorkspacePath: filepath.Join(workspaceRoot, slug),
	}
	p.AbsolutePath = filepath.Join(p.WorkspacePath, filepath.FromSlash(p.Path))
	if err := s.prepareWorktree(ctx, client, p, commit); err != nil {
		return Plan{}, err
	}
	if err := ctx.Err(); err != nil {
		return Plan{}, err
	}
	if err := writePlan(p); err != nil {
		return Plan{}, err
	}
	return p, nil
}

func resolveBase(ctx context.Context, client *git.Client, configured string) (string, string, error) {
	resolve := func(name string) (string, bool, error) {
		for _, prefix := range []string{"refs/remotes/origin/", "refs/heads/"} {
			commit, found, err := client.ResolveCommit(ctx, prefix+name)
			if err != nil || found {
				return commit, found, err
			}
		}
		return "", false, nil
	}
	name := configured
	if name == "" {
		var err error
		name, err = client.DefaultRemoteBranch(ctx)
		if err != nil {
			return "", "", err
		}
	}
	if name != "" {
		commit, found, err := resolve(name)
		if err != nil {
			return "", "", err
		}
		if found {
			return name, commit, nil
		}
		return "", "", &Error{Code: "base_not_found", Message: fmt.Sprintf("base branch %q has no local commit; fetch it or update branch.base", name)}
	}
	// Creating a plan stays offline. Without origin/HEAD, use an unambiguous
	// conventional base; never assume the caller's feature branch is the base.
	var selected, revision string
	for _, candidate := range []string{"main", "master"} {
		commit, found, err := resolve(candidate)
		if err != nil {
			return "", "", err
		}
		if !found {
			continue
		}
		if selected != "" {
			return "", "", &Error{Code: "ambiguous_base", Message: "both main and master exist; set branch.base or the local origin/HEAD reference"}
		}
		selected, revision = candidate, commit
	}
	if selected == "" {
		return "", "", &Error{Code: "base_not_found", Message: "no base branch with a commit was found; set branch.base or fetch the repository's default branch"}
	}
	return selected, revision, nil
}

func (s Service) prepareWorktree(ctx context.Context, client *git.Client, p Plan, baseCommit string) error {
	worktrees, err := client.Worktrees(ctx)
	if err != nil {
		return err
	}
	for _, worktree := range worktrees {
		if worktree.Branch != p.Branch && filepath.Clean(worktree.Path) != p.WorkspacePath {
			continue
		}
		if filepath.Clean(worktree.Path) != p.WorkspacePath || worktree.Branch != p.Branch {
			return &Error{Code: "workspace_conflict", Message: fmt.Sprintf("plan branch or workspace is already in use at %s", worktree.Path)}
		}
		if worktree.Prunable {
			return &Error{Code: "workspace_missing", Message: "Git still registers a missing plan worktree; repair or remove that registration before retrying"}
		}
		return s.verifyWorktree(ctx, client, p)
	}
	if _, err := os.Lstat(p.WorkspacePath); err == nil {
		return &Error{Code: "workspace_conflict", Message: fmt.Sprintf("workspace path already exists outside Git's worktree registry: %s", p.WorkspacePath)}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	head, exists, err := client.ResolveCommit(ctx, "refs/heads/"+p.Branch)
	if err != nil {
		return err
	}
	if !exists {
		head = baseCommit
	}
	contains, err := client.TreeContains(ctx, head, p.Path)
	if err != nil {
		return err
	}
	if contains {
		return &Error{Code: "plan_exists", Message: "the plan path already exists in the branch; choose a different title"}
	}
	if exists && head != baseCommit {
		return &Error{Code: "branch_conflict", Message: "the plan branch already contains work; create will not repurpose it"}
	}
	if err := client.AddWorktree(ctx, p.WorkspacePath, p.Branch, baseCommit, !exists); err != nil {
		return fmt.Errorf("create plan worktree: %w", err)
	}
	return s.verifyWorktree(ctx, client, p)
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
