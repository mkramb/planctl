// Package skill installs planctl's agent skills into the target agent's own
// skill directory.
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

// Path returns where an agent's skill lives without touching the filesystem.
func Path(home string, agent Agent) (string, error) {
	var dir string
	switch agent {
	case Claude:
		dir = filepath.Join(home, ".claude", "skills", "planctl")
	case OpenCode:
		dir = filepath.Join(home, ".config", "opencode", "skills", "planctl")
	default:
		return "", fmt.Errorf("unknown agent %q", agent)
	}
	return filepath.Join(dir, "SKILL.md"), nil
}

// Install writes the embedded skill for the given agent under home and returns
// the installed file path.
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
		return "", fmt.Errorf("create skill directory: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", fmt.Errorf("write skill file: %w", err)
	}
	return path, nil
}

func content(agent Agent) ([]byte, error) {
	switch agent {
	case Claude:
		return skills.ClaudeCode, nil
	case OpenCode:
		return skills.OpenCode, nil
	default:
		return nil, fmt.Errorf("unknown agent %q", agent)
	}
}
