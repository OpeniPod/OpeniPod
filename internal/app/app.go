package app

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

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
	state           AppState
	player          Player
	library         Library
	storage         Storage
	input           Input
	ui              Renderer
	settingsTimer   *time.Timer
	savedSettings   SettingsState
	pendingSettings SettingsState
	settingsDirty   bool
}

func New(player Player, library Library, storage Storage, input Input, ui Renderer) *App {
	return &App{
		state:   AppState{Screen: ScreenHome, Settings: DefaultSettings()},
		player:  player,
		library: library,
		storage: storage,
		input:   input,
		ui:      ui,
	}
}

// Run owns all state mutations. Only input collection runs in a separate goroutine.
func (a *App) Run(ctx context.Context) (result error) {
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
	a.savedSettings = a.state.Settings
	a.settingsDirty = false
	a.settingsTimer = time.NewTimer(time.Hour)
	a.settingsTimer.Stop()
	defer func() {
		a.settingsTimer.Stop()
		// Include a state change even if executing its player command failed.
		a.pendingSettings = a.state.Settings
		a.settingsDirty = a.pendingSettings != a.savedSettings
		// The run context may already be cancelled when shutdown starts.
		flushCtx, flushCancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		defer flushCancel()
		result = errors.Join(result, a.flushSettings(flushCtx))
	}()
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
		case <-a.settingsTimer.C:
			if err := a.flushSettings(runCtx); err != nil {
				if runCtx.Err() != nil {
					return nil
				}
				return err
			}
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
		a.pendingSettings = c.settings
		a.settingsDirty = c.settings != a.savedSettings
		a.settingsTimer.Stop()
		if a.settingsDirty {
			a.settingsTimer.Reset(500 * time.Millisecond)
		}
	}
	return nil
}

func (a *App) flushSettings(ctx context.Context) error {
	if !a.settingsDirty {
		return nil
	}
	if err := a.storage.SaveSettings(ctx, a.pendingSettings); err != nil {
		return fmt.Errorf("save settings: %w", err)
	}
	a.savedSettings = a.pendingSettings
	a.settingsDirty = false
	return nil
}

func (a *App) render() error {
	if err := a.ui.Render(a.state.clone()); err != nil {
		return fmt.Errorf("render UI: %w", err)
	}
	return nil
}
