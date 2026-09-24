# OpeniPod

OpeniPod is a team learning project for a standalone music player inspired by the iPod. The intended device uses an Orange Pi Zero, a small display, physical controls, local music, and Linux. This repository currently contains an **initial architecture prototype**, not a finished player.

The runnable desktop slice has a terminal UI, keyboard input, four sample tracks, a fake player that emits playback events without sound, and in-memory volume settings. It demonstrates the application flow while the hardware and real audio work remain separate.

## Build and run

Go 1.23 or newer is required. On Linux:

```sh
go build ./cmd/player
go run ./cmd/player
# or
make run
```

`--platform desktop` is the default and only supported platform. An unsupported value produces an error. Run in a terminal for immediate single-key input. Piped input also works for smoke tests.

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

Open **Tracks**, choose a sample track, then view its fake playback state in **Now Playing**. No audio is produced. Volume settings live only for the current process.

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
