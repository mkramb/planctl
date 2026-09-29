package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mkramb/planctl/integrations"
	"github.com/mkramb/planctl/internal/output"
	"github.com/spf13/cobra"
)

type agent string

const (
	agentClaude   agent = "claude"
	agentOpenCode agent = "opencode"
)

func allAgents() []agent { return []agent{agentClaude, agentOpenCode} }

// commandPath returns where an agent's /planctl command lives.
func commandPath(home string, a agent) (string, error) {
	switch a {
	case agentClaude:
		return filepath.Join(home, ".claude", "commands", "planctl.md"), nil
	case agentOpenCode:
		return filepath.Join(home, ".config", "opencode", "command", "planctl.md"), nil
	default:
		return "", fmt.Errorf("unknown agent %q", a)
	}
}

func commandContent(a agent) ([]byte, error) {
	switch a {
	case agentClaude:
		return integrations.ClaudeCommand, nil
	case agentOpenCode:
		return integrations.OpenCodeCommand, nil
	default:
		return nil, fmt.Errorf("unknown agent %q", a)
	}
}

// installCommand writes the /planctl slash command for the given agent.
func installCommand(home string, a agent) (string, error) {
	path, err := commandPath(home, a)
	if err != nil {
		return "", err
	}
	data, err := commandContent(a)
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

// uninstallCommand removes the /planctl slash command for the given agent.
// Removing an absent file succeeds. Empty parent directories are pruned
// best-effort.
func uninstallCommand(home string, a agent) (string, error) {
	path, err := commandPath(home, a)
	if err != nil {
		return "", err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	_ = os.Remove(filepath.Dir(path))
	return path, nil
}

func newInstall(deps Dependencies, opts *options) *cobra.Command {
	var name string
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install the /planctl command for a coding agent",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			agents, err := parseAgents(name)
			if err != nil {
				return usageError{err}
			}
			home, err := homeDir(deps.Env)
			if err != nil {
				return err
			}
			result := output.SkillsResult{Version: output.Version}
			for _, a := range agents {
				path, err := installCommand(home, a)
				if err != nil {
					return err
				}
				result.Installed = append(result.Installed, output.SkillInstall{Agent: string(a), Path: path})
			}
			return (output.Renderer{Stdout: deps.Stdout, Stderr: deps.Stderr, JSON: opts.json}).Skills(result)
		},
	}
	cmd.Flags().StringVar(&name, "agent", "all", "Agent to install for: claude, opencode, or all")
	return cmd
}

func newUninstall(deps Dependencies, opts *options) *cobra.Command {
	var name string
	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Remove the /planctl command installed for a coding agent",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			agents, err := parseAgents(name)
			if err != nil {
				return usageError{err}
			}
			home, err := homeDir(deps.Env)
			if err != nil {
				return err
			}
			result := output.UninstallResult{Version: output.Version}
			for _, a := range agents {
				path, err := uninstallCommand(home, a)
				if err != nil {
					return err
				}
				result.Removed = append(result.Removed, output.SkillInstall{Agent: string(a), Path: path})
			}
			return (output.Renderer{Stdout: deps.Stdout, Stderr: deps.Stderr, JSON: opts.json}).Uninstall(result)
		},
	}
	cmd.Flags().StringVar(&name, "agent", "all", "Agent to uninstall for: claude, opencode, or all")
	return cmd
}

func parseAgents(value string) ([]agent, error) {
	switch strings.ToLower(value) {
	case "all":
		return allAgents(), nil
	case string(agentClaude):
		return []agent{agentClaude}, nil
	case string(agentOpenCode):
		return []agent{agentOpenCode}, nil
	default:
		return nil, errors.New("--agent must be claude, opencode, or all")
	}
}

func homeDir(env []string) (string, error) {
	for _, entry := range env {
		if key, value, ok := strings.Cut(entry, "="); ok && key == "HOME" {
			return value, nil
		}
	}
	return os.UserHomeDir()
}
