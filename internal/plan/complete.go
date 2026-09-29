package plan

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	"github.com/mkramb/planctl/internal/git"
	"github.com/mkramb/planctl/internal/review"
)

type Completion struct {
	Plan             Plan
	Review           review.Review
	Retention        string
	PrunedRemote     bool
	WorkspaceRemoved bool
}

// Complete finalizes the plan lifecycle: close (pr-only) or merge (repository)
// the review, optionally prune the remote branch, and remove a clean worktree.
func (s Service) Complete(ctx context.Context, req PublishRequest, pruneRemote bool) (Completion, error) {
	if s.Provider == nil {
		return Completion{}, &Error{Code: "provider_unavailable", Message: "GitHub integration is not implemented yet"}
	}
	eval, err := s.Inspect(ctx, req)
	if err != nil {
		// A removed worktree blocks normal resolution. If an explicit plan was
		// requested and the review is already terminal, completion is idempotent
		// and needs no worktree.
		var planErr *Error
		if req.PlanID != "" && errors.As(err, &planErr) && planErr.Code == "workspace_missing" {
			return s.completeTerminal(ctx, req, pruneRemote)
		}
		return Completion{}, err
	}
	if eval.Review == nil {
		return Completion{}, &Error{Code: "review_not_found", Message: "publish the plan before completing it"}
	}
	found := *eval.Review
	expected := review.Closed
	if eval.Retention == "repository" {
		expected = review.Merged
	}
	alreadyFinal := false
	switch found.State {
	case expected:
		alreadyFinal = true
	case review.Closed, review.Merged:
		return Completion{}, &Error{Code: "unexpected_review_state", Message: "the review is already in a different terminal state; it cannot be completed"}
	}
	if !alreadyFinal {
		if !eval.Allowed {
			return Completion{}, &Error{Code: "completion_blocked", Message: "cannot complete until implementation is allowed: " + strings.Join(eval.BlockedReasons, ", ")}
		}
		if expected == review.Merged {
			if err := s.Provider.MergeReview(ctx, found.Ref); err != nil {
				return Completion{}, &Error{Code: "merge_failed", Message: "could not merge the plan review", Cause: err}
			}
			found.State = review.Merged
		} else {
			if err := s.Provider.CloseReview(ctx, found.Ref); err != nil {
				return Completion{}, &Error{Code: "close_failed", Message: "could not close the plan review", Cause: err}
			}
			found.State = review.Closed
		}
	}
	completion := Completion{Plan: eval.Plan, Review: found, Retention: eval.Retention}
	remote := git.New(s.Executor, eval.Plan.WorkspacePath, s.Env)
	if pruneRemote {
		if err := remote.DeleteRemoteBranch(ctx, "origin", eval.Plan.Branch); err != nil {
			return completion, &Error{Code: "prune_failed", Message: "the review was finalized but the remote branch could not be deleted; rerun complete --prune-remote to retry", Cause: err}
		}
		completion.PrunedRemote = true
	}
	dirty, err := remote.Dirty(ctx)
	if err != nil {
		return completion, err
	}
	if !dirty {
		if err := remote.RemoveWorktree(ctx, eval.Plan.WorkspacePath); err == nil {
			completion.WorkspaceRemoved = true
		}
		// A dirty or still-locked worktree is preserved; removal is best-effort.
	}
	return completion, nil
}

// completeTerminal handles idempotent completion after the worktree is gone.
func (s Service) completeTerminal(ctx context.Context, req PublishRequest, pruneRemote bool) (Completion, error) {
	loaded, err := s.loadConfiguration(ctx, req.Dir, req.ConfigPath)
	if err != nil {
		return Completion{}, err
	}
	if loaded.Config.Repositories.Plans != "current" {
		return Completion{}, &Error{Code: "dedicated_repository_unavailable", Message: "dedicated plans repositories are not implemented yet"}
	}
	client := git.New(s.Executor, loaded.RepositoryRoot, s.Env)
	branch := strings.ReplaceAll(loaded.Config.Branch.Pattern, "{slug}", req.PlanID)
	base, _, err := resolveBase(ctx, client, loaded.Config.Branch.Base)
	if err != nil {
		return Completion{}, err
	}
	remote, err := client.Origin(ctx)
	if err != nil {
		return Completion{}, &Error{Code: "invalid_remote", Message: "configure origin before completing", Cause: err}
	}
	repository, err := s.Provider.ResolveRepository(ctx, remote)
	if err != nil {
		return Completion{}, &Error{Code: "repository_lookup_failed", Message: "could not resolve origin's review repository", Cause: err}
	}
	found, err := s.Provider.FindReview(ctx, review.FindRequest{Repository: repository, HeadBranch: branch})
	if err != nil {
		return Completion{}, &Error{Code: "workspace_missing", Message: "plan has no available managed worktree; create the plan or repair its worktree first"}
	}
	if err := validateIdentity(found, repository, branch, Metadata{Version: 1, ID: req.PlanID, ImplementationRepository: repository}, base); err != nil {
		return Completion{}, err
	}
	expected := review.Closed
	if loaded.Config.Plan.Retention == "repository" {
		expected = review.Merged
	}
	if found.State != expected {
		return Completion{}, &Error{Code: "workspace_missing", Message: "plan has no available managed worktree; create the plan or repair its worktree first"}
	}
	p := Plan{
		ID: req.PlanID, Title: req.PlanID, Branch: branch, Base: base,
		Path: filepath.ToSlash(filepath.Join(loaded.Config.Plan.Directory, req.PlanID+".md")),
	}
	completion := Completion{Plan: p, Review: found, Retention: loaded.Config.Plan.Retention, WorkspaceRemoved: true}
	if pruneRemote {
		if err := client.DeleteRemoteBranch(ctx, "origin", branch); err != nil {
			return completion, &Error{Code: "prune_failed", Message: "the review was finalized but the remote branch could not be deleted", Cause: err}
		}
		completion.PrunedRemote = true
	}
	return completion, nil
}
