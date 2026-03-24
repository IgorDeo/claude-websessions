#!/bin/sh
# Install script for websessions
# Usage: curl -LsSf https://raw.githubusercontent.com/IgorDeo/claude-websessions/main/install.sh | sh
set -eu

REPO="IgorDeo/claude-websessions"
BINARY="websessions"
INSTALL_DIR="${WEBSESSIONS_INSTALL_DIR:-}"

say() {
    printf '%s\n' "$@"
}

err() {
    say "error: $*" >&2
    exit 1
}

detect_os() {
    case "$(uname -s)" in
        Linux*)  echo "linux" ;;
        Darwin*) echo "darwin" ;;
        *)       err "unsupported OS: $(uname -s)" ;;
    esac
}

detect_arch() {
    case "$(uname -m)" in
        x86_64|amd64)  echo "amd64" ;;
        aarch64|arm64) echo "arm64" ;;
        *)             err "unsupported architecture: $(uname -m)" ;;
    esac
}

get_latest_version() {
    if command -v curl > /dev/null 2>&1; then
        curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
            | grep '"tag_name"' | head -1 | sed 's/.*"tag_name": *"//;s/".*//'
    elif command -v wget > /dev/null 2>&1; then
        wget -qO- "https://api.github.com/repos/${REPO}/releases/latest" \
            | grep '"tag_name"' | head -1 | sed 's/.*"tag_name": *"//;s/".*//'
    else
        err "need 'curl' or 'wget'"
    fi
}

download() {
    url="$1"
    dest="$2"
    if command -v curl > /dev/null 2>&1; then
        curl -fsSL -o "$dest" "$url"
    elif command -v wget > /dev/null 2>&1; then
        wget -qO "$dest" "$url"
    fi
}

main() {
    say "websessions installer"
    say ""

    os="$(detect_os)"
    arch="$(detect_arch)"

    say "Detected platform: ${os}/${arch}"

    # Get version (respect WEBSESSIONS_VERSION env var)
    version="${WEBSESSIONS_VERSION:-}"
    if [ -z "$version" ]; then
        say "Fetching latest release..."
        version="$(get_latest_version)"
    fi

    if [ -z "$version" ]; then
        err "could not determine latest version. Set WEBSESSIONS_VERSION or check https://github.com/${REPO}/releases"
    fi

    say "Installing websessions ${version}"

    # Asset name matches release workflow: websessions-<os>-<arch>
    asset="${BINARY}-${os}-${arch}"
    url="https://github.com/${REPO}/releases/download/${version}/${asset}"

    # Create temp directory
    tmp="$(mktemp -d)"
    trap 'rm -rf "$tmp"' EXIT

    say "Downloading ${url}..."
    download "$url" "${tmp}/${BINARY}"

    chmod +x "${tmp}/${BINARY}"

    # Determine install directory
    if [ -n "$INSTALL_DIR" ]; then
        install_to="$INSTALL_DIR"
    elif [ -w "/usr/local/bin" ]; then
        install_to="/usr/local/bin"
    else
        install_to="${HOME}/.local/bin"
    fi

    mkdir -p "$install_to"
    mv "${tmp}/${BINARY}" "${install_to}/${BINARY}"

    say ""
    say "Installed websessions to ${install_to}/${BINARY}"

    # Check if install dir is in PATH
    case ":${PATH}:" in
        *":${install_to}:"*) ;;
        *)
            say ""
            say "WARNING: ${install_to} is not in your PATH."
            say "Add it with:"
            say ""
            say "  export PATH=\"${install_to}:\$PATH\""
            say ""
            say "Or add that line to your ~/.bashrc or ~/.zshrc"
            ;;
    esac

    say ""
    say "Run 'websessions' to start the server."
}

main
