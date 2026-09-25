# Development

## Commands

```sh
mise install
mise run fmt
mise run vet
mise run test
mise run test-integration
mise run build
```

mise pins Go in `mise.toml`. Go modules pin library dependencies in `go.mod` and
`go.sum`. Toolchain/module installation may use the network; test execution does
not. Real Git must be installed. Tests do not need `gh` or GitHub credentials.

## Test architecture

`test/testutil.Environment` owns a temporary Git repository, local bare remote,
stateful review provider, and isolated Git environment. Each `Run` creates a fresh
Cobra command tree, passes the working directory explicitly, and captures stdout,
stderr, and exit status. Tests do not change process working directory or global
environment, so environments can run in parallel.

The fixture disables system/global Git configuration, signing, hooks, inherited
Git repository overrides, and terminal prompts. Git identity is repository-local.
Commits and pushes go through the real Git wrapper and process runner. The fake
models review state and human actions instead of asserting method-call sequences.

Integration tests are the main behavioral tests. Focused package tests cover
process cancellation, output, and provider semantics where direct tests are useful.
Future GitHub adapter tests replace only its command executor with fixture output.

Use `github.com/stretchr/testify/require` for test prerequisites and
`github.com/stretchr/testify/assert` for independent result checks. The review
provider remains a hand-written stateful fake; no mocking framework is needed.

## Output contract

Operational JSON responses contain `version: 1` and use `snake_case`. An error is
one JSON document on stdout with a stable code and a nonzero process exit status;
diagnostics use stderr. Cobra must not print an extra error or usage banner.
Human and JSON renderers consume the same typed result. Conventional `--help` and
`--version` output are text exceptions, even when combined with `--json`.

Exit statuses: `0` success, `1` operation failure, `2` invalid CLI usage, `130`
cancellation. Future command-specific errors retain the same envelope.

Never print raw subprocess environments or include credentials in diagnostics.
