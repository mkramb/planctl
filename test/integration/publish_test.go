package integration_test

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/mkramb/planctl/internal/git"
	"github.com/mkramb/planctl/internal/output"
	"github.com/mkramb/planctl/internal/plan"
	"github.com/mkramb/planctl/internal/process"
	"github.com/mkramb/planctl/internal/review"
	"github.com/mkramb/planctl/test/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPublishReviseAndRepublish(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnvironment(t)
	env.Run(t, "init").RequireSuccess(t)
	created := createPlan(t, env, "Add SSO")
	env.WriteFile(t, "README.md", "staged implementation work\n")
	env.GitRun(t, "add", "README.md")
	env.WriteFile(t, "notes.txt", "untracked work\n")
	index := env.GitRun(t, "diff", "--cached")
	status := env.GitRun(t, "status", "--porcelain")
	head := env.GitRun(t, "rev-parse", "HEAD")
	first := publishPlan(t, env)
	assert.Equal(t, 1, first.Version)
	assert.Equal(t, created.Plan, first.Plan)
	assert.Equal(t, "mock", first.Review.Provider)
	assert.Equal(t, review.Open, first.Review.State)
	assert.False(t, first.Review.Draft)
	assert.NotEmpty(t, first.Review.URL)
	assert.NotEqual(t, head, first.Commit)
	ref := publishedRef(first)
	found, err := env.Provider.GetReview(t.Context(), ref)
	require.NoError(t, err)
	assert.Equal(t, "Plan: Add SSO", found.Title)
	assert.Equal(t, first.Commit, found.HeadCommit)
	metadata, err := plan.ParseMetadata(found.Body)
	require.NoError(t, err)
	assert.Equal(t, plan.Metadata{Version: 1, ID: "add-sso", ImplementationRepository: "acme/payments"}, metadata)
	require.NoError(t, env.Provider.AddFeedback(ref, review.Feedback{Author: "alice", Body: "Add rollback steps."}))
	require.NoError(t, env.Provider.Approve(ref, "bob"))
	require.NoError(t, os.WriteFile(first.Plan.AbsolutePath, []byte("# Add SSO\n\n## Rollback\nRestore the previous configuration.\n"), 0o644))
	second := publishPlan(t, env)
	assert.Equal(t, first.Review, second.Review)
	assert.NotEqual(t, first.Commit, second.Commit)
	latest, err := env.Provider.GetReview(t.Context(), ref)
	require.NoError(t, err)
	assert.Equal(t, second.Commit, latest.HeadCommit, "fake must follow the real remote branch")
	require.Len(t, latest.Decisions, 1)
	assert.Equal(t, first.Commit, latest.Decisions[0].CommitID, "old approvals remain tied to their revision")
	feedback, err := env.Provider.Feedback(t.Context(), ref)
	require.NoError(t, err)
	assert.Len(t, feedback, 1)
	noOp := publishPlan(t, env)
	assert.Equal(t, second.Commit, noOp.Commit, "no-op publish must not create an empty commit")
	assert.Equal(t, second.Review, noOp.Review)
	assert.Equal(t, 1, env.Provider.ReviewCount())
	remote := git.New(process.Runner{}, env.Remote, env.Env)
	remoteHead, exists, err := remote.ResolveCommit(t.Context(), "refs/heads/plan/add-sso")
	require.NoError(t, err)
	assert.True(t, exists)
	assert.Equal(t, second.Commit, remoteHead)
	assert.Equal(t, head, env.GitRun(t, "rev-parse", "HEAD"))
	assert.Equal(t, index, env.GitRun(t, "diff", "--cached"))
	assert.Equal(t, status, env.GitRun(t, "status", "--porcelain"))
}

func TestPublishSelection(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnvironment(t)
	env.Run(t, "init").RequireSuccess(t)
	assertConfigFailure(t, env.Run(t, "publish", "--json"), "plan_required")
	first := createPlan(t, env, "First")
	second := createPlan(t, env, "Second")
	assertConfigFailure(t, env.Run(t, "publish", "--json"), "ambiguous_plan")
	assertConfigFailure(t, env.Run(t, "publish", "--plan", "../escape", "--json"), "invalid_plan")
	assertConfigFailure(t, env.Run(t, "publish", "--plan", "missing", "--json"), "workspace_missing")
	assertConfigFailure(t, env.Run(t, "publish", "--plan=", "--json"), "invalid_arguments")
	chosen := publishPlan(t, env, "--plan", "second")
	assert.Equal(t, second.Plan.ID, chosen.Plan.ID)
	result := env.RunAt(t, first.Plan.WorkspacePath, "publish", "--json")
	result.RequireSuccess(t)
	fromWorktree := testutil.DecodeJSON[output.PublishResult](t, result)
	assert.Equal(t, first.Plan.ID, fromWorktree.Plan.ID)
	assert.NotEqual(t, chosen.Review.ID, fromWorktree.Review.ID)
	assert.Equal(t, 2, env.Provider.ReviewCount())
}

