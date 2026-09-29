// Package testutil provides real repositories and a stateful review platform for
// integration tests. No test-only provider can be selected in production config.
package testutil

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mkramb/planctl/internal/cli"
	"github.com/mkramb/planctl/internal/git"
	"github.com/mkramb/planctl/internal/process"
	"github.com/stretchr/testify/require"
)

type Environment struct {
	Root     string
	Remote   string
	Env      []string
	Git      *git.Client
	Provider *MockReviewProvider
	CacheDir string
}

type Result struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

func NewEnvironment(t testing.TB) *Environment {
	t.Helper()
	base := t.TempDir()
	root := filepath.Join(base, "repository")
	remote := filepath.Join(base, "remote.git")
	home := filepath.Join(base, "home")
	hooks := filepath.Join(base, "hooks")
	for _, dir := range []string{root, remote, home, hooks} {
		require.NoError(t, os.MkdirAll(dir, 0o755))
	}
	env := isolatedEnv(home)
	e := &Environment{
		Root: root, Remote: remote, Env: env,
		Git: git.New(process.Runner{}, root, env), Provider: NewMockReviewProvider(),
		CacheDir: filepath.Join(base, "cache"),
	}
	e.GitRun(t, "init", "--initial-branch=main")
	for _, setting := range [][2]string{
		{"user.name", "Planctl Test"}, {"user.email", "planctl@example.invalid"},
		{"commit.gpgsign", "false"}, {"tag.gpgsign", "false"},
		{"core.hooksPath", hooks}, {"core.autocrlf", "false"},
	} {
		e.GitRun(t, "config", "--local", setting[0], setting[1])
	}
	bare := git.New(process.Runner{}, remote, env)
	result, err := bare.Run(t.Context(), "init", "--bare", "--initial-branch=main")
	require.NoError(t, err, "initialize remote: %s", result.Stderr)
	e.WriteFile(t, "README.md", "# Integration fixture\n")
	require.NoError(t, e.Git.Add(t.Context(), "README.md"))
	require.NoError(t, e.Git.Commit(t.Context(), "Initial commit"))
	e.GitRun(t, "remote", "add", "origin", remote)
	require.NoError(t, e.Git.Push(t.Context(), "origin", "main"))
	e.Provider.RegisterRepository("acme/payments", remote, env)
	return e
}

func (e *Environment) Run(t testing.TB, args ...string) Result {
	t.Helper()
	return e.RunAt(t, e.Root, args...)
}

func (e *Environment) RunAt(t testing.TB, dir string, args ...string) Result {
	t.Helper()
	var stdout, stderr bytes.Buffer
	exit := cli.Run(t.Context(), args, cli.Dependencies{
		Dir: dir, Env: e.Env, Stdout: &stdout, Stderr: &stderr,
		Executor: process.Runner{}, Provider: e.Provider, Version: "test",
		CacheDir: e.CacheDir,
	})
	return Result{Stdout: stdout.String(), Stderr: stderr.String(), ExitCode: exit}
}

func (e *Environment) GitRun(t testing.TB, args ...string) string {
	t.Helper()
	result, err := e.Git.Run(t.Context(), args...)
	require.NoError(t, err, "git %v: %s", args, result.Stderr)
	return strings.TrimSpace(result.Stdout)
}

func (e *Environment) WriteFile(t testing.TB, name, contents string) {
	t.Helper()
	require.True(t, filepath.IsLocal(name), "fixture path must be relative and contained in the repository: %s", name)
	path := filepath.Join(e.Root, name)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(contents), 0o644))
}

func DecodeJSON[T any](t testing.TB, result Result) T {
	t.Helper()
	var decoded T
	decoder := json.NewDecoder(strings.NewReader(result.Stdout))
	require.NoError(t, decoder.Decode(&decoded), "stdout: %s\nstderr: %s", result.Stdout, result.Stderr)
	var trailing any
	require.ErrorIs(t, decoder.Decode(&trailing), io.EOF, "expected exactly one JSON document")
	return decoded
}

func (r Result) RequireSuccess(t testing.TB) {
	t.Helper()
	require.Zero(t, r.ExitCode, "stdout: %s\nstderr: %s", r.Stdout, r.Stderr)
}

func isolatedEnv(home string) []string {
	var env []string
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		upper := strings.ToUpper(key)
		if strings.HasPrefix(upper, "GIT_") || upper == "HOME" || upper == "USERPROFILE" || upper == "XDG_CONFIG_HOME" || upper == "XDG_CACHE_HOME" {
			continue
		}
		env = append(env, entry)
	}
	return append(env,
		"HOME="+home, "USERPROFILE="+home, "XDG_CONFIG_HOME="+home,
		"XDG_CACHE_HOME="+filepath.Join(filepath.Dir(home), "cache"),
		"GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_CONFIG_SYSTEM="+os.DevNull, "GIT_TERMINAL_PROMPT=0",
		"GIT_AUTHOR_NAME=Planctl Test", "GIT_AUTHOR_EMAIL=planctl@example.invalid",
		"GIT_COMMITTER_NAME=Planctl Test", "GIT_COMMITTER_EMAIL=planctl@example.invalid",
		"GIT_AUTHOR_DATE=2026-01-01T00:00:00Z", "GIT_COMMITTER_DATE=2026-01-01T00:00:00Z",
	)
}
