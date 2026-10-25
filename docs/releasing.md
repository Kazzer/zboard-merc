# Releasing

A release is a git tag `v<version>` with a GitHub release that carries the four bundles, the
source distribution and their checksums. Publishing the release runs
`.github/workflows/release.yaml`, which builds the files in the same container as a desk
build and attaches them; the same files come out of `make` by hand.

## What a release carries

| File | Content |
|---|---|
| `zboard-merc-<version>-linux-amd64.tar.xz`, `zboard-merc-<version>-linux-arm64.tar.xz` | The Linux bundle, unpacking into a directory of the same name: `zengine`, `zboard`, `zprobe`, `install.sh`, `uninstall.sh`, the udev rules, the hwdb entry, the modules-load file, the user unit, the example `config.yaml` and `mods/`. |
| `zboard-merc-<version>-windows-amd64.tar.xz`, `zboard-merc-<version>-windows-arm64.tar.xz` | The Windows bundle: `zengine.exe`, `zengine-tray.exe`, `zboard.exe`, `zprobe.exe`, `install.ps1`, `uninstall.ps1`, the example `config.yaml` and `mods/`. Windows 11 unpacks `.tar.xz` from Explorer. |
| `zboard-merc-<version>.tar.xz` | The source distribution: every tracked file, under `zboard-merc-<version>/`. |
| `SHA256SUMS` | `sha256sum` of the five archives. |

The Windows driver is not part of a release: its package is currently self-signed
(`driver/umdf/README.md`).

## Cutting a release

1. Put the new `major.minor.patch` in `internal/version/VERSION`. The agents, the probe,
   the Windows resources, the driver and the archive names all read it. Run `make check`,
   commit.
2. Tag that commit and push both:

   ```
   git tag v<version>
   git push origin main v<version>
   ```

3. Publish the release, from the web form or with

   ```
   gh release create v<version> --title v<version> --notes-file <notes>
   ```

   Publishing triggers the workflow: it checks that the tag is `v` plus the VERSION file,
   builds the five archives, writes `SHA256SUMS` and uploads them to the release. A draft
   does not trigger it; publishing the draft does. Re-running the job replaces the files.
4. Open the release page and check the six assets are there.

## By hand

The workflow does nothing `make` cannot; to attach the files from a machine with docker,
compose, git, GNU make and GNU tar:

```
make GOOS=linux GOARCH=amd64 release
make GOOS=linux GOARCH=arm64 release
make GOOS=windows GOARCH=amd64 release
make GOOS=windows GOARCH=arm64 release
make dist
sha256sum zboard-merc-*.tar.xz > SHA256SUMS
gh release upload v<version> zboard-merc-*.tar.xz SHA256SUMS --clobber
```

`make release` alone builds the archive for the system it runs on; `make mostlyclean`
removes all of them.
