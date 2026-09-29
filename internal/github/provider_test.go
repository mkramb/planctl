package github_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/mkramb/planctl/internal/github"
	"github.com/mkramb/planctl/internal/review"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type canned struct {
	match  string
	stdout string
	err    error
}

// fakeGH answers gh invocations by matching their joined arguments.
type fakeGH struct {
	responses []canned
	calls     []string
}

func (f *fakeGH) Run(_ context.Context, args ...string) (string, error) {
	joined := strings.Join(args, " ")
	f.calls = append(f.calls, joined)
	for i, c := range f.responses {
		if strings.Contains(joined, c.match) {
			f.responses = append(f.responses[:i], f.responses[i+1:]...)
			return c.stdout, c.err
		}
	}
	return "", errors.New("unexpected gh call: " + joined)
}

func (f *fakeGH) requireDrained(t *testing.T) {
	t.Helper()
	assert.Empty(t, f.responses, "expected gh calls were not made")
}

const prJSON = `{"number":142,"title":"Plan: Add SSO","body":"<!-- planctl -->","url":"https://github.com/acme/payments/pull/142","state":"OPEN","isDraft":false,"headRefName":"plan/add-sso","baseRefName":"main","headRefOid":"abc123"}`

func TestResolveRepository(t *testing.T) {
	t.Parallel()
	gh := &fakeGH{responses: []canned{
		{match: "repo view acme/payments", stdout: `{"nameWithOwner":"acme/payments"}`},
	}}
	p := github.NewProviderWithCmd(gh)
	repo, err := p.ResolveRepository(t.Context(), "git@github.com:acme/payments.git")
	require.NoError(t, err)
	assert.Equal(t, "acme/payments", repo)
	gh.requireDrained(t)

	gh = &fakeGH{responses: []canned{
		{match: "repo view", err: &github.Error{Command: "repo", Detail: "Could not resolve to a Repository"}},
	}}
	p = github.NewProviderWithCmd(gh)
	_, err = p.ResolveRepository(t.Context(), "https://github.com/acme/missing.git")
	require.ErrorIs(t, err, review.ErrRepositoryNotFound)

	_, err = p.ResolveRepository(t.Context(), "/tmp/local/remote.git")
	assert.ErrorContains(t, err, "not a GitHub URL")
}

func TestRepoFromRemote(t *testing.T) {
	t.Parallel()
	for remote, want := range map[string]string{
		"git@github.com:acme/payments.git":     "acme/payments",
		"git@github.com:acme/payments":         "acme/payments",
		"https://github.com/acme/payments.git": "acme/payments",
		"https://github.com/acme/payments/":    "acme/payments",
		"ssh://git@github.com/acme/payments":   "acme/payments",
		"https://ghe.acme.inc/team/repo.git":   "team/repo",
	} {
		got, err := github.RepoFromRemote(remote)
		require.NoError(t, err, remote)
		assert.Equal(t, want, got, remote)
	}
	for _, bad := range []string{"", "/tmp/repo", "https://github.com/only-owner", "git@github.com"} {
		_, err := github.RepoFromRemote(bad)
		assert.Error(t, err, bad)
	}
}

func TestCreateAndFindReview(t *testing.T) {
	t.Parallel()
	gh := &fakeGH{responses: []canned{
		{match: "pr create --repo acme/payments --head plan/add-sso --base main", stdout: "https://github.com/acme/payments/pull/142\n"},
		{match: "pr list --repo acme/payments --head plan/add-sso", stdout: `[{"number":142}]`},
		{match: "pr view 142", stdout: prJSON},
		{match: "api --paginate repos/acme/payments/pulls/142/reviews", stdout: `[]`},
	}}
	p := github.NewProviderWithCmd(gh)
	created, err := p.CreateReview(t.Context(), review.CreateRequest{
		Repository: "acme/payments", Title: "Plan: Add SSO", Body: "<!-- planctl -->",
		HeadBranch: "plan/add-sso", BaseBranch: "main", HeadCommit: "abc123",
	})
	require.NoError(t, err)
	assert.Equal(t, review.Ref{Provider: "github", Repository: "acme/payments", ID: "142"}, created.Ref)
	assert.Equal(t, review.Open, created.State)
	assert.False(t, created.Draft)
	assert.Equal(t, "abc123", created.HeadCommit)
	assert.Equal(t, "plan/add-sso", created.HeadBranch)
	assert.Equal(t, "main", created.BaseBranch)
	assert.Equal(t, "https://github.com/acme/payments/pull/142", created.URL)
	assert.NotContains(t, strings.Join(gh.calls, "\n"), "--draft")
	gh.requireDrained(t)
}

