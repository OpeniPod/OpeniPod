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

Reducer unit tests cover navigation bounds, volume bounds and commands, seeking
and clamping, library loading, playback events, and Back. The reducer does no
I/O, so these tests use plain values. The application integration test feeds
typed input events into a real App with FakeLibrary, FakePlayer, MemoryStorage,
and a small observing UI. It checks that selecting a track reaches FakePlayer
and returns as a rendered playing state; seek events reach the player fake.

Run manual keyboard checks on a laptop or in QEMU: Left and Right seek by five
seconds, Shift+Left and Shift+Right switch tracks, and `h` and `n` switch
tracks. Scroll events remain menu navigation.

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
listening check above to verify real output. The suite runs on the host and does
not count as a direct test of either phone.

## Native window

`internal/desktop` tests exercise key translation, clicks, frame updates, expose,
resize and aspect ratio, native driver errors, cancellation during input,
and concurrent rendering. A test driver replaces the native window, while
rendering uses the existing `ui.Display`. Command integration tests supply
FakePlayer, typed App input, and a capturing renderer to check filesystem
loading, playback requests, and saved settings without opening an audio device
or a display in CI.

For a manual check, run `go run ./cmd/player --music-dir /path/to/Music` from
an X11 desktop session or Wayland with XWayland. Navigate Home, Tracks, Now
Playing, and Settings; play and pause a real MP3; change volume and equalizer;
select Update Library. On a laptop or in QEMU, verify Left and Right seek by
five seconds, Shift+Left and Shift+Right switch tracks, and `h` and `n` work as
previous/next aliases. Scroll remains menu navigation. Resize and cover/uncover
the window, checking that the image retains its aspect ratio and redraws. Close
using the window manager and restart to check settings persistence. Also check
`q`, Ctrl+C, and SIGTERM shutdown. GUI smoke tests require a display and audio
device and are not part of `make check`.

Repeat the flow using the [on-screen controls](controls.md#on-screen-controls).
Load at least nine tracks, page down twice, select a track on the final page,
pause and resume, change volume and return Home. After resizing the window,
check the edges of a button and the black borders. A drag should leave the
playing track unchanged.

## Runtime test matrix

| Target | Checks |
| --- | --- |
| Laptop | Run `make check`, then do the native-window and listening checks above. Verify keyboard navigation, an X11 or XWayland session, and ALSA playback. |
| Nura in QEMU (x86-64) | Build and run inside the [VM](phone.md#nura-in-qemu). Check XWayland, keyboard and mouse navigation, playback through the virtual audio device, and persistence across app restarts. |
| Redmi 4X (`santoni`) and Pixel 4a (`sunfish`) | Build for ARM64 in [Nura](phone.md#build-and-launch-in-nura). Check X11/XWayland, a connected keyboard, playback on the available audio outputs, and recovery after screen lock. Record each model and Nura version tested. |

Check touch delivery on the phone. The VM uses virtual hardware. Audio
routing, physical inputs, and suspend behavior depend on the device.

## Diagnostic logging

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
