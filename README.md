# OpeniPod

OpeniPod is a team learning project for a standalone music player inspired by the iPod. The intended device uses an Orange Pi Zero, a small display, physical controls, local music, and Linux. The desktop app has a working audio player; hardware support is not yet implemented.

The desktop app provides a terminal UI and an interactive graphical preview,
keyboard input, a filesystem music library, real MP3 playback, and file-backed
volume and equalizer settings. Fake adapters remain available for tests.

## Build and run

Go 1.23 or newer is required. On Linux, building requires a C compiler, `pkg-config`, and ALSA development headers. Install the dependencies for your distribution:

```sh
# Debian or Ubuntu
sudo apt install build-essential pkg-config libasound2-dev

# Fedora
sudo dnf install gcc pkgconf-pkg-config alsa-lib-devel
```

Then build or run:

```sh
go build ./cmd/player
go run ./cmd/player
# or
make run
```

`--platform desktop` is the default and only supported platform. An unsupported value produces an error. Run in a terminal for immediate single-key input. Piped input also works for smoke tests.

To load music from a directory and its subdirectories:

```sh
go run ./cmd/player --music-dir "/path/to/Music"
```

The filesystem library stores a versioned, root-specific index at
`<cleaned-absolute-music-root>/.openipod-library.json`. On the first load, a
missing index triggers a recursive scan and an atomic index save. Later loads
read the index only; they do not walk the directory or stat tracks. New,
changed, or deleted files appear after opening **Settings** from Home with
`j` or ↓, pressing Enter, choosing **Update Library**, and pressing Enter
again. The update performs one synchronous scan and atomically replaces the
index; it is cancelled by Ctrl+C.
There is no background scan or filesystem watcher.

The library indexes regular files with `.mp3`, `.flac`, `.wav`, `.ogg`, `.m4a`,
and `.aac` extensions, ignoring case; only MP3 files can currently be played.
Titles are filenames including extensions; artist and album metadata are not
read. When an MP3 starts playing, the decoder supplies its duration so Now
Playing can show the total time and progress.
Files are sorted lexicographically by path within the directory. Each track
has an absolute, cleaned path and a stable ID derived from that path. Renaming
or moving a file changes its ID. Symbolic links inside the directory are
skipped. The music root must be writable for the first scan and manual
updates; cached startup loads only need a readable index.

During scans, extensions only select candidates; the contents must also be
recognized as supported audio: MP3 (MPEG Layer III), native FLAC, RIFF/WAVE,
Ogg (Vorbis, Opus, or FLAC), M4A/MP4 with an audio track, or AAC with ADTS
framing. MP3 and AAC require two consecutive audio frames; the other
recognizers check mandatory format headers. MP4 parsing uses
`github.com/abema/go-mp4`, and MP3 uses `github.com/tcolgate/mp3`.
Unrecognized files (including text renamed to an audio extension and video-only
MP4 renamed to `.m4a`) are skipped with a warning containing the path and
reason. Missing song tags do not prevent recognition. This recognizes the
format without decoding or checking the entire file for damage. Existing index
entries are checked only after Update Library.

An empty directory or one containing only unsupported files gives an empty
track list. A missing directory, a file supplied as the directory, or a read
error displays a library error without loading a partial list. A malformed,
root-mismatched, or incompatible index is a visible startup error; the
application does not silently rescan it. Choose Update Library to recover.
Refresh failures retain the current in-memory track list and saved index.

Without `--music-dir` (or with an empty value), the four sample tracks remain
available for development.

Settings are restored on startup and saved 500 ms after the last change.
Pending changes are saved before shutdown. On Linux the default
file is `$XDG_CONFIG_HOME/openipod/settings.json`, or
`$HOME/.config/openipod/settings.json` when `XDG_CONFIG_HOME` is unset.
Use `--settings-file /path/to/settings.json` to choose another location.
A missing file starts with default settings (volume 50%, equalizer off); an invalid file causes
a startup error. Parent directories are created on the first save.

In **Settings**, move from **Update Library** down to **Equalizer** with `j` or
↓. Enter cycles **Off → Bass boost → Vocal → Off**. The selected preset applies
to the current and subsequent tracks without restarting playback or changing
the volume setting. It also works while paused and is restored after a restart.
Older settings files without an equalizer field use Off; an unknown preset
falls back to Off.

Off leaves audio unchanged. Bass boost emphasizes low frequencies; Vocal
emphasizes the speech range. Enabled presets reserve headroom to avoid overload,
so their overall loudness can be lower than Off. No additional audio dependency
is required.

Diagnostics default to a separate JSON file at
`$XDG_STATE_HOME/openipod/player.log` (or
`$HOME/.local/state/openipod/player.log`). Use `--log-level=debug` for more
detail, and `--log-file` to choose a development file. Linux journal setup,
retention and viewing commands are in [diagnostic logging](docs/logging.md).

## Display previews

Run the existing 240×320 renderer in an interactive desktop window:

```sh
make preview
# Choose your music directory and initial window scale:
go run ./cmd/preview --music-dir "/path/to/Music" --scale 2
```

On Linux the window uses X11; run it from your graphical desktop session
(`DISPLAY` must be set). Under Wayland it needs XWayland. It uses Shiny's Go
X11 driver and needs no additional native GUI development packages on Linux.
The initial size is 480×640 (scale 2); `--scale` accepts 1..4. Resizing keeps the
original display's aspect ratio and adds black borders when necessary.

The window uses the real audio player and filesystem library. The default
music directory is `~/Music`; if it is missing, the UI shows a library error.
Choose another directory with `--music-dir`. MP3 playback, manual library
updates, volume, and equalizer use the same App as the terminal version.
`--settings-file` works here too; by default both commands share settings.
Avoid running both at the same time with the same settings file.

Focus the window and use the keys in the controls table below. Close the
window, press `q`, or use Ctrl+C to quit and save pending settings. Mouse
navigation and on-screen buttons are not implemented.

The 240×320 display renderer uses an embedded font and can be tested without
hardware. Generate PNG examples for each screen, an empty library, and error
states with:

```sh
make screenshots
```

The images are written to `examples/screens/`.

## Controls

Software processing of EC11 rotation and encoder, Back, and Play/Pause buttons
is available separately from the keyboard backend. Tests supply signal levels
and time; real GPIO is not connected yet. See [control signals](docs/controls.md)
for the decoder policy and integration API.

| Key | Action |
| --- | --- |
| `j` / ↓ | Move down |
| `k` / ↑ | Move up |
| Enter | Select |
| `b` / Esc | Back to Home |
| `p` / Space | Play or pause |
| `n` / → | Next track |
| `h` / ← | Previous track |
| `+` / `-` | Volume by 10% |
| `q` | Quit |

Open **Tracks** and choose a real MP3 from the filesystem library to play it
and view its progress in **Now Playing**. The terminal command's sample library
contains no audio files. Library updates run synchronously and replace the track
list on success. Volume settings persist across restarts.

## Checks

```sh
go test ./...
go test -race ./...
go vet ./...
make check
```

`make fmt` formats Go files. The Makefile expects `go` and `gofmt` in `PATH`.

## Architecture

```text
keyboard ──typed events──> App loop ──AppState──> terminal UI
                            │  ▲
                            │  └──player events
                            ├──Library.Load()
                            ├──Player commands
                            └──Storage settings
```

`internal/app` owns mutable state and chooses side effects. Adapters do not edit it. The composition root is `cmd/player/main.go`. See [architecture](docs/architecture.md), [development](docs/development.md), [testing](docs/testing.md), [diagnostic logging](docs/logging.md), and [contributing](CONTRIBUTING.md).
