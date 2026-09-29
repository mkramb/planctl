package github

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/mkramb/planctl/internal/process"
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

func (p *Provider) ResolveRepository(ctx context.Context, remote string) (string, error) {
	name := remote
	if parsed, err := RepoFromRemote(remote); err == nil {
		name = parsed
	}
	out, err := p.gh.Run(ctx, "repo", "view", name, "--json", "nameWithOwner")
	if err != nil {
		if NotFound(err) {
			return "", ErrRepositoryNotFound
		}
		return "", err
	}
	var repo repoView
	if err := json.Unmarshal([]byte(out), &repo); err != nil || repo.NameWithOwner == "" {
		return "", fmt.Errorf("could not parse gh repo view output")
	}
	return repo.NameWithOwner, nil
}

func (p *Provider) CreateReview(ctx context.Context, req CreateRequest) (Review, error) {
	args := []string{
		"pr", "create", "--repo", req.Repository,
		"--head", req.HeadBranch, "--base", req.BaseBranch,
		"--title", req.Title, "--body", req.Body,
	}
	for _, reviewer := range req.Reviewers {
		args = append(args, "--reviewer", reviewer)
	}
	if _, err := p.gh.Run(ctx, args...); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "already exists") {
			return Review{}, ErrExists
		}
		return Review{}, err
	}
	return p.FindReview(ctx, FindRequest{Repository: req.Repository, HeadBranch: req.HeadBranch})
}

func (p *Provider) FindReview(ctx context.Context, req FindRequest) (Review, error) {
	out, err := p.gh.Run(ctx, "pr", "list", "--repo", req.Repository,
		"--head", req.HeadBranch, "--state", "all", "--json", "number", "--limit", "50")
	if err != nil {
		return Review{}, err
	}
	var matches []struct {
		Number int `json:"number"`
	}
	if err := json.Unmarshal([]byte(out), &matches); err != nil {
		return Review{}, fmt.Errorf("could not parse gh pr list output")
	}
	switch len(matches) {
	case 0:
		return Review{}, ErrNotFound
	case 1:
	default:
		return Review{}, ErrAmbiguous
	}
	return p.GetReview(ctx, Ref{Provider: providerName, Repository: req.Repository, ID: strconv.Itoa(matches[0].Number)})
}

func (p *Provider) GetReview(ctx context.Context, ref Ref) (Review, error) {
	out, err := p.gh.Run(ctx, "pr", "view", ref.ID, "--repo", ref.Repository, "--json",
		"number,title,body,url,state,headRefName,baseRefName,headRefOid")
	if err != nil {
		if NotFound(err) {
			return Review{}, ErrNotFound
		}
		return Review{}, err
	}
	var pr prView
	if err := json.Unmarshal([]byte(out), &pr); err != nil || pr.Number == 0 {
		return Review{}, fmt.Errorf("could not parse gh pr view output")
	}
	decisions, err := p.decisions(ctx, ref.Repository, ref.ID)
	if err != nil {
		return Review{}, err
	}
	return Review{
		Ref:   Ref{Provider: providerName, Repository: ref.Repository, ID: strconv.Itoa(pr.Number)},
		Title: pr.Title, Body: pr.Body, URL: pr.URL,
		State:      prState(pr.State),
		HeadBranch: pr.HeadRefName, BaseBranch: pr.BaseRefName, HeadCommit: pr.HeadRefOid,
		Decisions: decisions,
	}, nil
}

func (p *Provider) Feedback(ctx context.Context, ref Ref, head string) ([]Feedback, error) {
	type stamped struct {
		item    Feedback
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
		// A review summary submitted against an older revision is stale.
		if head != "" && r.CommitID != "" && r.CommitID != head {
			continue
		}
		all = append(all, stamped{
			item:    Feedback{ID: "review-" + strconv.FormatInt(r.ID, 10), Author: r.User.Login, Body: r.Body},
			created: r.SubmittedAt,
		})
	}
	var lineComments []apiReviewComment
	if err := apiGet(ctx, p.gh, "repos/"+ref.Repository+"/pulls/"+ref.ID+"/comments", &lineComments); err != nil {
		return nil, err
	}
	for _, c := range lineComments {
		// A comment with no current line is outdated (tied to an older revision).
		if c.Line == nil {
			continue
		}
		all = append(all, stamped{
			item: Feedback{
				ID: "line-" + strconv.FormatInt(c.ID, 10), Author: c.User.Login,
				Body: c.Body, Path: c.Path, Line: c.Line,
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
	result := make([]Feedback, len(all))
	for i, entry := range all {
		result[i] = entry.item
	}
	return result, nil
}

func (p *Provider) decisions(ctx context.Context, repository, id string) ([]Decision, error) {
	reviews, err := p.reviews(ctx, repository, id)
	if err != nil {
		return nil, err
	}
	var decisions []Decision
	for _, r := range reviews {
		state, ok := decisionState(r.State)
		if !ok {
			continue
		}
		decisions = append(decisions, Decision{
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
			return ErrNotFound
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

func (p *Provider) CloseReview(ctx context.Context, ref Ref) error {
	_, err := p.gh.Run(ctx, "pr", "close", ref.ID, "--repo", ref.Repository)
	if err != nil {
		return err
	}
	return nil
}

func (p *Provider) AddReviewers(ctx context.Context, ref Ref, reviewers []string) error {
	for _, reviewer := range reviewers {
		if _, err := p.gh.Run(ctx, "pr", "edit", ref.ID, "--repo", ref.Repository, "--add-reviewer", reviewer); err != nil {
			return err
		}
	}
	return nil
}

func prState(state string) State {
	switch strings.ToUpper(state) {
	case "MERGED":
		return Merged
	case "CLOSED":
		return Closed
	default:
		return Open
	}
}

func decisionState(state string) (DecisionState, bool) {
	switch strings.ToUpper(state) {
	case "APPROVED":
		return Approved, true
	case "CHANGES_REQUESTED":
		return ChangesRequested, true
	case "COMMENTED":
		return Commented, true
	case "DISMISSED":
		return Dismissed, true
	default:
		return "", false
	}
}
