//go:build integration

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mkramb/planctl/internal/output"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// homeFromEnv reads HOME from a test environment's isolated variables.
func homeFromEnv(t *testing.T, env []string) string {
	t.Helper()
	for _, entry := range env {
		if key, value, ok := strings.Cut(entry, "="); ok && key == "HOME" {
			return value
		}
	}
	t.Fatal("HOME not set in environment")
	return ""
}

func TestSkillsInstall(t *testing.T) {
	t.Parallel()
	env := NewEnvironment(t)
	home := homeFromEnv(t, env.Env)

	result := env.Run(t, "install", "--json")
	result.RequireSuccess(t)
	assert.Empty(t, result.Stderr)
	installed := DecodeJSON[output.SkillsResult](t, result)
	require.Len(t, installed.Installed, 2)

	var claudeCommand, opencodeCommand string
	for _, item := range installed.Installed {
		switch item.Agent {
		case "claude":
			claudeCommand = item.Path
		case "opencode":
			opencodeCommand = item.Path
		}
	}
	assert.Equal(t, filepath.Join(home, ".claude", "commands", "planctl.md"), claudeCommand)
	assert.Equal(t, filepath.Join(home, ".config", "opencode", "command", "planctl.md"), opencodeCommand)

	for _, path := range []string{claudeCommand, opencodeCommand} {
		contents, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Contains(t, string(contents), "planctl $ARGUMENTS")
	}
}

func TestSkillsInstallSingleAgent(t *testing.T) {
	t.Parallel()
	env := NewEnvironment(t)
	home := homeFromEnv(t, env.Env)

	result := env.Run(t, "install", "--agent", "opencode", "--json")
	result.RequireSuccess(t)
	installed := DecodeJSON[output.SkillsResult](t, result)
	require.Len(t, installed.Installed, 1)
	assert.Equal(t, "opencode", installed.Installed[0].Agent)
	assert.FileExists(t, filepath.Join(home, ".config", "opencode", "command", "planctl.md"))
	assert.NoFileExists(t, filepath.Join(home, ".claude", "commands", "planctl.md"))
}

func TestSkillsInstallInvalidAgent(t *testing.T) {
	t.Parallel()
	env := NewEnvironment(t)
	result := env.Run(t, "install", "--agent", "vim", "--json")
	assert.Equal(t, 2, result.ExitCode)
	assert.Empty(t, result.Stderr)
	failure := DecodeJSON[output.ErrorResult](t, result)
	assert.Equal(t, "invalid_arguments", failure.Error.Code)
}

func TestUninstallRemovesCommand(t *testing.T) {
	t.Parallel()
	env := NewEnvironment(t)
	home := homeFromEnv(t, env.Env)

	env.Run(t, "install", "--json").RequireSuccess(t)
	claude := filepath.Join(home, ".claude", "commands", "planctl.md")
	opencode := filepath.Join(home, ".config", "opencode", "command", "planctl.md")
	require.FileExists(t, claude)
	require.FileExists(t, opencode)

	result := env.Run(t, "uninstall", "--json")
	result.RequireSuccess(t)
	assert.Empty(t, result.Stderr)
	removed := DecodeJSON[output.UninstallResult](t, result)
	require.Len(t, removed.Removed, 2)
	assert.NoFileExists(t, claude)
	assert.NoFileExists(t, opencode)

	// Idempotent: uninstalling again succeeds.
	env.Run(t, "uninstall", "--json").RequireSuccess(t)
}

func TestUninstallSingleAgent(t *testing.T) {
	t.Parallel()
	env := NewEnvironment(t)
	home := homeFromEnv(t, env.Env)

	env.Run(t, "install", "--json").RequireSuccess(t)
	result := env.Run(t, "uninstall", "--agent", "opencode", "--json")
	result.RequireSuccess(t)
	assert.NoFileExists(t, filepath.Join(home, ".config", "opencode", "command", "planctl.md"))
	assert.FileExists(t, filepath.Join(home, ".claude", "commands", "planctl.md"))
}
