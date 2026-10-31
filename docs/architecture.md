# Architecture

## Shape and dependencies

This is one Go process: a modular monolith. It keeps the first prototype easy to run and debug while allowing contributors to replace one adapter at a time. There is no internal network protocol or service deployment to coordinate.

```text
cmd/player (terminal composition root)
   ├──> internal/app
   ├──> internal/input
   ├──> internal/ui
   ├──> internal/player
   ├──> internal/library
   └──> internal/storage

cmd/preview (window composition root)
   ├──> internal/app
   ├──> internal/desktop ──> internal/ui
   ├──> internal/player
   ├──> internal/library
   └──> internal/storage

internal/input   ─┐
internal/desktop  │
internal/ui       │
internal/player   ├──> internal/app ──> internal/music
internal/storage ─┘
internal/player  ───────────────────────> internal/music
internal/library ───────────────────────> internal/music
```

Arrows show direct Go imports. `app` uses player, library, and storage through
interfaces; the entry points supply their concrete implementations.

`music` has the small `Track` type. `app` defines events, `AppState`, the reducer,
and consumer-side interfaces. `input` converts keys and control signals into
events. `ui` renders a state snapshot to the terminal or to a 240×320 RGBA
image. The image renderer uses an embedded font and can be exercised through
PNG examples without a physical display. `library` supplies filesystem and
fake adapters; `player` supplies real and fake players; `storage` supplies
file and memory adapters.
`cmd/player` wires the terminal frontend; `cmd/preview` wires the native window
with the real player and filesystem library. The application core imports no
terminal, audio, or board-specific package.

`internal/desktop` implements both `app.Input` and `app.Renderer`. Shiny owns
native window events; the adapter translates keys and window close to App
input events. `Render` passes each supplied snapshot to `ui.Display`, scales
its image, and publishes it to the window. Expose and resize events reuse only
the last image, without retaining or changing AppState. Linux uses Shiny's X11
driver; the core and image renderer have no window-system dependency.

## State and event flow

The `App` event loop is the sole owner of mutable `AppState`. The reducer computes a new state and small commands; it performs no I/O. The loop executes commands and renders a copy of the state. Adapters never mutate state, and UI never calls Player or Library.

```text
keyboard key → input event → App → Reduce → state + command
                                       │
                                       └→ Player.Play(track)
                                              ↓
                                        PlaybackStarted event
                                              ↓
                                        App → Reduce → Render(state)
```

Events are Go types, not string tags. `SelectPressed` on a track requests
`Play`; the player emits `PlaybackStarted`; only that event marks playback
as playing. The real player includes the decoded duration in `PlaybackStarted`.
Library and settings are loaded at startup through their
interfaces. The Settings screen exposes `Update Library` and `Equalizer` as
selectable items. Selecting `Update Library` invokes `Library.Refresh` as a synchronous one-off
operation. The app waits for it to finish and replaces its track list only
after success. A load or refresh failure becomes visible in state; a refresh
failure preserves the existing in-memory list and saved index. Command and
rendering failures return with context to `main`, where `slog` reports them
once.

Selecting `Equalizer` cycles the fixed Off, Bass boost, and Vocal presets in the
reducer. App sends the selected preset through `Player.SetEqualizer` and the
existing settings-save command. The preset lives in `SettingsState`; UI only
renders it. Startup applies the saved preset before playback. Older files
default to Off and unknown values are normalized to Off.

