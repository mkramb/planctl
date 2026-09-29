// Package github translates between GitHub (via the gh CLI) and planctl's
// review model. It is the only package that knows GitHub's shapes.
package github

type prView struct {
	Number      int    `json:"number"`
	Title       string `json:"title"`
	Body        string `json:"body"`
	URL         string `json:"url"`
	State       string `json:"state"` // OPEN, CLOSED, MERGED
	HeadRefName string `json:"headRefName"`
	BaseRefName string `json:"baseRefName"`
	HeadRefOid  string `json:"headRefOid"`
}

type repoView struct {
	NameWithOwner string `json:"nameWithOwner"`
}

type apiUser struct {
	Login string `json:"login"`
}

type apiReview struct {
	ID          int64   `json:"id"`
	User        apiUser `json:"user"`
	State       string  `json:"state"` // APPROVED, CHANGES_REQUESTED, COMMENTED, DISMISSED, PENDING
	CommitID    string  `json:"commit_id"`
	Body        string  `json:"body"`
	SubmittedAt string  `json:"submitted_at"`
}

type apiReviewComment struct {
	ID        int64   `json:"id"`
	User      apiUser `json:"user"`
	Body      string  `json:"body"`
	Path      string  `json:"path"`
	Line      *int    `json:"line"`
	CreatedAt string  `json:"created_at"`
}
