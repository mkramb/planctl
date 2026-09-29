package github

import "errors"

var (
	ErrNotFound           = errors.New("review not found")
	ErrExists             = errors.New("an open review already exists for this branch")
	ErrAmbiguous          = errors.New("multiple reviews match this branch")
	ErrRepositoryNotFound = errors.New("review repository not found")
)

// Ref scopes an ID to its repository; IDs alone are not globally unique.
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

// Decision preserves review history. Lifecycle code calculates effective votes.
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
	Reviewers  []string
}

type FindRequest struct {
	Repository string
	HeadBranch string
}
