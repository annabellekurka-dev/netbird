# Releasing the fork and following NetBird

## One-time setup on GitHub (the fork's Settings)

1. **Actions > General:** allow Actions. GitHub asks you to confirm that you understand the workflows.
2. **Actions tab, left side:** disable the workflow **`agent-network-e2e.yml`**. It is NetBird's nightly job; it needs their AWS secrets and would only fail every night. The other NetBird workflows react to pull requests, pushes to `main` and tags, which this fork's branch does not produce, so they stay idle. (Disable more of them by hand if you ever see one run.)
3. **Branches:** set the default branch to **`multi-instance`**. Scheduled workflows only run from the default branch, and the weekly upstream watch is one. `main` stays a plain copy of NetBird.
4. **Features:** switch **Issues** on. The upstream watch reports through an issue. (Without it the weekly job fails, and GitHub emails you about the failed run instead.)

## What runs when

| Workflow | When | What |
|---|---|---|
| SMS tests | every push to `multi-instance`, or by hand | `sms/test-windows.ps1`: the tests of our changes, and `go vet` on every package we touched |
| SMS release | by hand only | tests, build, zip, GitHub release |
| SMS upstream watch | Mondays, or by hand | opens an issue when NetBird has a newer stable release than the fork's latest release is built on |

## Publishing a release

1. Push the commits to `multi-instance` and wait for **SMS tests** to be green.
2. **Actions > SMS release > Run workflow**, branch `multi-instance`. Fill in:
   - `version`: the NetBird version the branch is based on, three numbers, e.g. `0.80.0`.
   - `revision`: `1` for the first build of that version, `2` for a rebuilt one, and so on.
3. The workflow publishes the release `v0.80.0-SMS.1` with `netbird-sms_0.80.0-SMS.1_windows_amd64.zip` and its checksum. The programs inside report the version `0.80.0+SMS`.
4. Clients notice the release within 30 minutes and show the "new version available" notice, which opens the releases page.

Why the tag is `v0.80.0-SMS.1` and not `v0.80.0`: the bare tag is NetBird's own and points at their code. The client takes the version from the tag and ignores the suffix, so `v0.80.0-SMS.1` announces version 0.80.0.

Limit: a second build of the same NetBird version (`-SMS.2`) is not announced by the notice, because versions are compared without the suffix. Clients that should get it need to be told by hand.

## Moving to a newer NetBird (for example 0.82.0)

```
git fetch https://github.com/netbirdio/netbird.git "+refs/tags/v0.82.0:refs/tags/v0.82.0"
git switch -c multi-instance-0.82 multi-instance
git rebase --onto v0.82.0 v0.80.0
```

- `git rebase --onto <new> <old>` replays the fork's commits on top of the new release. Where NetBird changed the same lines, git stops at that commit and asks for a decision.
- Compare old and new patch series: `git range-diff v0.80.0..multi-instance v0.82.0..multi-instance-0.82`.
- Run `sms\test-windows.ps1 -Base v0.82.0`, then `sms\build-windows.ps1`, and try the build on a test machine (see MANUAL.md).
- Move the branch: `git branch -f multi-instance multi-instance-0.82`, push it, then run **SMS release** with version `0.82.0`.
- The fork's workflow files and `sms/` are new files, so they do not conflict.
