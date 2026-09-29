package plan

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mkramb/planctl/internal/config"
	"github.com/mkramb/planctl/internal/git"
)

// PublishRequest carries everything needed to select files and publish them.
type PublishRequest struct {
	Dir        string
	ConfigPath string
	// Files are explicit file or directory paths relative to the working
	// directory. When empty, changed files are auto-detected.
	Files     []string
	Title     string
	Base      string
	Reviewers []string
}

// submission is the resolved, publishable state for one review.
type submission struct {
	plan       Plan
	config     config.Loaded
	source     *git.Client
	sourceRoot string
	baseCommit string
	repository string
}

func (s Service) resolve(ctx context.Context, req PublishRequest) (submission, error) {
	loaded, err := s.loadConfiguration(ctx, req.Dir, req.ConfigPath)
	if err != nil {
		return submission{}, err
	}
	root := loaded.RepositoryRoot
	source := git.New(s.Executor, root, s.Env)

	base := req.Base
	if base == "" {
		base = loaded.Config.Branch.Base
	}
	baseName, baseCommit, err := resolveBase(ctx, source, base)
	if err != nil {
		return submission{}, err
	}

	files, err := s.selectFiles(ctx, source, root, req.Dir, req.Files)
	if err != nil {
		return submission{}, err
	}

	title := req.Title
	if title == "" {
		title = filepath.Base(files[0])
	}
	slug := slugFor(files[0])
	branch := strings.ReplaceAll(loaded.Config.Branch.Pattern, "{slug}", slug)
	if err := source.CheckBranch(ctx, branch); err != nil {
		return submission{}, &Error{Code: "invalid_branch", Message: "branch pattern and file name do not produce a valid Git branch name", Cause: err}
	}

	workspaceRoot, err := s.workspaceRoot(ctx, source)
	if err != nil {
		return submission{}, err
	}
	if inside(root, workspaceRoot) {
		return submission{}, &Error{Code: "invalid_workspace", Message: "the user cache must be outside the repository checkout"}
	}

	repository, err := s.implRepoName(ctx, source)
	if err != nil {
		return submission{}, err
	}

	p := Plan{
		ID: slug, Title: title, Branch: branch, Base: baseName,
		Files: files, WorkspacePath: filepath.Join(workspaceRoot, slug),
	}
	return submission{
		plan: p, config: loaded, source: source, sourceRoot: root,
		baseCommit: baseCommit, repository: repository,
	}, nil
}

// implRepoName resolves the origin remote to an owner/repository name.
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

// selectFiles resolves explicit file/directory arguments, or auto-detects
// changed files when no arguments are given. Results are repository-root
// relative, deduplicated, and sorted.
func (s Service) selectFiles(ctx context.Context, source *git.Client, root, dir string, args []string) ([]string, error) {
	seen := map[string]bool{}
	var files []string
	add := func(path string) {
		path = filepath.ToSlash(path)
		if path != "" && !seen[path] {
			seen[path] = true
			files = append(files, path)
		}
	}
	if len(args) == 0 {
		changed, err := source.StatusFiles(ctx)
		if err != nil {
			return nil, err
		}
		for _, path := range changed {
			add(path)
		}
	} else {
		for _, arg := range args {
			abs := arg
			if !filepath.IsAbs(abs) {
				abs = filepath.Join(dir, arg)
			}
			abs, err := filepath.Abs(abs)
			if err != nil {
				return nil, err
			}
			abs, err = filepath.EvalSymlinks(abs)
			if err != nil {
				return nil, &Error{Code: "path_not_found", Message: fmt.Sprintf("path %q does not exist", arg)}
			}
			rel, err := filepath.Rel(root, abs)
			if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
				return nil, &Error{Code: "invalid_path", Message: fmt.Sprintf("path %q is outside the repository", arg)}
			}
			info, err := os.Stat(abs)
			if err != nil {
				return nil, &Error{Code: "path_not_found", Message: fmt.Sprintf("path %q does not exist", arg)}
			}
			if info.IsDir() {
				listed, err := source.ListFiles(ctx, rel)
				if err != nil {
					return nil, err
				}
				for _, path := range listed {
					add(path)
				}
			} else {
				add(rel)
			}
		}
	}
	if len(files) == 0 {
		return nil, &Error{Code: "no_files", Message: "no files selected; pass file or directory paths, or make changes first"}
	}
	sort.Strings(files)
	return files, nil
}
