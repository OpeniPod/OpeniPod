# Diagnostic logging

This records the decision for issue #6 and the implementation for issue #7.
No music player hardware is required. The current fake backends produce the
same diagnostic events as other implementations of the application interfaces.

## Decision and record format

Use Go's standard `log/slog` package, JSON records, and **journald on Linux**.
The application owns event logging. It receives an injected logger, rather
than using the global logger or making the reducer perform I/O.

| Storage | Benefits | Costs |
| --- | --- | --- |
| journald (selected) | App and kernel logs on the same host; time and priority filters; built-in persistence, size limits, rotation and export | Requires a running journald; binary storage is read with journalctl |
| Text files through rsyslog | Plain text and flexible routing/forwarding | Another daemon and routing configuration; rotation/retention must also be configured, normally with logrotate; avoid collecting the same message through multiple inputs |

For a Linux player using systemd, journald supplies the required storage and
inspection tools without an additional logging daemon. No third-party Go
logging library is necessary. When journald is unavailable, use a private
JSON file as a fallback; that file is not a replacement for the device's
journal configuration.

Every record has `time`, `level`, `msg`, `component` and `operation`.
Playback records include `track_id` when the current/selected track is known.
Failures include `error` as the reason, library success includes `track_count`,
and volume operations include `volume`. Do not log full track paths or music
metadata by default. A stop record has `reason`: `quit`, `input_eof`,
`cancelled` (including SIGINT/SIGTERM), or `error`.

| Level | Events |
| --- | --- |
| DEBUG | Settings loaded/saved, library loading or manual refresh begins |
| INFO | App start/stop, library loaded, accepted play/pause/resume requests, next/previous requests, playback transitions, track finished, volume changed |
| WARN | Library load failed; the application remains usable |
| ERROR | Playback failed, or a fatal input, rendering, player or storage operation failed |

Requests and confirmed playback transitions have different messages and
operations: `play` is a successful request to the adapter, `started` is its
confirmation event. They do not claim that audio is playing before that event.
Next/previous requests identify the track before switching; the subsequent
`play`/`started` records identify the new track.
There are no log records for redraws, cursor movement or playback-position
updates, even at DEBUG.

Fatal errors retain component, operation, track/volume context and the wrapped
cause while propagating to `App.Run`. Run logs them **once**, then records an
INFO stop without repeating the error. Main only exits with status 1; it does
not log that failure again. Recoverable library/playback errors are logged
where their typed events enter App and remain visible in UI state. Adapters
must not also log errors they return or send as events. Separate repeated
failures are still separate records; there is no global deduplication cache.

Library load results include both startup and manual refresh results. They
are logged at the state-update boundary, so a refresh is recorded even when
no intermediate UI state is rendered. A DEBUG `refreshing library` record
with operation `refresh` precedes the manual scan; its result is a
`library loaded` or `library load failed` record. A cancelled refresh is not
logged as a failure.

## Local file output

The default target is `file`, at INFO level. The default path is
`$XDG_STATE_HOME/openipod/player.log`, or
`$HOME/.local/state/openipod/player.log` when XDG_STATE_HOME is unset.
XDG_STATE_HOME must be absolute. The directory is created with mode 0700 and
new files with mode 0600. Records append across process restarts.

```sh
go build -o player ./cmd/player
./player --log-level=debug --log-file=/tmp/openipod-development.log
```

In another terminal:

```sh
tail -f /tmp/openipod-development.log
```

`--log-level` accepts `debug`, `info`, `warn`, or `error`; no code changes are
needed to change verbosity between launches. It does not hot-reload the level
inside a running process. `--log-target` accepts `file` or `journal`.
`--log-file` applies only to file mode. Configuration/open errors are reported
before the native window starts. Runtime errors are in the selected log
destination.

The local development file has no built-in rotation. Use it for bounded
development sessions, delete/archive it when no player process is writing,
or apply external rotation. Use the managed journal below for unattended
device runs. Existing file/directory permissions are not modified.

## Linux journal with the desktop window

Prerequisites: Linux, Go 1.23+, bash, `systemd-cat`, `journalctl`, a running
`systemd-journald`, an X11 or XWayland session, and a working audio device.
Installing the client commands alone is not enough: the journal service must
be active. Otherwise, use file mode.

```bash
go build -o player ./cmd/player
./player --log-target=journal --log-level=debug \
  2> >(systemd-cat --identifier=openipod --level-prefix=yes)
```

Only stderr is redirected to `systemd-cat`; the graphical window is displayed
by the desktop session. Keep stderr connected to the journal collector so
diagnostic records are not mixed into shell output.

The journal handler emits one JSON line prefixed with a syslog priority:
DEBUG `<7>`, INFO `<6>`, WARN `<4>`, ERROR `<3>`. `systemd-cat` consumes the
prefix and journald stores the appropriate native `PRIORITY`, so `journalctl
-p err` actually filters errors. JSON attributes are inside `MESSAGE`; they
are not separate native journal fields. All derived loggers share the output
lock so concurrent records keep their correct prefixes and fields.

