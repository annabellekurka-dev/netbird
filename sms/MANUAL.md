# Two (or more) NetBird networks on one Windows machine

This is NetBird 0.80.0 with a few patches that let several NetBird networks run at the same time on one Windows machine, each with its own tunnel, DNS, firewall rule and tray icon. Everything else is stock NetBird. The version shows as `0.80.0+SMS`. Unsigned, so Windows may warn.

| | Default network | Extra network |
|---|---|---|
| Service | `Netbird` | `NetBird-<name>` |
| Interface | `wt0` | `wt1`, `wt2`, ... |
| Daemon pipe | `npipe://netbird` | `npipe://netbird-<name>` |
| State folder | `C:\ProgramData\Netbird` | `C:\ProgramData\Netbird-<name>` |
| Log file | `C:\ProgramData\Netbird\client.log` | `C:\ProgramData\Netbird-<name>\client.log` |
| WireGuard port | 51820 | 51821, 51822, ... |

Use an administrator PowerShell for everything below.

## 1. Put the build on the machine

Needs `netbird.exe` and `netbird-ui.exe` from this bundle, and NetBird already installed (it supplies `wintun.dll`). Check the files against `SHA256SUMS` first.

```powershell
cd 'C:\Program Files\Netbird'
Stop-Service Netbird; Get-Process netbird-ui -ErrorAction SilentlyContinue | Stop-Process
Copy-Item netbird.exe netbird.exe.orig; Copy-Item netbird-ui.exe netbird-ui.exe.orig
Copy-Item <bundle>\netbird.exe . -Force; Copy-Item <bundle>\netbird-ui.exe . -Force
Start-Service Netbird; Start-Process .\netbird-ui.exe
.\netbird.exe status
```

The default network should come back with the same IP and connection. Its interface must be `wt0` (`Get-NetIPAddress -IPAddress <your NetBird IP> | Select InterfaceAlias`). An MSI repair would put the official files back.

## 2. Add an extra network (one command)

New: not yet tried on a real machine, use `--dry-run` first. It shows every step and changes nothing.

```powershell
.\netbird.exe instance add private --setup-key <KEY> --management-url https://<management> --dry-run
.\netbird.exe instance add private --setup-key <KEY> --management-url https://<management>
```

To start from a profile that already exists on the machine, so it comes up as that identity instead of registering a new peer:

```powershell
.\netbird.exe instance add work --from-profile default
```

`default` is the profile of the default network; or give the path of any profile file. The profile file holds the machine's private key, keep it private. Leave out `--setup-key` and `--from-profile` to sign in through the browser. Other options: `--interface wt3`, `--wireguard-port 51900`, `--no-shortcut`.

What it does: picks the next free interface and port, installs and starts the service, sets the copied profile to the new interface and port, connects (a login link may appear), and adds the tray to your Startup folder. The install refuses, with a message, anything that would share state with another network.

When it ends it prints how to start the tray and how to check.

Remove it again:

```powershell
.\netbird.exe instance remove private
```

`--keep-data` keeps the folder with the keys and logs, `--yes` skips the question. It only touches a network that `instance add` created and never the default one.

### The same by hand (tested)

```powershell
cd 'C:\Program Files\Netbird'
$env:NB_STATE_DIR = 'C:\ProgramData\Netbird-private'
.\netbird.exe service install -s NetBird-private --daemon-addr npipe://netbird-private --service-env NB_STATE_DIR=C:\ProgramData\Netbird-private --log-file C:\ProgramData\Netbird-private\client.log
# only when starting from an existing profile: copy it in and set wt1 / 51821 BEFORE the service starts
Copy-Item C:\ProgramData\Netbird\default.json C:\ProgramData\Netbird-private\default.json -Force
$p = 'C:\ProgramData\Netbird-private\default.json'; $t = [IO.File]::ReadAllText($p)
$t = $t -replace '"WgIface":\s*"wt0"','"WgIface": "wt1"' -replace '"WgPort":\s*51820','"WgPort": 51821'
[IO.File]::WriteAllText($p, $t, (New-Object System.Text.UTF8Encoding($false)))
.\netbird.exe service start -s NetBird-private
.\netbird.exe up --daemon-addr npipe://netbird-private --interface-name wt1 --wireguard-port 51821 [--setup-key <KEY> --management-url https://<management>]
```

