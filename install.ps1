# Addness MCP Server installer for Windows
# Usage: irm https://raw.githubusercontent.com/AddnessTech/addness-mcp/main/install.ps1 | iex

$repo = "AddnessTech/addness-mcp"
$binaryName = "addness-mcp"
$installDir = "$env:LOCALAPPDATA\addness-mcp"

# Detect architecture
$arch = if ([Environment]::Is64BitOperatingSystem) {
    if ($env:PROCESSOR_ARCHITECTURE -eq "ARM64") { "arm64" } else { "amd64" }
} else {
    Write-Error "32-bit systems are not supported."
    exit 1
}

$asset = "$binaryName-windows-$arch.exe"
$downloadUrl = "https://github.com/$repo/releases/latest/download/$asset"
$destPath = Join-Path $installDir "$binaryName.exe"

Write-Host "Downloading $asset..."

# Create install directory
New-Item -ItemType Directory -Force -Path $installDir | Out-Null

# Download
Invoke-WebRequest -Uri $downloadUrl -OutFile $destPath

Write-Host ""
Write-Host "Installed to $destPath"

# Add to PATH if not already
$currentPath = [Environment]::GetEnvironmentVariable("Path", "User")
if ($currentPath -notlike "*$installDir*") {
    [Environment]::SetEnvironmentVariable("Path", "$currentPath;$installDir", "User")
    Write-Host "Added $installDir to PATH. Restart your terminal to use 'addness-mcp'."
} else {
    Write-Host "$installDir is already in PATH."
}

Write-Host ""
Write-Host "Next: run 'addness-mcp login' to authenticate."
