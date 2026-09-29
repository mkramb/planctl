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

You never type `planctl` commands yourself — the agent does. Here's the whole
thing.

### Setup (once)

```sh
gh auth login
planctl skills install --agent opencode
# restart OpenCode
cd your-repo && planctl init
```

### Then it's just a conversation

> **You:** "Add single sign-on to the payments service."

OpenCode writes a Markdown plan and opens a GitHub pull request for it, then
waits. You don't run anything.

> **You** (on GitHub): review the pull request. Comment, request changes, or
> approve.

- If you **approve**, OpenCode sees it and starts writing code.
- If you **request changes**, OpenCode reads your comments, updates the plan, and
  re-opens it for another look. Repeat until you approve.

That's the whole loop: you ask, OpenCode plans, you review on GitHub, OpenCode
implements once you've approved.

### The one rule

OpenCode will **not** write implementation code until the plan is approved. It
enforces this itself with `planctl review`, which blocks and only returns
`implementation.allowed == true` once the review is approved. You stay in charge
through GitHub's normal review flow.

### Plan mode

Use planctl in a normal session, not OpenCode's plan mode. `planctl create` and
`planctl publish` write to disk (they create a worktree, push a branch, and open
a PR), so they're blocked while plan mode is read-only. If you're in plan mode,
switch out of it before the agent runs `planctl review`.

If you ever want to drive it yourself, a `/planctl` command is installed:

```
/planctl status
/planctl feedback
/planctl review
```

## Install

`planctl` is a single Go binary. It drives your installed `git` and GitHub CLI
(`gh`); authenticate `gh` before publishing.

Install the latest release (macOS arm64):

```sh
curl -fsSL https://raw.githubusercontent.com/mkramb/planctl/master/install.sh | sh
```

Set `PREFIX` to choose the install directory (defaults to `/usr/local/bin`, or
`~/.local/bin` if that isn't writable) and `PLANCTL_VERSION` to pin a version:

```sh
curl -fsSL https://raw.githubusercontent.com/mkramb/planctl/master/install.sh | PREFIX="$HOME/.local/bin" sh
```

Alternatively, download a binary from the latest
[GitHub Release](https://github.com/mkramb/planctl/releases), or build from
source with [mise](https://mise.jdx.dev/):

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
