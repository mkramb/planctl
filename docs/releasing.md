# Releasing

Releases are built with [GoReleaser](https://goreleaser.com/) for macOS arm64.

## Tag a release

1. Make sure `master` is green and up to date.
2. Tag and push:

   ```sh
   git tag v1.0.0
   git push origin v1.0.0
   ```

Pushing a `v*` tag triggers `.github/workflows/release.yml`, which builds the
binary and publishes it as a new GitHub Release.

## What gets published

- `planctl_1.0.0_darwin_arm64.tar.gz`
- `checksums.txt`
- Generated release notes (GitHub-native changelog)

The tag's leading `v` is stripped for the version reported by
`planctl --version` (tag `v1.0.0` → version `1.0.0`). The version is injected at
build time via `-X main.version={{ .Version }}`.

## Local verification

Validate the config and build a snapshot without publishing:

```sh
mise x goreleaser@latest -- goreleaser check      # validate .goreleaser.yaml
mise x goreleaser@latest -- goreleaser build --snapshot --clean
```

## Installing a release

Users install the latest release with:

```sh
curl -fsSL https://raw.githubusercontent.com/mkramb/planctl/master/install.sh | sh
```

`install.sh` downloads the latest `darwin/arm64` asset, verifies its SHA-256
checksum, and installs the binary. `PREFIX` selects the install directory and
`PLANCTL_VERSION` pins a version.
