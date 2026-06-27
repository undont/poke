#!/usr/bin/env bash
# poke installer.
#
#   curl -fsSL https://raw.githubusercontent.com/undont/poke/main/install.sh | bash
#
# downloads the poke and poked binaries for this machine from the latest github
# release. if no release exists yet and go is installed, it builds from source
# instead. override the target with POKE_INSTALL_DIR and the version with
# POKE_VERSION (a release tag, or "latest").
set -euo pipefail

REPO="undont/poke"
INSTALL_DIR="${POKE_INSTALL_DIR:-$HOME/.local/bin}"
VERSION="${POKE_VERSION:-latest}"

die() { printf 'install: %s\n' "$1" >&2; exit 1; }

command -v curl >/dev/null || die "curl is required"

os=$(uname -s | tr '[:upper:]' '[:lower:]')
arch=$(uname -m)
case "$arch" in
	x86_64 | amd64) arch=amd64 ;;
	arm64 | aarch64) arch=arm64 ;;
	*) die "unsupported architecture: $arch" ;;
esac
case "$os" in
	darwin | linux) ;;
	*) die "unsupported os: $os" ;;
esac

mkdir -p "$INSTALL_DIR"

download_release() {
	local tag="$VERSION"
	if [ "$tag" = latest ]; then
		tag=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" 2>/dev/null \
			| grep -oE '"tag_name":[[:space:]]*"[^"]+"' | head -1 | cut -d'"' -f4)
	fi
	[ -n "$tag" ] || return 1

	local base="https://github.com/$REPO/releases/download/$tag"
	for bin in poke poked; do
		printf 'downloading %s_%s_%s (%s)\n' "$bin" "$os" "$arch" "$tag"
		curl -fSL "$base/${bin}_${os}_${arch}" -o "$INSTALL_DIR/$bin" || return 1
		chmod +x "$INSTALL_DIR/$bin"
	done
}

build_from_source() {
	command -v go >/dev/null || return 1
	printf 'no release binaries found; building from source with go\n'
	# when latest is requested but nothing is tagged yet, fall back to main
	local refs="$VERSION"
	[ "$VERSION" = latest ] && refs="latest main"
	for ref in $refs; do
		if GOBIN="$INSTALL_DIR" go install "github.com/$REPO/cmd/poke@$ref" \
			&& GOBIN="$INSTALL_DIR" go install "github.com/$REPO/cmd/poked@$ref"; then
			return 0
		fi
	done
	return 1
}

if download_release; then
	:
elif build_from_source; then
	:
else
	die "no release for $os/$arch and go is not installed to build from source"
fi

printf '\ninstalled poke and poked to %s\n' "$INSTALL_DIR"
case ":$PATH:" in
	*":$INSTALL_DIR:"*) ;;
	*) printf 'add it to your PATH:  export PATH="%s:$PATH"\n' "$INSTALL_DIR" ;;
esac
printf 'next: set POKE_SECRET, then run `poke connect`\n'
