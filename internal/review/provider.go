// Package review defines provider-neutral review data and operations.
package review

import (
	"context"
	"errors"
)

var (
	ErrNotFound           = errors.New("review not found")
	ErrExists             = errors.New("an open review already exists for this branch")
	ErrAmbiguous          = errors.New("multiple reviews match this branch")
	ErrRepositoryNotFound = errors.New("review repository not found")
)

// Ref scopes an ID to its provider and repository; IDs alone are not globally unique.
type Ref struct {
	Provider   string `json:"provider"`
	Repository string `json:"repository"`
	ID         string `json:"id"`
}

type State string

const (
	Open   State = "open"
	Closed State = "closed"
	Merged State = "merged"
)

type DecisionState string

const (
	Approved         DecisionState = "approved"
	ChangesRequested DecisionState = "changes_requested"
	Commented        DecisionState = "commented"
	Dismissed        DecisionState = "dismissed"
)

// Decision preserves review history. Lifecycle code will calculate effective votes.
type Decision struct {
	ID       string        `json:"id"`
	Author   string        `json:"author"`
	State    DecisionState `json:"state"`
	CommitID string        `json:"commit_id"`
}

type Review struct {
	Ref        Ref        `json:"ref"`
	Title      string     `json:"title"`
	Body       string     `json:"body"`
	URL        string     `json:"url"`
	State      State      `json:"state"`
	Draft      bool       `json:"draft"`
	HeadBranch string     `json:"head_branch"`
	BaseBranch string     `json:"base_branch"`
	HeadCommit string     `json:"head_commit"`
	Decisions  []Decision `json:"decisions"`
}

type Feedback struct {
	ID     string `json:"id"`
	Author string `json:"author"`
	Body   string `json:"body"`
	Path   string `json:"path,omitempty"`
	Line   *int   `json:"line,omitempty"`
}

type CreateRequest struct {
	Repository string
	Title      string
	Body       string
	HeadBranch string
	BaseBranch string
	HeadCommit string
	Draft      bool
}

type FindRequest struct {
	Repository string
	HeadBranch string
}

// Provider is deliberately small. Completion operations will be added when used.
type Provider interface {
	ResolveRepository(context.Context, string) (string, error)
	CreateReview(context.Context, CreateRequest) (Review, error)
	FindReview(context.Context, FindRequest) (Review, error)
	GetReview(context.Context, Ref) (Review, error)
	Feedback(context.Context, Ref) ([]Feedback, error)
	// CloseReview closes without merging; MergeReview merges into the base.
	// Repeating the achieved terminal state succeeds; the other terminal state
	// is an error.
	CloseReview(context.Context, Ref) error
	MergeReview(context.Context, Ref) error
}
