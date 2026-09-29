package integration_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mkramb/planctl/internal/git"
	"github.com/mkramb/planctl/internal/output"
	"github.com/mkramb/planctl/internal/process"
	"github.com/mkramb/planctl/test/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateUsesCachedRemoteBase(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnvironment(t)
	env.Run(t, "init").RequireSuccess(t)
	remote := env.GitRun(t, "rev-parse", "origin/main")
	env.WriteFile(t, "unpublished.go", "package unpublished\n")
	env.GitRun(t, "add", "unpublished.go")
	env.GitRun(t, "commit", "-m", "Unpublished implementation")
	created := createPlan(t, env, "Add SSO")
	client := git.New(process.Runner{}, created.Plan.WorkspacePath, env.Env)
	head, err := client.Head(t.Context())
	require.NoError(t, err)
	assert.Equal(t, remote, head)
	assert.NoFileExists(t, filepath.Join(created.Plan.WorkspacePath, "unpublished.go"))
}

func TestCreateUsesRemoteDefaultBranch(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnvironment(t)
	env.Run(t, "init").RequireSuccess(t)
	env.GitRun(t, "update-ref", "refs/remotes/origin/trunk", "main")
	env.GitRun(t, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/trunk")
	created := createPlan(t, env, "Add SSO")
	assert.Equal(t, "trunk", created.Plan.Base)
}

func TestCreateRequiresUnambiguousBase(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnvironment(t)
	env.Run(t, "init").RequireSuccess(t)
	env.GitRun(t, "branch", "master", "main")
	assertConfigFailure(t, env.Run(t, "create", "Add SSO", "--json"), "ambiguous_base")
	env.WriteFile(t, ".planctl.yaml", "version: 1\nbranch:\n  base: master\n")
	created := createPlan(t, env, "Add SSO")
	assert.Equal(t, "master", created.Plan.Base)
}

func TestCreateUnknownBaseAndEmptyRepository(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnvironment(t)
	env.WriteFile(t, ".planctl.yaml", "version: 1\nbranch:\n  base: missing\n")
	assertConfigFailure(t, env.Run(t, "create", "Add SSO", "--json"), "base_not_found")
	root := t.TempDir()
	client := git.New(process.Runner{}, root, env.Env)
	_, err := client.Run(t.Context(), "init", "--initial-branch=main")
	require.NoError(t, err)
	env.RunAt(t, root, "init").RequireSuccess(t)
	assertConfigFailure(t, env.RunAt(t, root, "create", "Add SSO", "--json"), "base_not_found")
}

func TestCreatePreservesExistingBranchWork(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnvironment(t)
	env.Run(t, "init").RequireSuccess(t)
	env.WriteFile(t, "unrelated.txt", "existing work")
	env.GitRun(t, "add", "unrelated.txt")
	env.GitRun(t, "commit", "-m", "Existing branch work")
	env.GitRun(t, "branch", "plan/add-sso")
	head := env.GitRun(t, "rev-parse", "plan/add-sso")
	assertConfigFailure(t, env.Run(t, "create", "Add SSO", "--json"), "branch_conflict")
	assert.Equal(t, head, env.GitRun(t, "rev-parse", "plan/add-sso"))
}

func TestIndependentClonesUseSeparateWorkspaces(t *testing.T) {
	t.Parallel()
	first := testutil.NewEnvironment(t)
	second := testutil.NewEnvironment(t)
	second.CacheDir = first.CacheDir
	first.Run(t, "init").RequireSuccess(t)
	second.Run(t, "init").RequireSuccess(t)
	a := createPlan(t, first, "Add SSO")
	b := createPlan(t, second, "Add SSO")
	assert.NotEqual(t, a.Plan.WorkspacePath, b.Plan.WorkspacePath)
	assert.FileExists(t, a.Plan.AbsolutePath)
	assert.FileExists(t, b.Plan.AbsolutePath)
}

func TestManagedWorktreeRequiresExplicitConfigWhenAmbiguous(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnvironment(t)
	env.Run(t, "init").RequireSuccess(t)
	created := createPlan(t, env, "Add SSO")
	linked := filepath.Join(filepath.Dir(env.Root), "another checkout")
	env.GitRun(t, "worktree", "add", "-b", "feature/another", linked)
	require.NoError(t, os.WriteFile(filepath.Join(linked, ".planctl.yaml"), []byte("version: 1\nreview:\n  required_approvals: 2\n"), 0o644))
	assertConfigFailure(t, env.RunAt(t, created.Plan.WorkspacePath, "create", "Another plan", "--json"), "ambiguous_config")
	result := env.RunAt(t, created.Plan.WorkspacePath, "create", "Another plan", "--config", filepath.Join(env.Root, ".planctl.yaml"), "--json")
	result.RequireSuccess(t)
}

func TestCreateFromWorktreeWithBareBackingRepository(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnvironment(t)
	linked := filepath.Join(filepath.Dir(env.Root), "bare backed checkout")
	bare := git.New(process.Runner{}, env.Remote, env.Env)
	_, err := bare.Run(t.Context(), "worktree", "add", "-b", "feature/work", linked, "main")
	require.NoError(t, err)
	env.RunAt(t, linked, "init").RequireSuccess(t)
	result := env.RunAt(t, linked, "create", "Add SSO", "--json")
	result.RequireSuccess(t)
	created := testutil.DecodeJSON[output.CreateResult](t, result)
	assertConfigFailure(t, env.RunAt(t, created.Plan.WorkspacePath, "create", "Add SSO", "--json"), "plan_exists")
}
