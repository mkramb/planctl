package plan_test

import (
	"testing"

	"github.com/mkramb/planctl/internal/github"
	"github.com/mkramb/planctl/internal/plan"
	"github.com/stretchr/testify/assert"
)

func eval(rev *github.Review, reviewers []string, dirty bool) plan.Evaluation {
	return plan.Evaluate(plan.EvaluationInput{
		Plan: plan.Plan{ID: "add-sso", Base: "main"}, Repository: "acme/payments",
		Review: rev, Reviewers: reviewers, LocalDirty: dirty,
	})
}

func openReview(decisions ...github.Decision) *github.Review {
	return &github.Review{
		Ref:   github.Ref{Provider: "mock", Repository: "acme/payments", ID: "1"},
		State: github.Open, HeadCommit: "head-one", Decisions: decisions,
	}
}

func TestEvaluateUnpublished(t *testing.T) {
	t.Parallel()
	e := eval(nil, nil, false)
	assert.Equal(t, plan.StatusDraft, e.Status)
	assert.False(t, e.Allowed)
	assert.Equal(t, []string{"review_not_found"}, e.BlockedReasons)
}

func TestEvaluateTerminalStates(t *testing.T) {
	t.Parallel()
	closed := openReview()
	closed.State = github.Closed
	e := eval(closed, nil, false)
	assert.Equal(t, plan.StatusClosed, e.Status)
	assert.Equal(t, []string{"review_closed"}, e.BlockedReasons)
	merged := openReview()
	merged.State = github.Merged
	e = eval(merged, nil, false)
	assert.Equal(t, plan.StatusMerged, e.Status)
	assert.Equal(t, []string{"review_merged"}, e.BlockedReasons)
}

func TestEvaluateApprovalMatrix(t *testing.T) {
	t.Parallel()
	head := "head-one"
	for _, tt := range []struct {
		name      string
		decisions []github.Decision
		reviewers []string
		status    plan.Status
		allowed   bool
		reasons   []string
	}{
		{"no decisions", nil, nil, plan.StatusInReview, false, []string{"insufficient_approvals"}},
		{"one approval", []github.Decision{{Author: "alice", State: github.Approved, CommitID: head}}, nil, plan.StatusApproved, true, nil},
		{"stale approval", []github.Decision{{Author: "alice", State: github.Approved, CommitID: "old-head"}}, nil, plan.StatusInReview, false, []string{"insufficient_approvals"}},
		{"two reviewers one given", []github.Decision{{Author: "alice", State: github.Approved, CommitID: head}}, []string{"alice", "bob"}, plan.StatusInReview, false, []string{"insufficient_approvals"}},
		{"two approvals", []github.Decision{
			{Author: "alice", State: github.Approved, CommitID: head},
			{Author: "bob", State: github.Approved, CommitID: head},
		}, []string{"alice", "bob"}, plan.StatusApproved, true, nil},
		{"same reviewer counts once", []github.Decision{
			{Author: "alice", State: github.Approved, CommitID: head},
			{Author: "alice", State: github.Approved, CommitID: head},
		}, []string{"alice", "bob"}, plan.StatusInReview, false, []string{"insufficient_approvals"}},
		{"comment preserves approval", []github.Decision{
			{Author: "alice", State: github.Approved, CommitID: head},
			{Author: "alice", State: github.Commented, CommitID: head},
		}, nil, plan.StatusApproved, true, nil},
		{"changes requested", []github.Decision{{Author: "alice", State: github.ChangesRequested, CommitID: head}}, nil, plan.StatusChangesRequested, false, []string{"changes_requested", "insufficient_approvals"}},
		{"changes superseded by approval", []github.Decision{
			{Author: "alice", State: github.ChangesRequested, CommitID: "old-head"},
			{Author: "alice", State: github.Approved, CommitID: head},
		}, nil, plan.StatusApproved, true, nil},
		{"dismissal clears changes", []github.Decision{
			{Author: "alice", State: github.ChangesRequested, CommitID: head},
			{Author: "alice", State: github.Dismissed, CommitID: head},
			{Author: "bob", State: github.Approved, CommitID: head},
		}, nil, plan.StatusApproved, true, nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			e := eval(openReview(tt.decisions...), tt.reviewers, false)
			assert.Equal(t, tt.status, e.Status)
			assert.Equal(t, tt.allowed, e.Allowed)
			assert.Equal(t, tt.reasons, e.BlockedReasons)
		})
	}
}

func TestEvaluateReviewers(t *testing.T) {
	t.Parallel()
	head := "head-one"
	// Reviewers determine the required count and restrict whose decisions count.
	e := eval(openReview(
		github.Decision{Author: "alice", State: github.Approved, CommitID: head},
	), []string{"alice", "bob"}, false)
	assert.Equal(t, 2, e.RequiredApprovals)
	assert.Equal(t, plan.StatusInReview, e.Status)
	assert.Equal(t, []string{"insufficient_approvals"}, e.BlockedReasons)

	// A non-reviewer approval does not count toward the reviewer requirement.
	e = eval(openReview(
		github.Decision{Author: "alice", State: github.Approved, CommitID: head},
		github.Decision{Author: "mallory", State: github.Approved, CommitID: head},
	), []string{"alice", "bob"}, false)
	assert.Equal(t, 1, e.Approvals)
	assert.False(t, e.Allowed)

	// Both requested reviewers approving reaches approved.
	e = eval(openReview(
		github.Decision{Author: "alice", State: github.Approved, CommitID: head},
		github.Decision{Author: "bob", State: github.Approved, CommitID: head},
	), []string{"alice", "bob"}, false)
	assert.Equal(t, plan.StatusApproved, e.Status)
	assert.True(t, e.Allowed)

	// A non-reviewer change request does not block reviewer approvals.
	e = eval(openReview(
		github.Decision{Author: "alice", State: github.Approved, CommitID: head},
		github.Decision{Author: "bob", State: github.Approved, CommitID: head},
		github.Decision{Author: "mallory", State: github.ChangesRequested, CommitID: head},
	), []string{"alice", "bob"}, false)
	assert.True(t, e.Allowed)
}

func TestEvaluateLocalState(t *testing.T) {
	t.Parallel()
	approved := openReview(github.Decision{Author: "alice", State: github.Approved, CommitID: "head-one"})
	e := eval(approved, nil, false)
	assert.Equal(t, plan.StatusApproved, e.Status)
	assert.True(t, e.Allowed)

	e = eval(approved, nil, true)
	assert.Equal(t, plan.StatusApproved, e.Status, "status reflects the review; local state only blocks")
	assert.False(t, e.Allowed)
	assert.Equal(t, []string{"unpublished_changes"}, e.BlockedReasons)
}
