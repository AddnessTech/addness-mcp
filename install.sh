#!/bin/bash
set -euo pipefail

# Addness MCP Server installer
# Usage: curl -sL <raw-url> | bash
# Or:    gh api repos/AddnessTech/addness-mcp/contents/install.sh -q .content | base64 -d | bash

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
gh release download --repo "$REPO" --pattern "$ASSET" --dir "$INSTALL_DIR" --clobber
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