func TestPublishCustomPatternsAndHumanOutput(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnvironment(t)
	env.WriteFile(t, ".planctl.yaml", "version: 1\nbranch:\n  pattern: 'proposal/{slug}/review'\npull_request:\n  title: 'Design: {title}'\n  draft: true\n")
	createPlan(t, env, "Add SSO")
	result := env.Run(t, "publish")
	result.RequireSuccess(t)
	assert.Empty(t, result.Stderr)
	assert.Contains(t, result.Stdout, "Published plan: Add SSO\n")
	assert.Contains(t, result.Stdout, "Branch: proposal/add-sso/review\n")
	found, err := env.Provider.FindReview(t.Context(), review.FindRequest{Repository: "acme/payments", HeadBranch: "proposal/add-sso/review"})
	require.NoError(t, err)
	assert.True(t, found.Draft)
	assert.Equal(t, "Design: Add SSO", found.Title)
}

func TestPublishRejectsUnrelatedChanges(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"staged", "committed", "reverted"} {
		t.Run(kind, func(t *testing.T) {
			env := testutil.NewEnvironment(t)
			env.Run(t, "init").RequireSuccess(t)
			created := createPlan(t, env, "Add SSO")
			client := git.New(process.Runner{}, created.Plan.WorkspacePath, env.Env)
			path := filepath.Join(created.Plan.WorkspacePath, "implementation.go")
			require.NoError(t, os.WriteFile(path, []byte("package implementation\n"), 0o644))
			require.NoError(t, client.Add(t.Context(), "implementation.go"))
			if kind != "staged" {
				require.NoError(t, client.Commit(t.Context(), "Unrelated implementation"))
			}
			if kind == "reverted" {
				require.NoError(t, os.Remove(path))
				require.NoError(t, client.Add(t.Context(), "implementation.go"))
				require.NoError(t, client.Commit(t.Context(), "Revert implementation"))
			}
			before, err := client.Head(t.Context())
			require.NoError(t, err)
			assertConfigFailure(t, env.Run(t, "publish", "--json"), "unrelated_changes")
			after, err := client.Head(t.Context())
			require.NoError(t, err)
			assert.Equal(t, before, after)
			assert.Equal(t, 0, env.Provider.ReviewCount())
			_, exists, err := git.New(process.Runner{}, env.Remote, env.Env).ResolveCommit(t.Context(), "refs/heads/plan/add-sso")
			require.NoError(t, err)
			assert.False(t, exists)
		})
	}
}

func TestPublishRetriesReviewCreation(t *testing.T) {
	t.Parallel()
	for _, after := range []bool{false, true} {
		name := "failure before creation"
		if after {
			name = "response lost after creation"
		}
		t.Run(name, func(t *testing.T) {
			env := testutil.NewEnvironment(t)
			env.Run(t, "init").RequireSuccess(t)
			created := createPlan(t, env, "Add SSO")
			env.Provider.FailNextCreate(errors.New("simulated transport failure"), after)
			result := env.Run(t, "publish", "--json")
			if after {
				result.RequireSuccess(t)
			} else {
				assertConfigFailure(t, result, "review_create_failed")
			}
			client := git.New(process.Runner{}, created.Plan.WorkspacePath, env.Env)
			head, err := client.Head(t.Context())
			require.NoError(t, err)
			retried := publishPlan(t, env)
			assert.Equal(t, head, retried.Commit, "retry must reuse the pushed commit")
			assert.Equal(t, 1, env.Provider.ReviewCount())
		})
	}
}

func TestPublishRetriesFailedPush(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnvironment(t)
	env.Run(t, "init").RequireSuccess(t)
	created := createPlan(t, env, "Add SSO")
	moved := env.Remote + "-unavailable"
	require.NoError(t, os.Rename(env.Remote, moved))
	assertConfigFailure(t, env.Run(t, "publish", "--json"), "push_failed")
	client := git.New(process.Runner{}, created.Plan.WorkspacePath, env.Env)
	head, err := client.Head(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 0, env.Provider.ReviewCount())
	require.NoError(t, os.Rename(moved, env.Remote))
	retried := publishPlan(t, env)
	assert.Equal(t, head, retried.Commit)
	assert.Equal(t, 1, env.Provider.ReviewCount())
}

