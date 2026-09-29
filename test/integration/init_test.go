package integration_test

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/mkramb/planctl/internal/config"
	"github.com/mkramb/planctl/internal/git"
	"github.com/mkramb/planctl/internal/output"
	"github.com/mkramb/planctl/internal/process"
	"github.com/mkramb/planctl/test/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInitFromNestedDirectory(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnvironment(t)
	env.WriteFile(t, "src/nested/work.txt", "uncommitted work\n")
	require.NoError(t, env.Git.Add(t.Context(), "src/nested/work.txt"))
	before := env.GitRun(t, "diff", "--cached")
	head := env.GitRun(t, "rev-parse", "HEAD")
	nested := filepath.Join(env.Root, "src", "nested")
	result := env.RunAt(t, nested, "init", "--json")
	result.RequireSuccess(t)
	assert.Empty(t, result.Stderr)
	created := testutil.DecodeJSON[output.InitResult](t, result)
	root := canonicalPath(t, env.Root)
	assert.Equal(t, 1, created.Version)
	assert.Equal(t, root, created.Config.RepositoryRoot)
	assert.Equal(t, filepath.Join(root, config.Filename), created.Config.Path)
	contents, err := os.ReadFile(created.Config.Path)
	require.NoError(t, err)
	assert.Equal(t, "version: 1\n\nplan:\n  retention: pr-only\n", string(contents))
	assert.NoFileExists(t, filepath.Join(nested, config.Filename))
	assert.Equal(t, before, env.GitRun(t, "diff", "--cached"), "preserve staged changes")
	assert.Equal(t, head, env.GitRun(t, "rev-parse", "HEAD"), "init must not commit")
	loaded, err := (config.Loader{Executor: process.Runner{}, Env: env.Env}).Load(t.Context(), nested, "")
	require.NoError(t, err)
	assert.Equal(t, created.Config, loaded.Location)
	assert.False(t, loaded.Config.PullRequest.Draft)
	assert.Equal(t, "github", loaded.Config.Review.Provider)
}

func TestInitHumanOutputAndExistingConfig(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnvironment(t)
	result := env.Run(t, "init")
	result.RequireSuccess(t)
	assert.Equal(t, "Created "+filepath.Join(canonicalPath(t, env.Root), config.Filename)+"\n", result.Stdout)
	assert.Empty(t, result.Stderr)
	env.WriteFile(t, config.Filename, "# keep my configuration\nversion: 1\n")
	failed := env.Run(t, "init", "--json")
	assertConfigFailure(t, failed, "config_exists")
	contents, err := os.ReadFile(filepath.Join(env.Root, config.Filename))
	require.NoError(t, err)
	assert.Equal(t, "# keep my configuration\nversion: 1\n", string(contents))
}

func TestInitInEmptyRepository(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnvironment(t)
	root := t.TempDir()
	client := git.New(process.Runner{}, root, env.Env)
	_, err := client.Run(t.Context(), "init", "--initial-branch=master")
	require.NoError(t, err)
	result := env.RunAt(t, root, "init", "--json")
	result.RequireSuccess(t)
	created := testutil.DecodeJSON[output.InitResult](t, result)
	assert.Equal(t, canonicalPath(t, root), created.Config.RepositoryRoot)
	assert.FileExists(t, filepath.Join(root, config.Filename))
	_, err = client.Head(t.Context())
	assert.Error(t, err, "init must work without creating a commit")
}

func TestInitExplicitConfig(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnvironment(t)
	other := testutil.NewEnvironment(t)
	path := filepath.Join(other.Root, "custom.yaml")
	result := env.Run(t, "--config", path, "init", "--json")
	result.RequireSuccess(t)
	created := testutil.DecodeJSON[output.InitResult](t, result)
	assert.Equal(t, filepath.Join(canonicalPath(t, other.Root), "custom.yaml"), created.Config.Path)
	assert.Equal(t, canonicalPath(t, other.Root), created.Config.RepositoryRoot)
	assert.NoFileExists(t, filepath.Join(env.Root, config.Filename))
	loaded, err := (config.Loader{Executor: process.Runner{}, Env: env.Env}).Load(t.Context(), env.Root, path)
	require.NoError(t, err)
	assert.Equal(t, created.Config, loaded.Location)

	env.WriteFile(t, "nested/keep.txt", "keep")
	result = env.RunAt(t, filepath.Join(env.Root, "nested"), "init", "--config", "../alternate.yaml", "--json")
	result.RequireSuccess(t)
	assert.FileExists(t, filepath.Join(env.Root, "alternate.yaml"))
}

