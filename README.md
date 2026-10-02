# OpeniPod

OpeniPod is a team learning project for a standalone music player inspired by the iPod. The intended device uses an Orange Pi Zero, a small display, physical controls, local music, and Linux. This repository currently contains an **initial architecture prototype**, not a finished player.

The runnable desktop slice has a terminal UI, keyboard input, a filesystem or sample music library, a fake player that emits playback events without sound, and in-memory volume settings. It demonstrates the application flow while the hardware and real audio work remain separate.

## Build and run

Go 1.23 or newer is required. On Linux:

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

The library includes regular files with `.mp3`, `.flac`, `.wav`, `.ogg`, `.m4a`,
and `.aac` extensions, ignoring case. Titles are filenames including extensions;
tags and duration are not read. Files are sorted lexicographically by path
within the directory. Each track has an absolute, cleaned path and a stable
ID derived from that path. Renaming or moving a file changes its ID. Symbolic
links inside the directory are skipped. The music root must be writable for
the first scan and manual updates; cached startup loads only need a readable
index.

An empty directory or one containing only unsupported files gives an empty
track list. A missing directory, a file supplied as the directory, or a read
error displays a library error without loading a partial list. A malformed,
root-mismatched, or incompatible index is a visible startup error; the
application does not silently rescan it. Choose Update Library to recover.
Refresh failures retain the current in-memory track list and saved index.

Without `--music-dir` (or with an empty value), the four sample tracks remain
available for development. File selection is based on extensions; audio
contents are not validated or decoded.

## Controls

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

Open **Tracks**, choose a track, then view its fake playback state in **Now
Playing**. No audio is produced. Library updates run synchronously and replace
the track list on success.
Volume settings live only for the current process.

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

`internal/app` owns mutable state and chooses side effects. Adapters do not edit it. The composition root is `cmd/player/main.go`. See [architecture](docs/architecture.md), [development](docs/development.md), [testing](docs/testing.md), and [contributing](CONTRIBUTING.md).
