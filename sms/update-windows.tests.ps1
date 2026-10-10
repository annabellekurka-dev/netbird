<#
.SYNOPSIS
  Tests for the helper functions of update-windows.ps1. They use temporary files only.
#>
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

. (Join-Path $PSScriptRoot 'update-windows.ps1')

$script:failures = 0
$script:checks = 0

function Assert-Equal($actual, $expected, [string]$what) {
    $script:checks++
    if ($actual -ne $expected) {
        $script:failures++
        Write-Host "FAIL  $what`n      expected: $expected`n      actual  : $actual" -ForegroundColor Red
    }
}

function Assert-True($condition, [string]$what) { Assert-Equal ([bool]$condition) $true $what }

$tmp = Join-Path ([IO.Path]::GetTempPath()) ('sms-update-tests-' + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory $tmp | Out-Null

try {
    # --- reading the command line of a service -------------------------------------------------
    $d = ConvertFrom-ServicePath '"C:\Program Files\Netbird\netbird.exe" service run --log-level info --daemon-addr npipe://netbird'
    Assert-Equal $d.Exe 'C:\Program Files\Netbird\netbird.exe' 'default service: exe'
    Assert-Equal $d.Addr 'npipe://netbird' 'default service: address'

    $x = ConvertFrom-ServicePath '"C:\Program Files\Netbird\netbird.exe" -s NetBird-private service run --log-level info --daemon-addr npipe://netbird-private --log-file C:\x\client.log'
    Assert-Equal $x.Addr 'npipe://netbird-private' 'extra service: address'

    $nodef = ConvertFrom-ServicePath 'C:\NB\netbird.exe service run'
    Assert-Equal $nodef.Addr 'npipe://netbird' 'no address given means the default'
    Assert-Equal $nodef.Exe 'C:\NB\netbird.exe' 'unquoted path'

    Assert-Equal (ConvertFrom-ServicePath 'C:\Windows\System32\spoolsv.exe') $null 'a foreign service is ignored'
    Assert-Equal (ConvertFrom-ServicePath '"C:\NB\netbird.exe" service install') $null 'only "service run" is a daemon'
    Assert-Equal (ConvertFrom-ServicePath '') $null 'empty'
    Assert-Equal (ConvertFrom-ServicePath $null) $null 'null'

    # --- the version stamp ---------------------------------------------------------------------
    $stamped = Join-Path $tmp 'stamped.exe'
    $bytes = New-Object byte[] 5000
    (New-Object Random 1).NextBytes($bytes)
    [IO.File]::WriteAllBytes($stamped, $bytes + [Text.Encoding]::ASCII.GetBytes('xx0.80.0+SMSyy') + $bytes)
    Assert-Equal (Get-StampedVersion $stamped) '0.80.0+SMS' 'stamped version is found'

    $official = Join-Path $tmp 'official.exe'
    [IO.File]::WriteAllBytes($official, $bytes + [Text.Encoding]::ASCII.GetBytes('0.80.0') + $bytes)
    Assert-Equal (Get-StampedVersion $official) $null 'a build without the stamp has none'

    $dev = Join-Path $tmp 'dev.exe'
    [IO.File]::WriteAllBytes($dev, [Text.Encoding]::ASCII.GetBytes('development'))
    Assert-Equal (Get-StampedVersion $dev) $null 'a development build has none'

    # --- checksums -----------------------------------------------------------------------------
    $dir = Join-Path $tmp 'files'
    New-Item -ItemType Directory $dir | Out-Null
    Set-Content (Join-Path $dir 'a.exe') 'AAA' -NoNewline
    Set-Content (Join-Path $dir 'b.exe') 'BBB' -NoNewline
    $sums = Get-ChildItem $dir -Filter *.exe | ForEach-Object { '{0}  {1}' -f (Get-FileHash $_.FullName -Algorithm SHA256).Hash.ToLower(), $_.Name }
    $sums | Set-Content (Join-Path $dir 'SHA256SUMS') -Encoding ascii

    Assert-Equal @(Test-Checksums $dir).Count 0 'matching checksums are fine'

    Set-Content (Join-Path $dir 'b.exe') 'tampered' -NoNewline
    $problems = @(Test-Checksums $dir)
    Assert-Equal $problems.Count 1 'a changed file is reported'
    Assert-True ($problems[0] -like 'b.exe*') 'the report names the file'

    Remove-Item (Join-Path $dir 'a.exe')
    Assert-Equal @(Test-Checksums $dir).Count 2 'a missing file is reported too'

    $empty = Join-Path $tmp 'empty'
    New-Item -ItemType Directory $empty | Out-Null
    Assert-Equal @(Test-Checksums $empty).Count 1 'no SHA256SUMS at all is a problem'

    # --- trays ---------------------------------------------------------------------------------
    Assert-Equal (Get-TrayAddr '"C:\NB\netbird-ui.exe"') 'npipe://netbird' 'a tray without an address uses the default'
    Assert-Equal (Get-TrayAddr '"C:\NB\netbird-ui.exe" --daemon-addr npipe://netbird-private') 'npipe://netbird-private' 'tray with address'
    Assert-Equal (Get-TrayAddr '"C:\NB\netbird-ui.exe" --daemon-addr npipe://netbird-private --badge 1') 'npipe://netbird-private' 'tray with address and badge'
    Assert-Equal (Get-TrayAddr '--daemon-addr=npipe://x') 'npipe://x' 'tray with ='
    Assert-Equal (Get-TrayAddr $null) 'npipe://netbird' 'no command line'

    $shortcuts = @(
        [pscustomobject]@{ Path = 'C:\S\NetBird (private).lnk'; Arguments = '--daemon-addr npipe://netbird-private --badge 1' },
        [pscustomobject]@{ Path = 'C:\S\NetBird (work).lnk'; Arguments = '--daemon-addr npipe://netbird-work' }
    )
    Assert-Equal (Find-TrayShortcut -Shortcuts $shortcuts -Addr 'npipe://netbird-work').Path 'C:\S\NetBird (work).lnk' 'shortcut found by address'
    Assert-Equal (Find-TrayShortcut -Shortcuts $shortcuts -Addr 'npipe://netbird-lab') $null 'no shortcut for an unknown address'
    Assert-Equal (Find-TrayShortcut -Shortcuts @() -Addr 'npipe://netbird-work') $null 'no shortcuts at all'

    # --- keeping the newest backups ------------------------------------------------------------
    $names = @('20261001-100000-0.79.0', '20261008-100000-0.80.0', '20261005-100000-0.79.5', '20261010-100000-0.80.0+SMS', 'notes')
    $expired = @(Get-ExpiredBackups -Names $names -Keep 3)
    Assert-Equal $expired.Count 1 'one backup is too many'
    Assert-Equal $expired[0] '20261001-100000-0.79.0' 'the oldest one goes'
    Assert-Equal @(Get-ExpiredBackups -Names @() -Keep 3).Count 0 'no backups, nothing to remove'
    Assert-Equal @(Get-ExpiredBackups -Names $names -Keep 10).Count 0 'fewer than the limit, nothing to remove'
}
finally {
    Remove-Item $tmp -Recurse -Force -ErrorAction SilentlyContinue
}

if ($script:failures -gt 0) {
    Write-Host "$script:failures of $script:checks checks failed" -ForegroundColor Red
    exit 1
}
Write-Host "update-windows: all $script:checks checks passed"
