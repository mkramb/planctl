package plan

import (
	"github.com/mkramb/planctl/internal/review"
)

type Status string

const (
	StatusDraft            Status = "draft"
	StatusInReview         Status = "in_review"
	StatusChangesRequested Status = "changes_requested"
	StatusApproved         Status = "approved"
	StatusClosed           Status = "closed"
	StatusMerged           Status = "merged"
)

type Evaluation struct {
	Plan              Plan
	Repository        string
	PlansRepository   string
	Retention         string
	Review            *review.Review
	Feedback          []review.Feedback
	Status            Status
	Approvals         int
	RequiredApprovals int
	Allowed           bool
	BlockedReasons    []string
}

type EvaluationInput struct {
	Plan              Plan
	Repository        string
	PlansRepository   string
	Retention         string
	Review            *review.Review
	Feedback          []review.Feedback
	RequiredApprovals int
	LocalCommit       string
	LocalDirty        bool
}

func Evaluate(in EvaluationInput) Evaluation {
	e := Evaluation{
		Plan: in.Plan, Repository: in.Repository, PlansRepository: in.PlansRepository,
		Retention: in.Retention, Review: in.Review, Feedback: in.Feedback,
		RequiredApprovals: in.RequiredApprovals,
	}
	block := func(reason string) { e.BlockedReasons = append(e.BlockedReasons, reason) }
	if in.Review == nil {
		e.Status = StatusDraft
		block("review_not_found")
		return e
	}
	r := in.Review
	e.Approvals, _ = approvalState(r.Decisions, r.HeadCommit)
	_, changes := approvalState(r.Decisions, r.HeadCommit)
	switch {
	case r.State == review.Merged:
		e.Status = StatusMerged
		block("review_merged")
		return e
	case r.State == review.Closed:
		e.Status = StatusClosed
		block("review_closed")
		return e
	case changes:
		e.Status = StatusChangesRequested
	case r.Draft:
		e.Status = StatusDraft
	case e.Approvals >= in.RequiredApprovals:
		e.Status = StatusApproved
	default:
		e.Status = StatusInReview
	}
	if r.Draft {
		block("review_is_draft")
	}
	if changes {
		block("changes_requested")
	}
	if e.Approvals < in.RequiredApprovals {
		block("insufficient_approvals")
	}
	if in.LocalDirty || in.LocalCommit != r.HeadCommit {
		block("unpublished_changes")
	}
	e.Allowed = len(e.BlockedReasons) == 0
	return e
}

// approvalState reduces review history to each author's latest meaningful
// decision. Comment-only reviews do not erase earlier decisions; dismissal does.
// Only approvals of the current head count. An active changes request blocks
// until it is dismissed or superseded by that author's approval.
func approvalState(decisions []review.Decision, head string) (approvals int, changesRequested bool) {
	effective := map[string]review.Decision{}
	for _, d := range decisions {
		switch d.State {
		case review.Approved, review.ChangesRequested:
			effective[d.Author] = d
		case review.Dismissed:
			delete(effective, d.Author)
		}
	}
	for _, d := range effective {
		switch d.State {
		case review.Approved:
			if d.CommitID == head {
				approvals++
			}
		case review.ChangesRequested:
			changesRequested = true
		}
	}
	return approvals, changesRequested
}
