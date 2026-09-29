// Package skill installs planctl's /planctl slash command into the target
// agent's own command directory.
package skill

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/mkramb/planctl/skills"
)

type Agent string

const (
	Claude   Agent = "claude"
	OpenCode Agent = "opencode"
)

func All() []Agent { return []Agent{Claude, OpenCode} }

// Path returns where an agent's /planctl command lives without touching the
// filesystem.
func Path(home string, agent Agent) (string, error) {
	switch agent {
	case Claude:
		return filepath.Join(home, ".claude", "commands", "planctl.md"), nil
	case OpenCode:
		return filepath.Join(home, ".config", "opencode", "command", "planctl.md"), nil
	default:
		return "", fmt.Errorf("unknown agent %q", agent)
	}
}

// Install writes the /planctl slash command for the given agent under home and
// returns the installed path.
func Install(home string, agent Agent) (string, error) {
	path, err := Path(home, agent)
	if err != nil {
		return "", err
	}
	data, err := content(agent)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("create command directory: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", fmt.Errorf("write command file: %w", err)
	}
	return path, nil
}

func content(agent Agent) ([]byte, error) {
	switch agent {
	case Claude:
		return skills.ClaudeCommand, nil
	case OpenCode:
		return skills.OpenCodeCommand, nil
	default:
		return nil, fmt.Errorf("unknown agent %q", agent)
	}
}
