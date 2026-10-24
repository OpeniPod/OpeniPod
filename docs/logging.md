# Diagnostic logging

This records the decision for issue #6 and the implementation for issue #7.
No music player hardware is required. The current fake backends produce the
same diagnostic events as other implementations of the application interfaces.

## Decision and record format

Use Go's standard `log/slog` package, readable text records, and **journald on Linux**.
The application owns event logging. It receives an injected logger, rather
than using the global logger or making the reducer perform I/O.

| Storage | Benefits | Costs |
| --- | --- | --- |
| journald (selected) | App and kernel logs on the same host; time and priority filters; built-in persistence, size limits, rotation and export | Requires a running journald; binary storage is read with journalctl |
| Text files through rsyslog | Plain text and flexible routing/forwarding | Another daemon and routing configuration; rotation/retention must also be configured, normally with logrotate; avoid collecting the same message through multiple inputs |

For a Linux player using systemd, journald supplies the required storage and
inspection tools without an additional logging daemon. No third-party Go
logging library is necessary. When journald is unavailable, use a private
text file as a fallback; that file is not a replacement for the device's
journal configuration.

Records contain date/time, level, `[component/operation]`, then the message.
Additional attributes follow in parentheses as `key: value`, without JSON or
`time=`, `level=`, `msg=` prefixes. For example:

```text
2026-10-19 15:30:13 INFO [player/started] playback started (track_id: 123)
2026-10-19 15:30:14 ERROR [player/playback] playback failed (track_id: 123, error: decoder unavailable)
```

Timestamps use the process's local time, to seconds. Control characters such
as newlines and terminal escape sequences are escaped; values with delimiters
are quoted, so one event always stays on one line. Grouped attributes use
dotted names, for example `track.id: 123`. A missing component/operation is
shown as `-`; application records supply both.

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
not log that failure again if diagnostics were written successfully.
The output remembers the first write error (including short writes); shutdown
checks it and the file-close result. If either failed, main prints the logging
failure to stderr and exits with status 1. A simultaneous application error is
preserved too, rather than marked as already logged. The fallback cannot
guarantee delivery if stderr itself is broken; in journal mode stderr is still
the redirected collector stream. No retry or logging-error recursion is used.
Recoverable library/playback errors are logged
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
before the UI starts. Runtime errors are in the selected log destination, with
the stderr fallback above if writing or closing diagnostics fails.

The local development file has no built-in rotation. Use it for bounded
development sessions, delete/archive it when no player process is writing,
or apply external rotation. Use the managed journal below for unattended
device runs. Existing file/directory permissions are not modified.

## Linux journal without disrupting terminal UI

Prerequisites: Linux, Go 1.23+, bash, `systemd-cat`, `journalctl`, and a
running `systemd-journald`. Installing the client commands alone is not
enough: the journal service must be active. Otherwise, use file mode.

```bash
go build -o player ./cmd/player
./player --log-target=journal --log-level=debug \
  2> >(systemd-cat --identifier=openipod --level-prefix=yes)
```

Only stderr is redirected; stdin stays connected to the keyboard and stdout
stays connected to the terminal UI. Do not wrap the entire player command in
`systemd-cat`, since that would collect the UI too. Do not use journal mode
with stderr still attached to the UI terminal.

The journal handler emits one text line prefixed with a syslog priority:
DEBUG `<7>`, INFO `<6>`, WARN `<4>`, ERROR `<3>`. `systemd-cat` consumes the
prefix and journald stores the appropriate native `PRIORITY`, so `journalctl
-p err` actually filters errors. The readable line is stored in `MESSAGE`;
its attributes are not separate native journal fields. All derived loggers
share the output lock so concurrent records keep their correct prefixes and fields.

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
Here `-o json` exports journald's envelope; the application's `MESSAGE` remains
plain text. Use `-o cat` to display only the application's formatted lines.

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
# Requires a running journald, Python 3 and journal read/sync permissions.
bash scripts/check-journal.sh ./player
# Optional PTY check of SIGTERM and terminal restoration (Python 3).
python3 scripts/check-terminal.py ./player
```

The journal check uses a unique identifier, a FIFO collector and `journalctl
--sync` before reading. It verifies the real app-to-journal path, DEBUG/INFO
priorities, playback events, and absence of diagnostics in UI output. A second
run sends UI output to `/dev/full`, then checks exit status 1 and exactly one
native ERROR record for the renderer failure. It does not alter journal settings
or remove stored journal messages. Use `sudo` for the check on a dedicated test
machine if journal permissions require it; do not run the everyday player as
root just for logging.

Go tests also cover recovery errors, fatal errors from each subsystem,
cancellation, level filtering, file append/permissions, CLI wiring, and
concurrent grouped journal records. Tests also cover exact text formatting,
escaped multiline errors, short writes, close failures, and preservation of
both application and logging errors when a destination fails.

### Verification performed on 2026-10-24

Environment: isolated Debian bookworm arm64 test environments with Go 1.23
and Go 1.27. The journal checks used Go 1.27, real systemd-journald and
systemd-cat. Journald ran as a standalone process
with the example configuration; the checks did not exercise a full OS boot.

- Go 1.23 `make all` and Go 1.27 `make check`: formatting, all Go tests,
  race detector, vet and build passed.
- `check-journal.sh`: real collection/query, readable text, DEBUG/INFO priority mapping,
  terminal-output separation, failing exit status and exactly one native
  ERROR record passed.
- `check-terminal.py`: SIGTERM, restoration of terminal settings and separate
  log/UI output passed.
- A saved journal message remained readable after stopping and restarting
  the journald process with persistent storage enabled.
- A recoverable library failure was found with the native WARN priority;
  journal file verification passed.
- With both `--log-file=/dev/full` and stdout redirected to `/dev/full`, the
  executable exited with status 1 and stderr retained both original failures.
- The unreadable-directory library test passed under an unprivileged user.

Full Linux reboot persistence and useful kernel records were not verified.
Check them on a Linux computer/VM using the commands above; no device
hardware is required.

## References

- [Go log/slog](https://pkg.go.dev/log/slog)
- [systemd-cat](https://www.man7.org/linux/man-pages/man1/systemd-cat.1.html)
- [journalctl](https://www.man7.org/linux/man-pages/man1/journalctl.1.html)
- [journald configuration](https://www.man7.org/linux/man-pages/man5/journald.conf.5.html)
- [rsyslog file output](https://www.rsyslog.com/doc/configuration/modules/omfile.html)
