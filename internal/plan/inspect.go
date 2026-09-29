package plan

import (
	"context"
	"errors"

	"github.com/mkramb/planctl/internal/github"
)

// Inspect gathers everything status, feedback, and context need in one pass.
func (s Service) Inspect(ctx context.Context, req PublishRequest) (Evaluation, error) {
	if s.Provider == nil {
		return Evaluation{}, &Error{Code: "provider_unavailable", Message: "GitHub integration is not implemented yet; lifecycle commands work through the integration-test provider only"}
	}
	sub, err := s.resolve(ctx, req)
	if err != nil {
		return Evaluation{}, err
	}
	p := sub.plan
	repository := sub.repository
	var found *github.Review
	r, err := s.Provider.FindReview(ctx, github.FindRequest{Repository: repository, HeadBranch: p.Branch})
	if err != nil && !errors.Is(err, github.ErrNotFound) {
		code := "review_lookup_failed"
		if errors.Is(err, github.ErrAmbiguous) {
			code = "ambiguous_review"
		}
		return Evaluation{}, &Error{Code: code, Message: "could not uniquely identify the review", Cause: err}
	}
	var reviewers []string
	if err == nil {
		if err := validateIdentity(r, repository, p.Branch, p.ID, p.Base); err != nil {
			return Evaluation{}, err
		}
		found = &r
		metadata, parseErr := ParseMetadata(r.Body)
		if parseErr != nil {
			return Evaluation{}, parseErr
		}
		reviewers = metadata.Reviewers
	}
	var feedback []github.Feedback
	if found != nil {
		feedback, err = s.Provider.Feedback(ctx, found.Ref, found.HeadCommit)
		if err != nil {
			return Evaluation{}, &Error{Code: "feedback_lookup_failed", Message: "could not read review feedback", Cause: err}
		}
	}
	localDirty := false
	if found != nil {
		localDirty, err = sub.source.DiffersFrom(ctx, found.HeadCommit, p.Files)
		if err != nil {
			return Evaluation{}, err
		}
	}
	return Evaluate(EvaluationInput{
		Plan: p, Repository: repository, Review: found, Feedback: feedback,
		Reviewers: reviewers, LocalDirty: localDirty,
	}), nil
}
