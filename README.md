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

## Example session

A developer asks their coding agent to add single sign-on.

1. The agent turns the request into a plan:

   ```sh
   planctl init
   planctl create "Add SSO" --json
   ```

   The `create` output includes `plan.absolute_path`. The agent writes the
   Markdown (and any Mermaid diagrams) there and does not touch implementation
   code yet.

2. The agent publishes the plan for review:

   ```sh
   planctl publish --json
   ```

   This commits the plan, pushes a `plan/add-sso` branch, and opens a
   ready-for-review GitHub pull request.

3. The team reviews on GitHub: inline comments, review threads, "request
   changes", and approvals.

4. When asked to continue, the agent checks the review:

   ```sh
   planctl context --json
   ```

   The response reports `implementation.allowed` and, when blocked, the
   `blocked_reasons`.

5. If changes were requested, the agent reads them and revises:

   ```sh
   planctl feedback --json
   # edit plan.absolute_path to address the comments
   planctl publish --json   # updates the same pull request
   ```

6. Once the team approves the current revision:

   ```sh
   planctl context --json   # implementation.allowed == true
   ```

7. The agent implements the change in the normal repository checkout.

8. After implementation, the agent finishes the plan:

   ```sh
   planctl complete --json
   ```

   `pr-only` retention closes the pull request without merging; `repository`
   retention merges the plan into the base branch.

The core rule: **do not start implementing until `implementation.allowed` is
`true`.** Reviewers, approvals, and change requests all happen on GitHub;
`planctl` reads them and enforces the gate.

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
