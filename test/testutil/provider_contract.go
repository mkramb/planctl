package testutil

import (
	"testing"

	"github.com/mkramb/planctl/internal/review"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ReviewProviderContract exercises provider semantics rather than call counts.
// A fixture-backed GitHub adapter can run these same scenarios as it is added.
func ReviewProviderContract(t *testing.T, newProvider func(*testing.T) review.Provider) {
	t.Helper()
	t.Run("create and rediscover", func(t *testing.T) {
		provider := newProvider(t)
		req := review.CreateRequest{
			Repository: "acme/payments", Title: "Plan: Add SSO", Body: "A plan for SSO.",
			HeadBranch: "plan/add-sso", BaseBranch: "main", HeadCommit: "revision-one",
		}
		created, err := provider.CreateReview(t.Context(), req)
		require.NoError(t, err)
		assert.NotEmpty(t, created.Ref.ID)
		assert.NotEmpty(t, created.Ref.Provider)
		assert.Equal(t, req.Repository, created.Ref.Repository)
		assert.NotEmpty(t, created.URL)
		assert.Equal(t, review.Open, created.State)
		assert.False(t, created.Draft)
		assert.Equal(t, req.Title, created.Title)
		assert.Equal(t, req.HeadBranch, created.HeadBranch)
		assert.Equal(t, req.HeadCommit, created.HeadCommit)
		found, err := provider.FindReview(t.Context(), review.FindRequest{Repository: req.Repository, HeadBranch: req.HeadBranch})
		require.NoError(t, err)
		assert.Equal(t, created.Ref, found.Ref)
		fetched, err := provider.GetReview(t.Context(), created.Ref)
		require.NoError(t, err)
		assert.Equal(t, created.Ref, fetched.Ref)
		assert.Equal(t, review.Open, fetched.State)
		feedback, err := provider.Feedback(t.Context(), created.Ref)
		require.NoError(t, err)
		assert.Empty(t, feedback)
	})
	t.Run("missing review", func(t *testing.T) {
		provider := newProvider(t)
		_, err := provider.FindReview(t.Context(), review.FindRequest{Repository: "acme/payments", HeadBranch: "plan/missing"})
		assert.ErrorIs(t, err, review.ErrNotFound)
	})
	t.Run("preserve draft state", func(t *testing.T) {
		provider := newProvider(t)
		created, err := provider.CreateReview(t.Context(), review.CreateRequest{
			Repository: "acme/payments", Title: "Plan: Draft", HeadBranch: "plan/draft",
			BaseBranch: "main", HeadCommit: "revision-one", Draft: true,
		})
		require.NoError(t, err)
		fetched, err := provider.GetReview(t.Context(), created.Ref)
		require.NoError(t, err)
		assert.True(t, fetched.Draft)
	})
	t.Run("close and merge terminal states", func(t *testing.T) {
		provider := newProvider(t)
		created, err := provider.CreateReview(t.Context(), review.CreateRequest{
			Repository: "acme/payments", Title: "Plan: X", HeadBranch: "plan/x", BaseBranch: "main",
		})
		require.NoError(t, err)
		require.NoError(t, provider.CloseReview(t.Context(), created.Ref))
		got, err := provider.GetReview(t.Context(), created.Ref)
		require.NoError(t, err)
		assert.Equal(t, review.Closed, got.State)
		require.NoError(t, provider.CloseReview(t.Context(), created.Ref), "repeating close succeeds")

		merged, err := provider.CreateReview(t.Context(), review.CreateRequest{
			Repository: "acme/payments", Title: "Plan: Y", HeadBranch: "plan/y", BaseBranch: "main",
		})
		require.NoError(t, err)
		require.NoError(t, provider.MergeReview(t.Context(), merged.Ref))
		got, err = provider.GetReview(t.Context(), merged.Ref)
		require.NoError(t, err)
		assert.Equal(t, review.Merged, got.State)
		assert.Error(t, provider.MergeReview(t.Context(), created.Ref), "merging a closed review fails")
	})
}
