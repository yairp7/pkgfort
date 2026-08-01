#!/bin/sh
# Installs pkgfort: downloads a prebuilt binary from GitHub Releases if one
# matches this platform, falling back to `go install` otherwise. Then runs
# pkgfort's own installer to wire up shell integration.
#
#   curl -fsSL https://raw.githubusercontent.com/yairp7/pkgfort/main/install.sh | sh
#
# Pin a version with:
#   PKGFORT_VERSION=v0.1.0 curl -fsSL .../install.sh | sh
set -eu

REPO="yairp7/pkgfort"
MODULE="github.com/yairp7/pkgfort/cmd/pkgfort"
VERSION="${PKGFORT_VERSION:-latest}"

info() { printf '==> %s\n' "$1"; }
fail() { printf 'error: %s\n' "$1" >&2; exit 1; }

goos=""
case "$(uname -s)" in
    Darwin) goos="darwin" ;;
    Linux)  goos="linux" ;;
esac

goarch=""
case "$(uname -m)" in
    x86_64|amd64)  goarch="amd64" ;;
    arm64|aarch64) goarch="arm64" ;;
esac

TMP_DIR=""
cleanup() { [ -z "$TMP_DIR" ] || rm -rf "$TMP_DIR"; }
trap cleanup EXIT

# Downloads a prebuilt release binary into a temp dir and sets BIN to its
# path. Returns non-zero (BIN left unset) if no matching release asset
# exists — e.g. unsupported platform, or no releases published yet — so the
# caller can fall back to `go install`.
fetch_prebuilt() {
    [ -n "$goos" ] && [ -n "$goarch" ] || return 1
    command -v curl >/dev/null 2>&1 || return 1

    asset="pkgfort_${goos}_${goarch}.tar.gz"
    if [ "$VERSION" = "latest" ]; then
        url="https://github.com/${REPO}/releases/latest/download/${asset}"
    else
        url="https://github.com/${REPO}/releases/download/${VERSION}/${asset}"
    fi

    TMP_DIR="$(mktemp -d)"
    if ! curl -fsSL "$url" -o "${TMP_DIR}/${asset}" 2>/dev/null; then
        return 1
    fi

    info "Downloading ${url}..."
    tar -xzf "${TMP_DIR}/${asset}" -C "$TMP_DIR"
    [ -x "${TMP_DIR}/pkgfort" ] || return 1
    BIN="${TMP_DIR}/pkgfort"
}

# Builds and installs via `go install`, setting BIN to the resulting binary
# path. Requires a local Go toolchain.
fetch_via_go() {
    command -v go >/dev/null 2>&1 || return 1

    info "No prebuilt binary for ${goos:-unknown}/${goarch:-unknown}; installing ${MODULE}@${VERSION} with go install..."
    if ! go install "${MODULE}@${VERSION}"; then
        return 1
    fi

    gobin="$(go env GOBIN)"
    [ -n "$gobin" ] || gobin="$(go env GOPATH)/bin"
    [ -x "${gobin}/pkgfort" ] || return 1
    BIN="${gobin}/pkgfort"
}

BIN=""
fetch_prebuilt || fetch_via_go || fail "no prebuilt binary for this platform and go is not installed. Install Go from https://go.dev/dl/ and re-run this script, or build from source (see README)."

info "Running pkgfort install..."
"$BIN" install

APP_DIR="${HOME}/.pkgfort"
INIT_LINE="source ${APP_DIR}/init.sh"

add_to_rc() {
    rc="$1"
    [ -f "$rc" ] || return 1
    if grep -qF "$INIT_LINE" "$rc" 2>/dev/null; then
        info "Already sourced in ${rc}"
        return 0
    fi
    printf '\n# pkgfort\n%s\n' "$INIT_LINE" >> "$rc"
    info "Added init line to ${rc}"
}

rc_file=""
case "${SHELL:-}" in
    */zsh)  rc_file="${ZDOTDIR:-$HOME}/.zshrc" ;;
    */bash) rc_file="${HOME}/.bashrc" ;;
esac

if [ -n "$rc_file" ] && add_to_rc "$rc_file"; then
    info "Installed. Restart your shell or run: ${INIT_LINE}"
else
    info "Installed. Add this line to your shell rc file, then restart your shell:"
    printf '\n  %s\n\n' "$INIT_LINE"
fi
