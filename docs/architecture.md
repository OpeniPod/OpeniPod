# Architecture

## Shape and dependencies

This is one Go process: a modular monolith. It keeps the first prototype easy to run and debug while allowing contributors to replace one adapter at a time. There is no internal network protocol or service deployment to coordinate.

```text
cmd/player (composition root)
   ├──> internal/app
   ├──> internal/input
   ├──> internal/ui
   ├──> internal/player
   ├──> internal/library
   └──> internal/storage

internal/input   ─┐
internal/ui       │
internal/player   ├──> internal/app ──> internal/music
internal/storage ─┘
internal/player  ───────────────────────> internal/music
internal/library ───────────────────────> internal/music
```

Arrows show direct Go imports. `app` uses player, library, and storage through
interfaces; `cmd/player` supplies their concrete implementations.

`music` has the small `Track` type. `app` defines events, `AppState`, the reducer, and consumer-side interfaces. `input` converts keys into events. `ui` renders a state snapshot. `library` supplies filesystem and fake adapters; `player` and `storage` are concrete fake adapters. `cmd/player` wires them together. The application core imports no terminal, audio, or board-specific package.

## State and event flow

The `App` event loop is the sole owner of mutable `AppState`. The reducer computes a new state and small commands; it performs no I/O. The loop executes commands and renders a copy of the state. Adapters never mutate state, and UI never calls Player or Library.

```text
keyboard key → input event → App → Reduce → state + command
                                       │
                                       └→ FakePlayer.Play(track)
                                              ↓
                                        PlaybackStarted event
                                              ↓
                                        App → Reduce → Render(state)
```

Events are Go types, not string tags. `SelectPressed` on a track requests
`Play`; the fake player emits `PlaybackStarted`; only that event marks playback
as playing. Library and settings are loaded at startup through their
interfaces. The Settings screen exposes `Update Library` as its only selectable
item; selecting it invokes `Library.Refresh` as a synchronous one-off
operation. The app waits for it to finish and replaces its track list only
after success. A load or refresh failure becomes visible in state; a refresh
failure preserves the existing in-memory list and saved index. Command and
rendering failures return with context to `main`, where `slog` reports them
once.

Home and track-list movement **clamps** at the first and last item. Next and previous track commands wrap around the loaded library. Volume clamps to 0..100. Back returns to Home. These choices keep the initial navigation small and predictable.

## Concurrency and shutdown

The keyboard reader is the only worker goroutine in this slice. It sends input events to a buffered channel. The fake player has a buffered event channel but starts no goroutine. The App loop selects input events, player events, and context cancellation, then changes state sequentially. It cancels and waits for the keyboard reader on exit. On Linux the keyboard reader polls with a short timeout, so cancellation can release terminal raw mode and restore terminal settings. No mutex protects AppState because it has one owner.

## Adapters and future hardware

`FakeLibrary` supplies stable sample tracks. `FakePlayer` records the current track and reports playback events without audio. `Memory` stores volume settings only until process exit. These make UI and application work possible without music files or a board.

`Filesystem` implements the same Library interface and is selected by
`--music-dir` in `cmd/player`. It stores a versioned, root-specific index at
`<cleaned-absolute-music-root>/.openipod-library.json`. A missing index causes
the first `Load` to scan recursively and atomically save the result; subsequent
loads read the index only, without walking the directory or statting tracks.
`Refresh` performs that scan and atomic replacement synchronously. It filters
regular files by extension and returns tracks sorted by path. Track IDs are
SHA-256 hashes of cleaned absolute paths. Scans observe context cancellation
and return no partial list after a read or save error. Malformed, mismatched,
or incompatible indexes are errors rather than reasons to rescan. It owns no
application state, starts no goroutines, and uses no background scan or
filesystem watcher. The fake library remains the default without a directory.

An Orange Pi input adapter can later translate GPIO buttons or a rotary encoder into the same events as the keyboard. A display adapter can render the same `AppState` snapshot instead of the terminal UI. A real player adapter can implement the small Player interface and report playback events. These adapters should live outside `internal/app`; their composition belongs in a platform-specific entry point or wiring code. This prototype makes no claims about device drivers, audio decoding, or deployment yet.
