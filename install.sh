#!/bin/sh
# Install script for websessions
# Usage: curl -LsSf https://raw.githubusercontent.com/IgorDeo/claude-websessions/main/install.sh | sh
#
# Auto-detects whether GUI libraries are available and installs the
# GUI-enabled binary if so, otherwise installs the standard binary.
#
# Options:
#   --no-gui    Force standard (non-GUI) binary even if GUI deps are available
set -eu

REPO="IgorDeo/claude-websessions"
BINARY="websessions"
INSTALL_DIR="${WEBSESSIONS_INSTALL_DIR:-}"
FORCE_NO_GUI=false

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

# has_gui_deps returns 0 if the system has the libraries needed for GUI mode.
has_gui_deps() {
    os="$1"

    # macOS ships WebKit — always available
    if [ "$os" = "darwin" ]; then
        return 0
    fi

    # Linux: check for libwebkit2gtk-4.1
    if command -v pkg-config > /dev/null 2>&1; then
        pkg-config --exists webkit2gtk-4.1 2>/dev/null && return 0
    fi

    if command -v ldconfig > /dev/null 2>&1; then
        ldconfig -p 2>/dev/null | grep -q libwebkit2gtk-4.1 && return 0
    fi

    # Check for the .so directly
    for dir in /usr/lib /usr/lib64 /usr/lib/x86_64-linux-gnu /usr/lib/aarch64-linux-gnu; do
        if [ -d "$dir" ] && find "$dir" -name 'libwebkit2gtk-4.1.so*' 2>/dev/null | grep -q .; then
            return 0
        fi
    done

    return 1
}

# print_gui_install_hint shows distro-specific instructions for installing GUI deps.
print_gui_install_hint() {
    say ""
    say "To enable GUI mode (native window, no browser needed), install WebKit2GTK:"
    say ""

    distro="$(detect_distro)"
    case "$distro" in
        ubuntu|debian|pop|linuxmint|elementary)
            say "  sudo apt install libwebkit2gtk-4.1-0 libgtk-3-0"
            ;;
        fedora|rhel|centos|rocky|alma)
            say "  sudo dnf install webkit2gtk4.1 gtk3"
            ;;
        arch|manjaro|endeavouros)
            say "  sudo pacman -S webkit2gtk-4.1 gtk3"
            ;;
        opensuse*|suse*)
            say "  sudo zypper install libwebkit2gtk-4_1-0 gtk3"
            ;;
        *)
            say "  Install webkit2gtk-4.1 and gtk3 for your distribution."
            ;;
    esac

    say ""
    say "Then re-run this installer to get the GUI-enabled binary."
}

main() {
    # Parse arguments
    for arg in "$@"; do
        case "$arg" in
            --no-gui) FORCE_NO_GUI=true ;;
        esac
    done

    say "websessions installer"
    say ""

    os="$(detect_os)"
    arch="$(detect_arch)"

    say "Detected platform: ${os}/${arch}"

    # Decide whether to install the GUI binary
    use_gui=false
    if [ "$FORCE_NO_GUI" = false ]; then
        # GUI release assets are only built for linux/amd64
        # macOS uses the standard binary (WebKit is built-in, no separate GUI build needed)
        if [ "$os" = "linux" ] && [ "$arch" = "amd64" ] && has_gui_deps "$os"; then
            use_gui=true
        fi
    fi

    # Get version
    version="${WEBSESSIONS_VERSION:-}"
    if [ -z "$version" ]; then
        say "Fetching latest release..."
        version="$(get_latest_version)"
    fi

    if [ -z "$version" ]; then
        err "could not determine latest version. Set WEBSESSIONS_VERSION or check https://github.com/${REPO}/releases"
    fi

    # Build asset name
    if [ "$use_gui" = true ]; then
        asset="${BINARY}-gui-${os}-${arch}"
        say "Installing websessions ${version} (GUI-enabled)"
    else
        asset="${BINARY}-${os}-${arch}"
        say "Installing websessions ${version}"
    fi
    url="https://github.com/${REPO}/releases/download/${version}/${asset}"

    # Download
    tmp="$(mktemp -d)"
    trap 'rm -rf "$tmp"' EXIT

    say "Downloading ${url}..."
    download "$url" "${tmp}/${BINARY}"
    chmod +x "${tmp}/${BINARY}"

    # Install
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

    # PATH check
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
    if [ "$use_gui" = true ]; then
        say "Run 'websessions --gui' to start with a native window, or just 'websessions' for browser mode."
    else
        say "Run 'websessions' to start the server."
        # Only show GUI hint on Linux (macOS standard binary already supports --gui)
        if [ "$os" = "linux" ] && [ "$FORCE_NO_GUI" = false ]; then
            print_gui_install_hint
        fi
    fi
}

main "$@"
