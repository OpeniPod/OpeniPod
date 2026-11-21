package app

import (
	"reflect"
	"testing"

	"github.com/OpeniPod/OpeniPod/internal/music"
)

func TestItemActivatedSelectsHomeAndSettingsRows(t *testing.T) {
	home, commands := Reduce(AppState{
		Screen:     ScreenHome,
		Navigation: NavigationState{HomeIndex: 0},
	}, ItemActivated{Screen: ScreenHome, Index: 2})
	if home.Screen != ScreenSettings || home.Navigation.HomeIndex != 2 || len(commands) != 0 {
		t.Fatalf("home activation state = %+v, commands = %#v", home, commands)
	}

	settings, commands := Reduce(AppState{
		Screen:     ScreenSettings,
		Navigation: NavigationState{SettingsIndex: 0},
	}, ItemActivated{Screen: ScreenSettings, Index: 1})
	if settings.Navigation.SettingsIndex != 1 || settings.Settings.Equalizer != EqualizerBassBoost {
		t.Fatalf("settings activation state = %+v", settings)
	}
	if !reflect.DeepEqual(commands, []command{setEqualizer{EqualizerBassBoost}, saveSettings{}}) {
		t.Fatalf("settings activation commands = %#v", commands)
	}
}

func TestItemActivatedStartsSelectedTrack(t *testing.T) {
	tracks := []music.Track{{ID: "first"}, {ID: "second"}, {ID: "third"}}
	state, commands := Reduce(AppState{
		Screen:     ScreenTracks,
		Navigation: NavigationState{TrackIndex: 0},
		Library:    LibraryState{Tracks: tracks},
	}, ItemActivated{Screen: ScreenTracks, Index: 2})

	if state.Navigation.TrackIndex != 2 || state.Screen != ScreenNowPlaying || state.Player.Track.ID != "third" || state.Player.Status != StatusStarting {
		t.Fatalf("track activation state = %+v", state)
	}
	if !reflect.DeepEqual(commands, []command{playTrack{track: tracks[2]}}) {
		t.Fatalf("track activation commands = %#v", commands)
	}
}

func TestItemActivatedIgnoresInvalidOrStaleRows(t *testing.T) {
	base := AppState{
		Screen:     ScreenTracks,
		Navigation: NavigationState{TrackIndex: 1},
		Library:    LibraryState{Tracks: []music.Track{{ID: "first"}, {ID: "second"}}},
		Error:      "unchanged",
	}
	for _, event := range []ItemActivated{
		{Screen: ScreenTracks, Index: -1},
		{Screen: ScreenTracks, Index: 2},
		{Screen: ScreenSettings, Index: 0},
		{Screen: ScreenNowPlaying, Index: 0},
	} {
		got, commands := Reduce(base, event)
		if !reflect.DeepEqual(got, base) || len(commands) != 0 {
			t.Errorf("event %#v changed state to %+v or commands to %#v", event, got, commands)
		}
	}
}

func TestKeyboardSelectStillWorksAfterScroll(t *testing.T) {
	state := AppState{Screen: ScreenHome}
	state, commands := Reduce(state, Scroll{Delta: 1})
	if len(commands) != 0 || state.Navigation.HomeIndex != 1 {
		t.Fatalf("scroll state = %+v, commands = %#v", state, commands)
	}

	state, commands = Reduce(state, SelectPressed{})
	if state.Screen != ScreenNowPlaying || len(commands) != 0 {
		t.Fatalf("keyboard select after scroll state = %+v, commands = %#v", state, commands)
	}
}
