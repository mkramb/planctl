# planctl

A local, review-platform-native workflow for reviewing and approving coding-agent
implementation plans before implementation begins.

Git provides history. GitHub provides collaboration. `planctl` connects them.

## Development status

V1 is being built incrementally. `init`, `create`, `publish`, `status`,
`feedback`, `context`, and `complete` are implemented. The GitHub adapter is
wired and covered by fixture tests; it has not yet been exercised against live
GitHub. Dedicated plans repositories and agent skills are next.

See [architecture](docs/architecture.md) for design decisions.

## Initialize a repository

From an existing Git repository or one of its subdirectories:

```sh
planctl init
```

This creates `.planctl.yaml` at the worktree root. It does not overwrite an
existing file or change Git history. See [configuration](docs/configuration.md)
for defaults and `--config`.

## Create a plan

```sh
planctl create "Add SSO" --json
```

Edit the returned `plan.absolute_path`. The Markdown template lives in a separate
Git worktree; your current branch and files stay untouched. Creation works from
an existing worktree too. It creates a local plan branch but does not commit or
push. Repeating the same title reports the existing plan without overwriting it.

## Publishing (under development)

The publish workflow commits only the selected plan, pushes its branch, and
creates or reuses one review. See [publishing](docs/publishing.md) for selection
and retry behavior. The standalone binary currently reports `provider_unavailable`
without changing Git; real GitHub publishing arrives with the GitHub adapter.

## Development

Install [mise](https://mise.jdx.dev/) and Git, then run from this directory:

```sh
mise install
mise run build
./bin/planctl --help
mise run test
mise run vet
```

The Go toolchain and dependencies must be installed before testing offline.
Tests themselves use temporary local Git repositories and local bare remotes;
they require neither GitHub nor `gh` nor network access.

See [development](docs/development.md) for test architecture and commands.

## License

[MIT](LICENSE).
