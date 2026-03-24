#!/bin/sh
# Install script for websessions
# Usage:
#   curl -LsSf https://raw.githubusercontent.com/IgorDeo/claude-websessions/main/install.sh | sh
#   curl -LsSf https://raw.githubusercontent.com/IgorDeo/claude-websessions/main/install.sh | sh -s -- --gui
set -eu

REPO="IgorDeo/claude-websessions"
BINARY="websessions"
INSTALL_DIR="${WEBSESSIONS_INSTALL_DIR:-}"
GUI_MODE=false

say() {
    printf '%s\n' "$@"
}

warn() {
    printf 'WARNING: %s\n' "$@" >&2
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

detect_distro() {
    if [ -f /etc/os-release ]; then
        . /etc/os-release
        echo "$ID"
    else
        echo "unknown"
    fi
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

# check_gui_deps verifies that the runtime libraries needed for GUI mode
# are present on the system. Prints install instructions if missing.
check_gui_deps() {
    os="$1"

    if [ "$os" = "darwin" ]; then
        # macOS ships WebKit — nothing to install
        return 0
    fi

    # Linux: check for libwebkit2gtk-4.1
    missing=false

    if command -v pkg-config > /dev/null 2>&1; then
        if ! pkg-config --exists webkit2gtk-4.1 2>/dev/null; then
            missing=true
        fi
    elif command -v ldconfig > /dev/null 2>&1; then
        if ! ldconfig -p 2>/dev/null | grep -q libwebkit2gtk-4.1; then
            missing=true
        fi
    else
        # Can't detect — check for the .so directly
        if ! find /usr/lib /usr/lib64 /usr/lib/x86_64-linux-gnu /usr/lib/aarch64-linux-gnu \
             -name 'libwebkit2gtk-4.1.so*' 2>/dev/null | grep -q .; then
            missing=true
        fi
    fi

    if [ "$missing" = true ]; then
        say ""
        say "GUI mode requires WebKit2GTK 4.1 runtime libraries."
        say ""

        distro="$(detect_distro)"
        case "$distro" in
            ubuntu|debian|pop|linuxmint|elementary)
                say "Install with:"
                say ""
                say "  sudo apt install libwebkit2gtk-4.1-0 libgtk-3-0"
                say ""
                say "For building from source, also install dev headers:"
                say "  sudo apt install libwebkit2gtk-4.1-dev libgtk-3-dev"
                ;;
            fedora|rhel|centos|rocky|alma)
                say "Install with:"
                say ""
                say "  sudo dnf install webkit2gtk4.1 gtk3"
                ;;
            arch|manjaro|endeavouros)
                say "Install with:"
                say ""
                say "  sudo pacman -S webkit2gtk-4.1 gtk3"
                ;;
            opensuse*|suse*)
                say "Install with:"
                say ""
                say "  sudo zypper install libwebkit2gtk-4_1-0 gtk3"
                ;;
            *)
                say "Install the webkit2gtk-4.1 and gtk3 packages for your distribution."
                ;;
        esac

        say ""
        say "The binary has been installed but --gui will not work until the"
        say "libraries above are installed."
        return 1
    fi

    return 0
}

main() {
    # Parse arguments
    for arg in "$@"; do
        case "$arg" in
            --gui) GUI_MODE=true ;;
        esac
    done

    if [ "$GUI_MODE" = true ]; then
        say "websessions installer (GUI mode)"
    else
        say "websessions installer"
    fi
    say ""

    os="$(detect_os)"
    arch="$(detect_arch)"

    say "Detected platform: ${os}/${arch}"

    # GUI builds are only available for Linux
    if [ "$GUI_MODE" = true ] && [ "$os" != "linux" ]; then
        say ""
        say "Note: on macOS, the standard binary supports --gui natively (WebKit is built-in)."
        say "Installing the standard binary instead."
        say ""
        GUI_MODE=false
    fi

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

    # Asset name matches release workflow
    if [ "$GUI_MODE" = true ]; then
        asset="${BINARY}-gui-${os}-${arch}"
    else
        asset="${BINARY}-${os}-${arch}"
    fi
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

    # Check GUI dependencies if GUI mode was requested
    if [ "$GUI_MODE" = true ]; then
        check_gui_deps "$os" || true
    fi

    say ""
    if [ "$GUI_MODE" = true ]; then
        say "Run 'websessions --gui' to start with the native GUI."
    else
        say "Run 'websessions' to start the server."
        say "For GUI mode, reinstall with: curl -LsSf https://raw.githubusercontent.com/IgorDeo/claude-websessions/main/install.sh | sh -s -- --gui"
    fi
}

main "$@"
