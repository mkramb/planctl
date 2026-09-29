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

You drive planctl through a `/planctl` command installed into your agent. Here's
the whole thing.

### Setup (once)

```sh
gh auth login
planctl skills install --agent opencode   # installs the /planctl command
# restart OpenCode
cd your-repo && planctl init
```

### Using it

Ask OpenCode to plan a change, then run the command yourself:

```
/planctl create "Add single sign-on"
```

OpenCode creates the plan and tells you where the Markdown file is. Have it
write the plan, then publish it for review:

```
/planctl review
```

This opens a GitHub pull request and blocks until the review reaches a decision.
On GitHub, your team comments, requests changes, or approves:

- **Request changes** → `/planctl feedback` to read the comments, revise, then
  `/planctl review` again.
- **Approve** → `/planctl review` returns with `implementation.allowed == true`,
  and only then should OpenCode start writing code.

When implementation is done, finish the plan:

```
/planctl complete
```

### The one rule

`planctl review` blocks and only returns `implementation.allowed == true` once
the review is approved. You stay in charge through GitHub's normal review flow.

### Plan mode

Use planctl in a normal session, not OpenCode's plan mode. `planctl create` and
`planctl publish` write to disk (they create a worktree, push a branch, and open
a PR), so they're blocked while plan mode is read-only.

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
| `planctl skills install` | Install the `/planctl` command for an agent |

Every command supports `--json` for agents and plain text for humans. See
[architecture](docs/architecture.md), [configuration](docs/configuration.md),
and [publishing](docs/publishing.md) for details.

## Agent command

```sh
planctl skills install                  # Claude Code + OpenCode
planctl skills install --agent claude   # Claude Code only
```

This installs a `/planctl` slash command per agent, so you can run planctl
directly (`/planctl create`, `/planctl review`, `/planctl status`, and so on).

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
