package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
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
	Refresh(context.Context) ([]music.Track, error)
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
	logger          *slog.Logger
	settingsTimer   *time.Timer
	savedSettings   SettingsState
	pendingSettings SettingsState
	settingsDirty   bool
}

type Option func(*App)

// WithLogger injects diagnostics without changing the reducer or adapters.
func WithLogger(logger *slog.Logger) Option {
	return func(a *App) {
		if logger != nil {
			a.logger = logger
		}
	}
}

func New(player Player, library Library, storage Storage, input Input, ui Renderer, options ...Option) *App {
	a := &App{
		state:   AppState{Screen: ScreenHome, Settings: DefaultSettings()},
		player:  player,
		library: library,
		storage: storage,
		input:   input,
		ui:      ui,
		logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	for _, option := range options {
		option(a)
	}
	return a
}

// Run owns all state mutations. Only input collection runs in a separate goroutine.
func (a *App) Run(ctx context.Context) (result error) {
	a.logger.Info("application started", "component", "app", "operation", "start")
	stopReason := "input_eof"
	defer func() {
		if ctx.Err() != nil {
			stopReason = "cancelled"
		}
		if result != nil {
			// Fatal errors propagate without being logged at each layer.
			attrs := []any{"component", "app", "operation", "run", "error", result}
			var failure *operationError
			if errors.As(result, &failure) {
				// Keep the complete result: shutdown may have joined a failed
				// settings flush with the original operation failure.
				attrs = append([]any{"component", failure.component, "operation", failure.operation, "error", result}, failure.attrs...)
			}
			a.logger.Error("operation failed", attrs...)
			stopReason = "error"
		}
		a.logger.Info("application stopped", "component", "app", "operation", "stop", "reason", stopReason)
	}()
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
			stopReason = "cancelled"
			return nil
		}
		return failed("storage", "load_settings", err)
	}
	a.logger.Debug("settings loaded", "component", "storage", "operation", "load_settings", "volume", settings.Volume)
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
			stopReason = "cancelled"
			return nil
		}
		return failed("player", "set_volume", err, "volume", a.state.Settings.Volume)
	}
	a.logger.Debug("loading library", "component", "library", "operation", "load")
	tracks, err := a.library.Load(runCtx)
	if err != nil {
		if runCtx.Err() != nil {
			stopReason = "cancelled"
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
					return failed("player", "events", fmt.Errorf("player event stream closed"))
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
			stopReason = "cancelled"
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
				return failed("input", "read", err)
			}
		case event, ok := <-inputEventStream:
			if !ok {
				inputEventStream = nil
				continue
			}
			if _, quit := event.(QuitRequested); quit {
				stopReason = "quit"
				return nil
			}
			if err := a.apply(runCtx, event); err != nil {
				return err
			}
		case event, ok := <-playerEvents:
			if !ok {
				return failed("player", "events", fmt.Errorf("player event stream closed"))
			}
			if err := a.apply(runCtx, event); err != nil {
				return err
			}
		}
	}
}

func (a *App) apply(ctx context.Context, event Event) error {
	if err := a.update(ctx, event); err != nil {
		return err
	}
	if ctx.Err() != nil {
		return nil
	}
	return a.render()
}

// update applies an event and its commands without rendering intermediate states.
func (a *App) update(ctx context.Context, event Event) error {
	a.logEvent(event)
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
	return nil
}

func (a *App) execute(ctx context.Context, command command) error {
	switch c := command.(type) {
	case playTrack:
		if err := a.player.Play(ctx, c.track); err != nil {
			return failed("player", "play", err, "track_id", c.track.ID)
		}
		a.logger.Info("play requested", "component", "player", "operation", "play", "track_id", c.track.ID)
	case pausePlayback:
		if err := a.player.Pause(ctx); err != nil {
			return failed("player", "pause", err, "track_id", a.state.Player.Track.ID)
		}
		a.logger.Info("pause requested", "component", "player", "operation", "pause", "track_id", a.state.Player.Track.ID)
	case resumePlayback:
		if err := a.player.Resume(ctx); err != nil {
			return failed("player", "resume", err, "track_id", a.state.Player.Track.ID)
		}
		a.logger.Info("resume requested", "component", "player", "operation", "resume", "track_id", a.state.Player.Track.ID)
	case setVolume:
		if err := a.player.SetVolume(ctx, c.volume); err != nil {
			return failed("player", "set_volume", err, "volume", c.volume)
		}
		a.logger.Info("volume changed", "component", "player", "operation", "set_volume", "volume", c.volume)
	case saveSettings:
		a.pendingSettings = c.settings
		a.settingsDirty = c.settings != a.savedSettings
		a.settingsTimer.Stop()
		if a.settingsDirty {
			a.settingsTimer.Reset(500 * time.Millisecond)
		}
	case refreshLibrary:
		a.logger.Debug("refreshing library", "component", "library", "operation", "refresh")
		tracks, err := a.library.Refresh(ctx)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			return a.update(ctx, LibraryLoadFailed{Err: fmt.Errorf("update library: %w", err)})
		}
		return a.update(ctx, LibraryLoaded{Tracks: tracks})
	}
	return nil
}

func (a *App) flushSettings(ctx context.Context) error {
	if !a.settingsDirty {
		return nil
	}
	if err := a.storage.SaveSettings(ctx, a.pendingSettings); err != nil {
		return failed("storage", "save_settings", err)
	}
	a.logger.Debug("settings saved", "component", "storage", "operation", "save_settings", "volume", a.pendingSettings.Volume)
	a.savedSettings = a.pendingSettings
	a.settingsDirty = false
	return nil
}

func (a *App) render() error {
	if err := a.ui.Render(a.state.clone()); err != nil {
		return failed("ui", "render", err)
	}
	return nil
}

func (a *App) logEvent(event Event) {
	switch e := event.(type) {
	case LibraryLoaded:
		a.logger.Info("library loaded", "component", "library", "operation", "load", "track_count", len(e.Tracks))
	case LibraryLoadFailed:
		a.logger.Warn("library load failed", "component", "library", "operation", "load", "error", e.Err)
	case PlaybackStarted:
		a.logger.Info("playback started", "component", "player", "operation", "started", "track_id", e.Track.ID)
	case PlaybackPaused:
		a.logger.Info("playback paused", "component", "player", "operation", "paused", "track_id", a.state.Player.Track.ID)
	case PlaybackResumed:
		a.logger.Info("playback resumed", "component", "player", "operation", "resumed", "track_id", a.state.Player.Track.ID)
	case TrackFinished:
		a.logger.Info("track finished", "component", "player", "operation", "finished", "track_id", a.state.Player.Track.ID)
	case NextPressed:
		a.logger.Info("next track requested", "component", "player", "operation", "next", "track_id", a.state.Player.Track.ID)
	case PreviousPressed:
		a.logger.Info("previous track requested", "component", "player", "operation", "previous", "track_id", a.state.Player.Track.ID)
	case PlaybackError:
		a.logger.Error("playback failed", "component", "player", "operation", "playback", "track_id", a.state.Player.Track.ID, "error", e.Err)
	}
}

type operationError struct {
	component string
	operation string
	err       error
	attrs     []any
}

func failed(component, operation string, err error, attrs ...any) error {
	return &operationError{component: component, operation: operation, err: err, attrs: attrs}
}

func (e *operationError) Error() string {
	return fmt.Sprintf("%s %s: %v", e.component, e.operation, e.err)
}

func (e *operationError) Unwrap() error { return e.err }
