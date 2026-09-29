//go:build integration

package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/mkramb/planctl/internal/github"
	"github.com/mkramb/planctl/internal/plan"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPublishFileOpensReviewAndApproves(t *testing.T) {
	t.Parallel()
	env := NewEnvironment(t)
	env.WriteFile(t, "add-sso.md", "# Add SSO\n\n## Context\nUse the identity provider.\n")

	ch := startLoop(t, env, "add-sso.md", "--json")
	ref := findReview(t, env, "review/add-sso")
	found, err := env.Provider.GetReview(t.Context(), ref)
	require.NoError(t, err)
	assert.Equal(t, "Review: add-sso.md", found.Title)
	assert.Equal(t, github.Open, found.State)
	metadata, err := plan.ParseMetadata(found.Body)
	require.NoError(t, err)
	assert.Equal(t, plan.Metadata{Version: 1, ID: "add-sso"}, metadata)

	require.NoError(t, env.Mock.Approve(ref, "alice"))
	outcome := waitOutcome(t, ch)
	assert.True(t, outcome.Allowed)
	assert.Equal(t, plan.StatusApproved, outcome.Status)
	assert.Equal(t, "add-sso.md", outcome.Plan.Title)
	assert.Equal(t, []string{"add-sso.md"}, outcome.Plan.Files)
	assert.Equal(t, "main", outcome.Plan.Base)
}

func TestPublishAutoDetectChangedFiles(t *testing.T) {
	t.Parallel()
	env := NewEnvironment(t)
	env.WriteFile(t, "plan.md", "# Plan\n")
	env.WriteFile(t, "docs/notes.md", "# Notes\n")

	ch := startLoop(t, env, "--json")
	ref := findReview(t, env, "review/notes")
	require.NoError(t, env.Mock.Approve(ref, "alice"))
	outcome := waitOutcome(t, ch)
	assert.Equal(t, []string{"docs/notes.md", "plan.md"}, outcome.Plan.Files)
	assert.Equal(t, "notes", outcome.Plan.ID, "slug derives from the first file alphabetically")
}

func TestPublishFolderAndMultipleFiles(t *testing.T) {
	t.Parallel()
	env := NewEnvironment(t)
	env.WriteFile(t, "docs/a.md", "# A\n")
	env.WriteFile(t, "docs/b.md", "# B\n")

	ch := startLoop(t, env, "docs", "--json")
	ref := findReview(t, env, "review/a")
	require.NoError(t, env.Mock.Approve(ref, "alice"))
	outcome := waitOutcome(t, ch)
	assert.Equal(t, []string{"docs/a.md", "docs/b.md"}, outcome.Plan.Files)
}

func TestPublishReviewersAllMustApprove(t *testing.T) {
	t.Parallel()
	env := NewEnvironment(t)
	env.WriteFile(t, "add-sso.md", "# Add SSO\n")

	ch := startLoop(t, env, "--reviewer", "alice", "--reviewer", "bob", "add-sso.md", "--json")
	ref := findReview(t, env, "review/add-sso")
	found, err := env.Provider.GetReview(t.Context(), ref)
	require.NoError(t, err)
	metadata, err := plan.ParseMetadata(found.Body)
	require.NoError(t, err)
	assert.Equal(t, []string{"alice", "bob"}, metadata.Reviewers)

	require.NoError(t, env.Mock.Approve(ref, "alice"))
	require.NoError(t, env.Mock.Approve(ref, "mallory"))
	require.NoError(t, env.Mock.Approve(ref, "bob"))
	outcome := waitOutcome(t, ch)
	assert.True(t, outcome.Allowed)
	assert.Equal(t, 2, outcome.Approval.Current)
	assert.Equal(t, 2, outcome.Approval.Required)
	assert.Equal(t, []string{"alice", "bob"}, outcome.Reviewers)
}

func TestPublishCustomPatternsAndDefaults(t *testing.T) {
	t.Parallel()
	env := NewEnvironment(t)
	env.WriteFile(t, ".planctl.yaml", "version: 1\nbranch:\n  pattern: 'proposal/{slug}/review'\npull_request:\n  title: 'Design: {title}'\n")
	env.WriteFile(t, "add-sso.md", "# Add SSO\n")

	ch := startLoop(t, env, "add-sso.md", "--json")
	ref := findReview(t, env, "proposal/add-sso/review")
	found, err := env.Provider.GetReview(t.Context(), ref)
	require.NoError(t, err)
	assert.Equal(t, "Design: add-sso.md", found.Title)
	require.NoError(t, env.Mock.Approve(ref, "alice"))
	waitOutcome(t, ch)
}

func TestPublishWithoutConfigUsesDefaults(t *testing.T) {
	t.Parallel()
	env := NewEnvironment(t)
	env.WriteFile(t, "add-sso.md", "# Add SSO\n")

	ch := startLoop(t, env, "add-sso.md", "--json")
	ref := findReview(t, env, "review/add-sso")
	found, err := env.Provider.GetReview(t.Context(), ref)
	require.NoError(t, err)
	assert.Equal(t, "Review: add-sso.md", found.Title)
	require.NoError(t, env.Mock.Approve(ref, "alice"))
	outcome := waitOutcome(t, ch)
	assert.True(t, outcome.Allowed)
}

func TestPublishErrors(t *testing.T) {
	t.Parallel()
	env := NewEnvironment(t)

	assertConfigFailure(t, env.Run(t, "--json"), "no_files")
	assertConfigFailure(t, env.Run(t, "missing.md", "--json"), "path_not_found")
	outside := filepath.Join(t.TempDir(), "outside.md")
	require.NoError(t, os.WriteFile(outside, []byte("outside"), 0o644))
	assertConfigFailure(t, env.Run(t, outside, "--json"), "invalid_path")
}

func TestPublishRefusesTerminalReview(t *testing.T) {
	t.Parallel()
	env := NewEnvironment(t)
	env.WriteFile(t, "add-sso.md", "# Add SSO\n")

	ch := startLoop(t, env, "add-sso.md", "--json")
	ref := findReview(t, env, "review/add-sso")
	require.NoError(t, env.Mock.SetState(ref, github.Closed))
	waitOutcomeError(t, ch, "review_terminal")
}

func TestPublishRetriesReviewCreation(t *testing.T) {
	t.Parallel()
	env := NewEnvironment(t)
	env.WriteFile(t, "add-sso.md", "# Add SSO\n")
	env.Mock.FailNextCreate(errors.New("simulated transport failure"), true)
	ch := startLoop(t, env, "add-sso.md", "--json")
	ref := findReview(t, env, "review/add-sso")
	require.NoError(t, env.Mock.Approve(ref, "alice"))
	outcome := waitOutcome(t, ch)
	assert.True(t, outcome.Allowed)
}
