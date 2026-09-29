package github

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/mkramb/planctl/internal/process"
	"github.com/mkramb/planctl/internal/review"
)

const providerName = "github"

type Provider struct {
	gh Cmd
}

func NewProvider(exec process.Executor) *Provider {
	return &Provider{gh: runner{exec: exec}}
}

// NewProviderWithCmd builds a provider with a substituted gh (used in tests).
func NewProviderWithCmd(gh Cmd) *Provider {
	return &Provider{gh: gh}
}

var _ review.Provider = (*Provider)(nil)

func (p *Provider) ResolveRepository(ctx context.Context, remote string) (string, error) {
	name, err := RepoFromRemote(remote)
	if err != nil {
		return "", err
	}
	out, err := p.gh.Run(ctx, "repo", "view", name, "--json", "nameWithOwner")
	if err != nil {
		if NotFound(err) {
			return "", review.ErrRepositoryNotFound
		}
		return "", err
	}
	var repo repoView
	if err := json.Unmarshal([]byte(out), &repo); err != nil || repo.NameWithOwner == "" {
		return "", fmt.Errorf("could not parse gh repo view output")
	}
	return repo.NameWithOwner, nil
}

func (p *Provider) CreateReview(ctx context.Context, req review.CreateRequest) (review.Review, error) {
	args := []string{
		"pr", "create", "--repo", req.Repository,
		"--head", req.HeadBranch, "--base", req.BaseBranch,
		"--title", req.Title, "--body", req.Body,
	}
	if req.Draft {
		args = append(args, "--draft")
	}
	if _, err := p.gh.Run(ctx, args...); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "already exists") {
			return review.Review{}, review.ErrExists
		}
		return review.Review{}, err
	}
	return p.FindReview(ctx, review.FindRequest{Repository: req.Repository, HeadBranch: req.HeadBranch})
}

func (p *Provider) FindReview(ctx context.Context, req review.FindRequest) (review.Review, error) {
	out, err := p.gh.Run(ctx, "pr", "list", "--repo", req.Repository,
		"--head", req.HeadBranch, "--state", "all", "--json", "number", "--limit", "50")
	if err != nil {
		return review.Review{}, err
	}
	var matches []struct {
		Number int `json:"number"`
	}
	if err := json.Unmarshal([]byte(out), &matches); err != nil {
		return review.Review{}, fmt.Errorf("could not parse gh pr list output")
	}
	switch len(matches) {
	case 0:
		return review.Review{}, review.ErrNotFound
	case 1:
	default:
		return review.Review{}, review.ErrAmbiguous
	}
	return p.GetReview(ctx, review.Ref{Provider: providerName, Repository: req.Repository, ID: strconv.Itoa(matches[0].Number)})
}

func (p *Provider) GetReview(ctx context.Context, ref review.Ref) (review.Review, error) {
	out, err := p.gh.Run(ctx, "pr", "view", ref.ID, "--repo", ref.Repository, "--json",
		"number,title,body,url,state,isDraft,headRefName,baseRefName,headRefOid")
	if err != nil {
		if NotFound(err) {
			return review.Review{}, review.ErrNotFound
		}
		return review.Review{}, err
	}
	var pr prView
	if err := json.Unmarshal([]byte(out), &pr); err != nil || pr.Number == 0 {
		return review.Review{}, fmt.Errorf("could not parse gh pr view output")
	}
	decisions, err := p.decisions(ctx, ref.Repository, ref.ID)
	if err != nil {
		return review.Review{}, err
	}
	return review.Review{
		Ref:   review.Ref{Provider: providerName, Repository: ref.Repository, ID: strconv.Itoa(pr.Number)},
		Title: pr.Title, Body: pr.Body, URL: pr.URL,
		State: prState(pr.State), Draft: pr.IsDraft,
		HeadBranch: pr.HeadRefName, BaseBranch: pr.BaseRefName, HeadCommit: pr.HeadRefOid,
		Decisions: decisions,
	}, nil
}

