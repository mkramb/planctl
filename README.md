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

2. Install the planctl skill into OpenCode:

   ```sh
   planctl skills install --agent opencode
   ```

   Then restart OpenCode so it picks up the skill.

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

2. OpenCode runs `planctl publish`, which commits the plan, pushes a
   `plan/add-sso` branch, and opens a ready-for-review GitHub pull request.

3. OpenCode stops. It will not implement anything while the plan is under review.

4. Your team reviews on GitHub: inline comments, review threads, "request
   changes", and approvals.

5. When you ask OpenCode to continue, it runs `planctl context` to check the
   review. If `changes_requested` is set, it runs `planctl feedback` to read the
   comments, revises the plan, and runs `planctl publish` again to update the
   same pull request.

6. Only when the team approves and `planctl context` reports
   `implementation.allowed == true` does OpenCode start writing code — in your
   normal checkout, not the plan worktree.

7. After implementation, OpenCode runs `planctl complete`. `pr-only` retention
   closes the pull request without merging; `repository` retention merges the
   plan into the base branch.

The skill encodes the core rule: **do not start implementing until
`implementation.allowed` is `true`.** Reviewers, approvals, and change requests
all happen on GitHub; `planctl` reads them and enforces the gate.

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
| `planctl status` | Show review status and approvals |
| `planctl feedback` | List review comments |
| `planctl context` | Report whether implementation is allowed |
| `planctl complete` | Close (pr-only) or merge (repository) the review |
| `planctl skills install` | Install the agent skill |

Every command supports `--json` for agents and plain text for humans. See
[architecture](docs/architecture.md), [configuration](docs/configuration.md),
and [publishing](docs/publishing.md) for details.

## Agent skills

```sh
planctl skills install                  # Claude Code + OpenCode
planctl skills install --agent claude   # Claude Code only
```

The built-in skills ([Claude Code](skills/claude-code/SKILL.md),
[OpenCode](skills/opencode/SKILL.md)) automate the workflow above.

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
