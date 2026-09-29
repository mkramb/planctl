// Package skills embeds the agent skills shipped with planctl. The canonical
// skill sources live alongside this file; install copies them into the agent's
// own skill directory.
package skills

import _ "embed"

// ClaudeCode is the Claude Code skill definition.
//
//go:embed claude-code/SKILL.md
var ClaudeCode []byte

// OpenCode is the OpenCode skill definition.
//
//go:embed opencode/SKILL.md
var OpenCode []byte

// ClaudeCommand and OpenCodeCommand are the /planctl slash commands.
//
//go:embed claude-code/commands/planctl.md
var ClaudeCommand []byte

//go:embed opencode/commands/planctl.md
var OpenCodeCommand []byte
