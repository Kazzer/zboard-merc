# zboard-merc

An open replacement for Ideazon's ZEngine for the Zboard MERC Stealth (USB 1038:0510) and
the plain MERC (0210), on Linux and Windows, in Go. The keyboard has no onboard memory, so
everything ZEngine did lives in an agent on the host: Mods (profiles) that follow the
application in front, key remaps, chords, macros with a recorder, chat lines, launches,
layers, the two extra LEDs, the hotkeys and the Z key.

- `cmd/zengine` is the agent, with its control API, web UI and tray icon; `cmd/zboard` the
  command line for it; `cmd/zprobe` the capture and check tool, Linux and Windows.
- `internal/` holds the engine (pure, tested), the key and device tables, the profile
  format, the version, and the two platform backends.
- `driver/umdf/` is the Windows HID minidriver that makes the eleven table-2 keys visible;
  optional, the agent works without it.
- `docs/` is the record: `zengine-features.md` (parity with ZEngine), `ui.md`, `keys.md`
  (every key in each table and every key name, generated), `windows-dead-keys.md` (why the
  driver exists), `releasing.md` (how a release is cut and what it carries).
- `examples/` has a configuration and two Mods; `packaging/` the Linux udev, hwdb and
  systemd files and the Windows install scripts; `tools/` the capture report, the
  discovery-era Python capture, the Windows resource generator and the key-table generator.

Build with `make`: every Go command runs in the `golang:alpine` container that
`docker-compose.yaml` describes. `make` builds the executables for your system into
`bin/<os>-<arch>/` (`GOOS=windows` for the other), `make bundle` assembles the bundle a user
unpacks in `bundle/<os>-<arch>/`, `make check` runs `go fmt`, `go vet` for both platforms and
the tests, `sudo make install` puts the executables into `/usr/local/bin` and the udev, hwdb,
modules-load and systemd user files in their system directories (`prefix=`, `DESTDIR=` and
the other GNU variables apply), `make dist` creates the source distribution and `make usage`
lists the rest. Without docker, `CGO_ENABLED=0 go build ./...` builds everything,
the Windows binaries cross-compile the same way with `GOOS=windows`, and `go test ./...` runs
everything but the Windows platform tests, which run under wine with
`GOOS=windows go test -exec wine`. `go install github.com/Kazzer/zboard-merc/cmd/zengine@latest`
(or `cmd/zboard`, `cmd/zprobe`) installs one command straight from the published module. The
version is `internal/version/VERSION`, shared by the agents, the driver and the bundles.

Problems and questions go to the repository's issues, `https://github.com/Kazzer/zboard-merc/issues`.

MIT licence, Aether Software (`LICENSE.md`).
