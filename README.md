# planctl

A local, review-platform-native workflow for reviewing and approving coding-agent
implementation plans before implementation begins.

Git provides history. GitHub provides collaboration. `planctl` connects them.

## Development status

V1 is being built incrementally. The foundation provides CLI help, versioned
errors, external process execution, and an offline integration harness with real
Git and a stateful fake review provider. Lifecycle commands are next; they are
not yet available.

See [architecture](docs/architecture.md) for design decisions.

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
