# Testing guide

Run `go test ./...` for normal tests, `go test -race ./...` when touching concurrency, and `make check` before a PR. `make check` also checks formatting and runs `go vet ./...`.

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
