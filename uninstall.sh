#!/bin/sh
# Uninstall script for websessions
# Usage: curl -LsSf https://raw.githubusercontent.com/IgorDeo/claude-websessions/main/uninstall.sh | sh
#
# Removes the websessions binary, all data, and any background service.
# Prompts for confirmation before deleting data.
set -eu

BINARY="websessions"

say() {
    printf '%s\n' "$@"
}

err() {
    say "error: $*" >&2
    exit 1
}

confirm() {
    printf '%s [y/N] ' "$1"
    read -r answer </dev/tty 2>/dev/null || answer="y"
    case "$answer" in
        [yY]|[yY][eE][sS]) return 0 ;;
        *) return 1 ;;
    esac
}

find_binary() {
    # Check common install locations
    for dir in "$HOME/.local/bin" "/usr/local/bin" "/usr/bin"; do
        if [ -f "$dir/$BINARY" ]; then
            echo "$dir/$BINARY"
            return 0
        fi
    done
    # Try PATH
    command -v "$BINARY" 2>/dev/null || true
}

stop_service() {
    # Stop systemd service (Linux)
    if command -v systemctl > /dev/null 2>&1; then
        systemctl --user stop websessions.service 2>/dev/null || true
        systemctl --user disable websessions.service 2>/dev/null || true
    fi

    # Stop launchd service (macOS)
    plist="$HOME/Library/LaunchAgents/com.websessions.plist"
    if [ -f "$plist" ]; then
        launchctl unload "$plist" 2>/dev/null || true
    fi

    # Kill any running websessions process
    pkill -f "websessions" 2>/dev/null || true
}

remove_service() {
    removed=false

    # Systemd unit
    unit="$HOME/.config/systemd/user/websessions.service"
    if [ -f "$unit" ]; then
        rm -f "$unit"
        systemctl --user daemon-reload 2>/dev/null || true
        say "  Removed systemd service"
        removed=true
    fi

    # Launchd plist
    plist="$HOME/Library/LaunchAgents/com.websessions.plist"
    if [ -f "$plist" ]; then
        rm -f "$plist"
        say "  Removed launchd service"
        removed=true
    fi

    if [ "$removed" = false ]; then
        say "  No background service found"
    fi
}

main() {
    say "websessions uninstaller"
    say ""

    # Find binary
    bin_path="$(find_binary)"
    data_dir="$HOME/.websessions"

    # Show what will be removed
    say "This will remove:"
    say ""
    if [ -n "$bin_path" ]; then
        say "  Binary:  $bin_path"
    else
        say "  Binary:  (not found)"
    fi
    if [ -d "$data_dir" ]; then
        say "  Data:    $data_dir/"
        say "           (config, database, logs)"
    else
        say "  Data:    (no data directory found)"
    fi

    # Check for services
    has_service=false
    if [ -f "$HOME/.config/systemd/user/websessions.service" ]; then
        say "  Service: systemd user service"
        has_service=true
    fi
    if [ -f "$HOME/Library/LaunchAgents/com.websessions.plist" ]; then
        say "  Service: launchd agent"
        has_service=true
    fi
    if [ "$has_service" = false ]; then
        say "  Service: (none installed)"
    fi

    say ""

    if ! confirm "Proceed with uninstall?"; then
        say "Aborted."
        exit 0
    fi

    say ""

    # Stop running processes and services
    say "Stopping websessions..."
    stop_service
    sleep 1

    # Remove service files
    say "Removing background service..."
    remove_service

    # Remove binary
    if [ -n "$bin_path" ] && [ -f "$bin_path" ]; then
        rm -f "$bin_path"
        say "  Removed $bin_path"
    fi

    # Remove data
    if [ -d "$data_dir" ]; then
        if confirm "Delete all data in $data_dir? (config, database, logs)"; then
            rm -rf "$data_dir"
            say "  Removed $data_dir"
        else
            say "  Kept $data_dir"
        fi
    fi

    say ""
    say "websessions has been uninstalled."
}

main
