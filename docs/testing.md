# Testing guide

Run `go test ./...` for normal tests and `go test -race ./...` when touching
concurrency. Before a PR, run `make check`; it checks formatting, runs both test
commands, and runs `go vet ./...`.

## CI

GitHub Actions runs on every push to a branch or tag and when a pull request is
opened, reopened, or updated with new commits. It selects the Go version from
`go.mod` and runs `make all`: formatting, tests with and without the race
detector, `go vet ./...`, and a build of all packages. Results appear in the
pull request's Checks tab; step details and logs are available in the Actions
run. Use `make all` locally for the same checks and build.

## Test coverage

`internal/display/st7789` uses recording/erroring transports and substitute
SPI/GPIO operations to check ST7789V initialization, full 240×320 RGB565 frames,
image bounds, address windows, chunking, error propagation and resource cleanup.
`cmd/displaytest` tests a complete diagnostic dry-run without
device nodes. See [display verification](display.md) for Linux launch commands
and the orientation, colors, offsets and wiring checks deferred until delivery.

`internal/input/controls_test.go` supplies EC11 A/B and button snapshots with
controlled timestamps. It checks full cycles in both directions, bounce,
reversals, partial and invalid sequences, startup synchronization, fast
rotation, independent button debounce, release/rearm, held buttons, and stale
timestamps without real delays. See [control signals](controls.md) for the
decoder contract and future GPIO integration.

Reducer unit tests cover navigation bounds, volume bounds and commands, seeking
and clamping, library loading, playback events, and Back. The reducer does no
I/O, so these tests use plain values. The application integration test feeds
typed input events into a real App with FakeLibrary, FakePlayer, MemoryStorage,
and a small observing UI. It checks that selecting a track reaches FakePlayer
and returns as a rendered playing state; seek events reach the player fake.

Fakes let contributors test the application without audio files or device hardware. Add tests for observable behavior at the appropriate boundary. For event-loop changes, check cancellation, blocked sends, channel closure, and race behavior. Do not mock hardware behavior into unit tests merely to increase test count.

Filesystem library tests run on Linux in `internal/library/filesystem_test.go`.
They use temporary directories to cover recursive discovery,
extension filtering, stable IDs, ordering, empty directories, invalid roots,
persistent load, manual refresh, refresh failure, and malformed, mismatched,
or incompatible cache errors. Controlled filesystem failures and cancellation
make error tests deterministic. The permission test skips when the process can
read a directory despite its permissions, as can happen when running as root.
Application tests check that refresh failures retain the previous track list
and cancellation stops the app. `cmd/player` command tests supply FakePlayer,
typed App input, and a capturing renderer. They exercise `--music-dir`, the
`~/Music` default, empty libraries, load errors, cached startup, the Settings
`Update Library` item, saved settings, and recovery from an invalid index.
Audio scan tests use small generated silence fixtures for all supported
extensions and cover text renamed to each audio extension, audio with a
different extension, MP3 tag-only files, damaged metadata, cached versus
refreshed recognition, Ogg Opus, and video-only MP4 renamed to `.m4a`.
Tagged silence fixtures cover title, artist, album and track number for MP3,
FLAC, WAV, M4A, AAC and Ogg Vorbis/Opus/FLAC, including mismatched extensions.
Existing untagged and damaged-MP3 tests also check filename fallback and keeping
recognized audio. A command test checks that ordinary startup preserves all
saved metadata and Update Library reads and saves changed tags.
Fixtures and generation commands are committed, so normal tests require no
audio tools or hardware. Full audio decoding is outside these tests.

Equalizer tests check the fixed presets' frequency response with synthetic
tones, exact Off bypass, independent stereo channels, stream errors and partial
buffers, bounded output, and consistent filter state across buffer sizes and
preset changes. Player control tests check live updates, pause preservation,
invalid commands, and synchronized streaming under the race detector; they do
not open an audio device. Application, storage, and command tests check Settings
navigation, preset cycling, preservation of volume, error propagation, old
settings files, and restoration of the selected preset across restarts.

For a listening check, run with a real MP3 library, start a track, then open
Settings and cycle Equalizer. Check both running and paused playback, change
tracks, and restart with the same settings file. Use a track with bass and
vocals to compare the profiles; enabled presets deliberately reserve headroom.

Automated command tests do not open an audio device or native window. Use the
listening check above to verify real output. Hardware checks require the
actual board and peripherals.

## Native window

`internal/desktop` tests exercise key translation, frame updates, expose,
resize and aspect ratio, native driver errors, cancellation during input,
and concurrent rendering. A test driver replaces the native window, while
rendering uses the existing `ui.Display`. Command integration tests supply
FakePlayer, typed App input, and a capturing renderer to check filesystem
loading, playback requests, and saved settings without opening an audio device
or a display in CI.

For a manual check, run `go run ./cmd/player --music-dir /path/to/Music` from
an X11 desktop session or Wayland with XWayland. Navigate Home, Tracks, Now
Playing, and Settings; play and pause a real MP3; change volume and equalizer;
select Update Library. Resize and cover/uncover the window, checking that the
image retains its aspect ratio and redraws. Close using the window manager and
restart to check settings persistence. Also check `q`, Ctrl+C, and SIGTERM
shutdown. GUI smoke tests require a display and audio device and are not part
of `make check`.

Diagnostic tests cover lifecycle/playback records, recovery and fatal failures,
absence of error duplication, filtering, and concurrent journal priorities.
Integration checks also cover filesystem-library startup and recovery through
Update Library, plus successful, failed and cancelled refresh diagnostics
without duplicate records. On Linux with a running journald, X11 or XWayland,
and a working audio device, build the player and run
`bash scripts/check-journal.sh ./player`. The script launches the graphical
player, waits for its startup records, sends SIGTERM, and checks the graceful
stop records in the journal. See [logging](logging.md) for prerequisites and
storage configuration.
