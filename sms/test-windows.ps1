<#
.SYNOPSIS
  Runs the tests that belong to the SMS multi-instance changes, and vets what they touch.

.DESCRIPTION
  Selects the test files this branch ADDED since the upstream release it is based on, runs
  exactly the tests in them (not the whole upstream suite, whose privileged and network
  tests do not belong on an arbitrary machine) and runs "go vet" on every package with a
  changed Go file. The tests only use temporary folders and memory; they never touch the
  registry, services or adapters of the machine.

  Needs go and git on PATH. Run from anywhere inside the repository.

.EXAMPLE
  .\sms\test-windows.ps1
  .\sms\test-windows.ps1 -Base v0.80.0
#>
[CmdletBinding()]
param(
    # Upstream release the branch is based on. Default: nearest plain vX.Y.Z tag.
    [string]$Base
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$repo = (git rev-parse --show-toplevel).Trim()
if ($LASTEXITCODE -ne 0) { throw 'not inside a git repository' }
Set-Location $repo

if (-not $Base) {
    $tag = (git describe --tags --abbrev=0 --match 'v[0-9]*.[0-9]*.[0-9]*' --exclude '*-*' 2>$null)
    if (-not $tag) { throw 'no vX.Y.Z tag found; fetch the upstream tags or pass -Base' }
    $Base = $tag.Trim()
}
git rev-parse --verify --quiet "$Base^{commit}" | Out-Null
if ($LASTEXITCODE -ne 0) { throw "base '$Base' is not known here" }

$env:GOOS = 'windows'
$env:CGO_ENABLED = '0'

function ConvertTo-GoPath([string]$dir) { './' + ($dir -replace '\\', '/') }

# The UI embeds its frontend build. The tests do not need the real one, only a file to embed.
$distIndex = Join-Path $repo 'client/ui/frontend/dist/index.html'
if (-not (Test-Path $distIndex)) {
    New-Item -ItemType Directory -Force (Split-Path $distIndex) | Out-Null
    Set-Content $distIndex '<!doctype html><title>stub</title>'
}

# 1. vet every package with a changed Go file
$changed = @(git diff --name-only --diff-filter=AM "$Base..HEAD" -- '*.go')
$vetDirs = $changed | ForEach-Object { [IO.Path]::GetDirectoryName($_) } | Sort-Object -Unique
foreach ($dir in $vetDirs) {
    if (-not $dir) { continue }
    Write-Host "vet  $dir"
    go vet (ConvertTo-GoPath $dir)
    if ($LASTEXITCODE -ne 0) { throw "go vet failed in $dir" }
}

# 2. run the tests of the added test files, package by package
$added = @(git diff --name-only --diff-filter=A "$Base..HEAD" -- '*_test.go')
if ($added.Count -eq 0) { throw "no test files were added since $Base; something is wrong" }

$total = 0
foreach ($group in ($added | Group-Object { [IO.Path]::GetDirectoryName($_) })) {
    $names = @()
    foreach ($file in $group.Group) {
        $names += Select-String -Path $file -Pattern '^func (Test\w+)\(' | ForEach-Object { $_.Matches[0].Groups[1].Value }
    }
    $names = @($names | Sort-Object -Unique)
    if ($names.Count -eq 0) { continue }

    $total += $names.Count
    Write-Host ("test {0}  ({1} tests)" -f $group.Name, $names.Count)
    go test (ConvertTo-GoPath $group.Name) -run ('^(' + ($names -join '|') + ')$') -count=1
    if ($LASTEXITCODE -ne 0) { throw "tests failed in $($group.Name)" }
}

# 3. the helper functions of the update script
Write-Host 'test sms\update-windows.ps1'
& (Join-Path $PSScriptRoot 'update-windows.tests.ps1')
if ($LASTEXITCODE -ne 0) { throw 'the tests of update-windows.ps1 failed' }

Write-Host "All good: $total tests in $(@($added).Count) files, $(@($vetDirs).Count) packages vetted."
