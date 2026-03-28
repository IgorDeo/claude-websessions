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

    # Install the ws-open-url helper script for plannotator integration
    ws_open_url="${install_to}/ws-open-url"
    cat > "$ws_open_url" << 'SCRIPT'
#!/bin/sh
# ws-open-url: Opens a URL in a websessions iframe pane.
# Used as PLANNOTATOR_BROWSER to embed plannotator plans in websessions.
URL="$1"
WS_HOST="${WEBSESSIONS_HOST:-localhost:8080}"
if [ -z "$URL" ]; then
  echo "Usage: ws-open-url <url>" >&2
  exit 1
fi
curl -s -X POST "http://${WS_HOST}/api/panes/iframe" \
  -H "Content-Type: application/json" \
  -d "{\"url\":\"$URL\",\"title\":\"Plan Review\"}" \
  -o /dev/null -w "" 2>/dev/null || true
SCRIPT
    chmod +x "$ws_open_url"

    say ""
    say "Installed websessions to ${install_to}/${BINARY}"

    # Install .desktop entry and icon on Linux
    if [ "$os" = "linux" ]; then
        icon_dir="${HOME}/.local/share/icons/hicolor/scalable/apps"
        desktop_dir="${HOME}/.local/share/applications"
        mkdir -p "$icon_dir" "$desktop_dir"

        # Extract the SVG favicon from the binary or download it
        icon_path="${icon_dir}/websessions.svg"
        icon_url="https://raw.githubusercontent.com/${REPO}/${version}/web/static/favicon.svg"
        if download "$icon_url" "$icon_path" 2>/dev/null; then
            :
        else
            # Fallback: create a simple placeholder icon
            cat > "$icon_path" << 'SVGICON'
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 64 64">
  <rect x="4" y="8" width="56" height="48" rx="6" fill="#1a1c27" stroke="#6c8cff" stroke-width="2.5"/>
  <rect x="4" y="8" width="56" height="12" rx="6" fill="#272937"/>
  <rect x="4" y="14" width="56" height="6" fill="#272937"/>
  <circle cx="14" cy="14" r="2.5" fill="#e87070"/>
  <circle cx="22" cy="14" r="2.5" fill="#d4a843"/>
  <circle cx="30" cy="14" r="2.5" fill="#7ec87e"/>
  <text x="12" y="32" font-family="monospace" font-size="10" font-weight="bold" fill="#6c8cff">$</text>
  <rect x="22" y="27" width="28" height="3" rx="1.5" fill="#8d93b0" opacity="0.6"/>
  <text x="12" y="44" font-family="monospace" font-size="10" font-weight="bold" fill="#7ec87e">></text>
  <rect x="22" y="39" width="20" height="3" rx="1.5" fill="#8d93b0" opacity="0.4"/>
  <rect x="44" y="38" width="2" height="8" rx="1" fill="#6c8cff" opacity="0.9"/>
  <circle cx="48" cy="50" r="2" fill="#7ec87e"/>
  <circle cx="54" cy="50" r="2" fill="#6c8cff"/>
</svg>
SVGICON
        fi

        # Create .desktop entry
        cat > "${desktop_dir}/websessions.desktop" << DESKTOP
[Desktop Entry]
Name=websessions
Comment=Web-based command center for Claude Code sessions
Exec=${install_to}/${BINARY}
Icon=websessions
Terminal=false
Type=Application
Categories=Development;Utility;
StartupWMClass=websessions
DESKTOP
        # Update desktop database if available
        if command -v update-desktop-database > /dev/null 2>&1; then
            update-desktop-database "$desktop_dir" 2>/dev/null || true
        fi
        say "Desktop entry installed — websessions should appear in your app launcher."
    fi

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
