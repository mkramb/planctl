package integration_test

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/mkramb/planctl/internal/git"
	"github.com/mkramb/planctl/internal/output"
	"github.com/mkramb/planctl/internal/process"
	"github.com/mkramb/planctl/test/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateIsolatesPlanFromImplementation(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnvironment(t)
	env.Run(t, "init").RequireSuccess(t)
	base := env.GitRun(t, "rev-parse", "main")
	env.GitRun(t, "switch", "-c", "feature/sso")
	env.WriteFile(t, "implementation.go", "package implementation\n")
	env.GitRun(t, "add", "implementation.go")
	env.GitRun(t, "commit", "-m", "Implementation work")
	env.WriteFile(t, "README.md", "staged implementation edits\n")
	env.GitRun(t, "add", "README.md")
	env.WriteFile(t, "notes.txt", "untracked notes\n")
	head := env.GitRun(t, "rev-parse", "HEAD")
	status := env.GitRun(t, "status", "--porcelain")
	staged := env.GitRun(t, "diff", "--cached")

	created := createPlan(t, env, "Add SSO")
	p := created.Plan
	assert.Equal(t, 1, created.Version)
	assert.Equal(t, "add-sso", p.ID)
	assert.Equal(t, "Add SSO", p.Title)
	assert.Equal(t, ".plans/add-sso.md", p.Path)
	assert.Equal(t, "plan/add-sso", p.Branch)
	assert.Equal(t, "main", p.Base)
	assert.Equal(t, filepath.Join(p.WorkspacePath, ".plans", "add-sso.md"), p.AbsolutePath)
	assert.Contains(t, p.WorkspacePath, filepath.Join("planctl", "worktrees"))
	assert.NotContains(t, p.WorkspacePath, canonicalPath(t, env.Root)+string(filepath.Separator))
	contents, err := os.ReadFile(p.AbsolutePath)
	require.NoError(t, err)
	assert.Equal(t, "# Add SSO\n\n## Context\n\n## Proposed Approach\n\n## Implementation\n\n## Testing\n\n## Open Questions\n", string(contents))
	client := git.New(process.Runner{}, p.WorkspacePath, env.Env)
	planHead, err := client.Head(t.Context())
	require.NoError(t, err)
	assert.Equal(t, base, planHead, "branch must start at base, with no plan commit yet")
	branch, err := client.CurrentBranch(t.Context())
	require.NoError(t, err)
	assert.Equal(t, p.Branch, branch)
	assert.NoFileExists(t, filepath.Join(p.WorkspacePath, "implementation.go"))
	assert.NoFileExists(t, filepath.Join(env.Root, ".plans", "add-sso.md"))
	assert.Equal(t, "feature/sso", env.GitRun(t, "branch", "--show-current"))
	assert.Equal(t, head, env.GitRun(t, "rev-parse", "HEAD"))
	assert.Equal(t, status, env.GitRun(t, "status", "--porcelain"))
	assert.Equal(t, staged, env.GitRun(t, "diff", "--cached"))
}

func TestCreateCustomConfigurationAndHumanOutput(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnvironment(t)
	env.WriteFile(t, "custom.yaml", "version: 1\nplan:\n  directory: design/plans\nbranch:\n  base: main\n  pattern: proposal/{slug}\n")
	env.WriteFile(t, "src/nested/file.txt", "work")
	result := env.RunAt(t, filepath.Join(env.Root, "src", "nested"), "--config", "../../custom.yaml", "create", "Add SSO")
	result.RequireSuccess(t)
	assert.Empty(t, result.Stderr)
	assert.Contains(t, result.Stdout, "Created plan: Add SSO\n")
	assert.Contains(t, result.Stdout, "Branch: proposal/add-sso\n")
	worktrees, err := env.Git.Worktrees(t.Context())
	require.NoError(t, err)
	require.Len(t, worktrees, 2)
	for _, worktree := range worktrees {
		if worktree.Branch == "proposal/add-sso" {
			path := filepath.Join(worktree.Path, "design", "plans", "add-sso.md")
			assert.FileExists(t, path)
			assert.Contains(t, result.Stdout, "Plan file: "+path+"\n")
		}
	}
}

