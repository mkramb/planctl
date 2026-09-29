package cli

import (
	"errors"
	"os"
	"strings"

	"github.com/mkramb/planctl/internal/output"
	"github.com/mkramb/planctl/internal/skill"
	"github.com/spf13/cobra"
)

func newSkills(deps Dependencies, opts *options) *cobra.Command {
	var agent string
	install := &cobra.Command{
		Use:   "install",
		Short: "Install the /planctl command for a coding agent",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			agents, err := parseAgents(agent)
			if err != nil {
				return usageError{err}
			}
			home, err := homeDir(deps.Env)
			if err != nil {
				return err
			}
			result := output.SkillsResult{Version: output.Version}
			for _, a := range agents {
				path, err := skill.Install(home, a)
				if err != nil {
					return err
				}
				result.Installed = append(result.Installed, output.SkillInstall{Agent: string(a), Path: path})
			}
			return (output.Renderer{Stdout: deps.Stdout, Stderr: deps.Stderr, JSON: opts.json}).Skills(result)
		},
	}
	install.Flags().StringVar(&agent, "agent", "all", "Agent to install for: claude, opencode, or all")

	cmd := &cobra.Command{
		Use:   "skills",
		Short: "Install the /planctl command for coding agents",
	}
	cmd.AddCommand(install)
	return cmd
}

func parseAgents(value string) ([]skill.Agent, error) {
	switch strings.ToLower(value) {
	case "all":
		return skill.All(), nil
	case string(skill.Claude):
		return []skill.Agent{skill.Claude}, nil
	case string(skill.OpenCode):
		return []skill.Agent{skill.OpenCode}, nil
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
