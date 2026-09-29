package plan

import (
	"context"
	"time"

	"github.com/mkramb/planctl/internal/review"
)

// Review publishes the plan and then blocks until the review reaches a decision:
// either implementation.allowed (approved) or changes_requested. A terminal
// (closed/merged) review is an error. onPoll, if non-nil, observes each check so
// the CLI can report progress.
func (s Service) Review(ctx context.Context, req PublishRequest, pollInterval time.Duration, onPoll func(Evaluation)) (Evaluation, error) {
	if _, err := s.Publish(ctx, req); err != nil {
		return Evaluation{}, err
	}
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
			return Evaluation{}, &Error{Code: "review_not_found", Message: "publish the plan before reviewing it"}
		}
		if onPoll != nil {
			onPoll(eval)
		}
		if eval.Allowed || eval.Status == StatusChangesRequested {
			return eval, nil
		}
		if eval.Review.State == review.Closed || eval.Review.State == review.Merged {
			return Evaluation{}, &Error{Code: "review_terminal", Message: "the plan review was closed or merged before approval"}
		}
		select {
		case <-ctx.Done():
			return Evaluation{}, ctx.Err()
		case <-time.After(pollInterval):
		}
	}
}