func TestInitInLinkedWorktree(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnvironment(t)
	linked := filepath.Join(filepath.Dir(env.Root), "feature checkout")
	env.GitRun(t, "worktree", "add", "-b", "feature/sso", linked)
	result := env.RunAt(t, linked, "init", "--json")
	result.RequireSuccess(t)
	created := testutil.DecodeJSON[output.InitResult](t, result)
	assert.Equal(t, canonicalPath(t, linked), created.Config.RepositoryRoot)
	assert.FileExists(t, filepath.Join(linked, config.Filename))
	assert.NoFileExists(t, filepath.Join(env.Root, config.Filename))
	assert.Equal(t, "main", env.GitRun(t, "branch", "--show-current"))
}

func TestInitErrors(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnvironment(t)
	assertConfigFailure(t, env.RunAt(t, t.TempDir(), "init", "--json"), "not_git_repository")
	assertConfigFailure(t, env.RunAt(t, env.Remote, "init", "--json"), "not_git_repository")
	assertConfigFailure(t, env.Run(t, "init", "--config", "missing/config.yaml", "--json"), "config_path_invalid")
	assertConfigFailure(t, env.Run(t, "init", "extra", "--json"), "invalid_arguments")
	assert.NoFileExists(t, filepath.Join(env.Root, config.Filename))

	require.NoError(t, os.Mkdir(filepath.Join(env.Root, config.Filename), 0o755))
	assertConfigFailure(t, env.Run(t, "init", "--json"), "config_exists")
}

func TestInitDoesNotFollowExistingSymlink(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnvironment(t)
	target := filepath.Join(env.Root, "missing-target")
	if err := os.Symlink(target, filepath.Join(env.Root, config.Filename)); err != nil {
		t.Skipf("symlinks are unavailable: %v", err)
	}
	assertConfigFailure(t, env.Run(t, "init", "--json"), "config_exists")
	assert.NoFileExists(t, target)
	link, err := os.Readlink(filepath.Join(env.Root, config.Filename))
	require.NoError(t, err)
	assert.Equal(t, target, link)
}

func TestConcurrentInitCreatesOnlyOneConfiguration(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnvironment(t)
	var results [2]testutil.Result
	var workers sync.WaitGroup
	for i := range results {
		workers.Go(func() { results[i] = env.Run(t, "init", "--json") })
	}
	workers.Wait()
	var successes, conflicts int
	for _, result := range results {
		if result.ExitCode == 0 {
			successes++
		} else {
			assertConfigFailure(t, result, "config_exists")
			conflicts++
		}
	}
	assert.Equal(t, 1, successes)
	assert.Equal(t, 1, conflicts)
	_, err := (config.Loader{Executor: process.Runner{}, Env: env.Env}).Load(t.Context(), env.Root, "")
	require.NoError(t, err)
}

func canonicalPath(t *testing.T, path string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(path)
	require.NoError(t, err)
	return resolved
}

func assertConfigFailure(t *testing.T, result testutil.Result, code string) {
	t.Helper()
	wantExit := 1
	if code == "invalid_arguments" {
		wantExit = 2
	}
	assert.Equal(t, wantExit, result.ExitCode)
	assert.Empty(t, result.Stderr)
	failure := testutil.DecodeJSON[output.ErrorResult](t, result)
	assert.Equal(t, 1, failure.Version)
	assert.Equal(t, code, failure.Error.Code)
	assert.NotEmpty(t, failure.Error.Message)
}
