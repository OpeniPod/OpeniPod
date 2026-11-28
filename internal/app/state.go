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
	HomeIndex     int
	TrackIndex    int
	SettingsIndex int
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

// Keep the existing Update Library and Equalizer row indices stable.
const (
	SettingsUpdateLibrary = iota
	SettingsEqualizer
	SettingsVolume
	SettingsRepeat
	SettingsShuffle
)

type RepeatMode string

const (
	RepeatOff RepeatMode = ""
	RepeatAll RepeatMode = "all"
	RepeatOne RepeatMode = "one"
)

type SettingsState struct {
	Volume    int             `json:"volume"`
	Equalizer EqualizerPreset `json:"equalizer,omitempty"`
	Repeat    RepeatMode      `json:"repeat,omitempty"`
	Shuffle   bool            `json:"shuffle,omitempty"`
}

// DefaultSettings supplies defaults, including fields absent from older files.
func DefaultSettings() SettingsState {
	return SettingsState{Volume: 50}
}

type AppState struct {
	Screen     Screen
	Navigation NavigationState
	Player     PlayerState
	Library    LibraryState
	Settings   SettingsState
	Error      string

	// Session state, not persisted settings. PRNG inputs keep Reduce deterministic.
	playbackOrder     []music.TrackID
	shuffleSeed       uint64
	shuffleGeneration uint64
}

func (s AppState) clone() AppState {
	s.Library.Tracks = append([]music.Track(nil), s.Library.Tracks...)
	s.playbackOrder = append([]music.TrackID(nil), s.playbackOrder...)
	return s
}
