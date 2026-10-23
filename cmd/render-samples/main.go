package main

import (
	"errors"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"time"

	"github.com/OpeniPod/OpeniPod/internal/app"
	"github.com/OpeniPod/OpeniPod/internal/library"
	"github.com/OpeniPod/OpeniPod/internal/music"
	"github.com/OpeniPod/OpeniPod/internal/ui"
)

func main() {
	outputDir := "examples/screens"
	if len(os.Args) > 2 {
		fmt.Fprintln(os.Stderr, "usage: render-samples [output-directory]")
		os.Exit(2)
	}
	if len(os.Args) == 2 {
		outputDir = os.Args[1]
	}
	if err := renderSamples(outputDir); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func renderSamples(outputDir string) (result error) {
	display, err := ui.NewDisplay()
	if err != nil {
		return fmt.Errorf("create display renderer: %w", err)
	}
	defer func() {
		result = errors.Join(result, display.Close())
	}()
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return fmt.Errorf("create screenshot directory %q: %w", outputDir, err)
	}

	tracks := append(library.NewFake().Tracks, music.Track{
		ID:       "cyrillic-demo",
		Title:    "Ночные огни — длинное название для проверки переноса текста",
		Artist:   "Исполнитель",
		Duration: 4 * time.Minute,
	})
	nowPlaying := app.AppState{
		Screen: app.ScreenNowPlaying,
		Player: app.PlayerState{
			Track:    tracks[len(tracks)-1],
			HasTrack: true,
			Status:   app.StatusPlaying,
			Position: 92 * time.Second,
		},
		Settings: app.SettingsState{Volume: 70},
	}

	samples := []struct {
		name  string
		state app.AppState
	}{
		{name: "home.png", state: app.AppState{
			Screen:     app.ScreenHome,
			Navigation: app.NavigationState{HomeIndex: 0},
			Library:    app.LibraryState{Tracks: tracks},
		}},
		{name: "tracks.png", state: app.AppState{
			Screen:     app.ScreenTracks,
			Navigation: app.NavigationState{TrackIndex: len(tracks) - 1},
			Library:    app.LibraryState{Tracks: tracks},
		}},
		{name: "now-playing.png", state: nowPlaying},
		{name: "settings.png", state: app.AppState{
			Screen:     app.ScreenSettings,
			Navigation: app.NavigationState{SettingsIndex: 0},
			Settings:   app.SettingsState{Volume: 65},
			Library:    app.LibraryState{Tracks: tracks},
		}},
		{name: "empty-library.png", state: app.AppState{Screen: app.ScreenTracks}},
		{name: "library-error.png", state: app.AppState{
			Screen:   app.ScreenSettings,
			Settings: app.SettingsState{Volume: 50},
			Error:    "Не удалось обновить библиотеку: папка недоступна",
		}},
		{name: "playback-error.png", state: app.AppState{
			Screen:   app.ScreenNowPlaying,
			Player:   nowPlaying.Player,
			Settings: nowPlaying.Settings,
			Error:    "Не удалось открыть аудиофайл",
		}},
	}

	for _, sample := range samples {
		path := filepath.Join(outputDir, sample.name)
		file, err := os.Create(path)
		if err != nil {
			return fmt.Errorf("create screenshot %q: %w", path, err)
		}
		encodeErr := png.Encode(file, display.Render(sample.state))
		closeErr := file.Close()
		if encodeErr != nil || closeErr != nil {
			var sampleErrs []error
			if encodeErr != nil {
				sampleErrs = append(sampleErrs, fmt.Errorf("encode screenshot %q: %w", path, encodeErr))
			}
			if closeErr != nil {
				sampleErrs = append(sampleErrs, fmt.Errorf("close screenshot %q: %w", path, closeErr))
			}
			return errors.Join(sampleErrs...)
		}
	}
	return nil
}
