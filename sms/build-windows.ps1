<#
.SYNOPSIS
  Builds the SMS multi-instance NetBird client for Windows with a real version number.

.DESCRIPTION
  Stamps the version the way upstream's release does (linker flag), as
  <upstream version>+SMS. The part after "+" is semver build metadata: every version
  comparison, on the client and on the management server, ignores it, so the build
  counts as exactly that upstream release. A "-SMS" suffix would NOT be safe: it is a
  pre-release, which sorts below the release and fails ">= x.y.z" constraints.

  Needs on PATH: go, git; for the UI also pnpm and wails3 (same versions as upstream's
  release workflow). Output goes to dist/sms by default (dist/ is git-ignored).

.EXAMPLE
  .\sms\build-windows.ps1
  .\sms\build-windows.ps1 -Version 0.81.0 -SkipUI
#>
[CmdletBinding()]
param(
    # Upstream version to stamp. Default: the nearest plain vX.Y.Z tag (no canary, no suffix).
    [string]$Version,
    [string]$Suffix = 'SMS',
    [string]$OutDir,
    [switch]$SkipUI
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

function Invoke-Native {
    param([string]$What, [scriptblock]$Command)
    & $Command
    if ($LASTEXITCODE -ne 0) { throw "$What failed (exit code $LASTEXITCODE)" }
}

$repo = (git rev-parse --show-toplevel).Trim()
if ($LASTEXITCODE -ne 0) { throw 'not inside a git repository' }
Set-Location $repo

if (-not $Version) {
    $tag = (git describe --tags --abbrev=0 --match 'v[0-9]*.[0-9]*.[0-9]*' --exclude '*-*' 2>$null)
    if (-not $tag) { throw 'no vX.Y.Z tag found; pass -Version' }
    $Version = $tag.Trim().TrimStart('v')
}
if ($Version -notmatch '^\d+\.\d+\.\d+$') {
    throw "version '$Version' must look like 1.2.3 (three numbers, no suffix); the suffix is added for you"
}
if ($Suffix -notmatch '^[0-9A-Za-z-]+$') { throw "suffix '$Suffix' may only contain letters, digits and '-'" }

$stamped = "$Version+$Suffix"
$flagX = "-X github.com/netbirdio/netbird/version.version=$stamped"
if (-not $OutDir) { $OutDir = Join-Path $repo 'dist/sms' }
New-Item -ItemType Directory -Force $OutDir | Out-Null
$OutDir = (Resolve-Path $OutDir).Path

Write-Host "Building NetBird $stamped into $OutDir"

$env:GOOS = 'windows'
$env:GOARCH = 'amd64'
$env:CGO_ENABLED = '0'

Invoke-Native 'daemon build' { go build -trimpath -ldflags "-s -w $flagX" -o (Join-Path $OutDir 'netbird.exe') ./client }

if (-not $SkipUI) {
    $ui = Join-Path $repo 'client/ui'

    Push-Location $ui
    try {
        # Bindings are git-ignored and the frontend build refuses to run without them.
        Invoke-Native 'wails3 bindings' { wails3 generate bindings -clean=true -ts }

        Push-Location (Join-Path $ui 'frontend')
        try {
            $env:CI = 'true'
            Invoke-Native 'pnpm install' { pnpm install --frozen-lockfile }
            Invoke-Native 'frontend build' { pnpm build }
        } finally { Pop-Location }

        # Icon and manifest resources; removed again below so the tree stays clean.
        Push-Location (Join-Path $ui 'build')
        try {
            Invoke-Native 'resource file' {
                wails3 generate syso -arch amd64 -icon windows/icon.ico -manifest windows/wails.exe.manifest `
                    -info windows/info.json -out ../wails_windows_amd64.syso
            }
        } finally { Pop-Location }

        try {
            Invoke-Native 'UI build' {
                go build -tags production -trimpath -ldflags "-w -s -H windowsgui $flagX" -o (Join-Path $OutDir 'netbird-ui.exe') .
            }
            # Same program with a console, so its log output can be read when the tray misbehaves.
            Invoke-Native 'console UI build' {
                go build -tags production -trimpath -ldflags "-w -s $flagX" -o (Join-Path $OutDir 'netbird-ui-console.exe') .
            }
        } finally {
            Remove-Item (Join-Path $ui '*.syso') -ErrorAction SilentlyContinue
        }
    } finally { Pop-Location }
}

# The stamped version must really be inside the files; a build without the flag says "development".
foreach ($name in 'netbird.exe', 'netbird-ui.exe', 'netbird-ui-console.exe') {
    $path = Join-Path $OutDir $name
    if (-not (Test-Path $path)) { continue }
    $text = [Text.Encoding]::ASCII.GetString([IO.File]::ReadAllBytes($path))
    if (-not $text.Contains($stamped)) { throw "$name does not contain the version $stamped" }
}

$sums = Get-ChildItem $OutDir -Filter *.exe | ForEach-Object {
    '{0}  {1}' -f (Get-FileHash $_.FullName -Algorithm SHA256).Hash.ToLower(), $_.Name
}
$sums | Set-Content (Join-Path $OutDir 'SHA256SUMS') -Encoding ascii

Write-Host "Done. Version $stamped"
$sums | ForEach-Object { Write-Host "  $_" }
