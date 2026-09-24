# Testing guide

Run `go test ./...` for normal tests, `go test -race ./...` when touching concurrency, and `make check` before a PR. `make check` also checks formatting and runs `go vet ./...`.

Reducer unit tests cover navigation bounds, volume bounds and commands, library loading, playback events, and Back. The reducer does no I/O, so these tests use plain values. The application integration test feeds typed input events into a real App with FakeLibrary, FakePlayer, MemoryStorage, and a small observing UI. It checks that selecting a track reaches FakePlayer and returns as a rendered playing state.

Fakes let contributors test the application without audio files or device hardware. Add tests for observable behavior at the appropriate boundary. For event-loop changes, check cancellation, blocked sends, channel closure, and race behavior. Do not mock hardware behavior into unit tests merely to increase test count.

The terminal interaction can be smoke-tested with `go run ./cmd/player` and a keyboard, or by piping key bytes. Hardware tests and real audio integration tests will be added only when those implementations exist.
