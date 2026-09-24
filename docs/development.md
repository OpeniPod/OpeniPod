# Development guide

Start with `cmd/player/main.go`: it chooses concrete adapters and calls `App.Run`. Application decisions live in `internal/app`. `state.go` defines what the app knows, `event.go` defines messages into the app, `reducer.go` defines state transitions and commands, and `app.go` owns the loop and executes side effects. `internal/music` holds shared music data.

To add an event, define a concrete type in `event.go`, add its `isEvent` method, handle it in `Reduce`, and add a focused transition test. Map a key or hardware signal to that event in its input adapter. If the event requires I/O, have the reducer return a small command and execute it in `App`; keep I/O out of the reducer.

To add a backend, implement the consumer interface in `internal/app/app.go` and wire the implementation in `cmd/player/main.go`. Keep interfaces small and based on current needs. A real player sends typed player events back to App. A UI backend implements `Render(AppState)` and should only display the supplied state. Business logic belongs in the reducer and App loop, never in UI, input, or hardware code. Avoid globals and direct adapter-to-adapter calls.

Useful commands:

```sh
make run      # desktop prototype
make fmt      # format Go files
make test     # unit and integration tests
make race     # race detector
make vet      # static checks
make check    # formatting, tests, race detector, vet
```

Read [architecture](architecture.md) before changing package boundaries, state ownership, or event flow. Keep a branch and PR focused; see [contributing](../CONTRIBUTING.md).