Write the profile file WITHOUT a byte-order mark (the lines above do; `Set-Content -Encoding utf8` in Windows PowerShell does not). Keep `NB_STATE_DIR` set in the shell for every command aimed at the extra network, and add `--daemon-addr npipe://netbird-private`.

## 3. Check

```powershell
.\netbird.exe instances
```

Lists both services, the interfaces with their firewall profile, the DNS rules of each network, and any problems found (shared addresses, overlapping networks or DNS domains, Public firewall profile). Send this output, plus the last lines of both `client.log` files, when asking for help.

Quick checks: ping a host in each network; `Resolve-DnsName <peer>.<domain>` per network (use this, not `nslookup`).

## 4. The trays

Each network has its own tray with Connect, Disconnect, status and settings.

- **Telling them apart:** hover over the icon. With more than one network up the tooltip shows the active profile, e.g. `NetBird (Smartsys)`. A tray for an extra network also has a coloured dot on its icon (the number comes from `--badge <n>`, which `instance add` sets to the interface number: wt1 blue, wt2 green, wt3 purple, wt4 red, wt5 teal). The default network's icon has no dot.
- **Profile switching** is greyed out in every tray while more than one network is up, so a network cannot be pointed at another one's profile. The trays re-check this every 15 seconds.
- **Start an extra tray by hand:**

```powershell
Start-Process 'C:\Program Files\Netbird\netbird-ui.exe' -ArgumentList '--daemon-addr','npipe://netbird-private','--badge','1'
```

## 5. Autostart

- **Services** start with Windows (automatic start) and reconnect on their own. Check: `Get-Service Netbird,NetBird-* | Select Name,StartType,Status`.
- **The default tray** starts at login through NetBird's own "launch at login" entry (registry `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`).
- **An extra tray** starts at login through a shortcut in your Startup folder (`shell:startup`) that `instance add` creates. For a network you added by hand, create it yourself in a normal (not elevated) PowerShell:

```powershell
$s = (New-Object -ComObject WScript.Shell).CreateShortcut("$env:APPDATA\Microsoft\Windows\Start Menu\Programs\Startup\NetBird (private).lnk"); $s.TargetPath = 'C:\Program Files\Netbird\netbird-ui.exe'; $s.Arguments = '--daemon-addr npipe://netbird-private --badge 1'; $s.WorkingDirectory = 'C:\Program Files\Netbird'; $s.Save()
```

- **Do not use the "launch at login" switch in the settings of an extra tray.** There is only one such entry per program and it starts the default tray; the switch in any tray controls that one entry.

## 6. New versions

The client checks the releases of this fork on GitHub every 30 minutes and shows NetBird's usual "new version available" notice when a newer one exists. Its button opens the fork's releases page. Nothing is downloaded or installed automatically. To update: replace the exe files as in step 1 (stop all NetBird services first). Automatic update inside NetBird stays off on purpose, so the official files cannot overwrite these.

## 7. Building a release

`sms\build-windows.ps1` in the repository builds the daemon and the tray, stamps the version as `<upstream version>+SMS`, checks the stamp, and writes `SHA256SUMS`. A release is a GitHub release of the fork with tag `v<upstream version>` (for example `v0.82.0`, a suffix such as `-SMS.1` is allowed) with these files attached. The part after `+` in the version is build metadata: it is ignored by every version comparison, on the client and on the management server. Do not use `-SMS`, a pre-release suffix would make the build look older than the release.

## What the patches do

Each network gets its own adapter, firewall rule, DNS rules and tray identity, so they do not overwrite each other. Guards refuse setups that would share state, warn about overlapping networks or DNS domains, put the extra interfaces on the Private firewall profile, and switch off profile switching while several networks run.

## Known limits

- Short names (`ping host` without the domain) can fail for an extra network although full names (`host.domain`) work. We traced it to how Windows handles single-label names on the test machine, not to the DNS rules. Use full names, or try setting the network's domain as the interface's DNS suffix (`Set-DnsClient -InterfaceAlias wt1 -ConnectionSpecificSuffix <domain>`, not tested).
- Two networks that both use a catch-all DNS group (every domain) cannot both win. One catch-all next to specific domains is fine.
- Only one exit node can carry the default route at a time. Exit node together with an extra network has not been tested.
- A release of the fork for the same upstream version (a fix only in the fork) is not announced by the new-version notice, because versions are compared without the `+SMS` part.
- `instance add` and `instance remove` have unit tests but have not yet been run on a real machine.
