//go:build integration

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mkramb/planctl/internal/git"
	"github.com/mkramb/planctl/internal/output"
	"github.com/mkramb/planctl/internal/process"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCLIHelpAndVersion(t *testing.T) {
	t.Parallel()
	env := NewEnvironment(t)
	help := env.Run(t, "--help")
	help.RequireSuccess(t)
	for _, want := range []string{"planctl", "--json", "--config", "--verbose"} {
		assert.Contains(t, help.Stdout, want)
	}
	assert.Empty(t, help.Stderr)
	human := env.Run(t, "version")
	human.RequireSuccess(t)
	machine := env.Run(t, "version", "--json")
	machine.RequireSuccess(t)
	version := DecodeJSON[output.VersionResult](t, machine)
	assert.Equal(t, 1, version.Version)
	assert.Equal(t, "test", version.Build)
	assert.Equal(t, "planctl "+version.Build+"\n", human.Stdout)
	assert.Empty(t, machine.Stderr)
}

func TestStructuredUsageErrors(t *testing.T) {
	t.Parallel()
	env := NewEnvironment(t)
	for _, args := range [][]string{
		{"version", "extra", "--json"},
		{"version", "--unknown", "--json"},
		{"--config", "--json=true", "version", "extra", "--json"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			result := env.Run(t, args...)
			assert.Equal(t, 2, result.ExitCode)
			assert.Empty(t, result.Stderr)
			failure := DecodeJSON[output.ErrorResult](t, result)
			assert.Equal(t, 1, failure.Version)
			assert.Equal(t, "invalid_arguments", failure.Error.Code)
			assert.NotEmpty(t, failure.Error.Message)
		})
	}
	for _, args := range [][]string{
		{"version", "extra"},
		{"version", "--", "--json"},
		{"version", "--config", "--json", "extra"},
	} {
		result := env.Run(t, args...)
		assert.Equal(t, 2, result.ExitCode, "args: %v", args)
		assert.Empty(t, result.Stdout, "args: %v", args)
		assert.True(t, strings.HasPrefix(result.Stderr, "error: "), "args: %v, stderr: %s", args, result.Stderr)
	}
}

func TestRealGitCommitAndPush(t *testing.T) {
	t.Parallel()
	env := NewEnvironment(t)
	before, err := env.Git.Head(t.Context())
	require.NoError(t, err)
	env.WriteFile(t, ".plans/add-sso.md", "# Add SSO\n\n## Context\n")
	env.WriteFile(t, "unrelated.txt", "leave this untracked\n")
	require.NoError(t, env.Git.Add(t.Context(), ".plans/add-sso.md"))
	require.NoError(t, env.Git.Commit(t.Context(), "Add SSO plan"))
	require.NoError(t, env.Git.Push(t.Context(), "origin", "main"))
	head, err := env.Git.Head(t.Context())
	require.NoError(t, err)
	assert.NotEqual(t, before, head, "expected a new commit")
	remote := git.New(process.Runner{}, env.Remote, env.Env)
	remoteHead, err := remote.Head(t.Context())
	require.NoError(t, err)
	assert.Equal(t, head, remoteHead, "push must update local remote")
	files := env.GitRun(t, "show", "--pretty=format:", "--name-only", "HEAD")
	assert.Equal(t, ".plans/add-sso.md", files)
	assert.Equal(t, "?? unrelated.txt", env.GitRun(t, "status", "--porcelain"), "preserve unrelated work")
}

func TestGitDiscoveryFromLinkedWorktree(t *testing.T) {
	t.Parallel()
	env := NewEnvironment(t)
	linked := filepath.Join(filepath.Dir(env.Root), "linked checkout")
	env.GitRun(t, "worktree", "add", "-b", "feature/sso", linked)
	nested := filepath.Join(linked, "nested", "directory")
	require.NoError(t, os.MkdirAll(nested, 0o755))
	client := git.New(process.Runner{}, nested, env.Env)
	root, err := client.Root(t.Context())
	require.NoError(t, err)
	wantRoot, err := filepath.EvalSymlinks(linked)
	require.NoError(t, err)
	assert.Equal(t, wantRoot, root)
	common, err := client.CommonDirectory(t.Context())
	require.NoError(t, err)
	wantCommon, err := env.Git.CommonDirectory(t.Context())
	require.NoError(t, err)
	assert.Equal(t, wantCommon, common, "worktrees must share Git identity")
	branch, err := client.CurrentBranch(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "feature/sso", branch)
	mainBranch, err := env.Git.CurrentBranch(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "main", mainBranch, "preserve original branch")
	env.RunAt(t, nested, "version", "--json").RequireSuccess(t)
}
