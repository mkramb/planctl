package integration_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mkramb/planctl/internal/git"
	"github.com/mkramb/planctl/internal/output"
	"github.com/mkramb/planctl/internal/process"
	"github.com/mkramb/planctl/internal/review"
	"github.com/mkramb/planctl/test/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seedPlansRemote creates a bare repository with an initial commit on main.
func seedPlansRemote(t *testing.T, env *testutil.Environment) string {
	t.Helper()
	base := t.TempDir()
	work := filepath.Join(base, "work")
	require.NoError(t, os.MkdirAll(work, 0o755))
	client := git.New(process.Runner{}, work, env.Env)
	_, err := client.Run(t.Context(), "init", "--initial-branch=main")
	require.NoError(t, err)
	for _, setting := range [][2]string{{"user.name", "Plans Test"}, {"user.email", "plans@example.invalid"}} {
		_, err = client.Run(t.Context(), "config", "--local", setting[0], setting[1])
		require.NoError(t, err)
	}
	require.NoError(t, os.WriteFile(filepath.Join(work, "README.md"), []byte("# Engineering plans\n"), 0o644))
	require.NoError(t, client.Add(t.Context(), "README.md"))
	require.NoError(t, client.Commit(t.Context(), "Seed plans repository"))
	bare := filepath.Join(base, "plans.git")
	require.NoError(t, os.MkdirAll(bare, 0o755))
	bareClient := git.New(process.Runner{}, bare, env.Env)
	_, err = bareClient.Run(t.Context(), "init", "--bare", "--initial-branch=main")
	require.NoError(t, err)
	_, err = client.Run(t.Context(), "remote", "add", "origin", bare)
	require.NoError(t, err)
	require.NoError(t, client.Push(t.Context(), "origin", "main"))
	return bare
}

func TestDedicatedPlansRepositoryWorkflow(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnvironment(t)
	plansRemote := seedPlansRemote(t, env)
	env.Provider.RegisterPlansRemote("acme/engineering-plans", plansRemote, env.Env)
	env.WriteFile(t, ".planctl.yaml", "version: 1\nrepositories:\n  plans: acme/engineering-plans\n")

	created := createPlan(t, env, "Add SSO")
	assert.Equal(t, "plan/acme/payments/add-sso", created.Plan.Branch)
	assert.Equal(t, ".plans/acme/payments/add-sso.md", created.Plan.Path)
	require.NoError(t, os.WriteFile(created.Plan.AbsolutePath, []byte("# Add SSO\n\n## Context\nUse SSO.\n"), 0o644))

	published := publishPlan(t, env)
	assert.Equal(t, "acme/engineering-plans", published.Review.Repository)
	// The review lives in the plans repository, not the implementation repository.
	ref := review.Ref{Provider: published.Review.Provider, Repository: "acme/engineering-plans", ID: published.Review.ID}
	got, err := env.Provider.GetReview(t.Context(), ref)
	require.NoError(t, err)
	assert.Equal(t, "plan/acme/payments/add-sso", got.HeadBranch)

	require.NoError(t, env.Provider.Approve(ref, "alice"))
	ctx := runLifecycle[output.ContextResult](t, env, "context")
	assert.True(t, ctx.Implementation.Allowed)
	assert.Equal(t, "acme/payments", ctx.Repositories.Implementation)
	assert.Equal(t, "acme/engineering-plans", ctx.Repositories.Plans)

	completion := completeApprovedPlan(t, env, "--plan", "add-sso")
	assert.Equal(t, review.Closed, completion.Review.State)
}

func TestDedicatedPlansRepositoriesDoNotCollide(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnvironment(t)
	plansRemote := seedPlansRemote(t, env)
	env.Provider.RegisterPlansRemote("acme/engineering-plans", plansRemote, env.Env)
	env.WriteFile(t, ".planctl.yaml", "version: 1\nrepositories:\n  plans: acme/engineering-plans\n")

	first := createPlan(t, env, "Add SSO")
	second := createPlan(t, env, "Add SSO Migration")
	assert.NotEqual(t, first.Plan.Branch, second.Plan.Branch)
	// A second implementation repository would use a different namespace, but the
	// plans repository itself must allow multiple plans for this one.
	assert.Equal(t, filepath.Dir(first.Plan.WorkspacePath), filepath.Dir(second.Plan.WorkspacePath))
}

func TestDedicatedPlansRepositoryReusesClone(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnvironment(t)
	plansRemote := seedPlansRemote(t, env)
	env.Provider.RegisterPlansRemote("acme/engineering-plans", plansRemote, env.Env)
	env.WriteFile(t, ".planctl.yaml", "version: 1\nrepositories:\n  plans: acme/engineering-plans\n")
	createPlan(t, env, "Add SSO")
	createPlan(t, env, "Add OAuth")
	// Both plans share one clone; no second clone is created.
	entries, err := os.ReadDir(filepath.Join(env.CacheDir, "planctl", "plans"))
	require.NoError(t, err)
	assert.Len(t, entries, 1)
}
