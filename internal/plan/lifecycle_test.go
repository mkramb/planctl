package plan_test

import (
	"testing"

	"github.com/mkramb/planctl/internal/plan"
	"github.com/mkramb/planctl/internal/review"
	"github.com/stretchr/testify/assert"
)

func eval(rev *review.Review, required int, local string, dirty bool) plan.Evaluation {
	return plan.Evaluate(plan.EvaluationInput{
		Plan: plan.Plan{ID: "add-sso", Base: "main"}, Repository: "acme/payments",
		Review: rev, RequiredApprovals: required, LocalCommit: local, LocalDirty: dirty,
	})
}

func openReview(decisions ...review.Decision) *review.Review {
	return &review.Review{
		Ref:   review.Ref{Provider: "mock", Repository: "acme/payments", ID: "1"},
		State: review.Open, HeadCommit: "head-one", Decisions: decisions,
	}
}

func TestEvaluateUnpublished(t *testing.T) {
	t.Parallel()
	e := eval(nil, 1, "anything", false)
	assert.Equal(t, plan.StatusDraft, e.Status)
	assert.False(t, e.Allowed)
	assert.Equal(t, []string{"review_not_found"}, e.BlockedReasons)
}

func TestEvaluateTerminalStates(t *testing.T) {
	t.Parallel()
	closed := openReview()
	closed.State = review.Closed
	e := eval(closed, 1, "head-one", false)
	assert.Equal(t, plan.StatusClosed, e.Status)
	assert.Equal(t, []string{"review_closed"}, e.BlockedReasons)
	merged := openReview()
	merged.State = review.Merged
	e = eval(merged, 1, "head-one", false)
	assert.Equal(t, plan.StatusMerged, e.Status)
	assert.Equal(t, []string{"review_merged"}, e.BlockedReasons)
}

func TestEvaluateApprovalMatrix(t *testing.T) {
	t.Parallel()
	head := "head-one"
	for _, tt := range []struct {
		name      string
		decisions []review.Decision
		required  int
		status    plan.Status
		allowed   bool
		reasons   []string
	}{
		{"no decisions", nil, 1, plan.StatusInReview, false, []string{"insufficient_approvals"}},
		{"one approval", []review.Decision{{Author: "alice", State: review.Approved, CommitID: head}}, 1, plan.StatusApproved, true, nil},
		{"stale approval", []review.Decision{{Author: "alice", State: review.Approved, CommitID: "old-head"}}, 1, plan.StatusInReview, false, []string{"insufficient_approvals"}},
		{"two required one given", []review.Decision{{Author: "alice", State: review.Approved, CommitID: head}}, 2, plan.StatusInReview, false, []string{"insufficient_approvals"}},
		{"two approvals", []review.Decision{
			{Author: "alice", State: review.Approved, CommitID: head},
			{Author: "bob", State: review.Approved, CommitID: head},
		}, 2, plan.StatusApproved, true, nil},
		{"same reviewer counts once", []review.Decision{
			{Author: "alice", State: review.Approved, CommitID: head},
			{Author: "alice", State: review.Approved, CommitID: head},
		}, 2, plan.StatusInReview, false, []string{"insufficient_approvals"}},
		{"comment preserves approval", []review.Decision{
			{Author: "alice", State: review.Approved, CommitID: head},
			{Author: "alice", State: review.Commented, CommitID: head},
		}, 1, plan.StatusApproved, true, nil},
		{"changes requested", []review.Decision{{Author: "alice", State: review.ChangesRequested, CommitID: head}}, 1, plan.StatusChangesRequested, false, []string{"changes_requested", "insufficient_approvals"}},
		{"changes superseded by approval", []review.Decision{
			{Author: "alice", State: review.ChangesRequested, CommitID: "old-head"},
			{Author: "alice", State: review.Approved, CommitID: head},
		}, 1, plan.StatusApproved, true, nil},
		{"dismissal clears changes", []review.Decision{
			{Author: "alice", State: review.ChangesRequested, CommitID: head},
			{Author: "alice", State: review.Dismissed, CommitID: head},
			{Author: "bob", State: review.Approved, CommitID: head},
		}, 1, plan.StatusApproved, true, nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			e := eval(openReview(tt.decisions...), tt.required, head, false)
			assert.Equal(t, tt.status, e.Status)
			assert.Equal(t, tt.allowed, e.Allowed)
			assert.Equal(t, tt.reasons, e.BlockedReasons)
		})
	}
}

func TestEvaluateDraftAndLocalState(t *testing.T) {
	t.Parallel()
	draft := openReview(review.Decision{Author: "alice", State: review.Approved, CommitID: "head-one"})
	draft.Draft = true
	e := eval(draft, 1, "head-one", false)
	assert.Equal(t, plan.StatusDraft, e.Status)
	assert.False(t, e.Allowed)
	assert.Contains(t, e.BlockedReasons, "review_is_draft")

	approved := openReview(review.Decision{Author: "alice", State: review.Approved, CommitID: "head-one"})
	e = eval(approved, 1, "different-local", false)
	assert.Equal(t, plan.StatusApproved, e.Status, "status reflects the review; local state only blocks")
	assert.False(t, e.Allowed)
	assert.Equal(t, []string{"unpublished_changes"}, e.BlockedReasons)

	e = eval(approved, 1, "head-one", true)
	assert.False(t, e.Allowed)
	assert.Equal(t, []string{"unpublished_changes"}, e.BlockedReasons)
}