func TestCreateDuplicatePreservesAgentEdits(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnvironment(t)
	env.Run(t, "init").RequireSuccess(t)
	created := createPlan(t, env, "Add SSO")
	const revised = "# Add SSO\n\nAgent's revised plan with a Mermaid diagram.\n"
	require.NoError(t, os.WriteFile(created.Plan.AbsolutePath, []byte(revised), 0o644))
	assertConfigFailure(t, env.Run(t, "create", "Add SSO!", "--json"), "plan_exists")
	contents, err := os.ReadFile(created.Plan.AbsolutePath)
	require.NoError(t, err)
	assert.Equal(t, revised, string(contents))
	worktrees, err := env.Git.Worktrees(t.Context())
	require.NoError(t, err)
	assert.Len(t, worktrees, 2)
}

func TestCreateFromImplementationAndManagedWorktrees(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnvironment(t)
	env.Run(t, "init").RequireSuccess(t)
	linked := filepath.Join(filepath.Dir(env.Root), "feature checkout")
	env.GitRun(t, "worktree", "add", "-b", "feature/sso", linked)
	env.RunAt(t, linked, "init").RequireSuccess(t)
	first := createPlan(t, env, "First plan")
	result := env.RunAt(t, linked, "create", "Second plan", "--json")
	result.RequireSuccess(t)
	second := testutil.DecodeJSON[output.CreateResult](t, result)
	assert.Equal(t, filepath.Dir(first.Plan.WorkspacePath), filepath.Dir(second.Plan.WorkspacePath))
	assertConfigFailure(t, env.RunAt(t, linked, "create", "First plan", "--json"), "plan_exists")
	// init's config is uncommitted, so this also exercises Git-based config recovery.
	assertConfigFailure(t, env.RunAt(t, first.Plan.WorkspacePath, "create", "First plan", "--json"), "plan_exists")
	result = env.RunAt(t, first.Plan.WorkspacePath, "create", "Third plan", "--json")
	result.RequireSuccess(t)
	third := testutil.DecodeJSON[output.CreateResult](t, result)
	assert.Equal(t, filepath.Dir(first.Plan.WorkspacePath), filepath.Dir(third.Plan.WorkspacePath))
	assert.Equal(t, "main", env.GitRun(t, "branch", "--show-current"))
}

func TestCreateDoesNotReuseUnmanagedWorktree(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnvironment(t)
	env.Run(t, "init").RequireSuccess(t)
	manual := filepath.Join(filepath.Dir(env.Root), "manual worktree")
	env.GitRun(t, "worktree", "add", "-b", "plan/add-sso", manual)
	assertConfigFailure(t, env.Run(t, "create", "Add SSO", "--json"), "workspace_conflict")
	assert.NoFileExists(t, filepath.Join(manual, ".plans", "add-sso.md"))
	worktrees, err := env.Git.Worktrees(t.Context())
	require.NoError(t, err)
	assert.Len(t, worktrees, 2)
}