func TestPublishRejectsNonFastForward(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnvironment(t)
	env.Run(t, "init").RequireSuccess(t)
	createPlan(t, env, "Add SSO")
	first := publishPlan(t, env)
	otherPath := filepath.Join(filepath.Dir(env.Root), "remote author")
	env.GitRun(t, "worktree", "add", "-b", "remote-author", otherPath, first.Commit)
	other := git.New(process.Runner{}, otherPath, env.Env)
	require.NoError(t, os.WriteFile(filepath.Join(otherPath, ".plans", "add-sso.md"), []byte("# Add SSO\n\nRemote author's change.\n"), 0o644))
	require.NoError(t, other.Add(t.Context(), first.Plan.Path))
	require.NoError(t, other.Commit(t.Context(), "Remote plan change"))
	require.NoError(t, other.Push(t.Context(), "origin", "HEAD:refs/heads/"+first.Plan.Branch))
	remoteHead, err := other.Head(t.Context())
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(first.Plan.AbsolutePath, []byte("# Add SSO\n\nConflicting local change.\n"), 0o644))
	assertConfigFailure(t, env.Run(t, "publish", "--json"), "push_rejected")
	remote := git.New(process.Runner{}, env.Remote, env.Env)
	still, exists, err := remote.ResolveCommit(t.Context(), "refs/heads/"+first.Plan.Branch)
	require.NoError(t, err)
	assert.True(t, exists)
	assert.Equal(t, remoteHead, still, "never force-push over the remote author")
	assert.Equal(t, 1, env.Provider.ReviewCount())
}

func TestPublishRefusesTerminalReviewBeforeCommitting(t *testing.T) {
	t.Parallel()
	for _, state := range []review.State{review.Closed, review.Merged} {
		t.Run(string(state), func(t *testing.T) {
			env := testutil.NewEnvironment(t)
			env.Run(t, "init").RequireSuccess(t)
			createPlan(t, env, "Add SSO")
			first := publishPlan(t, env)
			require.NoError(t, env.Provider.SetState(publishedRef(first), state))
			require.NoError(t, os.WriteFile(first.Plan.AbsolutePath, []byte("# Add SSO\n\nLater edits.\n"), 0o644))
			assertConfigFailure(t, env.Run(t, "publish", "--json"), "review_terminal")
			head, err := git.New(process.Runner{}, first.Plan.WorkspacePath, env.Env).Head(t.Context())
			require.NoError(t, err)
			assert.Equal(t, first.Commit, head)
			assert.Equal(t, 1, env.Provider.ReviewCount())
		})
	}
}

func TestPublishStopsOnLookupFailure(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnvironment(t)
	env.Run(t, "init").RequireSuccess(t)
	created := createPlan(t, env, "Add SSO")
	client := git.New(process.Runner{}, created.Plan.WorkspacePath, env.Env)
	before, err := client.Head(t.Context())
	require.NoError(t, err)
	env.Provider.FailNextLookup(errors.New("authentication failure"))
	assertConfigFailure(t, env.Run(t, "publish", "--json"), "review_lookup_failed")
	after, err := client.Head(t.Context())
	require.NoError(t, err)
	assert.Equal(t, before, after)
	assert.Equal(t, 0, env.Provider.ReviewCount())
	_ = publishPlan(t, env)
}

func TestPublishDoesNotAdoptUnrelatedReview(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnvironment(t)
	env.Run(t, "init").RequireSuccess(t)
	created := createPlan(t, env, "Add SSO")
	client := git.New(process.Runner{}, created.Plan.WorkspacePath, env.Env)
	require.NoError(t, client.Add(t.Context(), created.Plan.Path))
	require.NoError(t, client.CommitOnly(t.Context(), "Manual plan commit", created.Plan.Path))
	head, err := client.Head(t.Context())
	require.NoError(t, err)
	require.NoError(t, client.PushPlan(t.Context(), head, created.Plan.Branch))
	manual, err := env.Provider.CreateReview(t.Context(), review.CreateRequest{
		Repository: "acme/payments", Title: "Manual review", Body: "Not owned by planctl.",
		HeadBranch: created.Plan.Branch, BaseBranch: "main", HeadCommit: head,
	})
	require.NoError(t, err)
	assertConfigFailure(t, env.Run(t, "publish", "--json"), "review_conflict")
	still, err := env.Provider.GetReview(t.Context(), manual.Ref)
	require.NoError(t, err)
	assert.Equal(t, manual, still)
	assert.Equal(t, 1, env.Provider.ReviewCount())
}

