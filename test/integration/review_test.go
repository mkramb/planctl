package integration_test

import (
	"os"
	"testing"
	"time"

	"github.com/mkramb/planctl/internal/output"
	"github.com/mkramb/planctl/internal/plan"
	"github.com/mkramb/planctl/internal/review"
	"github.com/mkramb/planctl/test/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runReview starts `planctl review` in the background and returns a channel that
// receives its result when the command unblocks.
func runReview(t *testing.T, env *testutil.Environment) <-chan testutil.Result {
	t.Helper()
	ch := make(chan testutil.Result, 1)
	go func() { ch <- env.Run(t, "review", "--json", "--poll", "20ms") }()
	return ch
}

// findOpenReview waits briefly and returns the published review for the plan.
func findOpenReview(t *testing.T, env *testutil.Environment) review.Ref {
	t.Helper()
	var found *review.Review
	for i := 0; i < 200 && found == nil; i++ {
		r, err := env.Provider.FindReview(t.Context(), review.FindRequest{Repository: "acme/payments", HeadBranch: "plan/add-sso"})
		if err == nil {
			found = &r
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	require.NotNil(t, found, "review was not published")
	return found.Ref
}

func TestReviewBlocksUntilApproved(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnvironment(t)
	env.Run(t, "init").RequireSuccess(t)
	created := createPlan(t, env, "Add SSO")
	require.NoError(t, os.WriteFile(created.Plan.AbsolutePath, []byte("# Add SSO\n\n## Context\nUse SSO.\n"), 0o644))

	ch := runReview(t, env)
	ref := findOpenReview(t, env)
	require.NoError(t, env.Provider.Approve(ref, "alice"))

	select {
	case result := <-ch:
		result.RequireSuccess(t)
		ctx := testutil.DecodeJSON[output.ContextResult](t, result)
		assert.True(t, ctx.Implementation.Allowed)
		assert.Equal(t, plan.StatusApproved, ctx.Plan.Status)
	case <-time.After(5 * time.Second):
		t.Fatal("review command did not unblock after approval")
	}
}

func TestReviewReturnsOnChangesRequested(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnvironment(t)
	env.Run(t, "init").RequireSuccess(t)
	created := createPlan(t, env, "Add SSO")
	require.NoError(t, os.WriteFile(created.Plan.AbsolutePath, []byte("# Add SSO\n\n## Context\nUse SSO.\n"), 0o644))

	ch := runReview(t, env)
	ref := findOpenReview(t, env)
	require.NoError(t, env.Provider.RequestChanges(ref, "bob"))

	select {
	case result := <-ch:
		result.RequireSuccess(t)
		ctx := testutil.DecodeJSON[output.ContextResult](t, result)
		assert.False(t, ctx.Implementation.Allowed)
		assert.Equal(t, plan.StatusChangesRequested, ctx.Plan.Status)
		assert.Contains(t, ctx.Implementation.BlockedReasons, "changes_requested")
	case <-time.After(5 * time.Second):
		t.Fatal("review command did not unblock after changes were requested")
	}
}

func TestReviewHumanOutput(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnvironment(t)
	env.Run(t, "init").RequireSuccess(t)
	created := createPlan(t, env, "Add SSO")
	require.NoError(t, os.WriteFile(created.Plan.AbsolutePath, []byte("# Add SSO\n\n## Context\nUse SSO.\n"), 0o644))

	ch := make(chan testutil.Result, 1)
	go func() { ch <- env.Run(t, "review", "--poll", "20ms") }()
	ref := findOpenReview(t, env)
	require.NoError(t, env.Provider.Approve(ref, "alice"))

	select {
	case result := <-ch:
		result.RequireSuccess(t)
		assert.Contains(t, result.Stderr, "Publishing plan and waiting for review...")
		assert.Contains(t, result.Stdout, "Implementation allowed")
		assert.Contains(t, result.Stdout, "Review       https://review.invalid/")
	case <-time.After(5 * time.Second):
		t.Fatal("review command did not unblock")
	}
}
