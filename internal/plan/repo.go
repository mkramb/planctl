package plan

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mkramb/planctl/internal/config"
	"github.com/mkramb/planctl/internal/git"
)

// target describes where plans live: the implementation repository itself
// (plans == "current") or a dedicated plans repository.
type target struct {
	impl      *git.Client
	plans     *git.Client
	implRepo  string
	plansRepo string
	dedicated bool
	implOwner string
	implName  string
}

// resolveTarget builds the plan target for a loaded configuration. When
// needImplName is false and plans are local, the implementation repository name
// is left unresolved so creation works in repositories without an origin.
func (s Service) resolveTarget(ctx context.Context, loaded config.Loaded, needImplName bool) (target, error) {
	impl := git.New(s.Executor, loaded.RepositoryRoot, s.Env)
	if loaded.Config.Repositories.Plans == "current" {
		t := target{impl: impl, plans: impl}
		if needImplName {
			name, err := s.implRepoName(ctx, impl)
			if err != nil {
				return target{}, err
			}
			t.implRepo, t.plansRepo = name, name
		}
		return t, nil
	}
	name, err := s.implRepoName(ctx, impl)
	if err != nil {
		return target{}, err
	}
	cloneDir, err := s.ensurePlansClone(ctx, loaded.Config.Repositories.Plans)
	if err != nil {
		return target{}, err
	}
	owner, repo, _ := strings.Cut(name, "/")
	return target{
		impl: impl, plans: git.New(s.Executor, cloneDir, s.Env),
		implRepo: name, plansRepo: loaded.Config.Repositories.Plans,
		dedicated: true, implOwner: owner, implName: repo,
	}, nil
}

func (s Service) implRepoName(ctx context.Context, impl *git.Client) (string, error) {
	if s.Provider == nil {
		return "", &Error{Code: "provider_unavailable", Message: "GitHub integration is not implemented yet"}
	}
	remote, err := impl.Origin(ctx)
	if err != nil {
		return "", &Error{Code: "invalid_remote", Message: "configure origin with the same single fetch and push URL", Cause: err}
	}
	name, err := s.Provider.ResolveRepository(ctx, remote)
	if err != nil {
		return "", &Error{Code: "repository_lookup_failed", Message: "could not resolve origin's review repository", Cause: err}
	}
	return name, nil
}

// namespacePrefix namespaces branches and paths for dedicated repositories so
// two implementation repositories can share one plans repository without
// colliding.
func (t target) namespacePrefix() string {
	if !t.dedicated {
		return ""
	}
	return t.implOwner + "/" + t.implRepoName() + "/"
}

func (t target) implRepoName() string { return t.implName }

func (t target) branchFor(pattern, id string) string {
	if t.dedicated {
		id = t.namespacePrefix() + id
	}
	return strings.ReplaceAll(pattern, "{slug}", id)
}

func (t target) pathFor(directory, id string) string {
	dir := directory
	if t.dedicated {
		dir = filepath.Join(directory, t.implOwner, t.implRepoName())
	}
	return filepath.ToSlash(filepath.Join(dir, id+".md"))
}

// branchID reverses branchFor, returning the slug for a branch that belongs to
// this target's implementation repository.
func (t target) branchID(pattern, branch string) string {
	before, after, _ := strings.Cut(pattern, "{slug}")
	if t.dedicated {
		before += t.namespacePrefix()
	}
	if !strings.HasPrefix(branch, before) || !strings.HasSuffix(branch, after) || len(branch) < len(before)+len(after) {
		return ""
	}
	id := branch[len(before) : len(branch)-len(after)]
	if !validID(id) {
		return ""
	}
	return id
}

// ensurePlansClone clones or reuses the dedicated plans repository.
func (s Service) ensurePlansClone(ctx context.Context, name string) (string, error) {
	if s.Provider == nil {
		return "", &Error{Code: "provider_unavailable", Message: "GitHub integration is not implemented yet"}
	}
	url, err := s.Provider.ResolvePlansRepository(ctx, name)
	if err != nil {
		return "", &Error{Code: "repository_lookup_failed", Message: "could not resolve plans repository " + name, Cause: err}
	}
	cache, err := s.cacheDir()
	if err != nil {
		return "", err
	}
	slug, err := Slug(name)
	if err != nil {
		slug = "repository"
	}
	key := sha256.Sum256([]byte(name))
	dir := filepath.Join(cache, "planctl", "plans", fmt.Sprintf("%s-%x", slug, key[:6]))
	if err := makeDirectories(filepath.Dir(dir)); err != nil {
		return "", err
	}
	if err := s.verifyClone(ctx, dir, url); err == nil {
		// Reuse the clone and refresh it; tolerate being offline.
		client := git.New(s.Executor, dir, s.Env)
		_, _ = client.Run(ctx, "fetch", "--quiet", "origin")
		return dir, nil
	}
	if _, err := os.Lstat(dir); err == nil {
		return "", &Error{Code: "workspace_conflict", Message: "plans repository cache path exists but is not a usable clone: " + dir}
	}
	if err := makeDirectories(filepath.Dir(dir)); err != nil {
		return "", err
	}
	client := git.New(s.Executor, filepath.Dir(dir), s.Env)
	if _, err := client.Run(ctx, "clone", "--quiet", "--", url, dir); err != nil {
		return "", &Error{Code: "plans_clone_failed", Message: "could not clone plans repository " + name, Cause: err}
	}
	return dir, nil
}

func (s Service) verifyClone(ctx context.Context, dir, url string) error {
	client := git.New(s.Executor, dir, s.Env)
	if _, err := client.Run(ctx, "rev-parse", "--git-dir"); err != nil {
		return err
	}
	remote, err := client.RemoteURL(ctx, "origin")
	if err != nil {
		return err
	}
	if strings.TrimSuffix(remote, ".git") != strings.TrimSuffix(url, ".git") {
		return fmt.Errorf("clone points at a different remote")
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
