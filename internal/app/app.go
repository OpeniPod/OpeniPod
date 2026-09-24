package app

import (
	"context"
	"fmt"
	"sync"

	"github.com/OpeniPod/OpeniPod/internal/music"
)

type Player interface {
	Events() <-chan Event
	Play(context.Context, music.Track) error
	Pause(context.Context) error
	Resume(context.Context) error
	SetVolume(context.Context, int) error
}

type Library interface {
	Load(context.Context) ([]music.Track, error)
}

type Storage interface {
	LoadSettings(context.Context) (SettingsState, error)
	SaveSettings(context.Context, SettingsState) error
}

type Input interface {
	Run(context.Context, chan<- Event) error
}

type Renderer interface {
	Render(AppState) error
}

type App struct {
	state   AppState
	player  Player
	library Library
	storage Storage
	input   Input
	ui      Renderer
}

func New(player Player, library Library, storage Storage, input Input, ui Renderer) *App {
	return &App{
		state:   AppState{Screen: ScreenHome, Settings: SettingsState{Volume: 50}},
		player:  player,
		library: library,
		storage: storage,
		input:   input,
		ui:      ui,
	}
}

// Run owns all state mutations. Only input collection runs in a separate goroutine.
func (a *App) Run(ctx context.Context) error {
	runCtx, cancel := context.WithCancel(ctx)
	var inputWorker sync.WaitGroup
	defer func() {
		cancel()
		inputWorker.Wait()
	}()

	if err := a.render(); err != nil {
		return err
	}
	settings, err := a.storage.LoadSettings(runCtx)
	if err != nil {
		if runCtx.Err() != nil {
			return nil
		}
		return fmt.Errorf("load settings: %w", err)
	}
	if err := a.apply(runCtx, SettingsLoaded{Settings: settings}); err != nil {
		return err
	}
	if err := a.player.SetVolume(runCtx, a.state.Settings.Volume); err != nil {
		if runCtx.Err() != nil {
			return nil
		}
		return fmt.Errorf("set initial volume: %w", err)
	}
	tracks, err := a.library.Load(runCtx)
	if err != nil {
		if runCtx.Err() != nil {
			return nil
		}
		err = a.apply(runCtx, LibraryLoadFailed{Err: fmt.Errorf("load library: %w", err)})
	} else {
		err = a.apply(runCtx, LibraryLoaded{Tracks: tracks})
	}
	if err != nil {
		return err
	}

	inputEvents := make(chan Event, 16)
	inputResult := make(chan error, 1)
	inputWorker.Add(1)
	go func() {
		defer inputWorker.Done()
		err := a.input.Run(runCtx, inputEvents)
		close(inputEvents)
		inputResult <- err
	}()

	playerEvents := a.player.Events()
	var inputEventStream <-chan Event = inputEvents
	var inputResultStream <-chan error = inputResult
	for {
		// A pipe may reach EOF with input and player events still buffered.
		if inputEventStream == nil && inputResultStream == nil {
			select {
			case event, ok := <-playerEvents:
				if !ok {
					return fmt.Errorf("player event stream closed")
				}
				if err := a.apply(runCtx, event); err != nil {
					return err
				}
			default:
				return nil
			}
			continue
		}
		select {
		case <-runCtx.Done():
			return nil
		case err := <-inputResultStream:
			inputResultStream = nil
			if err != nil && runCtx.Err() == nil {
				return fmt.Errorf("input: %w", err)
			}
		case event, ok := <-inputEventStream:
			if !ok {
				inputEventStream = nil
				continue
			}
			if _, quit := event.(QuitRequested); quit {
				return nil
			}
			if err := a.apply(runCtx, event); err != nil {
				return err
			}
		case event, ok := <-playerEvents:
			if !ok {
				return fmt.Errorf("player event stream closed")
			}
			if err := a.apply(runCtx, event); err != nil {
				return err
			}
		}
	}
}

func (a *App) apply(ctx context.Context, event Event) error {
	next, commands := Reduce(a.state, event)
	a.state = next
	for _, command := range commands {
		if err := a.execute(ctx, command); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
	}
	return a.render()
}

func (a *App) execute(ctx context.Context, command command) error {
	switch c := command.(type) {
	case playTrack:
		if err := a.player.Play(ctx, c.track); err != nil {
			return fmt.Errorf("play %q: %w", c.track.Title, err)
		}
	case pausePlayback:
		if err := a.player.Pause(ctx); err != nil {
			return fmt.Errorf("pause playback: %w", err)
		}
	case resumePlayback:
		if err := a.player.Resume(ctx); err != nil {
			return fmt.Errorf("resume playback: %w", err)
		}
	case setVolume:
		if err := a.player.SetVolume(ctx, c.volume); err != nil {
			return fmt.Errorf("set volume: %w", err)
		}
	case saveSettings:
		if err := a.storage.SaveSettings(ctx, c.settings); err != nil {
			return fmt.Errorf("save settings: %w", err)
		}
	}
	return nil
}

func (a *App) render() error {
	if err := a.ui.Render(a.state.clone()); err != nil {
		return fmt.Errorf("render UI: %w", err)
	}
	return nil
}