func TestCreateDoesNotOverwriteWorkspacePath(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnvironment(t)
	env.Run(t, "init").RequireSuccess(t)
	first := createPlan(t, env, "First")
	occupied := filepath.Join(filepath.Dir(first.Plan.WorkspacePath), "occupied")
	require.NoError(t, os.WriteFile(occupied, []byte("unrelated file"), 0o644))
	assertConfigFailure(t, env.Run(t, "create", "Occupied", "--json"), "workspace_conflict")
	contents, err := os.ReadFile(occupied)
	require.NoError(t, err)
	assert.Equal(t, "unrelated file", string(contents))
	_, exists, err := env.Git.ResolveCommit(t.Context(), "refs/heads/plan/occupied")
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestCreateRecoversIncompleteSetup(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnvironment(t)
	env.Run(t, "init").RequireSuccess(t)
	// A previous attempt created the branch but not its worktree.
	env.GitRun(t, "branch", "plan/add-sso", "main")
	first := createPlan(t, env, "Add SSO")
	// A previous attempt created its worktree but did not write the template.
	require.NoError(t, os.Remove(first.Plan.AbsolutePath))
	second := createPlan(t, env, "Add SSO")
	assert.Equal(t, first, second)
	assert.FileExists(t, second.Plan.AbsolutePath)
	worktrees, err := env.Git.Worktrees(t.Context())
	require.NoError(t, err)
	assert.Len(t, worktrees, 2)
}

func TestCreateRejectsExistingPlanOnBase(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnvironment(t)
	env.Run(t, "init").RequireSuccess(t)
	env.WriteFile(t, ".plans/add-sso.md", "# Existing retained plan\n")
	env.GitRun(t, "add", ".plans/add-sso.md")
	env.GitRun(t, "commit", "-m", "Retain plan")
	env.GitRun(t, "push", "origin", "main")
	assertConfigFailure(t, env.Run(t, "create", "Add SSO", "--json"), "plan_exists")
	worktrees, err := env.Git.Worktrees(t.Context())
	require.NoError(t, err)
	assert.Len(t, worktrees, 1)
}

func TestCreateRejectsPlanDirectorySymlink(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnvironment(t)
	env.Run(t, "init").RequireSuccess(t)
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(env.Root, ".plans")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	env.GitRun(t, "add", ".plans")
	env.GitRun(t, "commit", "-m", "Directory symlink fixture")
	env.GitRun(t, "push", "origin", "main")
	assertConfigFailure(t, env.Run(t, "create", "Add SSO", "--json"), "unsafe_path")
	assert.NoFileExists(t, filepath.Join(outside, "add-sso.md"))
}

func TestConcurrentCreate(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnvironment(t)
	env.Run(t, "init").RequireSuccess(t)
	var results [2]testutil.Result
	var workers sync.WaitGroup
	for i := range results {
		workers.Go(func() { results[i] = env.Run(t, "create", "Add SSO", "--json") })
	}
	workers.Wait()
	var successes int
	for _, result := range results {
		if result.ExitCode == 0 {
			successes++
		} else {
			assert.Equal(t, 1, result.ExitCode)
			failure := testutil.DecodeJSON[output.ErrorResult](t, result)
			assert.Contains(t, []string{"plan_busy", "plan_exists"}, failure.Error.Code)
		}
	}
	assert.Equal(t, 1, successes)
	worktrees, err := env.Git.Worktrees(t.Context())
	require.NoError(t, err)
	assert.Len(t, worktrees, 2)
}

func TestCreateErrors(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnvironment(t)
	assertConfigFailure(t, env.Run(t, "create", "Add SSO", "--json"), "config_not_found")
	env.Run(t, "init").RequireSuccess(t)
	for _, title := range []string{"", "!!!", "two\nlines", "NUL"} {
		assertConfigFailure(t, env.Run(t, "create", title, "--json"), "invalid_title")
	}
	assertConfigFailure(t, env.Run(t, "create", "--json"), "invalid_arguments")
	assertConfigFailure(t, env.Run(t, "create", "Add", "SSO", "--json"), "invalid_arguments")
	env.WriteFile(t, ".planctl.yaml", "version: 1\nrepositories:\n  plans: acme/plans\n")
	assertConfigFailure(t, env.Run(t, "create", "Add SSO", "--json"), "repository_lookup_failed")
	worktrees, err := env.Git.Worktrees(t.Context())
	require.NoError(t, err)
	assert.Len(t, worktrees, 1)
}

func createPlan(t *testing.T, env *testutil.Environment, title string) output.CreateResult {
	t.Helper()
	result := env.Run(t, "create", title, "--json")
	result.RequireSuccess(t)
	assert.Empty(t, result.Stderr)
	return testutil.DecodeJSON[output.CreateResult](t, result)
}
