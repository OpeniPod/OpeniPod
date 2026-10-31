package main

import (
	"context"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/OpeniPod/OpeniPod/internal/app"
	"github.com/OpeniPod/OpeniPod/internal/player"
	"github.com/OpeniPod/OpeniPod/internal/storage"
)

func TestParseOptions(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configDir)
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}

	t.Run("defaults", func(t *testing.T) {
		got, err := parseOptions(nil)
		if err != nil {
			t.Fatal(err)
		}
		if got.musicDir != filepath.Join(homeDir, "Music") {
			t.Errorf("musicDir = %q, want %q", got.musicDir, filepath.Join(homeDir, "Music"))
		}
		if got.settingsFile != filepath.Join(configDir, "openipod", "settings.json") {
			t.Errorf("settingsFile = %q, want %q", got.settingsFile, filepath.Join(configDir, "openipod", "settings.json"))
		}
		if got.scale != 2 {
			t.Errorf("scale = %d, want 2", got.scale)
		}
	})

	t.Run("explicit flags", func(t *testing.T) {
		got, err := parseOptions([]string{
			"--music-dir", "/tmp/music",
			"--settings-file", "/tmp/preferences.json",
			"--scale", "4",
		})
		if err != nil {
			t.Fatal(err)
		}
		want := options{musicDir: "/tmp/music", settingsFile: "/tmp/preferences.json", scale: 4}
		if got != want {
			t.Errorf("parseOptions() = %+v, want %+v", got, want)
		}
	})

	for _, scale := range []string{"1", "4"} {
		t.Run("scale boundary "+scale, func(t *testing.T) {
			got, err := parseOptions([]string{"--scale", scale})
			if err != nil {
				t.Fatal(err)
			}
			if got.scale != map[string]int{"1": 1, "4": 4}[scale] {
				t.Errorf("scale = %d, want %s", got.scale, scale)
			}
		})
	}

	for _, scale := range []string{"0", "5"} {
		t.Run("scale out of range "+scale, func(t *testing.T) {
			_, err := parseOptions([]string{"--scale", scale})
			if err == nil || !strings.Contains(err.Error(), "scale must be between 1 and 4") {
				t.Errorf("parseOptions() error = %v, want scale range error", err)
			}
		})
	}

	t.Run("help", func(t *testing.T) {
		_, err := parseOptions([]string{"--help"})
		if !errors.Is(err, flag.ErrHelp) {
			t.Errorf("parseOptions() error = %v, want flag.ErrHelp", err)
		}
	})

	t.Run("unexpected argument", func(t *testing.T) {
		_, err := parseOptions([]string{"extra"})
		if err == nil || !strings.Contains(err.Error(), `unexpected argument "extra"`) {
			t.Errorf("parseOptions() error = %v, want unexpected argument error", err)
		}
	})
}

