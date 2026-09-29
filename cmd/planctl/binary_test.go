//go:build integration

package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/mkramb/planctl/internal/output"
	"github.com/mkramb/planctl/internal/process"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A small subprocess smoke test checks the wiring that in-process Cobra tests
// cannot: main, actual exit statuses, and output from the compiled executable.
func TestBuiltBinary(t *testing.T) {
	t.Parallel()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	require.NoError(t, err)
	binary := filepath.Join(t.TempDir(), "planctl")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	runner := process.Runner{}
	build, err := runner.Run(t.Context(), process.Request{
		Executable: "go", Args: []string{"build", "-o", binary, "./cmd/planctl"}, Dir: root,
		Env: append(os.Environ(), "GOPROXY=off", "GOSUMDB=off", "GOTOOLCHAIN=local"),
	})
	require.NoError(t, err, "build binary: %s", build.Stderr)
	t.Run("init", func(t *testing.T) {
		env := NewEnvironment(t)
		result, err := runner.Run(t.Context(), process.Request{
			Executable: binary, Args: []string{"init", "--json"}, Dir: env.Root, Env: env.Env,
		})
		require.NoError(t, err, "stderr: %s", result.Stderr)
		assert.Empty(t, result.Stderr)
		created := DecodeJSON[output.InitResult](t, Result{Stdout: result.Stdout})
		assert.Equal(t, 1, created.Version)
		assert.Equal(t, canonicalPath(t, env.Root), created.Config.RepositoryRoot)
		assert.FileExists(t, created.Config.Path)
		env.WriteFile(t, "add-sso.md", "# Add SSO\n")
		before := env.GitRun(t, "rev-parse", "HEAD")
		result, err = runner.Run(t.Context(), process.Request{
			Executable: binary, Args: []string{"add-sso.md", "--json"}, Dir: env.Root, Env: env.Env,
		})
		require.Error(t, err)
		// The real GitHub adapter is now wired: a local-path remote cannot resolve.
		assertConfigFailure(t, Result{Stdout: result.Stdout, Stderr: result.Stderr, ExitCode: result.ExitCode}, "repository_lookup_failed")
		assert.Equal(t, before, env.GitRun(t, "rev-parse", "HEAD"), "missing adapter must not commit anything")
	})
	t.Run("missing git", func(t *testing.T) {
		result, err := runner.Run(t.Context(), process.Request{
			Executable: binary, Args: []string{"init", "--json"}, Dir: t.TempDir(),
			Env: append(os.Environ(), "PATH="+t.TempDir()),
		})
		require.Error(t, err)
		assertConfigFailure(t, Result{
			Stdout: result.Stdout, Stderr: result.Stderr, ExitCode: result.ExitCode,
		}, "git_not_found")
	})
}
