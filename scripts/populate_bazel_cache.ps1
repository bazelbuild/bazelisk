#Requires -Version 5.1
<#
.SYNOPSIS
    Pre-populate a bazelisk download cache with Bazel binaries.

.DESCRIPTION
    Downloads Bazel binaries (and their .sha256 files) into a local directory
    laid out as:
        <CacheRoot>/<version>/<filename>

    This matches what bazelisk expects when BAZELISK_BASE_URL is set to point
    at the cache root, e.g.:
        $env:BAZELISK_BASE_URL = "file:///C:/bazel-cache"

.PARAMETER CacheRoot
    Root directory for the bazelisk cache. Required.

.PARAMETER Versions
    One or more Bazel versions to download, e.g. "7.4.1","8.0.0". Required.

.PARAMETER Oses
    One or more target OSes: linux, darwin, windows.
    Defaults to the host OS (windows).

.PARAMETER Archs
    One or more target architectures: x86_64, arm64.
    Defaults to the host architecture.

.PARAMETER NoJdk
    Also download bazel_nojdk variants. Off by default.

.EXAMPLE
    .\populate_bazel_cache.ps1 `
        -CacheRoot C:\bazel-cache `
        -Versions 7.4.1,8.0.0 `
        -Oses linux,windows `
        -Archs x86_64,arm64
#>

[CmdletBinding()]
param(
    [Parameter(Mandatory)][string]   $CacheRoot,
    [Parameter(Mandatory)][string[]] $Versions,
    [string[]] $Oses  = @(),
    [string[]] $Archs = @(),
    [switch]   $NoJdk
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$BazelBaseUrl = "https://github.com/bazelbuild/bazel/releases/download"

# ------------------------------------------------------------------------------
function Get-HostOs {
    if ($IsLinux)   { return "linux" }
    if ($IsMacOS)   { return "darwin" }
    return "windows"
}

function Get-HostArch {
    switch ($env:PROCESSOR_ARCHITECTURE) {
        "AMD64" { return "x86_64" }
        "ARM64" { return "arm64"  }
        default {
            # Fallback for 32-bit host or unusual environments
            throw "Unsupported architecture: $($env:PROCESSOR_ARCHITECTURE)"
        }
    }
}

function Invoke-Download {
    param([string]$Url, [string]$Dest)
    try {
        Invoke-WebRequest -Uri $Url -OutFile $Dest -UseBasicParsing `
            -ErrorAction Stop
    } catch {
        throw "Failed to download ${Url}: $_"
    }
}

function Get-Sha256 {
    param([string]$File)
    return (Get-FileHash -Path $File -Algorithm SHA256).Hash.ToLower()
}

# ------------------------------------------------------------------------------
if ($Oses.Count  -eq 0) { $Oses  = @(Get-HostOs) }
if ($Archs.Count -eq 0) { $Archs = @(Get-HostArch) }

$Flavors = @("bazel")
if ($NoJdk) { $Flavors += "bazel_nojdk" }

$errors = 0

foreach ($version in $Versions) {
    foreach ($os in $Oses) {
        foreach ($arch in $Archs) {
            foreach ($flavor in $Flavors) {

                $suffix   = if ($os -eq "windows") { ".exe" } else { "" }
                $filename = "${flavor}-${version}-${os}-${arch}${suffix}"
                $destDir  = Join-Path $CacheRoot $version
                $binDest  = Join-Path $destDir $filename
                $shaDest  = "${binDest}.sha256"

                if ((Test-Path $binDest) -and (Test-Path $shaDest)) {
                    Write-Host "  [skip] $filename (already cached)"
                    continue
                }

                New-Item -ItemType Directory -Force -Path $destDir | Out-Null

                $binUrl = "${BazelBaseUrl}/${version}/${filename}"
                $shaUrl = "${binUrl}.sha256"

                Write-Host "Downloading $filename..."

                $tmpBin = Join-Path $destDir (".tmp." + [System.IO.Path]::GetRandomFileName())
                $tmpSha = Join-Path $destDir (".tmp." + [System.IO.Path]::GetRandomFileName())

                try {
                    try {
                        Invoke-Download $binUrl $tmpBin
                    } catch {
                        Write-Error "  ERROR: $_"
                        $errors++
                        continue
                    }

                    try {
                        Invoke-Download $shaUrl $tmpSha
                    } catch {
                        Write-Error "  ERROR: $_"
                        $errors++
                        continue
                    }

                    # The .sha256 file contains the hex digest, optionally
                    # followed by a filename. Take only the first token.
                    $expected = (Get-Content $tmpSha -Raw).Trim().Split()[0].ToLower()
                    $actual   = Get-Sha256 $tmpBin

                    if ($expected -ne $actual) {
                        Write-Error ("  ERROR: SHA256 mismatch for ${filename}`n" +
                                     "    expected: $expected`n" +
                                     "    actual:   $actual")
                        $errors++
                        continue
                    }

                    Write-Host "  verified: $actual"

                    Move-Item -Force $tmpBin $binDest
                    Move-Item -Force $tmpSha $shaDest

                } finally {
                    # Clean up temp files if they weren't successfully moved.
                    if (Test-Path $tmpBin) { Remove-Item -Force $tmpBin }
                    if (Test-Path $tmpSha) { Remove-Item -Force $tmpSha }
                }
            }
        }
    }
}

Write-Host ""
if ($errors -gt 0) {
    Write-Error "Completed with $errors error(s). Cache may be incomplete."
    exit 1
}
Write-Host "Done. Cache populated at $CacheRoot"
