# Development

## Setup

Install [mise](https://mise.jdx.dev/) and Git. Then:

```sh
mise install   # installs the pinned Go version
mise run build
./bin/planctl --help
```

## Tasks

```sh
mise run fmt               # format
mise run vet               # static analysis
mise run lint              # golangci-lint
mise run validate          # fmt + vet + lint
mise run test              # all tests
mise run test-integration  # integration tests only
mise run build             # build bin/planctl
```

## Testing

Integration tests are the main tests. Each one:

1. Creates a temp directory with a real Git repo and a local bare remote.
2. Runs real CLI commands through the Cobra tree.
3. Uses a stateful fake that stands in for GitHub.

Managed plan worktrees and locks use each test's temporary cache directory.
Worktree tests check that staged files, local edits, and feature commits stay out
of plan creation.

Publish tests use real commits and pushes. The fake provider reads branch heads
from the local bare remote, so republishing updates reviews through Git rather
than a fake update API. Test controls simulate comments, decisions, terminal
reviews, and temporary provider failures.

Tests need Git but never the network, `gh`, or GitHub credentials. The fake
provider models behavior (`Approve`, `RequestChanges`, `AddFeedback`), not
expected method calls.

Install the Go toolchain and module dependencies before running tests offline.

Use `require` for setup steps and `assert` for result checks (testify).

## Output rules

- `--json` prints one versioned JSON document on stdout; errors are JSON too.
- `--help` and `--version` remain text; use `version --json` for JSON.
- Human output is text; diagnostics go to stderr.
- Exit codes: 0 ok, 1 failure, 2 bad arguments, 130 canceled.

## Releasing

See [releasing](releasing.md) for the tag-and-push release flow and how users
install the built binary.

## Agent command

`planctl skills install` writes a `/planctl` slash command
(`skills/*/commands/planctl.md`) per agent so users can run planctl directly.
Implementation routes through the `planctl review` gate, which blocks until the
review is approved.
