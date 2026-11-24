#!/bin/sh
# Installs ctlctl from GitHub Releases.
#
#   curl -fsSL https://raw.githubusercontent.com/rafalmaciejewski/ctlctl/main/install.sh | sh
#
# Environment variables:
#   CTLCTL_VERSION      version to install, such as 1.2.0 (default: the latest)
#   CTLCTL_INSTALL_DIR  directory for the binary (default: ~/.local/bin)
#   CTLCTL_BASE_URL     download from a mirror of the release assets instead
set -eu

repo="rafalmaciejewski/ctlctl"

say() { printf '%s\n' "$*" >&2; }
fail() {
	say "ctlctl install: $*"
	exit 1
}

case "$(uname -s)" in
Darwin) os=darwin ;;
Linux) os=linux ;;
FreeBSD) os=freebsd ;;
MINGW* | MSYS* | CYGWIN*) fail "on Windows, use install.ps1 (see the README)" ;;
*) fail "unsupported operating system: $(uname -s)" ;;
esac

case "$(uname -m)" in
x86_64 | amd64) arch=amd64 ;;
arm64 | aarch64) arch=arm64 ;;
armv7* | armv8l) arch=armv7 ;;
*) fail "unsupported architecture: $(uname -m)" ;;
esac

# A shell running under Rosetta reports x86_64 on Apple silicon.
if [ "$os" = darwin ] && [ "$arch" = amd64 ] &&
	[ "$(sysctl -n sysctl.proc_translated 2>/dev/null || echo 0)" = 1 ]; then
	arch=arm64
fi

version="${CTLCTL_VERSION:-}"
if [ -n "$version" ]; then
	base="https://github.com/$repo/releases/download/v${version#v}"
else
	base="https://github.com/$repo/releases/latest/download"
fi
base="${CTLCTL_BASE_URL:-$base}"
asset="ctlctl_${os}_${arch}.tar.gz"
dir="${CTLCTL_INSTALL_DIR:-$HOME/.local/bin}"

download() {
	if command -v curl >/dev/null 2>&1; then
		curl -fsSL --retry 3 -o "$2" "$1"
	elif command -v wget >/dev/null 2>&1; then
		wget -q -O "$2" "$1"
	else
		fail "needs curl or wget"
	fi
}

sha256() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | cut -d' ' -f1
	elif command -v shasum >/dev/null 2>&1; then
		shasum -a 256 "$1" | cut -d' ' -f1
	else
		openssl dgst -sha256 "$1" | sed 's/.* //'
	fi
}

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT INT TERM

say "Downloading $asset..."
download "$base/$asset" "$tmp/$asset" || fail "couldn't download $base/$asset"
download "$base/checksums.txt" "$tmp/checksums.txt" || fail "couldn't download checksums.txt"

expected="$(grep " $asset\$" "$tmp/checksums.txt" | cut -d' ' -f1)"
[ -n "$expected" ] || fail "$asset is not in checksums.txt"
[ "$(sha256 "$tmp/$asset")" = "$expected" ] || fail "checksum mismatch for $asset"

tar -xzf "$tmp/$asset" -C "$tmp" ctlctl
mkdir -p "$dir" || fail "can't create $dir (set CTLCTL_INSTALL_DIR to another directory)"
if [ ! -w "$dir" ]; then
	fail "can't write to $dir (set CTLCTL_INSTALL_DIR to another directory)"
fi
mv "$tmp/ctlctl" "$dir/ctlctl"
chmod 755 "$dir/ctlctl"

say "Installed $("$dir/ctlctl" --version) to $dir/ctlctl"
case ":$PATH:" in
*":$dir:"*) ;;
*)
	say ""
	say "$dir is not on your PATH. Add it to your shell profile:"
	say "  export PATH=\"$dir:\$PATH\""
	;;
esac
