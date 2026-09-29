package integration_test

import (
	"os"
	"testing"

	"github.com/mkramb/planctl/internal/git"
	"github.com/mkramb/planctl/internal/output"
	"github.com/mkramb/planctl/internal/process"
	"github.com/mkramb/planctl/internal/review"
	"github.com/mkramb/planctl/test/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func completeApprovedPlan(t *testing.T, env *testutil.Environment, args ...string) output.CompleteResult {
	t.Helper()
	result := env.Run(t, append([]string{"complete", "--json"}, args...)...)
	result.RequireSuccess(t)
	assert.Empty(t, result.Stderr)
	return testutil.DecodeJSON[output.CompleteResult](t, result)
}

func approveAndComplete(t *testing.T, env *testutil.Environment, retention string) (output.CompleteResult, output.PublishResult) {
	t.Helper()
	env.Run(t, "init").RequireSuccess(t)
	if retention != "" {
		env.WriteFile(t, ".planctl.yaml", "version: 1\nplan:\n  retention: "+retention+"\n")
	}
	created := createPlan(t, env, "Add SSO")
	require.NoError(t, os.WriteFile(created.Plan.AbsolutePath, []byte("# Add SSO\n\n## Context\n"), 0o644))
	published := publishPlan(t, env)
	require.NoError(t, env.Provider.Approve(publishedRef(published), "alice"))
	ctx := runLifecycle[output.ContextResult](t, env, "context")
	require.True(t, ctx.Implementation.Allowed)
	return completeApprovedPlan(t, env), published
}

func TestCompleteCloseAndIdempotentRerun(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnvironment(t)
	completion, published := approveAndComplete(t, env, "pr-only")
	assert.Equal(t, "pr-only", completion.Retention)
	assert.Equal(t, review.Closed, completion.Review.State)
	assert.False(t, completion.PrunedRemote)
	assert.True(t, completion.WorkspaceRemoved, "clean worktree is removed after completion")
	got, err := env.Provider.GetReview(t.Context(), publishedRef(published))
	require.NoError(t, err)
	assert.Equal(t, review.Closed, got.State)
	assert.NoDirExists(t, published.Plan.WorkspacePath)
	// Rerunning requires the plan's worktree, which is gone after completion.
	assertConfigFailure(t, env.Run(t, "complete", "--json"), "workspace_missing")
	// But an explicit --plan against the already-closed review is idempotent.
	completion = completeApprovedPlan(t, env, "--plan", "add-sso")
	assert.Equal(t, review.Closed, completion.Review.State)
}

func TestCompleteRepositoryRetentionMerges(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnvironment(t)
	completion, published := approveAndComplete(t, env, "repository")
	assert.Equal(t, "repository", completion.Retention)
	assert.Equal(t, review.Merged, completion.Review.State)
	got, err := env.Provider.GetReview(t.Context(), publishedRef(published))
	require.NoError(t, err)
	assert.Equal(t, review.Merged, got.State)
}

func TestCompleteBlocksWithoutApproval(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnvironment(t)
	env.Run(t, "init").RequireSuccess(t)
	createPlan(t, env, "Add SSO")
	published := publishPlan(t, env)
	assertConfigFailure(t, env.Run(t, "complete", "--json"), "completion_blocked")
	got, err := env.Provider.GetReview(t.Context(), publishedRef(published))
	require.NoError(t, err)
	assert.Equal(t, review.Open, got.State)
	assert.DirExists(t, published.Plan.WorkspacePath)
}

func TestCompleteBlocksWithUnpublishedChanges(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnvironment(t)
	env.Run(t, "init").RequireSuccess(t)
	createPlan(t, env, "Add SSO")
	published := publishPlan(t, env)
	require.NoError(t, env.Provider.Approve(publishedRef(published), "alice"))
	require.NoError(t, os.WriteFile(published.Plan.AbsolutePath, []byte("# Add SSO\n\nUnpublished edit.\n"), 0o644))
	assertConfigFailure(t, env.Run(t, "complete", "--json"), "completion_blocked")
	got, err := env.Provider.GetReview(t.Context(), publishedRef(published))
	require.NoError(t, err)
	assert.Equal(t, review.Open, got.State)
}

