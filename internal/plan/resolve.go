package plan

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mkramb/planctl/internal/config"
	"github.com/mkramb/planctl/internal/git"
)

type PublishRequest struct {
	Dir        string
	ConfigPath string
	PlanID     string
}

type selection struct {
	plan       Plan
	config     config.Loaded
	client     *git.Client
	baseCommit string
}

func (s Service) resolve(ctx context.Context, req PublishRequest) (selection, error) {
	loaded, err := s.loadConfiguration(ctx, req.Dir, req.ConfigPath)
	if err != nil {
		return selection{}, err
	}
	if loaded.Config.Repositories.Plans != "current" {
		return selection{}, &Error{Code: "dedicated_repository_unavailable", Message: "dedicated plans repositories are not implemented yet"}
	}
	client := git.New(s.Executor, loaded.RepositoryRoot, s.Env)
	root, err := s.workspaceRoot(ctx, client)
	if err != nil {
		return selection{}, err
	}
	id := req.PlanID
	if id != "" && !validID(id) {
		return selection{}, &Error{Code: "invalid_plan", Message: "--plan must be an existing plan slug, not a title or path"}
	}
	pattern := loaded.Config.Branch.Pattern
	if id == "" {
		caller := git.New(s.Executor, req.Dir, s.Env)
		common, callerErr := caller.CommonDirectory(ctx)
		want, err := client.CommonDirectory(ctx)
		if err != nil {
			return selection{}, err
		}
		if callerErr == nil && common == want {
			branch, err := caller.CurrentBranch(ctx)
			if err != nil {
				return selection{}, err
			}
			id = branchID(pattern, branch)
		}
	}
	if id == "" {
		branches, err := client.Branches(ctx)
		if err != nil {
			return selection{}, err
		}
		var candidates []string
		for _, branch := range branches {
			if slug := branchID(pattern, branch); slug != "" {
				candidates = append(candidates, slug)
			}
		}
		switch len(candidates) {
		case 0:
			return selection{}, &Error{Code: "plan_required", Message: "no local plan found; run planctl create first"}
		case 1:
			id = candidates[0]
		default:
			return selection{}, &Error{Code: "ambiguous_plan", Message: "multiple plans found; use --plan with one of: " + strings.Join(candidates, ", ")}
		}
	}
	p := Plan{
		ID: id, Branch: strings.ReplaceAll(pattern, "{slug}", id),
		Path:          filepath.ToSlash(filepath.Join(loaded.Config.Plan.Directory, id+".md")),
		WorkspacePath: filepath.Join(root, id),
	}
	p.AbsolutePath = filepath.Join(p.WorkspacePath, filepath.FromSlash(p.Path))
	worktrees, err := client.Worktrees(ctx)
	if err != nil {
		return selection{}, err
	}
	found := false
	for _, worktree := range worktrees {
		if worktree.Branch != p.Branch {
			continue
		}
		if filepath.Clean(worktree.Path) != p.WorkspacePath {
			return selection{}, &Error{Code: "workspace_conflict", Message: fmt.Sprintf("plan branch is checked out outside its managed workspace: %s", worktree.Path)}
		}
		found = !worktree.Prunable
	}
	if !found {
		return selection{}, &Error{Code: "workspace_missing", Message: "plan has no available managed worktree; create the plan or repair its worktree first"}
	}
	base, baseCommit, err := resolveBase(ctx, client, loaded.Config.Branch.Base)
	if err != nil {
		return selection{}, err
	}
	p.Base = base
	return selection{plan: p, config: loaded, client: client, baseCommit: baseCommit}, nil
}

func branchID(pattern, branch string) string {
	prefix, suffix, _ := strings.Cut(pattern, "{slug}")
	if !strings.HasPrefix(branch, prefix) || !strings.HasSuffix(branch, suffix) || len(branch) < len(prefix)+len(suffix) {
		return ""
	}
	id := branch[len(prefix) : len(branch)-len(suffix)]
	if !validID(id) {
		return ""
	}
	return id
}

func validID(id string) bool {
	slug, err := Slug(id)
	return err == nil && slug == id
}

// Plan text is not validated. A first-line heading supplies the display title;
// otherwise use the slug. Files and their parent directories cannot be symlinks.
func readPlan(p Plan) (string, error) {
	if !inside(p.WorkspacePath, p.AbsolutePath) {
		return "", &Error{Code: "unsafe_path", Message: "plan path escapes its workspace"}
	}
	for current := p.AbsolutePath; ; current = filepath.Dir(current) {
		info, err := os.Lstat(current)
		if err != nil {
			return "", &Error{Code: "plan_not_found", Message: "could not read the plan file at " + p.AbsolutePath, Cause: err}
		}
		if info.Mode()&os.ModeSymlink != 0 || (current == p.AbsolutePath && !info.Mode().IsRegular()) || (current != p.AbsolutePath && !info.IsDir()) {
			return "", &Error{Code: "unsafe_path", Message: "plan must be a regular file inside real workspace directories"}
		}
		if current == p.WorkspacePath {
			break
		}
	}
	data, err := os.ReadFile(p.AbsolutePath)
	if err != nil {
		return "", err
	}
	first, _, _ := strings.Cut(strings.TrimSpace(string(data)), "\n")
	if strings.HasPrefix(first, "# ") && strings.TrimSpace(first[2:]) != "" {
		return strings.TrimSpace(first[2:]), nil
	}
	return p.ID, nil
}