### View, follow and export

Some systems require `sudo` to read the relevant journal. Use it if access is
denied or expected records are hidden by permissions.

```sh
# Recent application records and native error-priority records.
journalctl -t openipod --since '10 minutes ago' --no-pager
journalctl -t openipod -p err --no-pager

# Follow new application records (Ctrl+C stops the viewer, not the player).
journalctl -t openipod -f

# Current-boot kernel records; following may require elevated permissions.
sudo journalctl -k -b --no-pager
sudo journalctl -k -f

# Export app and current-boot kernel records for a bug report.
journalctl -t openipod --since today -o json --no-pager > openipod-journal.jsonl
sudo journalctl -k -b -o short-iso --no-pager > openipod-kernel.log
```

Review exports for private paths/system information before sharing them.

### Persistence, size and old records

[`deploy/journald/60-openipod.conf`](../deploy/journald/60-openipod.conf)
is a device configuration example: persistent storage, a 64 MiB system
journal target, 128 MiB free-space reserve, 8 MiB journal files, a 16 MiB
runtime target and seven-day retention. It disables forwarding to the console.

These are provisional defaults, following the
[maintainer's guidance in #6](https://github.com/OpeniPod/OpeniPod/issues/6#issuecomment-6100887514).
The final device-specific values will be chosen later. The starting values
keep diagnostic storage modest, reserve space for other data, and retain
recent debugging history. They are not measurements of the target hardware
or a guarantee of seven days of history: size limits can remove older
archived records sooner. No Go code change is needed to tune this drop-in.

**These settings apply to the whole default system journal, not only OpeniPod.**
Review values before installing on a desktop/shared computer. Journald's limits
rotate/delete archived files; active files and file granularity mean disk usage
is not an exact hard quota. Runtime storage can still be used if persistent
storage is unavailable. Existing forwarding/syslog configuration can create
additional copies outside the scope of this application.

On the Linux machine where you choose to install this example:

```sh
sudo install -d /etc/systemd/journald.conf.d
sudo install -m 0644 deploy/journald/60-openipod.conf \
  /etc/systemd/journald.conf.d/60-openipod.conf
sudo systemctl restart systemd-journald
sudo journalctl --flush
sudo journalctl --disk-usage
```

Journald manages rotation and expiration. To reclaim archived data immediately,
the following optional command deletes archived records older than seven days
throughout the default journal; it is not limited to the application:

```sh
sudo journalctl --rotate --vacuum-time=7d
```

To verify persistence, run the player, note a message and time, stop it, reboot
the Linux machine when convenient, and find that same message with
`journalctl -t openipod --since '<noted time>' --no-pager`.

## Reproducible verification

Run unit/integration checks on Linux:

```sh
make check
go build -o player ./cmd/player
# Requires journald, an X11/XWayland session, audio, Python 3 and journal
# read/sync permissions.
bash scripts/check-journal.sh ./player
```

The journal check uses a unique identifier and a `systemd-cat` collector, then
waits for startup records before sending SIGTERM to the graphical player. It
checks the app-to-journal path, record priorities, and graceful stop record
after `journalctl --sync`. A second run loads invalid settings JSON and checks
exit status 1 and exactly one native ERROR record for `storage/load_settings`.
The script does not alter journal settings or remove stored messages. Use `sudo` for the check on a dedicated test machine if journal
permissions require it; do not run the everyday player as root just for
logging.

Go tests also cover recovery errors, fatal errors from each subsystem,
cancellation, level filtering, file append/permissions, CLI wiring, and
concurrent grouped journal records.

### Verification performed on 2026-10-10

Environment: an isolated Debian bookworm arm64 test environment with Go 1.23,
real systemd-journald and systemd-cat. Journald ran as a standalone process
with the example configuration; the checks did not exercise a full OS boot.

- `make check`: formatting, all Go tests, race detector and vet passed.
- The journal integration check verified real collection/query and
  DEBUG/INFO priority mapping.
- A saved journal message remained readable after stopping and restarting
  the journald process with persistent storage enabled.
- Follow (`-f`), JSON export, disk-usage and kernel-query commands ran;
  journal configuration reported no unknown/invalid settings.

Full Linux reboot persistence and useful kernel records were not verified.
Check them on a Linux computer/VM using the commands above; no device
hardware is required.

## References

- [Go log/slog](https://pkg.go.dev/log/slog)
- [systemd-cat](https://www.man7.org/linux/man-pages/man1/systemd-cat.1.html)
- [journalctl](https://www.man7.org/linux/man-pages/man1/journalctl.1.html)
- [journald configuration](https://www.man7.org/linux/man-pages/man5/journald.conf.5.html)
- [rsyslog file output](https://www.rsyslog.com/doc/configuration/modules/omfile.html)
