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
mise run vet               # static analysis (unit + integration-tagged tests)
mise run lint              # golangci-lint
mise run validate          # fmt + vet + lint
mise run test              # unit tests (no git/gh/network)
mise run test-integration  # integration tests (real Git + in-memory GitHub)
mise run build             # build bin/planctl
```

## Tests

Tests are split into two tiers by build tag:

- **Unit** (`go test ./...`): pure logic in the `internal/` packages and CLI
  wiring. No git, `gh`, or network.
- **Integration** (`go test -tags integration ./...`): the flow in `cmd/planctl`,
  using a real local Git repository and a stateful in-memory GitHub
  (`internal/github/mock.go`). These run offline and need Git but never `gh` or
  the network. The mock observes real pushes to a bare remote, so republishing
  updates reviews through Git rather than a fake update API.

A live GitHub test (`cmd/planctl/gh_test.go`) shares the `integration` tag but
skips unless `PLANCTL_GH_TEST_REPO` is set to an accessible `owner/repository` —
so the same task runs offline in CI and against real GitHub when configured.

Use `require` for setup steps and `assert` for result checks (testify).

## Output rules

- `--json` prints one versioned JSON document on stdout; errors are JSON too.
- `--help` and `--version` remain text; use `version --json` for JSON.
- Human output is text; diagnostics and the PR URL go to stderr.
- Exit codes: 0 ok, 1 failure, 2 bad arguments, 130 canceled.

## Releasing

Releases are built with [GoReleaser](https://goreleaser.com/) for macOS arm64.
Tag and push to trigger `.github/workflows/release.yml`:

```sh
git tag v1.0.0
git push origin v1.0.0
```

The tag's leading `v` is stripped for `planctl --version` (tag `v1.0.0` ->
version `1.0.0`), injected at build time via `-X main.version={{ .Version }}`.

Validate the config and build a snapshot locally without publishing:

```sh
mise x goreleaser@latest -- goreleaser check
mise x goreleaser@latest -- goreleaser build --snapshot --clean
```

Users install the latest release with:

```sh
curl -fsSL https://raw.githubusercontent.com/mkramb/planctl/master/install.sh | sh
```

`install.sh` downloads the latest `darwin/arm64` asset, verifies its SHA-256
checksum, and installs the binary. `PREFIX` selects the directory and
`PLANCTL_VERSION` pins a version.
