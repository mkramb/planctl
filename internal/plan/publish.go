package plan

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	"github.com/mkramb/planctl/internal/git"
	"github.com/mkramb/planctl/internal/review"
)

type Publication struct {
	Plan   Plan
	Review review.Review
	Commit string
}

func (s Service) Publish(ctx context.Context, req PublishRequest) (Publication, error) {
	if s.Provider == nil {
		return Publication{}, &Error{Code: "provider_unavailable", Message: "GitHub publishing is not implemented yet; this build supports publishing through the integration-test provider only"}
	}
	selected, err := s.resolve(ctx, req)
	if err != nil {
		return Publication{}, err
	}
	p := selected.plan
	lock, err := lockWorkspace(filepath.Dir(p.WorkspacePath), p.ID)
	if err != nil {
		return Publication{}, err
	}
	defer lock.Unlock()
	if err := s.verifyWorktree(ctx, selected.client, p); err != nil {
		return Publication{}, err
	}
	p.Title, err = readPlan(p)
	if err != nil {
		return Publication{}, err
	}
	remote, err := selected.client.Origin(ctx)
	if err != nil {
		return Publication{}, &Error{Code: "invalid_remote", Message: "configure origin with the same single fetch and push URL before publishing", Cause: err}
	}
	repository, err := s.Provider.ResolveRepository(ctx, remote)
	if err != nil {
		return Publication{}, &Error{Code: "repository_lookup_failed", Message: "could not resolve origin's review repository", Cause: err}
	}
	metadata := Metadata{Version: 1, ID: p.ID, ImplementationRepository: repository}
	lookup := review.FindRequest{Repository: repository, HeadBranch: p.Branch}
	// Reject terminal or unrelated reviews before committing or pushing anything.
	if _, err := s.findPublishable(ctx, lookup, metadata, p.Base); err != nil && !errors.Is(err, review.ErrNotFound) {
		return Publication{}, err
	}
	client := git.New(s.Executor, p.WorkspacePath, s.Env)
	head, err := client.Head(ctx)
	if err != nil {
		return Publication{}, err
	}
	if err := checkBranchChanges(ctx, client, selected.baseCommit, head, p.Path); err != nil {
		return Publication{}, err
	}
	staged, err := client.StagedFiles(ctx)
	if err != nil {
		return Publication{}, err
	}
	if err := onlyPlan(staged, p.Path); err != nil {
		return Publication{}, err
	}
	if err := client.Add(ctx, p.Path); err != nil {
		return Publication{}, err
	}
	staged, err = client.StagedFiles(ctx)
	if err != nil {
		return Publication{}, err
	}
	if err := onlyPlan(staged, p.Path); err != nil {
		return Publication{}, err
	}
	if len(staged) > 0 {
		if err := client.CommitOnly(ctx, "Plan: "+p.Title, p.Path); err != nil {
			return Publication{}, err
		}
	}
	head, err = client.Head(ctx)
	if err != nil {
		return Publication{}, err
	}
	// Recheck after committing, including changes made by ordinary Git hooks.
	if err := checkBranchChanges(ctx, client, selected.baseCommit, head, p.Path); err != nil {
		return Publication{}, err
	}
	contains, err := client.TreeContains(ctx, head, p.Path)
	if err != nil {
		return Publication{}, err
	}
	if !contains {
		return Publication{}, &Error{Code: "plan_not_found", Message: "the committed branch does not contain the plan file"}
	}
	if err := client.PushPlan(ctx, head, p.Branch); err != nil {
		code := "push_failed"
		if errors.Is(err, git.ErrPushRejected) {
			code = "push_rejected"
		}
		return Publication{}, &Error{Code: code, Message: "could not push the plan branch; local commits are preserved; reconcile remote changes before retrying (no force push was attempted)", Cause: err}
	}
	existing, err := s.findPublishable(ctx, lookup, metadata, p.Base)
	if err == nil {
		return Publication{Plan: p, Review: existing, Commit: head}, nil
	}
	if !errors.Is(err, review.ErrNotFound) {
		return Publication{}, err
	}
	body, err := metadata.Body()
	if err != nil {
		return Publication{}, err
	}
	created, err := s.Provider.CreateReview(ctx, review.CreateRequest{
		Repository: repository, Title: strings.ReplaceAll(selected.config.Config.PullRequest.Title, "{title}", p.Title),
		Body: body, HeadBranch: p.Branch, BaseBranch: p.Base, HeadCommit: head,
		Draft: selected.config.Config.PullRequest.Draft,
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
		if !errors.Is(lookupErr, review.ErrNotFound) {
			return Publication{}, lookupErr
		}
		return Publication{}, &Error{Code: "review_create_failed", Message: "the branch was pushed but review creation failed; retry publish to rediscover or create the review", Cause: err}
	}
	if err := validateReview(created, lookup, metadata, p.Base); err != nil {
		return Publication{}, err
	}
	return Publication{Plan: p, Review: created, Commit: head}, nil
}

func (s Service) findPublishable(ctx context.Context, req review.FindRequest, metadata Metadata, base string) (review.Review, error) {
	found, err := s.Provider.FindReview(ctx, req)
	if errors.Is(err, review.ErrNotFound) {
		return review.Review{}, review.ErrNotFound
	}
	if err != nil {
		code := "review_lookup_failed"
		if errors.Is(err, review.ErrAmbiguous) {
			code = "ambiguous_review"
		}
		return review.Review{}, &Error{Code: code, Message: "could not uniquely identify the plan review; resolve the lookup error before retrying", Cause: err}
	}
	return found, validateReview(found, req, metadata, base)
}

func validateReview(found review.Review, req review.FindRequest, want Metadata, base string) error {
	if err := validateIdentity(found, req.Repository, req.HeadBranch, want, base); err != nil {
		return err
	}
	if found.State != review.Open {
		return &Error{Code: "review_terminal", Message: "the plan review is already closed or merged; publish will not reopen or replace it"}
	}
	return nil
}

func validateIdentity(found review.Review, repository, branch string, want Metadata, base string) error {
	metadata, err := ParseMetadata(found.Body)
	if err != nil || metadata != want || found.Ref.Repository != repository || found.HeadBranch != branch || found.BaseBranch != base || found.Ref.ID == "" || found.Ref.Provider == "" {
		return &Error{Code: "review_conflict", Message: "the existing review does not match this plan's identity and base branch"}
	}
	return nil
}

func checkBranchChanges(ctx context.Context, client *git.Client, base, head, path string) error {
	files, err := client.ChangedFiles(ctx, base, head)
	if err != nil {
		return err
	}
	if err := onlyPlan(files, path); err != nil {
		return err
	}
	files, err = client.HistoryFiles(ctx, base, head)
	if err != nil {
		return &Error{Code: "unsafe_plan_history", Message: "cannot publish this branch history; keep the plan branch free of implementation commits and merges", Cause: err}
	}
	return onlyPlan(files, path)
}

func onlyPlan(paths []string, planPath string) error {
	for _, path := range paths {
		if path != planPath {
			return &Error{Code: "unrelated_changes", Message: "plan branch or index contains changes outside " + planPath + "; move implementation work to its own branch before publishing"}
		}
	}
	return nil
}
