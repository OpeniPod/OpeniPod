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

The terminal interaction can be smoke-tested with `go run ./cmd/player` and a keyboard, or by piping key bytes. Hardware tests and real audio integration tests will be added only when those implementations exist.

Diagnostic tests cover lifecycle/playback records, recovery and fatal failures,
absence of error duplication, readable text formatting, escaping control
characters, filtering, concurrent journal priorities and UI
separation. Integration checks also cover filesystem-library startup and
recovery through Update Library, plus successful, failed and cancelled
refresh diagnostics without duplicate records. Write-failure tests cover short
writes, retained write/close errors, and simultaneous application/log failures
without suppressing the stderr fallback. On Linux with a running
journald, build the player and run
`bash scripts/check-journal.sh ./player` for real journal collection and query
checks. See [logging](logging.md) for prerequisites and storage configuration.
