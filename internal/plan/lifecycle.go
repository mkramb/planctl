package plan

import (
	"github.com/mkramb/planctl/internal/github"
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
	Review            *github.Review
	Feedback          []github.Feedback
	Reviewers         []string
	Status            Status
	Approvals         int
	RequiredApprovals int
	Allowed           bool
	BlockedReasons    []string
}

type EvaluationInput struct {
	Plan       Plan
	Repository string
	Review     *github.Review
	Feedback   []github.Feedback
	Reviewers  []string
	LocalDirty bool
}

func Evaluate(in EvaluationInput) Evaluation {
	e := Evaluation{
		Plan: in.Plan, Repository: in.Repository, Review: in.Review, Feedback: in.Feedback,
		Reviewers: in.Reviewers,
	}
	block := func(reason string) { e.BlockedReasons = append(e.BlockedReasons, reason) }
	required := requiredApprovals(in)
	e.RequiredApprovals = required
	if in.Review == nil {
		e.Status = StatusDraft
		block("review_not_found")
		return e
	}
	r := in.Review
	approvals, changes := approvalState(r.Decisions, r.HeadCommit, in.Reviewers)
	e.Approvals = approvals
	switch {
	case r.State == github.Merged:
		e.Status = StatusMerged
		block("review_merged")
		return e
	case r.State == github.Closed:
		e.Status = StatusClosed
		block("review_closed")
		return e
	case changes:
		e.Status = StatusChangesRequested
	case approvals >= required:
		e.Status = StatusApproved
	default:
		e.Status = StatusInReview
	}
	if changes {
		block("changes_requested")
	}
	if approvals < required {
		block("insufficient_approvals")
	}
	if in.LocalDirty {
		block("unpublished_changes")
	}
	e.Allowed = len(e.BlockedReasons) == 0
	return e
}

// requiredApprovals returns the number of reviewers when reviewers were
// requested (all must approve), otherwise one (a self-review).
func requiredApprovals(in EvaluationInput) int {
	if len(in.Reviewers) > 0 {
		return len(in.Reviewers)
	}
	return 1
}

// approvalState reduces review history to each author's latest meaningful
// decision. Comment-only reviews do not erase earlier decisions; dismissal does.
// Only approvals of the current head count. When reviewers are requested, only
// their decisions count toward approvals and change requests.
func approvalState(decisions []github.Decision, head string, reviewers []string) (approvals int, changesRequested bool) {
	effective := map[string]github.Decision{}
	for _, d := range decisions {
		switch d.State {
		case github.Approved, github.ChangesRequested:
			effective[d.Author] = d
		case github.Dismissed:
			delete(effective, d.Author)
		}
	}
	eligible := func(author string) bool {
		if len(reviewers) == 0 {
			return true
		}
		for _, r := range reviewers {
			if r == author {
				return true
			}
		}
		return false
	}
	for _, d := range effective {
		switch d.State {
		case github.Approved:
			if d.CommitID == head && eligible(d.Author) {
				approvals++
			}
		case github.ChangesRequested:
			if eligible(d.Author) {
				changesRequested = true
			}
		}
	}
	return approvals, changesRequested
}
