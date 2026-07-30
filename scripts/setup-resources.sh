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
CONFIG_DIR="$RESOURCE_DIR/config"

# 1. Create target directory structure
mkdir -p "$BIN_DIR"
mkdir -p "$NODE_DIR"
mkdir -p "$TOOLS_DIR"
mkdir -p "$CONFIG_DIR"

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

if [ "${FORCE_NPM_INSTALL:-}" = "1" ]; then
    echo "=== FORCE_NPM_INSTALL=1: Pre-installing Claude Code and OpenCode in bundle ==="
    npm install -g --prefix "$TOOLS_DIR" @anthropic-ai/claude-code opencode-ai || true
else
    echo "=== Skipping npm install -g for bundled tools in dev mode (use system PATH) ==="
    mkdir -p "$TOOLS_DIR/bin"
fi

# 4. Copy compiled Go binaries
echo "=== Copying compiled Go/C binaries ==="
EXE_SUFFIX=""
if [ -f "build/1agents.exe" ] || [[ "$OSTYPE" == "msys" ]] || [[ "$OSTYPE" == "cygwin" ]]; then
    EXE_SUFFIX=".exe"
fi

if [ ! -f "build/1agents$EXE_SUFFIX" ] || [ ! -f "build/ttyd$EXE_SUFFIX" ] || [ ! -f "build/cc-connect$EXE_SUFFIX" ] || [ ! -f "build/cc-switch$EXE_SUFFIX" ] || [ ! -f "build/hk$EXE_SUFFIX" ]; then
    echo "WARNING: Precompiled binaries not found in build/. Running build first..."
    make all
fi

cp "build/1agents$EXE_SUFFIX" "$BIN_DIR/1agents$EXE_SUFFIX"
cp "build/ttyd$EXE_SUFFIX" "$BIN_DIR/ttyd$EXE_SUFFIX"
cp "build/cc-connect$EXE_SUFFIX" "$BIN_DIR/cc-connect$EXE_SUFFIX"
cp "build/cc-switch$EXE_SUFFIX" "$BIN_DIR/cc-switch$EXE_SUFFIX"
cp "build/hk$EXE_SUFFIX" "$BIN_DIR/hk$EXE_SUFFIX"
cp "config/agent-extension-map.json" "$CONFIG_DIR/agent-extension-map.json"
./scripts/stage-harnesskit-compliance.sh "$RESOURCE_DIR/licenses"

chmod +x "$BIN_DIR/1agents$EXE_SUFFIX" "$BIN_DIR/ttyd$EXE_SUFFIX" "$BIN_DIR/cc-connect$EXE_SUFFIX" "$BIN_DIR/cc-switch$EXE_SUFFIX" "$BIN_DIR/hk$EXE_SUFFIX"

# Copy the happy bundle (relay/C2 sidecar + RPC adapter) if built.
# build-happy-bundle.sh produces build/happy-cli, build/adapter, build/happy.
# resolveHappy() in the daemon finds <binDir>/happy-cli + <binDir>/adapter and
# runs it through the bundled node at runtime/node/bin/node.
if [ -d "build/happy-cli" ]; then
    echo "=== Copying happy bundle ==="
    rm -rf "$BIN_DIR/happy-cli" "$BIN_DIR/adapter"
    cp -r "build/happy-cli" "$BIN_DIR/happy-cli"
    cp -r "build/adapter" "$BIN_DIR/adapter"
    if [ -f "build/happy" ]; then cp "build/happy" "$BIN_DIR/happy"; chmod +x "$BIN_DIR/happy"; fi
else
    echo "WARNING: build/happy-cli not found. The happy daemon will be unavailable in this bundle."
fi

if [ -d "build/alipay-coffee" ]; then
    echo "=== Copying Alipay coffee payment service ==="
    rm -rf "$RESOURCE_DIR/alipay-coffee"
    cp -r "build/alipay-coffee" "$RESOURCE_DIR/alipay-coffee"
else
    echo "WARNING: build/alipay-coffee not found. The coffee payment module will be unavailable."
fi

# 4.1. Ad-hoc sign binaries on macOS to satisfy Gatekeeper
if [ "$(uname)" = "Darwin" ]; then
    echo "=== Ad-hoc signing binaries for macOS ==="
    codesign --force --deep --sign - "$BIN_DIR/1agents"
    codesign --force --deep --sign - "$BIN_DIR/ttyd"
    codesign --force --deep --sign - "$BIN_DIR/cc-connect"
    codesign --force --deep --sign - "$BIN_DIR/cc-switch"
    codesign --force --deep --sign - "$BIN_DIR/hk"
    codesign --force --deep --sign - "$NODE_DIR/node"
fi

# 5. Copy frontend assets
echo "=== Copying frontend static assets ==="
if [ ! -d "frontend/dist" ]; then
    echo "WARNING: frontend/dist not found. Running frontend build first..."
    make frontend
fi

rm -rf "$RESOURCE_DIR/dist"
cp -r frontend/dist "$RESOURCE_DIR/dist"

./scripts/check-harnesskit-artifacts.sh "$RESOURCE_DIR"

echo "=== Resources setup completed successfully ==="
