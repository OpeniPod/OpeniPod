package app

import (
	"time"

	"github.com/OpeniPod/OpeniPod/internal/music"
)

type Screen uint8

const (
	ScreenHome Screen = iota
	ScreenTracks
	ScreenNowPlaying
	ScreenSettings
)

type PlaybackStatus uint8

const (
	StatusStopped PlaybackStatus = iota
	StatusStarting
	StatusPlaying
	StatusPaused
)

type NavigationState struct {
	HomeIndex  int
	TrackIndex int
}

type PlayerState struct {
	Track    music.Track
	HasTrack bool
	Status   PlaybackStatus
	Position time.Duration
}

type LibraryState struct {
	Tracks []music.Track
}

type SettingsState struct {
	Volume int
}

type AppState struct {
	Screen     Screen
	Navigation NavigationState
	Player     PlayerState
	Library    LibraryState
	Settings   SettingsState
	Error      string
}

func (s AppState) clone() AppState {
	s.Library.Tracks = append([]music.Track(nil), s.Library.Tracks...)
	return s
}
