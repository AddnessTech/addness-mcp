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

CHECKSUMS_FILE="checksums.txt"

# Try gh first, fall back to curl
if command -v gh &> /dev/null; then
  gh release download --repo "$REPO" --pattern "$ASSET" --dir "$INSTALL_DIR" --clobber
  gh release download --repo "$REPO" --pattern "$CHECKSUMS_FILE" --dir "$INSTALL_DIR" --clobber 2>/dev/null || true
else
  DOWNLOAD_URL="https://github.com/${REPO}/releases/latest/download"
  curl -sfL -o "${INSTALL_DIR}/${ASSET}" "${DOWNLOAD_URL}/${ASSET}"
  curl -sfL -o "${INSTALL_DIR}/${CHECKSUMS_FILE}" "${DOWNLOAD_URL}/${CHECKSUMS_FILE}" || true
fi

# Verify checksum
echo "Verifying checksum..."
if [ ! -f "${INSTALL_DIR}/${CHECKSUMS_FILE}" ]; then
  echo "Warning: ${CHECKSUMS_FILE} not found, skipping verification"
  EXPECTED=""
else
  EXPECTED=$(grep "${ASSET}$" "${INSTALL_DIR}/${CHECKSUMS_FILE}" | awk '{print $1}')
fi
if [ -z "$EXPECTED" ]; then
  echo "Warning: no checksum found for ${ASSET} in ${CHECKSUMS_FILE}, skipping verification"
else
  if command -v sha256sum &> /dev/null; then
    ACTUAL=$(sha256sum "${INSTALL_DIR}/${ASSET}" | awk '{print $1}')
  else
    ACTUAL=$(shasum -a 256 "${INSTALL_DIR}/${ASSET}" | awk '{print $1}')
  fi
  if [ "$EXPECTED" != "$ACTUAL" ]; then
    echo "Checksum verification failed!"
    echo "  Expected: ${EXPECTED}"
    echo "  Actual:   ${ACTUAL}"
    rm -f "${INSTALL_DIR}/${ASSET}" "${INSTALL_DIR}/${CHECKSUMS_FILE}"
    exit 1
  fi
  echo "Checksum OK"
fi
rm -f "${INSTALL_DIR}/${CHECKSUMS_FILE}"

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
