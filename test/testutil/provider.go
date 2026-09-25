package testutil

import (
	"context"
	"fmt"
	"strconv"
	"sync"

	"github.com/mkramb/planctl/internal/review"
)

// MockReviewProvider models remote state, not expected method calls. Its name is
// intentionally not github: lifecycle tests must not depend on a provider name.
type MockReviewProvider struct {
	mu       sync.Mutex
	nextID   int
	reviews  map[review.Ref]review.Review
	feedback map[review.Ref][]review.Feedback
}

var _ review.Provider = (*MockReviewProvider)(nil)

func NewMockReviewProvider() *MockReviewProvider {
	return &MockReviewProvider{
		reviews: make(map[review.Ref]review.Review), feedback: make(map[review.Ref][]review.Feedback),
	}
}

func (p *MockReviewProvider) CreateReview(ctx context.Context, req review.CreateRequest) (review.Review, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return review.Review{}, err
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
	p.reviews[ref] = created
	p.feedback[ref] = []review.Feedback{}
	return cloneReview(created), nil
}

func (p *MockReviewProvider) FindReview(ctx context.Context, req review.FindRequest) (review.Review, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := ctx.Err(); err != nil {
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
	return *found, nil
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
	return cloneReview(found), nil
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
	found.Decisions = append(found.Decisions, review.Decision{
		ID: strconv.Itoa(len(found.Decisions) + 1), Author: author, State: state, CommitID: found.HeadCommit,
	})
	p.reviews[ref] = found
	return nil
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
