package plan

import (
	"context"
	"errors"

	"github.com/mkramb/planctl/internal/git"
	"github.com/mkramb/planctl/internal/review"
)

// Inspect gathers everything status, feedback, and context need in one pass.
func (s Service) Inspect(ctx context.Context, req PublishRequest) (Evaluation, error) {
	if s.Provider == nil {
		return Evaluation{}, &Error{Code: "provider_unavailable", Message: "GitHub integration is not implemented yet; lifecycle commands work through the integration-test provider only"}
	}
	selected, err := s.resolve(ctx, req)
	if err != nil {
		return Evaluation{}, err
	}
	p := selected.plan
	p.Title, err = readPlan(p)
	if err != nil {
		return Evaluation{}, err
	}
	remote, err := selected.client.Origin(ctx)
	if err != nil {
		return Evaluation{}, &Error{Code: "invalid_remote", Message: "configure origin with the same single fetch and push URL", Cause: err}
	}
	repository, err := s.Provider.ResolveRepository(ctx, remote)
	if err != nil {
		return Evaluation{}, &Error{Code: "repository_lookup_failed", Message: "could not resolve origin's review repository", Cause: err}
	}
	want := Metadata{Version: 1, ID: p.ID, ImplementationRepository: repository}
	var found *review.Review
	r, err := s.Provider.FindReview(ctx, review.FindRequest{Repository: repository, HeadBranch: p.Branch})
	if err != nil && !errors.Is(err, review.ErrNotFound) {
		code := "review_lookup_failed"
		if errors.Is(err, review.ErrAmbiguous) {
			code = "ambiguous_review"
		}
		return Evaluation{}, &Error{Code: code, Message: "could not uniquely identify the plan review", Cause: err}
	}
	if err == nil {
		if err := validateIdentity(r, repository, p.Branch, want, p.Base); err != nil {
			return Evaluation{}, err
		}
		found = &r
	}
	var feedback []review.Feedback
	if found != nil {
		feedback, err = s.Provider.Feedback(ctx, found.Ref)
		if err != nil {
			return Evaluation{}, &Error{Code: "feedback_lookup_failed", Message: "could not read review feedback", Cause: err}
		}
	}
	client := git.New(s.Executor, p.WorkspacePath, s.Env)
	localCommit, err := client.Head(ctx)
	if err != nil {
		return Evaluation{}, err
	}
	dirty, err := client.PlanDirty(ctx, p.Path)
	if err != nil {
		return Evaluation{}, err
	}
	return Evaluate(EvaluationInput{
		Plan: p, Repository: repository, PlansRepository: repository,
		Review: found, Feedback: feedback,
		RequiredApprovals: selected.config.Config.Review.RequiredApprovals,
		LocalCommit:       localCommit, LocalDirty: dirty,
		Retention: selected.config.Config.Plan.Retention,
	}), nil
}
