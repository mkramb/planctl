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

Tests need Git but never the network, `gh`, or GitHub credentials. The fake
provider models behavior (`Approve`, `RequestChanges`, `AddFeedback`), not
expected method calls.

Use `require` for setup steps and `assert` for result checks (testify).

## Output rules

- `--json` prints one versioned JSON document on stdout; errors are JSON too.
- Human output is text; diagnostics go to stderr.
- Exit codes: 0 ok, 1 failure, 2 bad arguments, 130 canceled.
