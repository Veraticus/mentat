#!/usr/bin/env bash
# Script to remove system-wide claude-code installation on NixOS

set -e

echo "Removing system-wide claude-code installation on NixOS..."

# Check if running as root or with sudo
if [ "$EUID" -ne 0 ]; then 
    echo "Please run with sudo: sudo bash ./implode-claude-system.sh"
    exit 1
fi

# Remove the symlink
if [ -L /usr/local/bin/claude-mentat ]; then
    echo "Removing symlink /usr/local/bin/claude-mentat"
    rm -f /usr/local/bin/claude-mentat
else
    echo "Symlink /usr/local/bin/claude-mentat not found (already removed?)"
fi

# Remove the npm installation
SYSTEM_NPM_PREFIX="/usr/local/npm-global"
if [ -d "$SYSTEM_NPM_PREFIX" ]; then
    echo "Removing npm global directory at $SYSTEM_NPM_PREFIX"
    rm -rf "$SYSTEM_NPM_PREFIX"
else
    echo "NPM global directory not found (already removed?)"
fi

echo ""
echo "✓ System-wide claude-code installation removed"
echo ""
echo "Note: Your personal installation in ~/.npm-global is unchanged"