func TestPublishRejectsAmbiguousReviews(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnvironment(t)
	env.Run(t, "init").RequireSuccess(t)
	createPlan(t, env, "Add SSO")
	first := publishPlan(t, env)
	stored, err := env.Provider.GetReview(t.Context(), publishedRef(first))
	require.NoError(t, err)
	require.NoError(t, env.Provider.SetState(stored.Ref, review.Closed))
	_, err = env.Provider.CreateReview(t.Context(), review.CreateRequest{
		Repository: stored.Ref.Repository, Title: stored.Title, Body: stored.Body,
		HeadBranch: stored.HeadBranch, BaseBranch: stored.BaseBranch, HeadCommit: stored.HeadCommit,
	})
	require.NoError(t, err)
	assertConfigFailure(t, env.Run(t, "publish", "--json"), "ambiguous_review")
	assert.Equal(t, 2, env.Provider.ReviewCount())
}

func TestPublishPreservesUntrackedWorkspaceFiles(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnvironment(t)
	env.Run(t, "init").RequireSuccess(t)
	created := createPlan(t, env, "Add SSO")
	notes := filepath.Join(created.Plan.WorkspacePath, "notes.txt")
	require.NoError(t, os.WriteFile(notes, []byte("private notes"), 0o644))
	// Markdown content is not validated; a plan without a heading uses its slug.
	require.NoError(t, os.WriteFile(created.Plan.AbsolutePath, []byte("A plain Markdown plan.\n"), 0o644))
	published := publishPlan(t, env)
	assert.Equal(t, "add-sso", published.Plan.Title)
	client := git.New(process.Runner{}, created.Plan.WorkspacePath, env.Env)
	tracked, err := client.TreeContains(t.Context(), published.Commit, "notes.txt")
	require.NoError(t, err)
	assert.False(t, tracked)
	assert.FileExists(t, notes)
}

func TestConcurrentPublishCreatesOneReview(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnvironment(t)
	env.Run(t, "init").RequireSuccess(t)
	createPlan(t, env, "Add SSO")
	var results [2]testutil.Result
	var workers sync.WaitGroup
	for i := range results {
		workers.Go(func() { results[i] = env.Run(t, "publish", "--json") })
	}
	workers.Wait()
	var successful []output.PublishResult
	for _, result := range results {
		if result.ExitCode == 0 {
			successful = append(successful, testutil.DecodeJSON[output.PublishResult](t, result))
		} else {
			assertConfigFailure(t, result, "plan_busy")
		}
	}
	require.NotEmpty(t, successful)
	for _, result := range successful {
		assert.Equal(t, successful[0].Commit, result.Commit)
		assert.Equal(t, successful[0].Review, result.Review)
	}
	assert.Equal(t, 1, env.Provider.ReviewCount())
}

func TestPublishRejectsSymlinkPlan(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnvironment(t)
	env.Run(t, "init").RequireSuccess(t)
	created := createPlan(t, env, "Add SSO")
	outside := filepath.Join(t.TempDir(), "private.md")
	require.NoError(t, os.WriteFile(outside, []byte("private data"), 0o644))
	require.NoError(t, os.Remove(created.Plan.AbsolutePath))
	if err := os.Symlink(outside, created.Plan.AbsolutePath); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	assertConfigFailure(t, env.Run(t, "publish", "--json"), "unsafe_path")
	assert.Equal(t, 0, env.Provider.ReviewCount())
	contents, err := os.ReadFile(outside)
	require.NoError(t, err)
	assert.Equal(t, "private data", string(contents))
}

func publishPlan(t *testing.T, env *testutil.Environment, args ...string) output.PublishResult {
	t.Helper()
	command := append([]string{"publish", "--json"}, args...)
	result := env.Run(t, command...)
	result.RequireSuccess(t)
	assert.Empty(t, result.Stderr)
	return testutil.DecodeJSON[output.PublishResult](t, result)
}

func publishedRef(result output.PublishResult) review.Ref {
	return review.Ref{Provider: result.Review.Provider, Repository: "acme/payments", ID: result.Review.ID}
}
