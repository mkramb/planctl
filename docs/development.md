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
mise run test              # all tests
mise run test-integration  # integration tests only
mise run build             # build bin/planctl
```

## Testing

Integration tests are the main tests. Each one:

1. Creates a temp directory with a real Git repo and a local bare remote.
2. Runs real CLI commands through the Cobra tree.
3. Uses a stateful fake review provider instead of GitHub.

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

## Agent skills

`skills/claude-code/SKILL.md` and `skills/opencode/SKILL.md` drive the plan
workflow. They use `--json` output and stop before implementation until
`context --json` reports `implementation.allowed == true`.
