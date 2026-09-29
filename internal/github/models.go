// Package github translates between GitHub (via the gh CLI) and planctl's
// provider-neutral review model. It is the only package that knows GitHub shapes.
package github

type prView struct {
	Number      int    `json:"number"`
	Title       string `json:"title"`
	Body        string `json:"body"`
	URL         string `json:"url"`
	State       string `json:"state"` // OPEN, CLOSED, MERGED
	IsDraft     bool   `json:"isDraft"`
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

type apiComment struct {
	ID        int64   `json:"id"`
	User      apiUser `json:"user"`
	Body      string  `json:"body"`
	CreatedAt string  `json:"created_at"`
}

type apiReviewComment struct {
	ID           int64   `json:"id"`
	User         apiUser `json:"user"`
	Body         string  `json:"body"`
	Path         string  `json:"path"`
	Line         *int    `json:"line"`
	OriginalLine *int    `json:"original_line"`
	CreatedAt    string  `json:"created_at"`
}
