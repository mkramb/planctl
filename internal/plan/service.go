package plan

import (
	"context"
	"fmt"

	"github.com/mkramb/planctl/internal/config"
	"github.com/mkramb/planctl/internal/git"
	"github.com/mkramb/planctl/internal/github"
	"github.com/mkramb/planctl/internal/process"
)

type Service struct {
	Executor process.Executor
	Env      []string
	// CacheDir is injectable so tests never write into the user's cache.
	CacheDir string
	Provider *github.Provider
}

func (s Service) loadConfiguration(ctx context.Context, dir, override string) (config.Loaded, error) {
	loader := config.Loader{Executor: s.Executor, Env: s.Env}
	return loader.LoadOrDefaults(ctx, dir, override)
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
	// Publishing stays offline. Without origin/HEAD, use an unambiguous
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
