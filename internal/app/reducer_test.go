package app

import (
	"errors"
	"testing"

	"github.com/OpeniPod/OpeniPod/internal/music"
)

func TestScrollClampsSelection(t *testing.T) {
	for _, tc := range []struct {
		name   string
		screen Screen
		start  int
		delta  int
		want   int
	}{
		{"home moves down", ScreenHome, 0, 1, 1},
		{"home lower bound", ScreenHome, 0, -1, 0},
		{"home upper bound", ScreenHome, 2, 1, 2},
		{"tracks move down", ScreenTracks, 0, 1, 1},
		{"tracks lower bound", ScreenTracks, 0, -3, 0},
		{"tracks upper bound", ScreenTracks, 1, 5, 1},
		{"empty tracks", ScreenTracks, 0, 1, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := AppState{
				Screen:     tc.screen,
				Navigation: NavigationState{HomeIndex: tc.start, TrackIndex: tc.start},
			}
			if tc.name != "empty tracks" {
				state.Library.Tracks = []music.Track{{ID: "one"}, {ID: "two"}}
			}
			got, _ := Reduce(state, Scroll{Delta: tc.delta})
			selection := got.Navigation.HomeIndex
			if tc.screen == ScreenTracks {
				selection = got.Navigation.TrackIndex
			}
			if selection != tc.want {
				t.Fatalf("selection = %d, want %d", selection, tc.want)
			}
		})
	}
}

func TestVolumeClampsAndRequestsEffects(t *testing.T) {
	for _, tc := range []struct {
		name   string
		start  int
		delta  int
		want   int
		change bool
	}{
		{"increase", 50, 10, 60, true},
		{"upper bound", 95, 10, 100, true},
		{"lower bound", 5, -10, 0, true},
		{"already at bound", 100, 10, 100, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := AppState{Settings: SettingsState{Volume: tc.start}}
			got, commands := Reduce(state, VolumeChanged{Delta: tc.delta})
			if got.Settings.Volume != tc.want {
				t.Fatalf("volume = %d, want %d", got.Settings.Volume, tc.want)
			}
			if tc.change && len(commands) != 2 {
				t.Fatalf("got %d commands, want set volume and save settings", len(commands))
			}
			if !tc.change && len(commands) != 0 {
				t.Fatalf("got %d commands at bound, want none", len(commands))
			}
		})
	}
}

func TestPlaybackEvents(t *testing.T) {
	track := music.Track{ID: "digital-love", Title: "Digital Love"}
	state, _ := Reduce(AppState{}, PlaybackStarted{Track: track})
	if !state.Player.HasTrack || state.Player.Track.ID != track.ID || state.Player.Status != StatusPlaying {
		t.Fatalf("playback start state = %+v", state.Player)
	}
	state, _ = Reduce(state, PlaybackPaused{})
	if state.Player.Status != StatusPaused {
		t.Fatalf("status after pause = %v", state.Player.Status)
	}
	state, _ = Reduce(state, PlaybackResumed{})
	if state.Player.Status != StatusPlaying {
		t.Fatalf("status after resume = %v", state.Player.Status)
	}
}

func TestLibraryLoadedCopiesTracks(t *testing.T) {
	tracks := []music.Track{{ID: "one", Title: "One"}}
	state, _ := Reduce(AppState{Error: "old failure"}, LibraryLoaded{Tracks: tracks})
	tracks[0].Title = "Changed outside App"
	if len(state.Library.Tracks) != 1 || state.Library.Tracks[0].Title != "One" || state.Error != "" {
		t.Fatalf("library state = %+v", state.Library)
	}
}

func TestHomeTracksBack(t *testing.T) {
	state, _ := Reduce(AppState{Screen: ScreenHome}, SelectPressed{})
	if state.Screen != ScreenTracks {
		t.Fatalf("select from home led to %v", state.Screen)
	}
	state, _ = Reduce(state, BackPressed{})
	if state.Screen != ScreenHome {
		t.Fatalf("back led to %v", state.Screen)
	}
}

func TestSelectingTrackRequestsPlayback(t *testing.T) {
	track := music.Track{ID: "one", Title: "One"}
	state := AppState{Screen: ScreenTracks, Library: LibraryState{Tracks: []music.Track{track}}}
	got, commands := Reduce(state, SelectPressed{})
	if got.Screen != ScreenNowPlaying || got.Player.Status != StatusStarting {
		t.Fatalf("selected track state = %+v", got)
	}
	if len(commands) != 1 {
		t.Fatalf("got %d commands, want one", len(commands))
	}
	play, ok := commands[0].(playTrack)
	if !ok || play.track.ID != track.ID {
		t.Fatalf("command = %#v, want play track", commands[0])
	}
}

func TestLibraryFailureIsVisible(t *testing.T) {
	state, _ := Reduce(AppState{}, LibraryLoadFailed{Err: errors.New("sample error")})
	if state.Error != "sample error" {
		t.Fatalf("error = %q", state.Error)
	}
}