func (p *Provider) Feedback(ctx context.Context, ref review.Ref) ([]review.Feedback, error) {
	type stamped struct {
		item    review.Feedback
		created string
	}
	var all []stamped
	reviews, err := p.reviews(ctx, ref.Repository, ref.ID)
	if err != nil {
		return nil, err
	}
	for _, r := range reviews {
		if strings.TrimSpace(r.Body) == "" || strings.EqualFold(r.State, "PENDING") {
			continue
		}
		all = append(all, stamped{
			item:    review.Feedback{ID: "review-" + strconv.FormatInt(r.ID, 10), Author: r.User.Login, Body: r.Body},
			created: r.SubmittedAt,
		})
	}
	var issueComments []apiComment
	if err := apiGet(ctx, p.gh, "repos/"+ref.Repository+"/issues/"+ref.ID+"/comments", &issueComments); err != nil {
		return nil, err
	}
	for _, c := range issueComments {
		all = append(all, stamped{
			item:    review.Feedback{ID: "comment-" + strconv.FormatInt(c.ID, 10), Author: c.User.Login, Body: c.Body},
			created: c.CreatedAt,
		})
	}
	var lineComments []apiReviewComment
	if err := apiGet(ctx, p.gh, "repos/"+ref.Repository+"/pulls/"+ref.ID+"/comments", &lineComments); err != nil {
		return nil, err
	}
	for _, c := range lineComments {
		line := c.Line
		if line == nil {
			line = c.OriginalLine
		}
		all = append(all, stamped{
			item: review.Feedback{
				ID: "line-" + strconv.FormatInt(c.ID, 10), Author: c.User.Login,
				Body: c.Body, Path: c.Path, Line: line,
			},
			created: c.CreatedAt,
		})
	}
	// Deterministic order: by time, then by ID, across all feedback sources.
	sort.SliceStable(all, func(i, j int) bool {
		if all[i].created != all[j].created {
			return all[i].created < all[j].created
		}
		return all[i].item.ID < all[j].item.ID
	})
	result := make([]review.Feedback, len(all))
	for i, entry := range all {
		result[i] = entry.item
	}
	return result, nil
}

func (p *Provider) decisions(ctx context.Context, repository, id string) ([]review.Decision, error) {
	reviews, err := p.reviews(ctx, repository, id)
	if err != nil {
		return nil, err
	}
	var decisions []review.Decision
	for _, r := range reviews {
		state, ok := decisionState(r.State)
		if !ok {
			continue
		}
		decisions = append(decisions, review.Decision{
			ID: strconv.FormatInt(r.ID, 10), Author: r.User.Login, State: state, CommitID: r.CommitID,
		})
	}
	return decisions, nil
}

func (p *Provider) reviews(ctx context.Context, repository, id string) ([]apiReview, error) {
	var reviews []apiReview
	if err := apiGet(ctx, p.gh, "repos/"+repository+"/pulls/"+id+"/reviews", &reviews); err != nil {
		return nil, err
	}
	return reviews, nil
}

// apiGet fetches a list endpoint. gh api --paginate emits one JSON array per
// page; each page must decode into a fresh slice because Go resets slices.
func apiGet[T any](ctx context.Context, gh Cmd, endpoint string, target *[]T) error {
	out, err := gh.Run(ctx, "api", "--paginate", endpoint)
	if err != nil {
		if NotFound(err) {
			return review.ErrNotFound
		}
		return err
	}
	decoder := json.NewDecoder(strings.NewReader(out))
	for decoder.More() {
		var page []T
		if err := decoder.Decode(&page); err != nil {
			return fmt.Errorf("could not parse gh api output for %s", endpoint)
		}
		*target = append(*target, page...)
	}
	return nil
}

func (p *Provider) CloseReview(ctx context.Context, ref review.Ref) error {
	_, err := p.gh.Run(ctx, "pr", "close", ref.ID, "--repo", ref.Repository)
	if err != nil {
		return err
	}
	return nil
}

func (p *Provider) MergeReview(ctx context.Context, ref review.Ref) error {
	_, err := p.gh.Run(ctx, "pr", "merge", ref.ID, "--repo", ref.Repository, "--merge")
	if err != nil {
		return err
	}
	return nil
}

func prState(state string) review.State {
	switch strings.ToUpper(state) {
	case "MERGED":
		return review.Merged
	case "CLOSED":
		return review.Closed
	default:
		return review.Open
	}
}

func decisionState(state string) (review.DecisionState, bool) {
	switch strings.ToUpper(state) {
	case "APPROVED":
		return review.Approved, true
	case "CHANGES_REQUESTED":
		return review.ChangesRequested, true
	case "COMMENTED":
		return review.Commented, true
	case "DISMISSED":
		return review.Dismissed, true
	default:
		return "", false
	}
}