func TestCompletePrunesRemoteBranch(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnvironment(t)
	completion, published := approveAndComplete(t, env, "pr-only")
	assert.False(t, completion.PrunedRemote)
	remote := git.New(process.Runner{}, env.Remote, env.Env)
	_, exists, err := remote.ResolveCommit(t.Context(), "refs/heads/"+published.Plan.Branch)
	require.NoError(t, err)
	assert.True(t, exists, "default completion keeps the remote branch")
}

func TestCompletePruneRemoteOptIn(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnvironment(t)
	env.Run(t, "init").RequireSuccess(t)
	created := createPlan(t, env, "Add SSO")
	require.NoError(t, os.WriteFile(created.Plan.AbsolutePath, []byte("# Add SSO\n\n## Context\n"), 0o644))
	published := publishPlan(t, env)
	require.NoError(t, env.Provider.Approve(publishedRef(published), "alice"))
	completion := completeApprovedPlan(t, env, "--prune-remote")
	assert.True(t, completion.PrunedRemote)
	remote := git.New(process.Runner{}, env.Remote, env.Env)
	_, exists, err := remote.ResolveCommit(t.Context(), "refs/heads/"+published.Plan.Branch)
	require.NoError(t, err)
	assert.False(t, exists, "pruned remote branch is gone")
	// An already-pruned branch counts as success on retry.
	completion = completeApprovedPlan(t, env, "--plan", "add-sso", "--prune-remote")
	assert.True(t, completion.PrunedRemote)
}

func TestCompleteKeepsDirtyWorktree(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnvironment(t)
	env.Run(t, "init").RequireSuccess(t)
	created := createPlan(t, env, "Add SSO")
	require.NoError(t, os.WriteFile(created.Plan.AbsolutePath, []byte("# Add SSO\n\n## Context\n"), 0o644))
	published := publishPlan(t, env)
	require.NoError(t, env.Provider.Approve(publishedRef(published), "alice"))
	require.NoError(t, os.WriteFile(created.Plan.WorkspacePath+"/notes.txt", []byte("keep me"), 0o644))
	completion := completeApprovedPlan(t, env)
	assert.False(t, completion.WorkspaceRemoved)
	assert.DirExists(t, published.Plan.WorkspacePath)
	contents, err := os.ReadFile(created.Plan.WorkspacePath + "/notes.txt")
	require.NoError(t, err)
	assert.Equal(t, "keep me", string(contents))
}

func TestCompleteUnpublishedAndUnexpectedState(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnvironment(t)
	env.Run(t, "init").RequireSuccess(t)
	createPlan(t, env, "Add SSO")
	assertConfigFailure(t, env.Run(t, "complete", "--json"), "review_not_found")

	env2 := testutil.NewEnvironment(t)
	env2.Run(t, "init").RequireSuccess(t)
	created2 := createPlan(t, env2, "Add SSO")
	require.NoError(t, os.WriteFile(created2.Plan.AbsolutePath, []byte("# Add SSO\n\n## Context\n"), 0o644))
	published2 := publishPlan(t, env2)
	require.NoError(t, env2.Provider.SetState(publishedRef(published2), review.Merged))
	assertConfigFailure(t, env2.Run(t, "complete", "--json"), "unexpected_review_state")
}

func TestCompleteHumanOutput(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnvironment(t)
	env.Run(t, "init").RequireSuccess(t)
	created := createPlan(t, env, "Add SSO")
	require.NoError(t, os.WriteFile(created.Plan.AbsolutePath, []byte("# Add SSO\n\n## Context\n"), 0o644))
	published := publishPlan(t, env)
	require.NoError(t, env.Provider.Approve(publishedRef(published), "alice"))
	result := env.Run(t, "complete")
	result.RequireSuccess(t)
	assert.Contains(t, result.Stdout, "Completed plan: Add SSO")
	assert.Contains(t, result.Stdout, "Retention: pr-only")
}
