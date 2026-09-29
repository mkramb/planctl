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
	require.Len(t, installed.Installed, 2)

	var claude, opencode string
	for _, item := range installed.Installed {
		switch item.Agent {
		case "claude":
			claude = item.Path
		case "opencode":
			opencode = item.Path
		}
	}
	assert.Equal(t, filepath.Join(home, ".claude", "skills", "planctl", "SKILL.md"), claude)
	assert.Equal(t, filepath.Join(home, ".config", "opencode", "skills", "planctl", "SKILL.md"), opencode)

	for _, path := range []string{claude, opencode} {
		contents, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Contains(t, string(contents), "name: planctl")
		assert.Contains(t, string(contents), "implementation.allowed")
	}
}

func TestSkillsInstallSingleAgent(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnvironment(t)
	home := homeFromEnv(t, env.Env)

	result := env.Run(t, "skills", "install", "--agent", "opencode", "--json")
	result.RequireSuccess(t)
	installed := testutil.DecodeJSON[output.SkillsResult](t, result)
	require.Len(t, installed.Installed, 1)
	assert.Equal(t, "opencode", installed.Installed[0].Agent)
	assert.FileExists(t, filepath.Join(home, ".config", "opencode", "skills", "planctl", "SKILL.md"))
	assert.NoFileExists(t, filepath.Join(home, ".claude", "skills", "planctl", "SKILL.md"))
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