func TestRunApplicationLoadsAndPlaysFilesystemTrack(t *testing.T) {
	musicDir := t.TempDir()
	fixture, err := os.ReadFile(filepath.Join("..", "..", "internal", "library", "testdata", "valid.mp3"))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"02 Second Song.mp3", "01 First Song.mp3"} {
		if err := os.WriteFile(filepath.Join(musicDir, name), fixture, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	renderer := newStateRenderer()
	audio := player.NewFake()
	input := previewInputFunc(func(ctx context.Context, events chan<- app.Event) error {
		if err := waitForState(ctx, renderer.states, func(state app.AppState) bool {
			return len(state.Library.Tracks) == 2
		}); err != nil {
			return err
		}
		if err := sendPreviewEvent(ctx, events, app.SelectPressed{}); err != nil {
			return err
		}
		if err := waitForState(ctx, renderer.states, func(state app.AppState) bool {
			return state.Screen == app.ScreenTracks && len(state.Library.Tracks) == 2
		}); err != nil {
			return err
		}
		if err := sendPreviewEvent(ctx, events, app.SelectPressed{}); err != nil {
			return err
		}
		if err := waitForState(ctx, renderer.states, func(state app.AppState) bool {
			return state.Screen == app.ScreenNowPlaying && state.Player.Status == app.StatusPlaying &&
				state.Player.Track.Title == "01 First Song.mp3"
		}); err != nil {
			return err
		}
		return sendPreviewEvent(ctx, events, app.QuitRequested{})
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := runApplication(ctx, options{musicDir: musicDir, settingsFile: filepath.Join(t.TempDir(), "settings.json")}, audio, input, renderer); err != nil {
		t.Fatal(err)
	}
	if ctx.Err() != nil {
		t.Fatal("application timed out before playback completed")
	}
	if !audio.Playing {
		t.Fatal("fake player is not playing after selecting a library track")
	}
	if audio.Track.Title != "01 First Song.mp3" || audio.Track.Path != filepath.Join(musicDir, "01 First Song.mp3") {
		t.Fatalf("fake player track = %+v, want first sorted file", audio.Track)
	}
}

func TestRunApplicationRestoresSettingsAndSavesPendingChangesOnQuit(t *testing.T) {
	musicDir := t.TempDir()
	settingsFile := filepath.Join(t.TempDir(), "openipod", "settings.json")
	firstRenderer := newStateRenderer()
	firstInput := previewInputFunc(func(ctx context.Context, events chan<- app.Event) error {
		for _, event := range []app.Event{
			app.Scroll{Delta: 2}, // Settings on the home screen.
			app.SelectPressed{},
			app.Scroll{Delta: 1}, // Equalizer setting.
			app.SelectPressed{},
			app.VolumeChanged{Delta: 20},
			app.QuitRequested{},
		} {
			if err := sendPreviewEvent(ctx, events, event); err != nil {
				return err
			}
		}
		return nil
	})

	firstCtx, cancelFirst := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancelFirst()
	firstPlayer := player.NewFake()
	if err := runApplication(firstCtx, options{musicDir: musicDir, settingsFile: settingsFile}, firstPlayer, firstInput, firstRenderer); err != nil {
		t.Fatal(err)
	}
	if firstCtx.Err() != nil {
		t.Fatal("first application run timed out")
	}
	gotSettings, err := storage.NewFile(settingsFile).LoadSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	wantSettings := app.SettingsState{Volume: 70, Equalizer: app.EqualizerBassBoost}
	if gotSettings != wantSettings {
		t.Fatalf("saved settings = %+v, want %+v", gotSettings, wantSettings)
	}

	secondRenderer := newStateRenderer()
	secondPlayer := player.NewFake()
	secondInput := previewInputFunc(func(ctx context.Context, events chan<- app.Event) error {
		if err := waitForState(ctx, secondRenderer.states, func(state app.AppState) bool {
			return state.Settings == wantSettings
		}); err != nil {
			return err
		}
		return sendPreviewEvent(ctx, events, app.QuitRequested{})
	})
	secondCtx, cancelSecond := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancelSecond()
	if err := runApplication(secondCtx, options{musicDir: musicDir, settingsFile: settingsFile}, secondPlayer, secondInput, secondRenderer); err != nil {
		t.Fatal(err)
	}
	if secondCtx.Err() != nil {
		t.Fatal("second application run timed out")
	}
	if secondPlayer.Volume != wantSettings.Volume || secondPlayer.Equalizer != wantSettings.Equalizer {
		t.Fatalf("restored player settings = volume %d, equalizer %q; want %+v", secondPlayer.Volume, secondPlayer.Equalizer, wantSettings)
	}
}

func TestRunApplicationShowsMissingLibraryError(t *testing.T) {
	missingDir := filepath.Join(t.TempDir(), "missing-music")
	renderer := newStateRenderer()
	input := previewInputFunc(func(ctx context.Context, events chan<- app.Event) error {
		if err := waitForState(ctx, renderer.states, func(state app.AppState) bool {
			return strings.Contains(state.Error, "stat library root")
		}); err != nil {
			return err
		}
		return sendPreviewEvent(ctx, events, app.QuitRequested{})
	})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := runApplication(ctx, options{musicDir: missingDir, settingsFile: filepath.Join(t.TempDir(), "settings.json")}, player.NewFake(), input, renderer); err != nil {
		t.Fatal(err)
	}
	if ctx.Err() != nil {
		t.Fatal("application timed out before rendering the missing-library error")
	}
}

type previewInputFunc func(context.Context, chan<- app.Event) error

func (f previewInputFunc) Run(ctx context.Context, events chan<- app.Event) error {
	return f(ctx, events)
}

type stateRenderer struct {
	states chan app.AppState
}

func newStateRenderer() *stateRenderer {
	return &stateRenderer{states: make(chan app.AppState, 32)}
}

func (r *stateRenderer) Render(state app.AppState) error {
	r.states <- state
	return nil
}

func waitForState(ctx context.Context, states <-chan app.AppState, matches func(app.AppState) bool) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case state := <-states:
			if matches(state) {
				return nil
			}
		}
	}
}

func sendPreviewEvent(ctx context.Context, events chan<- app.Event, event app.Event) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case events <- event:
		return nil
	}
}
