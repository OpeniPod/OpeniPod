# Testing guide

Run `go test ./...` for normal tests and `go test -race ./...` when touching
concurrency. Before a PR, run `make check`; it checks formatting, runs both test
commands, and runs `go vet ./...`.

## CI

GitHub Actions runs on every push to a branch or tag and when a pull request is
opened, reopened, or updated with new commits. It selects the Go version from
`go.mod`, runs `make check` to verify formatting, run tests with and without the
race detector, and run `go vet ./...`, then builds all packages with
`go build ./...`. Results appear in the pull request's Checks tab; step details
and logs are available in the Actions run. Run the same checks and build locally
with `make all`.

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
Audio decoding is outside these tests.

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

The terminal interaction can be smoke-tested with `go run ./cmd/player` and a keyboard, or by piping key bytes. Hardware tests and real audio integration tests will be added only when those implementations exist.
