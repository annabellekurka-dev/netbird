<#
.SYNOPSIS
  Updates the SMS multi-instance NetBird programs on this machine, safely.

.DESCRIPTION
  Replaces netbird.exe, netbird-ui.exe and netbird-ui-console.exe of an installed NetBird
  with a newer build, for every NetBird service on the machine (the default one and any extra
  network added with "netbird instance add"). It:

    1. finds the NetBird services and the install folder,
    2. takes the new files from -Source, or with -FromGitHub from the latest release of the
       fork, and checks them against their checksums,
    3. stops the services and the trays, keeps a backup of the old files,
    4. copies the new files, starts the services and checks that each daemon answers,
    5. puts the old files back by itself if anything went wrong,
    6. starts the trays again as the normal user.

  Nothing is changed before the new files are verified. Run it in an administrator PowerShell.
  It does not run by itself: it is an update you start. -DryRun shows the plan and changes nothing.

.EXAMPLE
  .\update-windows.ps1 -FromGitHub          # fetch the latest release and install it
  .\update-windows.ps1                      # install the files next to this script
  .\update-windows.ps1 -Source C:\temp\Netbirds -Yes
  .\update-windows.ps1 -FromGitHub -DryRun
#>
[CmdletBinding()]
param(
    # Folder with the new netbird.exe and netbird-ui.exe. Default: the folder of this script.
    [string]$Source,
    # Download the latest release of the fork and use it.
    [switch]$FromGitHub,
    # Where NetBird is installed. Default: where the service "Netbird" runs from.
    [string]$InstallDir,
    # Do not start the trays again afterwards.
    [switch]$NoTray,
    # Install even when the version and files are the same.
    [switch]$Force,
    # Do not ask for confirmation.
    [switch]$Yes,
    # Show what would happen, change nothing.
    [switch]$DryRun
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$Repo = 'annabellekurka-dev/netbird'
$ProgramNames = @('netbird.exe', 'netbird-ui.exe', 'netbird-ui-console.exe')
$KeepBackups = 3

# ---------------------------------------------------------------------------------------------
# Pure helpers (no changes to the machine); the tests load them by dot-sourcing this file.
# ---------------------------------------------------------------------------------------------

# Reads the command line of a Windows service. Returns $null unless it is "<netbird.exe> ... service run ...".
function ConvertFrom-ServicePath {
    param([string]$PathName)

    if (-not $PathName) { return $null }
    $m = [regex]::Match($PathName, '^\s*"?(?<exe>[^"]*?netbird\.exe)"?\s+(?<args>.*)$', 'IgnoreCase')
    if (-not $m.Success) { return $null }

    $args_ = $m.Groups['args'].Value
    if ($args_ -notmatch '(^|\s)service\s+run(\s|$)') { return $null }

    $addr = 'npipe://netbird'
    $a = [regex]::Match($args_, '--daemon-addr\s+(\S+)')
    if ($a.Success) { $addr = $a.Groups[1].Value }

    [pscustomobject]@{ Exe = $m.Groups['exe'].Value; Addr = $addr }
}

# The "x.y.z+SMS" stamp inside one of our programs, or $null (an official or development build has none).
function Get-StampedVersion {
    param([string]$Path)

    $text = [Text.Encoding]::ASCII.GetString([IO.File]::ReadAllBytes($Path))
    $m = [regex]::Match($text, '\d+\.\d+\.\d+\+SMS')
    if ($m.Success) { $m.Value } else { $null }
}

# Compares the files of a folder with its SHA256SUMS file. Returns a list of problems (empty = fine).
function Test-Checksums {
    param([string]$Dir)

    $sums = Join-Path $Dir 'SHA256SUMS'
    if (-not (Test-Path $sums)) { return @("no SHA256SUMS file in $Dir") }

    $problems = @()
    foreach ($line in Get-Content $sums) {
        $parts = $line.Trim().Split(' ', [StringSplitOptions]::RemoveEmptyEntries)
        if ($parts.Count -ne 2) { continue }
        $file = Join-Path $Dir $parts[1]
        if (-not (Test-Path $file)) { $problems += "$($parts[1]) is listed but missing"; continue }
        $actual = (Get-FileHash $file -Algorithm SHA256).Hash.ToLower()
        if ($actual -ne $parts[0].ToLower()) { $problems += "$($parts[1]) does not match its checksum" }
    }
    return $problems
}

# The --daemon-addr a tray was started with ('npipe://netbird' when it has none).
function Get-TrayAddr {
    param([string]$CommandLine)

    $m = [regex]::Match([string]$CommandLine, '--daemon-addr[ =]+"?(\S+?)"?(\s|$)')
    if ($m.Success) { $m.Groups[1].Value } else { 'npipe://netbird' }
}

# The startup shortcut whose arguments name this daemon address, or $null.
function Find-TrayShortcut {
    param([object[]]$Shortcuts, [string]$Addr)

    foreach ($s in $Shortcuts) {
        if ($s.Arguments -and (Get-TrayAddr $s.Arguments) -eq $Addr) { return $s }
    }
    return $null
}

# Backup folders to delete so that only the newest $Keep remain (names start with yyyyMMdd-HHmmss).
function Get-ExpiredBackups {
    param([string[]]$Names, [int]$Keep)

    $Names | Where-Object { $_ -match '^\d{8}-\d{6}' } | Sort-Object -Descending | Select-Object -Skip $Keep
}

# ---------------------------------------------------------------------------------------------
# Actions
# ---------------------------------------------------------------------------------------------

function Test-Admin {
    $id = [Security.Principal.WindowsIdentity]::GetCurrent()
    ([Security.Principal.WindowsPrincipal]$id).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
}

# The NB_STATE_DIR a service runs with (it is stored with the service), or $null for the default.
function Get-ServiceStateDir {
    param([string]$Name)

    try {
        $env_ = (Get-ItemProperty "HKLM:\SYSTEM\CurrentControlSet\Services\$Name" -ErrorAction Stop).Environment
    } catch { return $null }

    foreach ($kv in @($env_)) {
        if ($kv -and $kv -like 'NB_STATE_DIR=*') { return $kv.Substring('NB_STATE_DIR='.Length) }
    }
    return $null
}

function Get-NetBirdServices {
    foreach ($s in Get-CimInstance Win32_Service) {
        $info = ConvertFrom-ServicePath $s.PathName
        if ($info) {
            [pscustomobject]@{
                Name = $s.Name; State = $s.State; Exe = $info.Exe; Addr = $info.Addr
                StateDir = Get-ServiceStateDir $s.Name
            }
        }
    }
}

function Get-RunningTrays {
    $procs = Get-CimInstance Win32_Process -Filter "Name='netbird-ui.exe' OR Name='netbird-ui-console.exe'" -ErrorAction SilentlyContinue
    foreach ($p in $procs) { [pscustomobject]@{ Id = $p.ProcessId; Addr = Get-TrayAddr $p.CommandLine } }
}

function Get-StartupShortcuts {
    $dir = Join-Path $env:APPDATA 'Microsoft\Windows\Start Menu\Programs\Startup'
    if (-not (Test-Path $dir)) { return @() }

    $shell = New-Object -ComObject WScript.Shell
    foreach ($f in Get-ChildItem $dir -Filter 'NetBird*.lnk') {
        $lnk = $shell.CreateShortcut($f.FullName)
        [pscustomobject]@{ Path = $f.FullName; Target = $lnk.TargetPath; Arguments = $lnk.Arguments }
    }
}

function Get-InstalledVersion {
    param([string]$Exe)

    try { (& $Exe version 2>$null | Select-Object -First 1) } catch { 'unknown' }
}

function Save-LatestRelease {
    param([string]$Dest, [switch]$OnlyShow)

    [Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
    $headers = @{ 'User-Agent' = 'netbird-sms-update'; 'Accept' = 'application/vnd.github+json' }
    $release = Invoke-RestMethod "https://api.github.com/repos/$Repo/releases/latest" -Headers $headers

    $zip = $release.assets | Where-Object { $_.name -like '*.zip' } | Select-Object -First 1
    $sha = $release.assets | Where-Object { $_.name -like '*.zip.sha256' } | Select-Object -First 1
    if (-not $zip -or -not $sha) { throw "release $($release.tag_name) has no zip with a checksum" }

    Write-Host "Latest release: $($release.tag_name)  ($($zip.name), $([int]($zip.size / 1MB)) MB)"
    if ($OnlyShow) { return $null }

    New-Item -ItemType Directory -Force $Dest | Out-Null
    $zipPath = Join-Path $Dest $zip.name
    Invoke-WebRequest $zip.browser_download_url -OutFile $zipPath -UseBasicParsing
    $want = ((Invoke-RestMethod $sha.browser_download_url -Headers $headers) -as [string]).Trim().Split(' ')[0].ToLower()
    $got = (Get-FileHash $zipPath -Algorithm SHA256).Hash.ToLower()
    if ($want -ne $got) { throw "the downloaded zip does not match its published checksum" }

    $files = Join-Path $Dest 'files'
    Expand-Archive $zipPath -DestinationPath $files -Force
    return $files
}

function Stop-AllNetBird {
    param([object[]]$Services)

    foreach ($s in ($Services | Sort-Object { $_.Name -eq 'Netbird' })) {
        $svc = Get-Service $s.Name
        if ($svc.Status -ne 'Stopped') {
            Write-Host "  stopping $($s.Name)"
            Stop-Service $s.Name -Force
            $svc.WaitForStatus('Stopped', [TimeSpan]::FromSeconds(60))
        }
    }
    Get-Process netbird-ui, netbird-ui-console -ErrorAction SilentlyContinue | Stop-Process -Force
    Start-Sleep -Seconds 1
}

function Start-AllNetBird {
    param([object[]]$Services)

    foreach ($s in ($Services | Sort-Object { $_.Name -ne 'Netbird' })) {
        Write-Host "  starting $($s.Name)"
        Start-Service $s.Name
        (Get-Service $s.Name).WaitForStatus('Running', [TimeSpan]::FromSeconds(60))
    }
}

# The daemon of a service answers on its pipe. A freshly started daemon needs a moment.
function Wait-DaemonAnswers {
    param([string]$Exe, [string]$Addr, [string]$StateDir, [int]$Seconds = 60)

    # the command line tool reads its notes in the state folder of the network it talks to
    $oldState = $env:NB_STATE_DIR
    if ($StateDir) { $env:NB_STATE_DIR = $StateDir } else { Remove-Item Env:NB_STATE_DIR -ErrorAction SilentlyContinue }

    try {
        $deadline = (Get-Date).AddSeconds($Seconds)
        do {
            & $Exe status --daemon-addr $Addr *> $null
            if ($LASTEXITCODE -eq 0) { return $true }
            Start-Sleep -Seconds 2
        } while ((Get-Date) -lt $deadline)
        return $false
    }
    finally {
        if ($oldState) { $env:NB_STATE_DIR = $oldState } else { Remove-Item Env:NB_STATE_DIR -ErrorAction SilentlyContinue }
    }
}

function Restore-Backup {
    param([string]$BackupDir, [string]$InstallDir, [object[]]$Services)

    Write-Host "Putting the old files back from $BackupDir" -ForegroundColor Yellow
    Stop-AllNetBird $Services
    foreach ($f in Get-ChildItem $BackupDir -File) { Copy-Item $f.FullName (Join-Path $InstallDir $f.Name) -Force }
    Start-AllNetBird $Services
}

function Start-TrayAsUser {
    param([string]$Path)

    # explorer.exe starts it with the rights of the logged-in user, not of this administrator window
    Start-Process explorer.exe -ArgumentList ('"{0}"' -f $Path)
}

function Invoke-Update {
    if (-not $DryRun -and -not (Test-Admin)) { throw 'Run this in an administrator PowerShell: services cannot be stopped without it.' }

    # 1. what is installed
    $services = @(Get-NetBirdServices)
    if ($services.Count -eq 0) { throw 'No NetBird service found on this machine.' }

    $default = $services | Where-Object { $_.Name -eq 'Netbird' } | Select-Object -First 1
    $exe = if ($default) { $default.Exe } else { $services[0].Exe }
    if (-not $InstallDir) { $InstallDir = Split-Path $exe -Parent }
    $installedExe = Join-Path $InstallDir 'netbird.exe'

    $mine = @($services | Where-Object { (Split-Path $_.Exe -Parent) -eq $InstallDir })
    $others = @($services | Where-Object { (Split-Path $_.Exe -Parent) -ne $InstallDir })
    if ($others.Count) {
        Write-Warning ("These services run from another folder and are NOT updated: " + (($others | ForEach-Object Name) -join ', '))
    }
    if ($mine.Count -eq 0) { throw "No NetBird service runs from $InstallDir." }

    # 2. the new files
    $tempRoot = $null
    if ($FromGitHub) {
        if ($DryRun) { [void](Save-LatestRelease -Dest $null -OnlyShow); $Source = '<downloaded release>' }
        else {
            $tempRoot = Join-Path $env:TEMP ('netbird-sms-' + (Get-Date).ToString('yyyyMMdd-HHmmss'))
            Write-Host 'Downloading the latest release ...'
            $Source = Save-LatestRelease -Dest $tempRoot
        }
    }
    elseif (-not $Source) { $Source = Split-Path $PSCommandPath -Parent }

    if (-not $DryRun -or $Source -ne '<downloaded release>') {
        foreach ($n in @('netbird.exe', 'netbird-ui.exe')) {
            if (-not (Test-Path (Join-Path $Source $n))) { throw "$n is missing in $Source" }
        }
        Get-ChildItem $Source -Filter *.exe | Unblock-File

        $problems = @(Test-Checksums $Source)
        if ($problems.Count) { throw ("The new files do not check out:`n  " + ($problems -join "`n  ")) }
    }

    $newVersion = if ($Source -eq '<downloaded release>') { '(latest release)' } else { Get-StampedVersion (Join-Path $Source 'netbird.exe') }
    if (-not $newVersion) { throw 'The new netbird.exe carries no x.y.z+SMS version stamp: is it an official or a development build?' }
    $oldVersion = Get-InstalledVersion $installedExe

    $same = $false
    if ($Source -ne '<downloaded release>') {
        $same = (Get-FileHash $installedExe -Algorithm SHA256).Hash -eq (Get-FileHash (Join-Path $Source 'netbird.exe') -Algorithm SHA256).Hash
    }

    # 3. the plan
    $trays = @(Get-RunningTrays)
    Write-Host ''
    Write-Host 'Update plan'
    Write-Host "  install folder : $InstallDir"
    Write-Host "  installed      : $oldVersion"
    Write-Host "  new            : $newVersion   (from $Source)"
    Write-Host "  services       : $(($mine | ForEach-Object { "$($_.Name) [$($_.State)]" }) -join ', ')"
    Write-Host "  trays running  : $(@($trays).Count)"

    if ($same -and -not $Force) {
        Write-Host "`nThe installed netbird.exe is identical to the new one: nothing to do (use -Force to install anyway)."
        return
    }
    if ($DryRun) { Write-Host "`nDry run: nothing was changed."; return }

    if (-not $Yes) {
        $answer = Read-Host "`nStop NetBird, replace the programs and start it again? Type yes"
        if ($answer -ne 'yes') { Write-Host 'Cancelled.'; return }
    }

    $shortcuts = @(Get-StartupShortcuts)
    $log = Join-Path $env:TEMP ('netbird-sms-update-' + (Get-Date).ToString('yyyyMMdd-HHmmss') + '.log')
    Start-Transcript -Path $log | Out-Null

    # 4. stop, back up, replace
    Write-Host "`n[1/4] Stopping NetBird"
    Stop-AllNetBird $mine

    $backupRoot = Join-Path $InstallDir 'backup'
    $safeOld = ($oldVersion -replace '[^0-9A-Za-z.+-]', '_')
    $backupDir = Join-Path $backupRoot ((Get-Date).ToString('yyyyMMdd-HHmmss') + '-' + $safeOld)
    New-Item -ItemType Directory -Force $backupDir | Out-Null
    foreach ($n in $ProgramNames) {
        $p = Join-Path $InstallDir $n
        if (Test-Path $p) { Copy-Item $p $backupDir -Force }
    }
    Write-Host "[2/4] Old files saved in $backupDir"

    try {
        foreach ($n in $ProgramNames) {
            $src = Join-Path $Source $n
            if (Test-Path $src) { Copy-Item $src (Join-Path $InstallDir $n) -Force }
        }
        Write-Host '[3/4] New files copied'

        Write-Host '[4/4] Starting NetBird'
        Start-AllNetBird $mine
        foreach ($s in $mine) {
            if (-not (Wait-DaemonAnswers -Exe $installedExe -Addr $s.Addr -StateDir $s.StateDir)) {
                throw "the daemon of $($s.Name) does not answer on $($s.Addr)"
            }
        }
    }
    catch {
        Write-Host "`nThe update failed: $($_.Exception.Message)" -ForegroundColor Red
        Restore-Backup -BackupDir $backupDir -InstallDir $InstallDir -Services $mine
        Stop-Transcript | Out-Null
        throw "Update failed and the old programs are back. Details: $log"
    }

    # old backups beyond the newest few
    if (Test-Path $backupRoot) {
        $existing = @(Get-ChildItem $backupRoot -Directory | ForEach-Object Name)
        foreach ($name in @(Get-ExpiredBackups -Names $existing -Keep $KeepBackups)) {
            Remove-Item (Join-Path $backupRoot $name) -Recurse -Force
        }
    }

    # trays, as the normal user
    if (-not $NoTray -and $trays.Count) {
        Write-Host "`nStarting the trays again"
        $started = @{}
        foreach ($t in $trays) {
            if ($started[$t.Addr]) { continue }
            $started[$t.Addr] = $true
            if ($t.Addr -eq 'npipe://netbird') {
                Start-TrayAsUser (Join-Path $InstallDir 'netbird-ui.exe')
                continue
            }
            $lnk = Find-TrayShortcut -Shortcuts $shortcuts -Addr $t.Addr
            if ($lnk) { Start-TrayAsUser $lnk.Path }
            else { Write-Warning "No startup shortcut for the tray of $($t.Addr). Start it by hand: netbird-ui.exe --daemon-addr $($t.Addr)" }
        }
    }

    Stop-Transcript | Out-Null
    Write-Host "`nDone. Installed: $(Get-InstalledVersion $installedExe)" -ForegroundColor Green
    Write-Host "Check everything with:  & '$installedExe' instances"
    Write-Host "Old files are kept in $backupRoot (newest $KeepBackups). Log: $log"

    if ($tempRoot -and (Test-Path $tempRoot)) { Remove-Item $tempRoot -Recurse -Force }
}

# Run unless the file is only being loaded (dot-sourced) to use its functions.
if ($MyInvocation.InvocationName -ne '.') { Invoke-Update }
