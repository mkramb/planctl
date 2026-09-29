package integration_test

import (
	"os"
	"testing"

	"github.com/mkramb/planctl/internal/output"
	"github.com/mkramb/planctl/internal/plan"
	"github.com/mkramb/planctl/internal/review"
	"github.com/mkramb/planctl/test/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The full product workflow from the spec: publish, feedback, revision, approval.
func TestPlanReviewWorkflow(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnvironment(t)
	env.Run(t, "init").RequireSuccess(t)
	created := createPlan(t, env, "Add SSO")
	require.NoError(t, os.WriteFile(created.Plan.AbsolutePath, []byte("# Add SSO\n\n## Context\nUse the identity provider.\n"), 0o644))
	published := publishPlan(t, env)
	ref := publishedRef(published)

	line := 4
	require.NoError(t, env.Provider.AddFeedback(ref, review.Feedback{
		Author: "alice", Body: "Describe the rollback strategy.", Path: created.Plan.Path, Line: &line,
	}))
	feedback := runLifecycle[output.FeedbackResult](t, env, "feedback")
	assert.Equal(t, "add-sso", feedback.Plan.ID)
	assert.Equal(t, plan.StatusInReview, feedback.Status)
	require.Len(t, feedback.Feedback, 1)
	assert.Equal(t, "alice", feedback.Feedback[0].Author)
	assert.Equal(t, created.Plan.Path, feedback.Feedback[0].Path)
	require.NotNil(t, feedback.Feedback[0].Line)
	assert.Equal(t, 4, *feedback.Feedback[0].Line)

	require.NoError(t, env.Provider.Approve(ref, "alice"))
	ctx := runLifecycle[output.ContextResult](t, env, "context")
	assert.True(t, ctx.Implementation.Allowed)
	assert.Empty(t, ctx.Implementation.BlockedReasons)
	assert.Equal(t, "main", ctx.Implementation.Base)
	assert.Equal(t, "acme/payments", ctx.Repositories.Implementation)
	assert.Equal(t, "acme/payments", ctx.Repositories.Plans)
	require.NotNil(t, ctx.Review)
	assert.Equal(t, published.Review.ID, ctx.Review.ID)
	assert.Equal(t, created.Plan.AbsolutePath, ctx.Plan.AbsolutePath)

	status := runLifecycle[output.StatusResult](t, env, "status")
	assert.Equal(t, plan.StatusApproved, status.Plan.Status)
	assert.Equal(t, 1, status.Approval.Current)
	assert.Equal(t, 1, status.Approval.Required)
	assert.Equal(t, 1, status.FeedbackCount)
	require.NotNil(t, status.Review)
	assert.Equal(t, "mock", status.Review.Provider)
}

func TestChangesRequestedBlocksImplementation(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnvironment(t)
	env.Run(t, "init").RequireSuccess(t)
	createPlan(t, env, "Add SSO")
	published := publishPlan(t, env)
	ref := publishedRef(published)
	require.NoError(t, env.Provider.Approve(ref, "alice"))
	require.NoError(t, env.Provider.RequestChanges(ref, "bob"))

	ctx := runLifecycle[output.ContextResult](t, env, "context")
	assert.False(t, ctx.Implementation.Allowed)
	assert.Contains(t, ctx.Implementation.BlockedReasons, "changes_requested")
	status := runLifecycle[output.StatusResult](t, env, "status")
	assert.Equal(t, plan.StatusChangesRequested, status.Plan.Status)

	require.NoError(t, os.WriteFile(published.Plan.AbsolutePath, []byte("# Add SSO\n\nRevised after feedback.\n"), 0o644))
	ctx = runLifecycle[output.ContextResult](t, env, "context")
	assert.False(t, ctx.Implementation.Allowed)
	assert.Contains(t, ctx.Implementation.BlockedReasons, "unpublished_changes")
	republished := publishPlan(t, env)
	assert.NotEqual(t, published.Commit, republished.Commit)
	ctx = runLifecycle[output.ContextResult](t, env, "context")
	assert.False(t, ctx.Implementation.Allowed, "old approval is stale and changes request is active")
	assert.NotContains(t, ctx.Implementation.BlockedReasons, "unpublished_changes")

	require.NoError(t, env.Provider.Approve(ref, "bob"))
	ctx = runLifecycle[output.ContextResult](t, env, "context")
	assert.True(t, ctx.Implementation.Allowed)
}

func TestRequiredApprovalsThreshold(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnvironment(t)
	env.WriteFile(t, ".planctl.yaml", "version: 1\nreview:\n  required_approvals: 2\n")
	createPlan(t, env, "Add SSO")
	published := publishPlan(t, env)
	ref := publishedRef(published)
	require.NoError(t, env.Provider.Approve(ref, "alice"))
	ctx := runLifecycle[output.ContextResult](t, env, "context")
	assert.False(t, ctx.Implementation.Allowed)
	assert.Contains(t, ctx.Implementation.BlockedReasons, "insufficient_approvals")
	status := runLifecycle[output.StatusResult](t, env, "status")
	assert.Equal(t, 1, status.Approval.Current)
	assert.Equal(t, 2, status.Approval.Required)
	require.NoError(t, env.Provider.Approve(ref, "bob"))
	ctx = runLifecycle[output.ContextResult](t, env, "context")
	assert.True(t, ctx.Implementation.Allowed)
}

func TestLifecycleBeforePublish(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnvironment(t)
	env.Run(t, "init").RequireSuccess(t)
	createPlan(t, env, "Add SSO")
	status := runLifecycle[output.StatusResult](t, env, "status")
	assert.Equal(t, plan.StatusDraft, status.Plan.Status)
	assert.Nil(t, status.Review)
	ctx := runLifecycle[output.ContextResult](t, env, "context")
	assert.False(t, ctx.Implementation.Allowed)
	assert.Equal(t, []string{"review_not_found"}, ctx.Implementation.BlockedReasons)
	assert.Nil(t, ctx.Review)
}

func TestLifecycleHumanOutput(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnvironment(t)
	env.Run(t, "init").RequireSuccess(t)
	createPlan(t, env, "Add SSO")
	publishPlan(t, env)
	status := env.Run(t, "status")
	status.RequireSuccess(t)
	assert.Contains(t, status.Stdout, "add-sso")
	assert.Contains(t, status.Stdout, "In review")
	assert.Contains(t, status.Stdout, "Approvals   0 / 1")
	ctx := env.Run(t, "context")
	ctx.RequireSuccess(t)
	assert.Contains(t, ctx.Stdout, "Implementation blocked: insufficient_approvals")
}

func runLifecycle[T any](t *testing.T, env *testutil.Environment, command string, args ...string) T {
	t.Helper()
	result := env.Run(t, append([]string{command, "--json"}, args...)...)
	result.RequireSuccess(t)
	assert.Empty(t, result.Stderr)
	return testutil.DecodeJSON[T](t, result)
}
