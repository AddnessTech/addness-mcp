#!/bin/bash
set -euo pipefail

# Addness MCP Server installer
# Usage: curl -sL https://raw.githubusercontent.com/AddnessTech/addness-mcp/main/install.sh | bash

REPO="AddnessTech/addness-mcp"
INSTALL_DIR="${HOME}/.local/bin"
BINARY_NAME="addness-mcp"

# Detect platform
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case "$ARCH" in
  x86_64)  ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *) echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

ASSET="${BINARY_NAME}-${OS}-${ARCH}"
echo "Downloading ${ASSET}..."

mkdir -p "$INSTALL_DIR"

# Try gh first, fall back to curl
if command -v gh &> /dev/null; then
  gh release download --repo "$REPO" --pattern "$ASSET" --dir "$INSTALL_DIR" --clobber
else
  DOWNLOAD_URL="https://github.com/${REPO}/releases/latest/download/${ASSET}"
  curl -sL -o "${INSTALL_DIR}/${ASSET}" "$DOWNLOAD_URL"
fi

chmod +x "${INSTALL_DIR}/${ASSET}"
mv "${INSTALL_DIR}/${ASSET}" "${INSTALL_DIR}/${BINARY_NAME}"

echo ""
echo "Installed to ${INSTALL_DIR}/${BINARY_NAME}"

# Check PATH
if ! echo "$PATH" | grep -q "$INSTALL_DIR"; then
  echo ""
  echo "Add to your shell profile:"
  echo "  export PATH=\"${INSTALL_DIR}:\$PATH\""
fi

echo ""
echo "Next: run 'addness-mcp login' to authenticate."
