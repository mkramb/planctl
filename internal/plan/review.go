package plan

import (
	"context"
	"time"

	"github.com/mkramb/planctl/internal/github"
)

// Wait blocks until an already-published review reaches a decision: either
// allowed (approved) or changes_requested. A terminal (closed/merged) review is
// an error. onPoll, if non-nil, observes each check so the CLI can report
// progress.
func (s Service) Wait(ctx context.Context, req PublishRequest, pollInterval time.Duration, onPoll func(Evaluation)) (Evaluation, error) {
	if pollInterval <= 0 {
		pollInterval = 5 * time.Second
	}
	for {
		if err := ctx.Err(); err != nil {
			return Evaluation{}, err
		}
		eval, err := s.Inspect(ctx, req)
		if err != nil {
			return Evaluation{}, err
		}
		if eval.Review == nil {
			return Evaluation{}, &Error{Code: "review_not_found", Message: "publish the files before waiting for review"}
		}
		if onPoll != nil {
			onPoll(eval)
		}
		if eval.Allowed || eval.Status == StatusChangesRequested || len(eval.Feedback) > 0 {
			return eval, nil
		}
		if eval.Review.State == github.Closed || eval.Review.State == github.Merged {
			return Evaluation{}, &Error{Code: "review_terminal", Message: "the review was closed or merged before approval"}
		}
		select {
		case <-ctx.Done():
			return Evaluation{}, ctx.Err()
		case <-time.After(pollInterval):
		}
	}
}