The real player filters the resampled 48 kHz stereo stream before `beep.Ctrl`
and volume. Off is an exact bypass. Bass boost uses a +6 dB low shelf at 150 Hz;
Vocal uses a +4 dB peaking filter at 2 kHz with Q 0.8. Biquad coefficients follow
the [W3C Audio EQ Cookbook](https://www.w3.org/TR/audio-eq-cookbook/).
Input attenuation compensates each preset's maximum gain; enabled profiles
bound output samples to `[-1, 1]`. Each channel has independent filter history.
Each new track gets a fresh filter; a preset change resets filter history while
retaining source position and pause state. The player takes its mutex and then
the speaker lock when updating the active filter, following the existing audio
lock order. Audio callbacks do not acquire the player mutex.

Home and track-list movement **clamps** at the first and last item. Next and previous track commands wrap around the loaded library. Volume clamps to 0..100. Back returns to Home. These choices keep the initial navigation small and predictable.

## Concurrency and shutdown

The keyboard reader sends input events to a buffered channel. The App loop
selects input events, player events, and context cancellation, then changes
state sequentially. On exit it cancels and waits for the keyboard reader, then
calls `Player.Close` on every exit path. The real player stops speaker output,
closes the active decoder, and waits for any completion callback worker. The
fake player implements `Close` as a no-op. On Linux the keyboard reader polls
with a short timeout, so cancellation can release terminal raw mode and
restore terminal settings. No mutex protects AppState because it has one
owner.

For the window frontend, the same input worker handles native keyboard,
resize, expose, and close events. App calls `Render` in its own loop. An
adapter-local mutex serializes drawing, buffer replacement, and repainting;
it does not protect application state. Context cancellation wakes the native
event queue so a blocked input worker can exit. The window and display font
resources are released only after `App.Run` has stopped its input worker and
closed the player. Window close sends `QuitRequested`, allowing App to flush
pending settings before exit.

Beep's `speaker.Close` stops audio playback, but its Oto dependency documents
that the underlying driver context cannot be closed. The application creates
no separate audio process; the operating system reclaims the driver context
when the application process exits.

The App loop also handles a settings-save timer. Settings writes run
synchronously in that loop; storage starts no worker goroutine. After settings
have loaded successfully, orderly exits through quit, input EOF, cancellation,
or an error flush pending changes. The flush uses a fresh context independent
of cancellation, with a two-second timeout. Save errors are returned alongside
any existing application error. The entry point handles both interrupt and
SIGTERM; abrupt termination or power loss can lose pending changes.

## Adapters and future hardware

`FakeLibrary` supplies stable sample tracks. `FakePlayer` records the current
track and reports playback events without audio. `Memory` stores settings only
until process exit and remains available for tests. These make UI and
application work possible without music files or a board.

`File` implements the same Storage interface and is used by both entry points.
It stores the complete `SettingsState` as compact JSON. The default path is
`openipod/settings.json` under `os.UserConfigDir()`; `--settings-file` overrides
it. A missing file uses default settings, including volume 50%; invalid JSON
or a read error prevents startup. Missing fields retain their default values.
Saving creates parent directories, writes and syncs a temporary file in the
same directory, then renames it over the destination.

App owns the save schedule: each settings change restarts a 500 ms timer, so
rapid changes are combined into one write after changes stop. Returning to the
last saved values cancels the pending write. Navigation and rendering do not
save settings. The reducer emits a save command; App schedules it and calls
Storage when the timer fires or pending changes need flushing on shutdown.

`Filesystem` implements the same Library interface and is selected by
`--music-dir` in both entry points. It stores a versioned, root-specific index at
`<cleaned-absolute-music-root>/.openipod-library.json`. A missing index causes
the first `Load` to scan recursively and atomically save the result; subsequent
loads read the index only, without walking the directory or statting tracks.
`Refresh` performs that scan and atomic replacement synchronously. It filters
regular files by extension, then recognizes audio from their contents in
`internal/library`. MP3 and AAC/ADTS require two consecutive frames; FLAC,
RIFF/WAVE, and Ogg require mandatory audio headers. MP4/M4A requires an audio
handler and media data, using selective box parsing with
`github.com/abema/go-mp4`. MP3 framing uses `github.com/tcolgate/mp3`.
Optional song metadata is not interpreted and full audio integrity is not
checked; duration is not read. Unrecognized files are skipped with a
diagnostic warning containing the path and reason. It returns tracks sorted by
path. Track IDs are
SHA-256 hashes of cleaned absolute paths. Scans observe context cancellation
and return no partial list after a read or save error. Malformed, mismatched,
or incompatible indexes are errors rather than reasons to rescan. It owns no
application state, starts no goroutines, and uses no background scan or
filesystem watcher. The terminal command defaults to the fake library without
a directory; the window command defaults to the filesystem library at `~/Music`.

`input.Controls` now processes caller-supplied EC11 A/B and button snapshots
with caller-supplied time. A full-cycle quadrature decoder handles rotation;
stable-level debouncing handles button presses. It returns existing app events
and owns no goroutines or application state. See [control signals](controls.md)
for the signal contract, assumptions, and future GPIO integration.

An Orange Pi input adapter can later feed real GPIO signals into that processor.
The current image renderer is hardware-independent; a future display adapter
can transfer its RGBA output to a physical display. The window preview exercises
the existing app and MP3 player without board-specific display drivers or
deployment.