func TestCreateReviewDraftAndAlreadyExists(t *testing.T) {
	t.Parallel()
	gh := &fakeGH{responses: []canned{
		{match: "--draft", stdout: "https://github.com/acme/payments/pull/1\n"},
		{match: "pr list", stdout: `[{"number":1}]`},
		{match: "pr view 1", stdout: strings.Replace(prJSON, `"isDraft":false`, `"isDraft":true`, 1)},
		{match: "reviews", stdout: `[]`},
	}}
	p := github.NewProviderWithCmd(gh)
	created, err := p.CreateReview(t.Context(), review.CreateRequest{
		Repository: "acme/payments", HeadBranch: "plan/add-sso", BaseBranch: "main", Draft: true,
	})
	require.NoError(t, err)
	assert.True(t, created.Draft)

	gh = &fakeGH{responses: []canned{
		{match: "pr create", err: errors.New("a pull request already exists")},
	}}
	p = github.NewProviderWithCmd(gh)
	_, err = p.CreateReview(t.Context(), review.CreateRequest{Repository: "acme/payments"})
	require.ErrorIs(t, err, review.ErrExists)
}

func TestFindReviewMissingAmbiguousAndStates(t *testing.T) {
	t.Parallel()
	gh := &fakeGH{responses: []canned{{match: "pr list", stdout: `[]`}}}
	p := github.NewProviderWithCmd(gh)
	_, err := p.FindReview(t.Context(), review.FindRequest{Repository: "acme/payments", HeadBranch: "plan/x"})
	require.ErrorIs(t, err, review.ErrNotFound)

	gh = &fakeGH{responses: []canned{{match: "pr list", stdout: `[{"number":1},{"number":2}]`}}}
	p = github.NewProviderWithCmd(gh)
	_, err = p.FindReview(t.Context(), review.FindRequest{Repository: "acme/payments", HeadBranch: "plan/x"})
	require.ErrorIs(t, err, review.ErrAmbiguous)

	for state, want := range map[string]review.State{"MERGED": review.Merged, "CLOSED": review.Closed, "OPEN": review.Open} {
		gh = &fakeGH{responses: []canned{
			{match: "pr view 142", stdout: strings.Replace(prJSON, `"state":"OPEN"`, `"state":"`+state+`"`, 1)},
			{match: "reviews", stdout: `[]`},
		}}
		p = github.NewProviderWithCmd(gh)
		got, err := p.GetReview(t.Context(), review.Ref{Provider: "github", Repository: "acme/payments", ID: "142"})
		require.NoError(t, err, state)
		assert.Equal(t, want, got.State, state)
	}
}

func TestGetReviewDecisions(t *testing.T) {
	t.Parallel()
	gh := &fakeGH{responses: []canned{
		{match: "pr view 142", stdout: prJSON},
		{match: "reviews", stdout: `[
			{"id":10,"user":{"login":"alice"},"state":"CHANGES_REQUESTED","commit_id":"old","body":"fix it","submitted_at":"2026-01-01T10:00:00Z"},
			{"id":11,"user":{"login":"alice"},"state":"APPROVED","commit_id":"abc123","body":"lgtm","submitted_at":"2026-01-02T10:00:00Z"},
			{"id":12,"user":{"login":"bob"},"state":"COMMENTED","commit_id":"abc123","body":"note","submitted_at":"2026-01-03T10:00:00Z"},
			{"id":13,"user":{"login":"carol"},"state":"DISMISSED","commit_id":"abc123","body":"","submitted_at":"2026-01-04T10:00:00Z"},
			{"id":14,"user":{"login":"dave"},"state":"PENDING","commit_id":"abc123","body":"","submitted_at":""}
		]`},
	}}
	p := github.NewProviderWithCmd(gh)
	got, err := p.GetReview(t.Context(), review.Ref{Provider: "github", Repository: "acme/payments", ID: "142"})
	require.NoError(t, err)
	assert.Equal(t, []review.Decision{
		{ID: "10", Author: "alice", State: review.ChangesRequested, CommitID: "old"},
		{ID: "11", Author: "alice", State: review.Approved, CommitID: "abc123"},
		{ID: "12", Author: "bob", State: review.Commented, CommitID: "abc123"},
		{ID: "13", Author: "carol", State: review.Dismissed, CommitID: "abc123"},
	}, got.Decisions)
}

