// Package skill installs planctl's agent integration (a skill plus a /planctl
// slash command) into the target agent's own directory.
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

// File describes one installed file.
type File struct {
	Kind string
	Path string
}

// Paths returns where the skill and command live without touching the filesystem.
func Paths(home string, agent Agent) (skill, command string, err error) {
	switch agent {
	case Claude:
		return filepath.Join(home, ".claude", "skills", "planctl", "SKILL.md"),
			filepath.Join(home, ".claude", "commands", "planctl.md"), nil
	case OpenCode:
		return filepath.Join(home, ".config", "opencode", "skills", "planctl", "SKILL.md"),
			filepath.Join(home, ".config", "opencode", "command", "planctl.md"), nil
	default:
		return "", "", fmt.Errorf("unknown agent %q", agent)
	}
}

// Install writes the skill and slash command for the given agent under home.
func Install(home string, agent Agent) ([]File, error) {
	skillPath, commandPath, err := Paths(home, agent)
	if err != nil {
		return nil, err
	}
	skillData, commandData, err := content(agent)
	if err != nil {
		return nil, err
	}
	if err := write(skillPath, skillData); err != nil {
		return nil, err
	}
	if err := write(commandPath, commandData); err != nil {
		return nil, err
	}
	return []File{
		{Kind: "skill", Path: skillPath},
		{Kind: "command", Path: commandPath},
	}, nil
}

func write(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write file: %w", err)
	}
	return nil
}

func content(agent Agent) (skill, command []byte, err error) {
	switch agent {
	case Claude:
		return skills.ClaudeCode, skills.ClaudeCommand, nil
	case OpenCode:
		return skills.OpenCode, skills.OpenCodeCommand, nil
	default:
		return nil, nil, fmt.Errorf("unknown agent %q", agent)
	}
}
