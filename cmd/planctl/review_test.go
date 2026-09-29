//go:build integration

package main

import (
	"testing"

	"github.com/mkramb/planctl/internal/github"
	"github.com/mkramb/planctl/internal/plan"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReviewBlocksUntilApproved(t *testing.T) {
	t.Parallel()
	env := NewEnvironment(t)
	env.WriteFile(t, "add-sso.md", "# Add SSO\n\n## Context\nUse SSO.\n")

	ch := startLoop(t, env, "add-sso.md", "--json")
	ref := findReview(t, env, "review/add-sso")
	require.NoError(t, env.Mock.Approve(ref, "alice"))
	outcome := waitOutcome(t, ch)
	assert.True(t, outcome.Allowed)
	assert.Equal(t, plan.StatusApproved, outcome.Status)
}

func TestReviewReturnsFeedbackOnChangesRequested(t *testing.T) {
	t.Parallel()
	env := NewEnvironment(t)
	env.WriteFile(t, "add-sso.md", "# Add SSO\n\n## Context\nUse SSO.\n")

	ch := startLoop(t, env, "add-sso.md", "--json")
	ref := findReview(t, env, "review/add-sso")
	line := 4
	require.NoError(t, env.Mock.AddFeedback(ref, github.Feedback{
		Author: "bob", Body: "Describe the rollback strategy.", Path: "add-sso.md", Line: &line,
	}))
	require.NoError(t, env.Mock.RequestChanges(ref, "bob"))

	outcome := waitOutcome(t, ch)
	assert.False(t, outcome.Allowed)
	assert.Equal(t, plan.StatusChangesRequested, outcome.Status)
	assert.Contains(t, outcome.BlockedReasons, "changes_requested")
	require.Len(t, outcome.Feedback, 1)
	assert.Equal(t, "bob", outcome.Feedback[0].Author)
	assert.Equal(t, "add-sso.md", outcome.Feedback[0].Path)
	assert.Equal(t, 4, *outcome.Feedback[0].Line)
}

func TestReviewReturnsOnCommentOnly(t *testing.T) {
	t.Parallel()
	env := NewEnvironment(t)
	env.WriteFile(t, "add-sso.md", "# Add SSO\n\n## Context\nUse SSO.\n")

	ch := startLoop(t, env, "add-sso.md", "--json")
	ref := findReview(t, env, "review/add-sso")
	line := 4
	require.NoError(t, env.Mock.AddFeedback(ref, github.Feedback{
		Author: "bob", Body: "Describe the rollback strategy.", Path: "add-sso.md", Line: &line,
	}))

	outcome := waitOutcome(t, ch)
	assert.False(t, outcome.Allowed)
	assert.Equal(t, plan.StatusInReview, outcome.Status)
	require.Len(t, outcome.Feedback, 1)
	assert.Equal(t, "bob", outcome.Feedback[0].Author)
}

func TestReviewHumanOutput(t *testing.T) {
	t.Parallel()
	env := NewEnvironment(t)
	env.WriteFile(t, "add-sso.md", "# Add SSO\n\n## Context\nUse SSO.\n")

	ch := startLoop(t, env, "add-sso.md")
	ref := findReview(t, env, "review/add-sso")
	require.NoError(t, env.Mock.Approve(ref, "alice"))
	result := <-ch
	result.RequireSuccess(t)
	assert.Contains(t, result.Stderr, "Review: https://review.invalid/")
	assert.Contains(t, result.Stdout, "Approved on base main")
}
