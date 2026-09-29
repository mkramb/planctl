package testutil

import (
	"context"
	"fmt"
	"strconv"
	"sync"

	"github.com/mkramb/planctl/internal/git"
	"github.com/mkramb/planctl/internal/process"
	"github.com/mkramb/planctl/internal/review"
)

// MockReviewProvider models remote state, not expected method calls. Its name is
// intentionally not github: lifecycle tests must not depend on a provider name.
type MockReviewProvider struct {
	mu              sync.Mutex
	nextID          int
	reviews         map[review.Ref]review.Review
	feedback        map[review.Ref][]review.Feedback
	remoteIDs       map[string]string
	repositories    map[string]*git.Client
	createFailure   error
	failAfterCreate bool
	lookupFailure   error
}

var _ review.Provider = (*MockReviewProvider)(nil)

func NewMockReviewProvider() *MockReviewProvider {
	return &MockReviewProvider{
		reviews: make(map[review.Ref]review.Review), feedback: make(map[review.Ref][]review.Feedback),
		remoteIDs: make(map[string]string), repositories: make(map[string]*git.Client),
	}
}

// RegisterRepository lets the fake observe actual pushes to a local bare remote.
// This models the review platform following branch updates without adding an
// artificial UpdateReview method to the production provider interface.
func (p *MockReviewProvider) RegisterRepository(id, remote string, env []string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.remoteIDs[remote] = id
	p.repositories[id] = git.New(process.Runner{}, remote, env)
}

func (p *MockReviewProvider) ResolveRepository(ctx context.Context, remote string) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return "", err
	}
	id, ok := p.remoteIDs[remote]
	if !ok {
		return "", review.ErrRepositoryNotFound
	}
	return id, nil
}

func (p *MockReviewProvider) CreateReview(ctx context.Context, req review.CreateRequest) (review.Review, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return review.Review{}, err
	}
	failure, after := p.createFailure, p.failAfterCreate
	p.createFailure = nil
	if failure != nil && !after {
		return review.Review{}, failure
	}
	for _, existing := range p.reviews {
		if existing.Ref.Repository == req.Repository && existing.HeadBranch == req.HeadBranch && existing.State == review.Open {
			return review.Review{}, review.ErrExists
		}
	}
	p.nextID++
	ref := review.Ref{Provider: "mock", Repository: req.Repository, ID: strconv.Itoa(p.nextID)}
	created := review.Review{
		Ref: ref, Title: req.Title, Body: req.Body, URL: "https://review.invalid/" + req.Repository + "/" + ref.ID,
		State: review.Open, Draft: req.Draft, HeadBranch: req.HeadBranch, BaseBranch: req.BaseBranch,
		HeadCommit: req.HeadCommit, Decisions: []review.Decision{},
	}
	if remote := p.repositories[req.Repository]; remote != nil {
		head, found, err := remote.ResolveCommit(ctx, "refs/heads/"+req.HeadBranch)
		if err != nil {
			return review.Review{}, err
		}
		if !found || head != req.HeadCommit {
			return review.Review{}, fmt.Errorf("review head must exist on the remote before review creation")
		}
	}
	p.reviews[ref] = created
	p.feedback[ref] = []review.Feedback{}
	if failure != nil {
		return review.Review{}, failure
	}
	return cloneReview(created), nil
}

func (p *MockReviewProvider) FindReview(ctx context.Context, req review.FindRequest) (review.Review, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return review.Review{}, err
	}
	if p.lookupFailure != nil {
		err := p.lookupFailure
		p.lookupFailure = nil
		return review.Review{}, err
	}
	var found *review.Review
	for _, candidate := range p.reviews {
		if candidate.Ref.Repository != req.Repository || candidate.HeadBranch != req.HeadBranch {
			continue
		}
		if found != nil {
			return review.Review{}, review.ErrAmbiguous
		}
		copy := cloneReview(candidate)
		found = &copy
	}
	if found == nil {
		return review.Review{}, review.ErrNotFound
	}
	return p.refresh(ctx, *found)
}

func (p *MockReviewProvider) GetReview(ctx context.Context, ref review.Ref) (review.Review, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return review.Review{}, err
	}
	found, ok := p.reviews[ref]
	if !ok {
		return review.Review{}, review.ErrNotFound
	}
	return p.refresh(ctx, found)
}

