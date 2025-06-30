#!/usr/bin/env bash
# Script to install claude-code system-wide on NixOS

set -e

echo "Installing claude-code system-wide for mentat on NixOS..."

# Check if running as root or with sudo
if [ "$EUID" -ne 0 ]; then 
    echo "Please run with sudo: sudo bash ./install-claude-system.sh"
    exit 1
fi

# Create a directory for system-wide npm packages outside of Nix store
SYSTEM_NPM_PREFIX="/usr/local/npm-global"
mkdir -p "$SYSTEM_NPM_PREFIX"

# Configure npm to use this directory for global installs
export NPM_CONFIG_PREFIX="$SYSTEM_NPM_PREFIX"

# Install claude-code globally to our custom location
echo "Installing @anthropic-ai/claude-code to $SYSTEM_NPM_PREFIX..."
npm install -g @anthropic-ai/claude-code

# Find the actual claude binary location
CLAUDE_BIN="$SYSTEM_NPM_PREFIX/bin/claude"

if [ ! -f "$CLAUDE_BIN" ]; then
    echo "Error: Could not find claude binary at $CLAUDE_BIN"
    exit 1
fi

# Create a wrapper script instead of a symlink
echo "Creating wrapper script /usr/local/bin/claude-mentat"
mkdir -p /usr/local/bin
cat > /usr/local/bin/claude-mentat << 'EOF'
#!/usr/bin/env bash
# Wrapper script for claude that sets HOME directory
export HOME=/home/joshsymonds
exec "$CLAUDE_BIN" "$@"
EOF

# Replace CLAUDE_BIN in the wrapper
sed -i "s|\$CLAUDE_BIN|$CLAUDE_BIN|g" /usr/local/bin/claude-mentat

# Make sure it's executable
chmod +x "$CLAUDE_BIN"
chmod +x /usr/local/bin/claude-mentat

# Test that it works
echo "Testing installation..."
if /usr/local/bin/claude-mentat --version > /dev/null 2>&1; then
    echo "✓ claude-mentat installed successfully"
    echo "  Binary: /usr/local/bin/claude-mentat"
    echo "  Target: $CLAUDE_BIN"
    echo "  NPM prefix: $SYSTEM_NPM_PREFIX"
else
    echo "✗ Installation test failed"
    exit 1
fi

echo ""
echo "Installation complete! You can now use claude-mentat system-wide."
echo "The signal-cli user should be able to execute it."