// Package integrations embeds the /planctl slash commands shipped with planctl.
// The canonical command sources live alongside this file; install copies them
// into the agent's own command directory.
package integrations

import _ "embed"

// ClaudeCommand and OpenCodeCommand are the /planctl slash commands.
//
//go:embed claude-code/commands/planctl.md
var ClaudeCommand []byte

//go:embed opencode/commands/planctl.md
var OpenCodeCommand []byte