func (p *MockReviewProvider) Feedback(ctx context.Context, ref review.Ref) ([]review.Feedback, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	found, ok := p.feedback[ref]
	if !ok {
		return nil, review.ErrNotFound
	}
	return cloneFeedback(found), nil
}

func (p *MockReviewProvider) AddFeedback(ref review.Ref, item review.Feedback) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, ok := p.reviews[ref]; !ok {
		return review.ErrNotFound
	}
	if item.ID == "" {
		item.ID = strconv.Itoa(len(p.feedback[ref]) + 1)
	}
	p.feedback[ref] = append(p.feedback[ref], cloneFeedback([]review.Feedback{item})[0])
	return nil
}

func (p *MockReviewProvider) Approve(ref review.Ref, author string) error {
	return p.addDecision(ref, author, review.Approved)
}

func (p *MockReviewProvider) RequestChanges(ref review.Ref, author string) error {
	return p.addDecision(ref, author, review.ChangesRequested)
}

func (p *MockReviewProvider) addDecision(ref review.Ref, author string, state review.DecisionState) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	found, ok := p.reviews[ref]
	if !ok {
		return review.ErrNotFound
	}
	if found.State != review.Open {
		return fmt.Errorf("cannot review a %s review", found.State)
	}
	var err error
	found, err = p.refresh(context.Background(), found)
	if err != nil {
		return err
	}
	found.Decisions = append(found.Decisions, review.Decision{
		ID: strconv.Itoa(len(found.Decisions) + 1), Author: author, State: state, CommitID: found.HeadCommit,
	})
	p.reviews[ref] = found
	return nil
}

// refresh is called with the mutex held and never changes existing decisions.
func (p *MockReviewProvider) refresh(ctx context.Context, found review.Review) (review.Review, error) {
	if remote := p.repositories[found.Ref.Repository]; remote != nil && found.State == review.Open {
		head, exists, err := remote.ResolveCommit(ctx, "refs/heads/"+found.HeadBranch)
		if err != nil {
			return review.Review{}, err
		}
		if !exists {
			return review.Review{}, fmt.Errorf("open review's remote branch is missing")
		}
		found.HeadCommit = head
		p.reviews[found.Ref] = found
	}
	return cloneReview(found), nil
}

func (p *MockReviewProvider) FailNextCreate(err error, afterCreation bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.createFailure, p.failAfterCreate = err, afterCreation
}

func (p *MockReviewProvider) FailNextLookup(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.lookupFailure = err
}

func (p *MockReviewProvider) SetState(ref review.Ref, state review.State) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	found, ok := p.reviews[ref]
	if !ok {
		return review.ErrNotFound
	}
	found.State = state
	p.reviews[ref] = found
	return nil
}

func (p *MockReviewProvider) CloseReview(ctx context.Context, ref review.Ref) error {
	return p.terminate(ctx, ref, review.Closed)
}

func (p *MockReviewProvider) MergeReview(ctx context.Context, ref review.Ref) error {
	return p.terminate(ctx, ref, review.Merged)
}

func (p *MockReviewProvider) terminate(ctx context.Context, ref review.Ref, state review.State) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	found, ok := p.reviews[ref]
	if !ok {
		return review.ErrNotFound
	}
	if found.State == state {
		return nil
	}
	if found.State != review.Open {
		return fmt.Errorf("cannot move a %s review to %s", found.State, state)
	}
	found.State = state
	p.reviews[ref] = found
	return nil
}

func (p *MockReviewProvider) ReviewCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.reviews)
}

// SetHead simulates a remote branch push. Previous review decisions retain the
// revision they actually reviewed rather than being silently moved to the head.
func (p *MockReviewProvider) SetHead(ref review.Ref, commit string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	found, ok := p.reviews[ref]
	if !ok {
		return review.ErrNotFound
	}
	found.HeadCommit = commit
	p.reviews[ref] = found
	return nil
}

func cloneReview(value review.Review) review.Review {
	value.Decisions = append([]review.Decision{}, value.Decisions...)
	return value
}

func cloneFeedback(values []review.Feedback) []review.Feedback {
	result := make([]review.Feedback, len(values))
	copy(result, values)
	for i := range result {
		if result[i].Line != nil {
			line := *result[i].Line
			result[i].Line = &line
		}
	}
	return result
}
