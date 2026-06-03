#!/bin/bash
set -e

# setup-resources.sh
# Prepares the resources/ directory for Tauri build by copying Go binaries,
# frontend static assets, local Node.js binary, and pre-installing Claude Code.

echo "=== Preparing resources for Tauri build ==="

# Define paths (run from workspace root)
RESOURCE_DIR="src-tauri/resources"
BIN_DIR="$RESOURCE_DIR/bin"
NODE_DIR="$RESOURCE_DIR/runtime/node/bin"
TOOLS_DIR="$RESOURCE_DIR/bundled_tools"

# 1. Create target directory structure
mkdir -p "$BIN_DIR"
mkdir -p "$NODE_DIR"
mkdir -p "$TOOLS_DIR"

# 2. Copy host Node.js binary to resources
echo "=== Locating and copying Node.js runtime ==="
NODE_PATH=$(node -e 'console.log(process.execPath)' 2>/dev/null || which node)
if [ -z "$NODE_PATH" ]; then
    echo "ERROR: Node.js was not found on your host system. Please install Node.js."
    exit 1
fi
echo "Copying Node.js binary from: $NODE_PATH"
if [[ "$NODE_PATH" == *.exe ]]; then
    cp "$NODE_PATH" "$NODE_DIR/node.exe"
    chmod +x "$NODE_DIR/node.exe"
else
    cp "$NODE_PATH" "$NODE_DIR/node"
    chmod +x "$NODE_DIR/node"
fi

# 3. Pre-install Claude Code using npm
echo "=== Pre-installing Claude Code in bundle ==="
npm install -g --prefix "$TOOLS_DIR" @anthropic-ai/claude-code

# 4. Copy compiled Go binaries
echo "=== Copying compiled Go/C binaries ==="
EXE_SUFFIX=""
if [ -f "build/1agents.exe" ] || [[ "$OSTYPE" == "msys" ]] || [[ "$OSTYPE" == "cygwin" ]]; then
    EXE_SUFFIX=".exe"
fi

if [ ! -f "build/1agents$EXE_SUFFIX" ] || [ ! -f "build/ttyd$EXE_SUFFIX" ] || [ ! -f "build/cc-connect$EXE_SUFFIX" ]; then
    echo "WARNING: Precompiled binaries not found in build/. Running build first..."
    make all
fi

cp "build/1agents$EXE_SUFFIX" "$BIN_DIR/1agents$EXE_SUFFIX"
cp "build/ttyd$EXE_SUFFIX" "$BIN_DIR/ttyd$EXE_SUFFIX"
cp "build/cc-connect$EXE_SUFFIX" "$BIN_DIR/cc-connect$EXE_SUFFIX"

chmod +x "$BIN_DIR/1agents$EXE_SUFFIX" "$BIN_DIR/ttyd$EXE_SUFFIX" "$BIN_DIR/cc-connect$EXE_SUFFIX"

# 4.1. Ad-hoc sign binaries on macOS to satisfy Gatekeeper
if [ "$(uname)" = "Darwin" ]; then
    echo "=== Ad-hoc signing binaries for macOS ==="
    codesign --force --deep --sign - "$BIN_DIR/1agents"
    codesign --force --deep --sign - "$BIN_DIR/ttyd"
    codesign --force --deep --sign - "$BIN_DIR/cc-connect"
    codesign --force --deep --sign - "$NODE_DIR/node"
fi

# 5. Copy frontend assets
echo "=== Copying frontend static assets ==="
if [ ! -d "html/dist" ]; then
    echo "WARNING: html/dist not found. Running frontend build first..."
    make frontend
fi

rm -rf "$RESOURCE_DIR/dist"
cp -r html/dist "$RESOURCE_DIR/dist"

echo "=== Resources setup completed successfully ==="
