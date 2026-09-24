package ui

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/OpeniPod/OpeniPod/internal/app"
)

type Terminal struct{ out io.Writer }

func NewTerminal(out io.Writer) *Terminal { return &Terminal{out: out} }

func (t *Terminal) Render(state app.AppState) error {
	var view strings.Builder
	view.WriteString("\x1b[H\x1b[2J")
	switch state.Screen {
	case app.ScreenHome:
		view.WriteString("Music\n\n")
		for i, item := range []string{"Tracks", "Now Playing", "Settings"} {
			mark := "  "
			if i == state.Navigation.HomeIndex {
				mark = "> "
			}
			fmt.Fprintf(&view, "%s%s\n", mark, item)
		}
	case app.ScreenTracks:
		view.WriteString("Tracks\n\n")
		if len(state.Library.Tracks) == 0 {
			view.WriteString("No tracks loaded\n")
		}
		for i, track := range state.Library.Tracks {
			mark := "  "
			if i == state.Navigation.TrackIndex {
				mark = "> "
			}
			fmt.Fprintf(&view, "%s%s — %s\n", mark, track.Title, track.Artist)
		}
	case app.ScreenNowPlaying:
		view.WriteString("Now Playing\n\n")
		if state.Player.HasTrack {
			fmt.Fprintf(&view, "%s\n%s\n\n%s\n\n%s / %s\nVolume: %d%%\n",
				state.Player.Track.Title, state.Player.Track.Artist,
				status(state.Player.Status), clock(state.Player.Position), clock(state.Player.Track.Duration), state.Settings.Volume)
		} else {
			view.WriteString("Select a track to start\n")
		}
	case app.ScreenSettings:
		fmt.Fprintf(&view, "Settings\n\nVolume: %d%%\n", state.Settings.Volume)
	}
	if state.Error != "" {
		fmt.Fprintf(&view, "\nError: %s\n", state.Error)
	}
	view.WriteString("\n j/k or ↑/↓ move · Enter select · b/Esc back\n p/Space play/pause · n/→ next · h/← previous\n +/- volume · q quit\n")
	_, err := io.WriteString(t.out, view.String())
	return err
}

func status(value app.PlaybackStatus) string {
	switch value {
	case app.StatusStarting:
		return "Starting"
	case app.StatusPlaying:
		return "Playing"
	case app.StatusPaused:
		return "Paused"
	default:
		return "Stopped"
	}
}

func clock(value time.Duration) string {
	seconds := int(value / time.Second)
	return fmt.Sprintf("%02d:%02d", seconds/60, seconds%60)
}