func TestFeedbackAggregationAndOrdering(t *testing.T) {
	t.Parallel()
	gh := &fakeGH{responses: []canned{
		{match: "pulls/142/reviews", stdout: `[
			{"id":10,"user":{"login":"alice"},"state":"COMMENTED","body":"summary note","submitted_at":"2026-01-03T10:00:00Z"},
			{"id":11,"user":{"login":"bob"},"state":"APPROVED","body":"","submitted_at":"2026-01-01T10:00:00Z"}
		]`},
		{match: "issues/142/comments", stdout: `[
			{"id":20,"user":{"login":"carol"},"body":"general comment","created_at":"2026-01-02T10:00:00Z"}
		]`},
		{match: "pulls/142/comments", stdout: `[
			{"id":30,"user":{"login":"dave"},"body":"line note","path":".plans/add-sso.md","line":42,"created_at":"2026-01-04T10:00:00Z"},
			{"id":31,"user":{"login":"erin"},"body":"outdated note","path":".plans/add-sso.md","original_line":7,"created_at":"2026-01-01T10:00:00Z"}
		]`},
	}}
	p := github.NewProviderWithCmd(gh)
	feedback, err := p.Feedback(t.Context(), review.Ref{Provider: "github", Repository: "acme/payments", ID: "142"})
	require.NoError(t, err)
	require.Len(t, feedback, 4)
	assert.Equal(t, []string{"line-31", "comment-20", "review-10", "line-30"},
		[]string{feedback[0].ID, feedback[1].ID, feedback[2].ID, feedback[3].ID})
	assert.Equal(t, 7, *feedback[0].Line, "fall back to original_line")
	assert.Equal(t, ".plans/add-sso.md", feedback[3].Path)
	assert.Equal(t, 42, *feedback[3].Line)
	gh.requireDrained(t)
}

func TestPaginatedAPI(t *testing.T) {
	t.Parallel()
	gh := &fakeGH{responses: []canned{
		{match: "pr view 142", stdout: prJSON},
		{match: "reviews", stdout: `[{"id":1,"user":{"login":"a"},"state":"APPROVED","commit_id":"abc123","body":"","submitted_at":""}]` +
			`[{"id":2,"user":{"login":"b"},"state":"APPROVED","commit_id":"abc123","body":"","submitted_at":""}]`},
	}}
	p := github.NewProviderWithCmd(gh)
	got, err := p.GetReview(t.Context(), review.Ref{Provider: "github", Repository: "acme/payments", ID: "142"})
	require.NoError(t, err)
	assert.Len(t, got.Decisions, 2)
}

func TestMalformedAndAuthFailures(t *testing.T) {
	t.Parallel()
	gh := &fakeGH{responses: []canned{{match: "pr list", stdout: `not json`}}}
	p := github.NewProviderWithCmd(gh)
	_, err := p.FindReview(t.Context(), review.FindRequest{Repository: "acme/payments", HeadBranch: "plan/x"})
	require.ErrorContains(t, err, "could not parse")

	gh = &fakeGH{responses: []canned{{match: "pr view 999", err: &github.Error{Command: "pr", Detail: "no pull requests found"}}}}
	p = github.NewProviderWithCmd(gh)
	_, err = p.GetReview(t.Context(), review.Ref{Provider: "github", Repository: "acme/payments", ID: "999"})
	require.ErrorIs(t, err, review.ErrNotFound)

	gh = &fakeGH{responses: []canned{{match: "pr list", err: &github.Error{Command: "pr", Detail: "gh auth login required"}}}}
	p = github.NewProviderWithCmd(gh)
	_, err = p.FindReview(t.Context(), review.FindRequest{Repository: "acme/payments", HeadBranch: "plan/x"})
	require.ErrorContains(t, err, "gh auth login required")
	assert.NotErrorIs(t, err, review.ErrNotFound)
}
