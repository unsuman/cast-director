#!/bin/sh
# Install cast-director from GitHub releases.
#
#   curl -fsSL https://raw.githubusercontent.com/unsuman/cast-director/main/install.sh | sh
#
# Settings (environment variables):
#   VERSION  release to install, e.g. v0.1.0 (default: latest)
#   BIN_DIR  where to put the binary (default: /usr/local/bin if writable,
#            otherwise ~/.local/bin)
set -eu

repo=unsuman/cast-director
name=cast-director

fail() {
	echo "install.sh: $*" >&2
	exit 1
}

case "$(uname -s)" in
Linux) os=linux ;;
Darwin) os=darwin ;;
*) fail "unsupported OS $(uname -s); cast-director runs on Linux and macOS" ;;
esac

case "$(uname -m)" in
x86_64 | amd64) arch=amd64 ;;
arm64 | aarch64) arch=arm64 ;;
*) fail "unsupported CPU $(uname -m)" ;;
esac

if [ -z "${VERSION:-}" ]; then
	# github.com/OWNER/REPO/releases/latest redirects to .../tag/vX.Y.Z.
	# This avoids the GitHub API and its rate limit.
	url=$(curl -fsSLI -o /dev/null -w '%{url_effective}' "https://github.com/$repo/releases/latest") ||
		fail "could not find the latest release"
	VERSION=${url##*/}
fi
case "$VERSION" in
v*) ;;
*) VERSION=v$VERSION ;;
esac

archive="${name}_${VERSION#v}_${os}_${arch}.tar.gz"
base="https://github.com/$repo/releases/download/$VERSION"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

echo "Downloading $name $VERSION for $os/$arch"
curl -fsSL -o "$tmp/$archive" "$base/$archive" || fail "download failed: $base/$archive"
curl -fsSL -o "$tmp/checksums.txt" "$base/checksums.txt" || fail "download failed: $base/checksums.txt"

want=$(awk -v f="$archive" '$2 == f { print $1 }' "$tmp/checksums.txt")
[ -n "$want" ] || fail "$archive is not in checksums.txt"
if command -v sha256sum >/dev/null 2>&1; then
	got=$(sha256sum "$tmp/$archive" | awk '{ print $1 }')
else
	got=$(shasum -a 256 "$tmp/$archive" | awk '{ print $1 }')
fi
[ "$got" = "$want" ] || fail "checksum mismatch for $archive"

tar -xzf "$tmp/$archive" -C "$tmp" "$name"

if [ -z "${BIN_DIR:-}" ]; then
	if [ -w /usr/local/bin ]; then
		BIN_DIR=/usr/local/bin
	else
		BIN_DIR=$HOME/.local/bin
	fi
fi
mkdir -p "$BIN_DIR"
mv "$tmp/$name" "$BIN_DIR/$name"
chmod 755 "$BIN_DIR/$name"
echo "Installed $BIN_DIR/$name"

case ":$PATH:" in
*":$BIN_DIR:"*) ;;
*) echo "Note: $BIN_DIR is not in your PATH. Add it, e.g. export PATH=\"$BIN_DIR:\$PATH\"" ;;
esac
