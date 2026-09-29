package plan

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	"github.com/mkramb/planctl/internal/git"
	"github.com/mkramb/planctl/internal/github"
)

type Publication struct {
	Plan   Plan
	Review github.Review
	Commit string
}

func (s Service) Publish(ctx context.Context, req PublishRequest) (Publication, error) {
	if s.Provider == nil {
		return Publication{}, &Error{Code: "provider_unavailable", Message: "GitHub publishing is not implemented yet; this build supports publishing through the integration-test provider only"}
	}
	sub, err := s.resolve(ctx, req)
	if err != nil {
		return Publication{}, err
	}
	p := sub.plan
	lock, err := lockWorkspace(filepath.Dir(p.WorkspacePath), p.ID)
	if err != nil {
		return Publication{}, err
	}
	defer func() { _ = lock.Unlock() }()
	if err := s.prepareWorktree(ctx, sub.source, p, sub.baseCommit); err != nil {
		return Publication{}, err
	}
	if err := s.verifyWorktree(ctx, sub.source, p); err != nil {
		return Publication{}, err
	}
	repository := sub.repository
	metadata := Metadata{Version: 1, ID: p.ID, Reviewers: req.Reviewers}
	lookup := github.FindRequest{Repository: repository, HeadBranch: p.Branch}
	// Reject terminal or mismatched reviews before committing or pushing.
	if _, err := s.findPublishable(ctx, lookup, metadata, p.Base); err != nil && !errors.Is(err, github.ErrNotFound) {
		return Publication{}, err
	}
	if err := s.syncFiles(ctx, p.WorkspacePath, sub.sourceRoot, p.Files, sub.baseCommit); err != nil {
		return Publication{}, err
	}
	ws := git.New(s.Executor, p.WorkspacePath, s.Env)
	staged, err := ws.StagedFiles(ctx)
	if err != nil {
		return Publication{}, err
	}
	title := strings.ReplaceAll(sub.config.Config.PullRequest.Title, "{title}", p.Title)
	if len(staged) > 0 {
		if err := ws.Commit(ctx, title); err != nil {
			return Publication{}, err
		}
	}
	head, err := ws.Head(ctx)
	if err != nil {
		return Publication{}, err
	}
	if err := ws.PushPlan(ctx, head, p.Branch); err != nil {
		code := "push_failed"
		if errors.Is(err, git.ErrPushRejected) {
			code = "push_rejected"
		}
		return Publication{}, &Error{Code: code, Message: "could not push the review branch; local commits are preserved; reconcile remote changes before retrying (no force push was attempted)", Cause: err}
	}
	existing, err := s.findPublishable(ctx, lookup, metadata, p.Base)
	if err == nil {
		if err := s.syncReviewers(ctx, existing, req.Reviewers); err != nil {
			return Publication{}, err
		}
		return Publication{Plan: p, Review: existing, Commit: head}, nil
	}
	if !errors.Is(err, github.ErrNotFound) {
		return Publication{}, err
	}
	body, err := metadata.Body()
	if err != nil {
		return Publication{}, err
	}
	created, err := s.Provider.CreateReview(ctx, github.CreateRequest{
		Repository: repository, Title: title,
		Body: body, HeadBranch: p.Branch, BaseBranch: p.Base, HeadCommit: head,
		Reviewers: req.Reviewers,
	})
	if err != nil {
		// The platform may have created the review before the response was lost.
		// Rediscover it before allowing a retry to attempt another creation.
		if ctx.Err() != nil {
			return Publication{}, ctx.Err()
		}
		recovered, lookupErr := s.findPublishable(ctx, lookup, metadata, p.Base)
		if lookupErr == nil {
			return Publication{Plan: p, Review: recovered, Commit: head}, nil
		}
		if !errors.Is(lookupErr, github.ErrNotFound) {
			return Publication{}, lookupErr
		}
		return Publication{}, &Error{Code: "review_create_failed", Message: "the branch was pushed but review creation failed; retry publish to rediscover or create the review", Cause: err}
	}
	if err := validateReview(created, lookup, metadata, p.Base); err != nil {
		return Publication{}, err
	}
	return Publication{Plan: p, Review: created, Commit: head}, nil
}

func (s Service) findPublishable(ctx context.Context, req github.FindRequest, metadata Metadata, base string) (github.Review, error) {
	found, err := s.Provider.FindReview(ctx, req)
	if errors.Is(err, github.ErrNotFound) {
		return github.Review{}, github.ErrNotFound
	}
	if err != nil {
		code := "review_lookup_failed"
		if errors.Is(err, github.ErrAmbiguous) {
			code = "ambiguous_review"
		}
		return github.Review{}, &Error{Code: code, Message: "could not uniquely identify the review; resolve the lookup error before retrying", Cause: err}
	}
	return found, validateReview(found, req, metadata, base)
}

func validateReview(found github.Review, req github.FindRequest, want Metadata, base string) error {
	if err := validateIdentity(found, req.Repository, req.HeadBranch, want.ID, base); err != nil {
		return err
	}
	if found.State != github.Open {
		return &Error{Code: "review_terminal", Message: "the review is already closed or merged; publish will not reopen or replace it"}
	}
	return nil
}

func validateIdentity(found github.Review, repository, branch, id, base string) error {
	metadata, err := ParseMetadata(found.Body)
	if err != nil || metadata.ID != id || found.Ref.Repository != repository || found.HeadBranch != branch || found.BaseBranch != base || found.Ref.ID == "" || found.Ref.Provider == "" {
		return &Error{Code: "review_conflict", Message: "the existing review does not match this file set's identity and base branch"}
	}
	return nil
}

// syncReviewers requests any reviewers that were not already requested.
func (s Service) syncReviewers(ctx context.Context, found github.Review, requested []string) error {
	if len(requested) == 0 {
		return nil
	}
	existing, err := ParseMetadata(found.Body)
	if err != nil {
		return err
	}
	have := make(map[string]bool, len(existing.Reviewers))
	for _, reviewer := range existing.Reviewers {
		have[reviewer] = true
	}
	var missing []string
	for _, reviewer := range requested {
		if !have[reviewer] {
			missing = append(missing, reviewer)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	if err := s.Provider.AddReviewers(ctx, found.Ref, missing); err != nil {
		return &Error{Code: "reviewer_update_failed", Message: "could not add reviewers to the review", Cause: err}
	}
	return nil
}
