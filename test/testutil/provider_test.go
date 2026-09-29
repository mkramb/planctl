package testutil_test

import (
	"context"
	"testing"

	"github.com/mkramb/planctl/internal/review"
	"github.com/mkramb/planctl/test/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMockReviewProviderContract(t *testing.T) {
	t.Parallel()
	testutil.ReviewProviderContract(t, func(t *testing.T) review.Provider {
		return testutil.NewMockReviewProvider()
	})
}

func TestSimulatedHumanReviewAndRevision(t *testing.T) {
	t.Parallel()
	provider := testutil.NewMockReviewProvider()
	created, err := provider.CreateReview(t.Context(), review.CreateRequest{
		Repository: "acme/payments", Title: "Plan: SSO", HeadBranch: "plan/sso",
		BaseBranch: "main", HeadCommit: "first-revision",
	})
	require.NoError(t, err)
	line := 42
	require.NoError(t, provider.AddFeedback(created.Ref, review.Feedback{
		Author: "alice", Body: "Describe the rollback strategy.", Path: ".plans/sso.md", Line: &line,
	}))
	line = 99 // The provider must own a snapshot, not the caller's pointer.
	require.NoError(t, provider.RequestChanges(created.Ref, "alice"))
	require.NoError(t, provider.Approve(created.Ref, "bob"))
	require.NoError(t, provider.SetHead(created.Ref, "second-revision"))
	require.NoError(t, provider.Approve(created.Ref, "alice"))
	current, err := provider.GetReview(t.Context(), created.Ref)
	require.NoError(t, err)
	assert.Equal(t, "second-revision", current.HeadCommit)
	assert.Equal(t, []review.Decision{
		{ID: "1", Author: "alice", State: review.ChangesRequested, CommitID: "first-revision"},
		{ID: "2", Author: "bob", State: review.Approved, CommitID: "first-revision"},
		{ID: "3", Author: "alice", State: review.Approved, CommitID: "second-revision"},
	}, current.Decisions)
	require.Len(t, current.Decisions, 3)
	feedback, err := provider.Feedback(t.Context(), created.Ref)
	require.NoError(t, err)
	require.Len(t, feedback, 1)
	require.NotNil(t, feedback[0].Line)
	assert.Equal(t, 42, *feedback[0].Line)
	current.Decisions[0].Author = "mutated"
	*feedback[0].Line = 100
	feedback[0].Body = "mutated"
	again, err := provider.GetReview(t.Context(), created.Ref)
	require.NoError(t, err)
	require.NotEmpty(t, again.Decisions)
	assert.Equal(t, "alice", again.Decisions[0].Author, "caller must not mutate remote decisions")
	feedbackAgain, err := provider.Feedback(t.Context(), created.Ref)
	require.NoError(t, err)
	require.Len(t, feedbackAgain, 1)
	require.NotNil(t, feedbackAgain[0].Line)
	assert.Equal(t, 42, *feedbackAgain[0].Line)
	assert.Equal(t, "Describe the rollback strategy.", feedbackAgain[0].Body)
}

func TestMockRepositoryScopeAndDuplicateReview(t *testing.T) {
	t.Parallel()
	provider := testutil.NewMockReviewProvider()
	request := review.CreateRequest{Repository: "acme/one", HeadBranch: "plan/sso", BaseBranch: "main", Title: "SSO"}
	first, err := provider.CreateReview(t.Context(), request)
	require.NoError(t, err)
	_, err = provider.CreateReview(t.Context(), request)
	require.ErrorIs(t, err, review.ErrExists)
	request.Repository = "acme/two"
	second, err := provider.CreateReview(t.Context(), request)
	require.NoError(t, err)
	for _, want := range []review.Review{first, second} {
		found, err := provider.FindReview(t.Context(), review.FindRequest{Repository: want.Ref.Repository, HeadBranch: want.HeadBranch})
		require.NoError(t, err)
		assert.Equal(t, want.Ref, found.Ref)
	}
	wrong := first.Ref
	wrong.Repository = "acme/two"
	_, err = provider.GetReview(t.Context(), wrong)
	require.ErrorIs(t, err, review.ErrNotFound)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = provider.CreateReview(ctx, request)
	assert.ErrorIs(t, err, context.Canceled)
}
