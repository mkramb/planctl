# planctl

`planctl` reviews coding-agent implementation plans as GitHub pull requests,
before any code is written.

A coding agent writes a Markdown plan, `planctl` opens a GitHub pull request for
it, your team reviews and approves it on GitHub, and only then does the agent
start implementing. GitHub provides the review experience; `planctl` provides
the plan lifecycle.

**planctl uses GitHub only.** An internal interface lets the test suite swap in
a fake for GitHub so tests run fast and offline; that interface is a testing
seam, not a plugin system.

## Example session with OpenCode

Here's how a plan flows from a coding agent through GitHub review.

### Setup

1. Install `planctl` (see [Install](#install)) and authenticate the GitHub CLI
   (`gh auth login`).

2. Install the planctl integration (skill + `/planctl` command) into OpenCode:

   ```sh
   planctl skills install --agent opencode
   ```

   Then restart OpenCode so it picks up the skill and command.

3. Initialize the repository you want to plan in:

   ```sh
   planctl init
   ```

### Using it

Ask OpenCode to implement something non-trivial, for example:

> "Add single sign-on to the payments service."

The skill takes over and drives `planctl` for you:

1. OpenCode runs `planctl create "Add SSO"` and writes the Markdown plan (with
   any Mermaid diagrams) at the returned path. It has not touched implementation
   code.

2. OpenCode runs `planctl review`, which commits the plan, pushes a
   `plan/add-sso` branch, opens a ready-for-review GitHub pull request, and then
   **blocks**. It does not return until the review reaches a decision.

3. Your team reviews on GitHub: inline comments, review threads, "request
   changes", and approvals.

4. If a reviewer requests changes, `planctl review` returns with
   `implementation.allowed == false` and `blocked_reasons: ["changes_requested"]`.
   OpenCode runs `planctl feedback`, revises the plan, and runs `planctl review`
   again.

5. When the team approves, `planctl review` returns with
   `implementation.allowed == true`. Only then does OpenCode start writing code —
   in your normal checkout, not the plan worktree.

6. After implementation, OpenCode runs `planctl complete`. `pr-only` retention
   closes the pull request without merging; `repository` retention merges the
   plan into the base branch.

The gate is deterministic: **`planctl review` blocks and cannot return with
`implementation.allowed == true` until the plan is approved.** Reviewers,
approvals, and change requests all happen on GitHub; `planctl` reads them and
enforces the gate.

The `/planctl` command lets you drive planctl directly from either agent:

```
/planctl status
/planctl review
/planctl feedback
```

## Install

`planctl` is a single Go binary. It drives your installed `git` and GitHub CLI
(`gh`); authenticate `gh` before publishing.

```sh
go install github.com/mkramb/planctl/cmd/planctl@latest
```

Or build from source with [mise](https://mise.jdx.dev/):

```sh
mise install
mise run build   # produces bin/planctl
```

## Commands

| Command | What it does |
|---|---|
| `planctl init` | Create `.planctl.yaml` in the current repository |
| `planctl create "<title>"` | Create a plan in its own Git worktree |
| `planctl publish` | Commit, push, and open/update the review PR |
| `planctl review` | Publish, then block until approved or changes requested |
| `planctl status` | Show review status and approvals |
| `planctl feedback` | List review comments |
| `planctl context` | Report whether implementation is allowed |
| `planctl complete` | Close (pr-only) or merge (repository) the review |
| `planctl skills install` | Install the agent skill and `/planctl` command |

Every command supports `--json` for agents and plain text for humans. See
[architecture](docs/architecture.md), [configuration](docs/configuration.md),
and [publishing](docs/publishing.md) for details.

## Agent skills

```sh
planctl skills install                  # Claude Code + OpenCode
planctl skills install --agent claude   # Claude Code only
```

This installs both a skill and a `/planctl` slash command per agent. The skills
([Claude Code](skills/claude-code/SKILL.md), [OpenCode](skills/opencode/SKILL.md))
automate the workflow above; the command exposes `planctl` directly.

## Development

Install [mise](https://mise.jdx.dev/) and Git, then from this directory:

```sh
mise install
mise run build
./bin/planctl --help
mise run validate   # fmt + vet + lint
mise run test       # all tests
```

Tests use temporary Git repositories and a fake GitHub adapter, so they run
offline without `gh` or network access. See [development](docs/development.md).

## License

[MIT](LICENSE).
