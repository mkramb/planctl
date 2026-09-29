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

Releases are built with [GoReleaser](https://goreleaser.com/) for macOS arm64.
Tagging a release triggers `.github/workflows/release.yml`, which builds the
binary and attaches it to a new GitHub Release.

1. Make sure `master` is green and up to date.
2. Tag and push:

   ```sh
   git tag v1.0.0
   git push origin v1.0.0
   ```

The workflow builds `planctl_1.0.0_darwin_arm64.tar.gz` plus `checksums.txt` and
publishes them as a GitHub Release with generated release notes. The tag's
leading `v` is stripped for the version reported by `planctl --version`.

Users install the latest release with:

```sh
curl -fsSL https://raw.githubusercontent.com/mkramb/planctl/master/install.sh | sh
```

To build a release locally (or verify the config) without publishing:

```sh
mise x goreleaser@latest -- goreleaser check      # validate .goreleaser.yaml
mise x goreleaser@latest -- goreleaser build --snapshot --clean
```

## Agent skills

`skills/claude-code/SKILL.md` and `skills/opencode/SKILL.md` drive the plan
workflow, and `planctl skills install` also writes a `/planctl` slash command
(`skills/*/commands/planctl.md`) so users can run planctl directly. Both route
implementation through the `planctl review` gate, which blocks until the review
is approved.
