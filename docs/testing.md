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

`internal/input/controls_test.go` supplies EC11 A/B and button snapshots with
controlled timestamps. It checks full cycles in both directions, bounce,
reversals, partial and invalid sequences, startup synchronization, fast
rotation, independent button debounce, release/rearm, held buttons, and stale
timestamps without real delays. See [control signals](controls.md) for the
decoder contract and future GPIO integration.

Reducer unit tests cover navigation bounds, volume bounds and commands, library loading, playback events, and Back. The reducer does no I/O, so these tests use plain values. The application integration test feeds typed input events into a real App with FakeLibrary, FakePlayer, MemoryStorage, and a small observing UI. It checks that selecting a track reaches FakePlayer and returns as a rendered playing state.

Fakes let contributors test the application without audio files or device hardware. Add tests for observable behavior at the appropriate boundary. For event-loop changes, check cancellation, blocked sends, channel closure, and race behavior. Do not mock hardware behavior into unit tests merely to increase test count.

Filesystem library tests run on Linux in `internal/library/filesystem_test.go`.
They use temporary directories to cover recursive discovery,
extension filtering, stable IDs, ordering, empty directories, invalid roots,
persistent load, manual refresh, refresh failure, and malformed, mismatched,
or incompatible cache errors. Controlled filesystem failures and cancellation
make error tests deterministic. The permission test skips when the process can
read a directory despite its permissions, as can happen when running as root.
Application tests check that refresh failures retain the previous track list
and cancellation stops the app. Command tests exercise `--music-dir`, the
sample default, empty libraries, load errors, cached startup, the Settings
`Update Library` item, and recovery from an invalid index through the terminal
UI with piped input.
Audio scan tests use small generated silence fixtures for all supported
extensions and cover text renamed to each audio extension, audio with a
different extension, MP3 tag-only files, damaged metadata, cached versus
refreshed recognition, Ogg Opus, and video-only MP4 renamed to `.m4a`.
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

The terminal interaction can be smoke-tested with `go run ./cmd/player` and a
keyboard, or by piping key bytes. Automated tests do not open an audio device;
use the listening check above to verify real output. Hardware checks require
the actual board and peripherals.

## Interactive window

`internal/desktop` tests exercise key translation, frame updates, expose,
resize and aspect ratio, native driver errors, cancellation during input,
and concurrent rendering. A test driver replaces the native window, while
rendering uses the existing `ui.Display`. Command integration tests supply
FakePlayer and a test frontend to check filesystem loading, playback requests,
and saved settings without opening an audio device or a display in CI.

For a manual check, run `go run ./cmd/preview --music-dir /path/to/Music` from
an X11 desktop session. Navigate Home, Tracks, Now Playing, and Settings;
play and pause a real MP3; change volume and equalizer; select Update Library.
Resize and cover/uncover the window, checking that the image retains its
aspect ratio and redraws. Close using the window manager and restart to check
settings persistence. Also check `q`, Ctrl+C, and SIGTERM shutdown. GUI smoke
tests require a display and audio device and are not part of `make check`.

Diagnostic tests cover lifecycle/playback records, recovery and fatal failures,
absence of error duplication, filtering, concurrent journal priorities and UI
separation. Integration checks also cover filesystem-library startup and
recovery through Update Library, plus successful, failed and cancelled
refresh diagnostics without duplicate records. On Linux with a running
journald, build the player and run
`bash scripts/check-journal.sh ./player` for real journal collection and query
checks. See [logging](logging.md) for prerequisites and storage configuration.
