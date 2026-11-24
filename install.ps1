# Installs ctlctl from GitHub Releases on Windows.
#
#   irm https://raw.githubusercontent.com/rafalmaciejewski/ctlctl/main/install.ps1 | iex
#
# Environment variables:
#   CTLCTL_VERSION      version to install, such as 1.2.0 (default: the latest)
#   CTLCTL_INSTALL_DIR  directory for the binary (default: %LOCALAPPDATA%\Programs\ctlctl)
#   CTLCTL_BASE_URL     download from a mirror of the release assets instead
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'

$repo = 'rafalmaciejewski/ctlctl'
$arch = switch ($env:PROCESSOR_ARCHITECTURE) {
    'AMD64' { 'amd64' }
    'ARM64' { 'arm64' }
    default { throw "ctlctl install: unsupported architecture $env:PROCESSOR_ARCHITECTURE" }
}
$base = if ($env:CTLCTL_VERSION) {
    "https://github.com/$repo/releases/download/v$($env:CTLCTL_VERSION.TrimStart('v'))"
} else {
    "https://github.com/$repo/releases/latest/download"
}
if ($env:CTLCTL_BASE_URL) { $base = $env:CTLCTL_BASE_URL }
$asset = "ctlctl_windows_$arch.zip"
$dir = if ($env:CTLCTL_INSTALL_DIR) { $env:CTLCTL_INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA 'Programs\ctlctl' }

$tmp = Join-Path ([IO.Path]::GetTempPath()) ([Guid]::NewGuid())
New-Item -ItemType Directory -Path $tmp | Out-Null
try {
    Write-Host "Downloading $asset..."
    Invoke-WebRequest "$base/$asset" -OutFile (Join-Path $tmp $asset) -UseBasicParsing
    Invoke-WebRequest "$base/checksums.txt" -OutFile (Join-Path $tmp 'checksums.txt') -UseBasicParsing

    $line = Get-Content (Join-Path $tmp 'checksums.txt') | Where-Object { $_ -match " $([regex]::Escape($asset))$" }
    if (-not $line) { throw "ctlctl install: $asset is not in checksums.txt" }
    $expected = ($line -split '\s+')[0]
    $actual = (Get-FileHash (Join-Path $tmp $asset) -Algorithm SHA256).Hash.ToLower()
    if ($expected -ne $actual) { throw "ctlctl install: checksum mismatch for $asset" }

    Expand-Archive (Join-Path $tmp $asset) -DestinationPath $tmp -Force
    New-Item -ItemType Directory -Force -Path $dir | Out-Null
    Copy-Item (Join-Path $tmp 'ctlctl.exe') (Join-Path $dir 'ctlctl.exe') -Force
    Write-Host "Installed $(& (Join-Path $dir 'ctlctl.exe') --version) to $dir"

    $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
    if (($userPath -split ';') -notcontains $dir) {
        [Environment]::SetEnvironmentVariable('Path', "$userPath;$dir", 'User')
        Write-Host "Added $dir to your PATH. Open a new terminal to use ctlctl."
    }
} finally {
    Remove-Item -Recurse -Force $tmp
}
