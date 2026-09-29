#!/usr/bin/env sh
# planctl installer for macOS (arm64).
# Downloads the latest release from GitHub, verifies its checksum, and installs
# the binary. Override the install directory with PREFIX or the version with
# PLANCTL_VERSION.

set -eu

REPO="mkramb/planctl"
BINARY="planctl"
OS="darwin"
ARCH="arm64"

error() { echo "error: $*" >&2; exit 1; }

command -v curl >/dev/null 2>&1 || error "curl is required"
command -v tar >/dev/null 2>&1 || error "tar is required"

# Resolve the install directory.
if [ -n "${PREFIX:-}" ]; then
    install_dir="$PREFIX"
elif [ -d /usr/local/bin ] && [ -w /usr/local/bin ]; then
    install_dir="/usr/local/bin"
else
    install_dir="${HOME}/.local/bin"
fi

# Resolve the version, stripping a leading "v".
version="${PLANCTL_VERSION:-}"
if [ -z "$version" ]; then
    version="$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" \
        | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p')"
    [ -n "$version" ] || error "could not determine the latest version"
fi
version="${version#v}"

archive="${BINARY}_${version}_${OS}_${ARCH}.tar.gz"
url="https://github.com/$REPO/releases/download/v${version}/${archive}"
checksums_url="https://github.com/$REPO/releases/download/v${version}/checksums.txt"

tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT

echo "Downloading $archive..."
curl -fsSL -o "$tmpdir/$archive" "$url" || error "could not download $url"
curl -fsSL -o "$tmpdir/checksums.txt" "$checksums_url" || true

verify_checksum() {
    local expected actual
    expected="$(awk -v f=" $archive$" '$2==f {print $1}' "$tmpdir/checksums.txt")"
    [ -n "$expected" ] || return 0
    actual="$(shasum -a 256 "$tmpdir/$archive" | awk '{print $1}')"
    [ "$expected" = "$actual" ] || error "checksum verification failed"
}

if [ -s "$tmpdir/checksums.txt" ]; then
    verify_checksum
fi

tar -xzf "$tmpdir/$archive" -C "$tmpdir"

mkdir -p "$install_dir" || error "cannot create $install_dir"
if ! install -m 0755 "$tmpdir/$BINARY" "$install_dir/$BINARY" 2>/dev/null; then
    cp "$tmpdir/$BINARY" "$install_dir/$BINARY"
    chmod +x "$install_dir/$BINARY"
fi

echo "Installed $BINARY v$version to $install_dir/$BINARY"
