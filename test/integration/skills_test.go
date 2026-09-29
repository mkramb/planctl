package integration_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mkramb/planctl/internal/output"
	"github.com/mkramb/planctl/test/testutil"
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
	env := testutil.NewEnvironment(t)
	home := homeFromEnv(t, env.Env)

	result := env.Run(t, "skills", "install", "--json")
	result.RequireSuccess(t)
	assert.Empty(t, result.Stderr)
	installed := testutil.DecodeJSON[output.SkillsResult](t, result)
	require.Len(t, installed.Installed, 4)

	var claudeSkill, claudeCommand, opencodeSkill, opencodeCommand string
	for _, item := range installed.Installed {
		switch item.Agent + "/" + item.Kind {
		case "claude/skill":
			claudeSkill = item.Path
		case "claude/command":
			claudeCommand = item.Path
		case "opencode/skill":
			opencodeSkill = item.Path
		case "opencode/command":
			opencodeCommand = item.Path
		}
	}
	assert.Equal(t, filepath.Join(home, ".claude", "skills", "planctl", "SKILL.md"), claudeSkill)
	assert.Equal(t, filepath.Join(home, ".claude", "commands", "planctl.md"), claudeCommand)
	assert.Equal(t, filepath.Join(home, ".config", "opencode", "skills", "planctl", "SKILL.md"), opencodeSkill)
	assert.Equal(t, filepath.Join(home, ".config", "opencode", "command", "planctl.md"), opencodeCommand)

	for _, path := range []string{claudeSkill, opencodeSkill} {
		contents, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Contains(t, string(contents), "name: planctl")
		assert.Contains(t, string(contents), "implementation.allowed")
	}
	for _, path := range []string{claudeCommand, opencodeCommand} {
		contents, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Contains(t, string(contents), "planctl $ARGUMENTS")
	}
}

func TestSkillsInstallSingleAgent(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnvironment(t)
	home := homeFromEnv(t, env.Env)

	result := env.Run(t, "skills", "install", "--agent", "opencode", "--json")
	result.RequireSuccess(t)
	installed := testutil.DecodeJSON[output.SkillsResult](t, result)
	require.Len(t, installed.Installed, 2)
	for _, item := range installed.Installed {
		assert.Equal(t, "opencode", item.Agent)
	}
	assert.FileExists(t, filepath.Join(home, ".config", "opencode", "skills", "planctl", "SKILL.md"))
	assert.FileExists(t, filepath.Join(home, ".config", "opencode", "command", "planctl.md"))
	assert.NoFileExists(t, filepath.Join(home, ".claude", "skills", "planctl", "SKILL.md"))
	assert.NoFileExists(t, filepath.Join(home, ".claude", "commands", "planctl.md"))
}

func TestSkillsInstallInvalidAgent(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnvironment(t)
	result := env.Run(t, "skills", "install", "--agent", "vim", "--json")
	assert.Equal(t, 2, result.ExitCode)
	assert.Empty(t, result.Stderr)
	failure := testutil.DecodeJSON[output.ErrorResult](t, result)
	assert.Equal(t, "invalid_arguments", failure.Error.Code)
}
