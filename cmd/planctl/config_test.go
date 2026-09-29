//go:build integration

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mkramb/planctl/internal/config"
	"github.com/mkramb/planctl/internal/git"
	"github.com/mkramb/planctl/internal/process"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigurationDiscoveryAndOverride(t *testing.T) {
	t.Parallel()
	env := NewEnvironment(t)
	env.Run(t, "init").RequireSuccess(t)
	env.WriteFile(t, "nested/deeper/file.txt", "work")
	env.WriteFile(t, "nested/.planctl.yaml", "version: 1\nbranch:\n  pattern: 'nested/{slug}'\n")
	loader := config.Loader{Executor: process.Runner{}, Env: env.Env}
	dir := filepath.Join(env.Root, "nested", "deeper")
	nearest, err := loader.Load(t.Context(), dir, "")
	require.NoError(t, err)
	assert.Equal(t, "nested/{slug}", nearest.Config.Branch.Pattern)
	assert.Equal(t, filepath.Join(canonicalPath(t, env.Root), "nested", config.Filename), nearest.Path)

	override, err := loader.Load(t.Context(), dir, "../../.planctl.yaml")
	require.NoError(t, err)
	assert.Equal(t, "review/{slug}", override.Config.Branch.Pattern)
	assert.Equal(t, filepath.Join(canonicalPath(t, env.Root), config.Filename), override.Path)
	assert.Equal(t, canonicalPath(t, env.Root), override.RepositoryRoot)

	_, err = loader.Load(t.Context(), dir, "missing.yaml")
	assertLoaderFailure(t, err, "config_not_found")
}

func TestDiscoveryDoesNotCrossRepositoryBoundary(t *testing.T) {
	t.Parallel()
	env := NewEnvironment(t)
	env.Run(t, "init").RequireSuccess(t)
	inner := filepath.Join(env.Root, "inner")
	require.NoError(t, os.Mkdir(inner, 0o755))
	_, err := git.New(process.Runner{}, inner, env.Env).Run(t.Context(), "init", "--initial-branch=main")
	require.NoError(t, err)
	_, err = (config.Loader{Executor: process.Runner{}, Env: env.Env}).Load(t.Context(), inner, "")
	assertLoaderFailure(t, err, "config_not_found")
}

func TestInvalidConfigurationIsNotReplacedByDefaults(t *testing.T) {
	t.Parallel()
	env := NewEnvironment(t)
	env.Run(t, "init").RequireSuccess(t)
	loader := config.Loader{Executor: process.Runner{}, Env: env.Env}
	for _, scenario := range []struct {
		name, contents, code string
	}{
		{"unsupported version", "version: 9\n", "unsupported_config_version"},
		{"invalid YAML", "version: [", "invalid_config"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			env.WriteFile(t, "nested/.planctl.yaml", scenario.contents)
			_, err := loader.Load(t.Context(), filepath.Join(env.Root, "nested"), "")
			assertLoaderFailure(t, err, scenario.code)
		})
	}
}

func TestConfigDirectoryIsAnError(t *testing.T) {
	t.Parallel()
	env := NewEnvironment(t)
	require.NoError(t, os.Mkdir(filepath.Join(env.Root, config.Filename), 0o755))
	_, err := (config.Loader{Executor: process.Runner{}, Env: env.Env}).Load(t.Context(), env.Root, "")
	assertLoaderFailure(t, err, "config_read_failed")
}

func assertLoaderFailure(t *testing.T, err error, code string) {
	t.Helper()
	var failure *config.Error
	require.ErrorAs(t, err, &failure)
	assert.Equal(t, code, failure.Code)
}
