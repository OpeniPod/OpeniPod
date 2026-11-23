package app

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/OpeniPod/OpeniPod/internal/music"
)

func playbackState(count int, shuffle bool, repeat RepeatMode) AppState {
	state := AppState{Screen: ScreenTracks, Settings: SettingsState{Volume: 50, Repeat: repeat, Shuffle: shuffle}, shuffleSeed: 42}
	tracks := make([]music.Track, count)
	for i := range tracks {
		tracks[i].ID = music.TrackID(fmt.Sprint(i))
	}
	state, _ = Reduce(state, LibraryLoaded{Tracks: tracks})
	return state
}

func confirmPlayback(t *testing.T, state AppState, commands []command) AppState {
	t.Helper()
	if len(commands) != 1 {
		t.Fatalf("commands = %#v, want exactly one play command", commands)
	}
	play, ok := commands[0].(playTrack)
	if !ok || state.Player.Status != StatusStarting || state.Player.Track.ID != play.track.ID {
		t.Fatalf("play request state = %+v, commands = %#v", state.Player, commands)
	}
	state, _ = Reduce(state, PlaybackStarted{Track: play.track})
	return state
}

func TestSettingsSelectionAndDefaults(t *testing.T) {
	if got := DefaultSettings(); got.Repeat != RepeatOff || got.Shuffle || got.Volume != 50 {
		t.Fatalf("defaults = %+v", got)
	}
	state := AppState{Screen: ScreenSettings, Navigation: NavigationState{SettingsIndex: SettingsVolume}, Settings: DefaultSettings()}
	state, commands := Reduce(state, SelectPressed{})
	if state.Settings.Volume != 60 || len(commands) != 2 {
		t.Fatalf("volume selection = %+v, %v", state.Settings, commands)
	}
	state, _ = Reduce(state, Scroll{Delta: 1})
	for _, want := range []RepeatMode{RepeatAll, RepeatOne, RepeatOff} {
		state, commands = Reduce(state, SelectPressed{})
		if state.Settings.Repeat != want || len(commands) != 1 {
			t.Fatalf("repeat selection = %+v, %v", state.Settings, commands)
		}
		if _, ok := commands[0].(saveSettings); !ok {
			t.Fatalf("new settings were not passed to Storage: %v", commands)
		}
	}
	state, _ = Reduce(state, Scroll{Delta: 1})
	for _, want := range []bool{true, false} {
		state, commands = Reduce(state, SelectPressed{})
		if state.Settings.Shuffle != want || len(commands) != 1 {
			t.Fatalf("shuffle selection = %+v, %v", state.Settings, commands)
		}
	}
	state, _ = Reduce(state, Scroll{Delta: 100})
	if state.Navigation.SettingsIndex != SettingsShuffle {
		t.Fatalf("settings upper bound = %d", state.Navigation.SettingsIndex)
	}
	state, _ = Reduce(state, Scroll{Delta: -100})
	if state.Navigation.SettingsIndex != SettingsUpdateLibrary {
		t.Fatalf("settings lower bound = %d", state.Navigation.SettingsIndex)
	}
}

func TestInvalidOrUnchangedModesDoNotSave(t *testing.T) {
	state := playbackState(3, false, RepeatOff)
	for _, event := range []Event{RepeatChanged{Mode: RepeatMode("invalid")}, RepeatChanged{Mode: RepeatOff}, ShuffleChanged{Enabled: false}} {
		got, commands := Reduce(state, event)
		if !reflect.DeepEqual(got, state) || len(commands) != 0 {
			t.Fatalf("event %#v changed state or requested a save: %+v, %v", event, got, commands)
		}
	}
	got, commands := Reduce(state, SettingsLoaded{Settings: SettingsState{Volume: 150, Repeat: RepeatMode("invalid"), Shuffle: true}})
	if got.Settings.Volume != 100 || got.Settings.Repeat != RepeatOff || !got.Settings.Shuffle || len(got.playbackOrder) != 3 || len(commands) != 0 {
		t.Fatalf("loaded settings not normalized: %+v, %v", got, commands)
	}
}

func TestFinishedModesAndManualNavigation(t *testing.T) {
	for _, shuffle := range []bool{false, true} {
		for _, mode := range []RepeatMode{RepeatOff, RepeatAll, RepeatOne} {
			for index := 0; index < 3; index++ {
				t.Run(fmt.Sprintf("shuffle=%t/repeat=%s/index=%d", shuffle, mode, index), func(t *testing.T) {
					state := playbackState(3, shuffle, mode)
					order := []music.TrackID{"0", "1", "2"}
					if shuffle {
						order = []music.TrackID{"2", "0", "1"}
						state.playbackOrder = slices.Clone(order)
					}
					state.Player = PlayerState{Track: state.Library.Tracks[trackIndex(state.Library.Tracks, order[index])], HasTrack: true, Status: StatusPlaying}
					state.Screen = ScreenSettings
					got, commands := Reduce(state, TrackFinished{})
					if mode == RepeatOff && index == 2 {
						if got.Player.Status != StatusStopped || len(commands) != 0 {
							t.Fatalf("end of pass did not stop: %+v, %v", got.Player, commands)
						}
					} else {
						want := order[(index+1)%3]
						if mode == RepeatOne {
							want = order[index]
						} else if shuffle && index == 2 {
							want = got.playbackOrder[0]
							if want == order[index] {
								t.Fatal("repeat-all immediately repeated the last track")
							}
						}
						got = confirmPlayback(t, got, commands)
						if got.Player.Track.ID != want || got.Screen != ScreenSettings {
							t.Fatalf("automatic transition = %+v, want %s without leaving Settings", got, want)
						}
					}
					for delta, event := range map[int]Event{-1: PreviousPressed{}, 1: NextPressed{}} {
						next, commands := Reduce(state, event)
						next = confirmPlayback(t, next, commands)
						if next.Player.Track.ID != order[(index+delta+3)%3] {
							t.Fatalf("manual navigation with repeat=%s: got %s", mode, next.Player.Track.ID)
						}
					}
					if shuffle && !slices.Equal(state.playbackOrder, order) {
						t.Fatal("Reduce mutated the incoming queue")
					}
				})
			}
		}
	}
}

func TestShufflePassesDoNotRepeatTracks(t *testing.T) {
	for seed := uint64(0); seed < 16; seed++ {
		for _, size := range []int{1, 2, 7} {
			for _, mode := range []RepeatMode{RepeatOff, RepeatAll} {
				state := playbackState(size, true, mode)
				state.shuffleSeed = seed
				state.Navigation.TrackIndex = size - 1
				state, commands := Reduce(state, SelectPressed{})
				state = confirmPlayback(t, state, commands)
				if state.Player.Track.ID != state.Library.Tracks[size-1].ID {
					t.Fatal("explicit track selection was ignored")
				}
				passes := 1
				if mode == RepeatAll {
					passes = 3
				}
				for pass := 0; pass < passes; pass++ {
					seen := make(map[music.TrackID]bool)
					for i := 0; i < size; i++ {
						id := state.Player.Track.ID
						if seen[id] || trackIndex(state.Library.Tracks, id) < 0 {
							t.Fatalf("seed=%d size=%d pass=%d: repeated or unknown track %s", seed, size, pass, id)
						}
						seen[id] = true
						state, commands = Reduce(state, TrackFinished{})
						if mode == RepeatOff && i == size-1 {
							if state.Player.Status != StatusStopped || len(commands) != 0 {
								t.Fatal("repeat-off did not stop after the whole pass")
							}
						} else {
							state = confirmPlayback(t, state, commands)
						}
					}
				}
			}
		}
	}
}

func TestRepeatOneOverridesShuffleButNotManualNext(t *testing.T) {
	state := playbackState(5, true, RepeatOne)
	state, commands := Reduce(state, SelectPressed{})
	state = confirmPlayback(t, state, commands)
	id := state.Player.Track.ID
	for i := 0; i < 5; i++ {
		state, commands = Reduce(state, TrackFinished{})
		state = confirmPlayback(t, state, commands)
		if state.Player.Track.ID != id {
			t.Fatal("shuffle took priority over repeat-one")
		}
	}
	state, commands = Reduce(state, NextPressed{})
	state = confirmPlayback(t, state, commands)
	if state.Player.Track.ID == id {
		t.Fatal("repeat-one blocked manual next")
	}
	state, commands = Reduce(state, PreviousPressed{})
	state = confirmPlayback(t, state, commands)
	if state.Player.Track.ID != id {
		t.Fatal("previous did not return to the prior queue item")
	}
}

func TestEmptyLibraryAndInactiveFinished(t *testing.T) {
	for _, shuffle := range []bool{false, true} {
		for _, mode := range []RepeatMode{RepeatOff, RepeatAll, RepeatOne} {
			state := playbackState(0, shuffle, mode)
			for _, event := range []Event{SelectPressed{}, NextPressed{}, PreviousPressed{}, TrackFinished{}} {
				got, commands := Reduce(state, event)
				if got.Player.HasTrack || len(commands) != 0 {
					t.Fatalf("empty library started playback: %+v, %v", got.Player, commands)
				}
			}
			for _, status := range []PlaybackStatus{StatusStopped, StatusPaused, StatusStarting} {
				state = playbackState(1, shuffle, mode)
				state.Player = PlayerState{Track: state.Library.Tracks[0], HasTrack: true, Status: status}
				got, commands := Reduce(state, TrackFinished{})
				if got.Player.Status != status || len(commands) != 0 {
					t.Fatalf("inactive/duplicate finish was accepted: %+v, %v", got.Player, commands)
				}
			}
		}
	}
}

func TestShuffleToggleAndLibraryRefresh(t *testing.T) {
	state := playbackState(3, false, RepeatOff)
	state.Player = PlayerState{Track: state.Library.Tracks[1], HasTrack: true, Status: StatusPaused}
	state.Screen = ScreenSettings
	on, commands := Reduce(state, ShuffleChanged{Enabled: true})
	if len(commands) != 1 || on.Player != state.Player || on.Screen != state.Screen || on.playbackOrder[0] != "1" {
		t.Fatalf("toggle interrupted playback or lost current track: %+v, %v", on, commands)
	}
	failed, _ := Reduce(on, LibraryLoadFailed{Err: errors.New("scan failed")})
	if !slices.Equal(on.playbackOrder, failed.playbackOrder) {
		t.Fatal("failed refresh destroyed playback order")
	}
	refreshed, _ := Reduce(on, LibraryLoaded{Tracks: []music.Track{{ID: "new-a"}, {ID: "new-b"}, {ID: "new-c"}}})
	for _, id := range refreshed.playbackOrder {
		if trackIndex(refreshed.Library.Tracks, id) < 0 {
			t.Fatalf("successful refresh retained stale ID %s", id)
		}
	}
	refreshed.Player.Status = StatusPlaying
	refreshed, commands = Reduce(refreshed, TrackFinished{})
	refreshed = confirmPlayback(t, refreshed, commands)
	if trackIndex(refreshed.Library.Tracks, refreshed.Player.Track.ID) < 0 {
		t.Fatal("finish after removing current track chose a stale track")
	}
	off, _ := Reduce(on, ShuffleChanged{Enabled: false})
	if off.Settings.Shuffle || len(off.playbackOrder) != 0 || off.Player != on.Player {
		t.Fatalf("disabling shuffle changed playback: %+v", off)
	}
	off, commands = Reduce(off, NextPressed{})
	off = confirmPlayback(t, off, commands)
	if off.Player.Track.ID != "2" {
		t.Fatal("disabling shuffle did not restore library order")
	}
}

func TestShuffleIsDeterministicAndCloned(t *testing.T) {
	state := playbackState(8, false, RepeatOff)
	first, _ := Reduce(state, ShuffleChanged{Enabled: true})
	second, _ := Reduce(state, ShuffleChanged{Enabled: true})
	if !reflect.DeepEqual(first, second) || len(state.playbackOrder) != 0 {
		t.Fatal("identical reducer inputs produced different states")
	}
	copy := first.clone()
	copy.playbackOrder[0] = "changed outside App"
	if first.playbackOrder[0] == copy.playbackOrder[0] {
		t.Fatal("rendered snapshot shares mutable queue storage")
	}
	copy.Library.Tracks[0].ID = "changed outside App"
	if first.Library.Tracks[0].ID == copy.Library.Tracks[0].ID {
		t.Fatal("rendered snapshot shares mutable library storage")
	}
	orders := make(map[string]bool)
	for seed := uint64(0); seed < 16; seed++ {
		state.shuffleSeed = seed
		shuffled, _ := Reduce(state, ShuffleChanged{Enabled: true})
		orders[fmt.Sprint(shuffled.playbackOrder)] = true
	}
	if len(orders) < 2 {
		t.Fatal("shuffle ignored its random seed and always used the same order")
	}
}

func TestSingleTrackRepeatAndManualNavigation(t *testing.T) {
	for _, shuffle := range []bool{false, true} {
		for _, mode := range []RepeatMode{RepeatOff, RepeatAll, RepeatOne} {
			state := playbackState(1, shuffle, mode)
			state, commands := Reduce(state, SelectPressed{})
			state = confirmPlayback(t, state, commands)
			for _, event := range []Event{NextPressed{}, PreviousPressed{}} {
				next, commands := Reduce(state, event)
				next = confirmPlayback(t, next, commands)
				if next.Player.Track.ID != "0" {
					t.Fatalf("single-track navigation chose %s", next.Player.Track.ID)
				}
			}
			state, commands = Reduce(state, TrackFinished{})
			if mode == RepeatOff {
				if len(commands) != 0 || state.Player.Status != StatusStopped {
					t.Fatal("single-track repeat-off did not stop")
				}
			} else {
				state = confirmPlayback(t, state, commands)
				if state.Player.Track.ID != "0" {
					t.Fatal("single-track repeat chose another track")
				}
			}
		}
	}
}

func TestPendingManualSwitchIgnoresOlderConfirmations(t *testing.T) {
	for _, shuffle := range []bool{false, true} {
		state := playbackState(4, shuffle, RepeatOff)
		state, _ = Reduce(state, SelectPressed{})
		old := state.Player.Track
		state, _ = Reduce(state, NextPressed{})
		pending := state.Player
		for _, event := range []Event{PlaybackStarted{Track: old}, PlaybackPaused{}, PlaybackResumed{}, TrackFinished{}} {
			got, commands := Reduce(state, event)
			if got.Player != pending || len(commands) != 0 {
				t.Fatalf("queued %#v interrupted pending play: %+v, %v", event, got.Player, commands)
			}
			state = got
		}
		state, _ = Reduce(state, PlaybackStarted{Track: pending.Track})
		if state.Player.Status != StatusPlaying || state.Player.Track.ID != pending.Track.ID {
			t.Fatalf("current confirmation was lost: %+v", state.Player)
		}
	}
}

func TestAutomaticCompletionPreservesBrowsingSelection(t *testing.T) {
	for _, screen := range []Screen{ScreenTracks, ScreenSettings} {
		state := playbackState(3, false, RepeatOff)
		state.Player = PlayerState{Track: state.Library.Tracks[0], HasTrack: true, Status: StatusPlaying}
		state.Screen = screen
		state.Navigation = NavigationState{TrackIndex: 2, SettingsIndex: SettingsShuffle}
		next, commands := Reduce(state, TrackFinished{})
		next = confirmPlayback(t, next, commands)
		if next.Screen != state.Screen || next.Navigation != state.Navigation || next.Player.Track.ID != "1" {
			t.Fatalf("automatic transition stole navigation: %+v", next)
		}
	}
}

func TestSeekingPreservesPlaybackModesAndOrder(t *testing.T) {
	for _, shuffle := range []bool{false, true} {
		for _, mode := range []RepeatMode{RepeatOff, RepeatAll, RepeatOne} {
			for _, status := range []PlaybackStatus{StatusPlaying, StatusPaused, StatusStarting} {
				state := playbackState(4, shuffle, mode)
				state.Player = PlayerState{Track: state.Library.Tracks[0], HasTrack: true, Status: status, Position: 20 * time.Second}
				for _, delta := range []time.Duration{-SeekStep, SeekStep} {
					got, commands := Reduce(state, SeekBy{Delta: delta})
					if !reflect.DeepEqual(got, state) {
						t.Fatalf("seek changed modes, queue or state before confirmation: %+v", got)
					}
					if status == StatusStarting {
						if len(commands) != 0 {
							t.Fatalf("seek accepted while waiting for playback: %v", commands)
						}
						continue
					}
					if len(commands) != 1 {
						t.Fatalf("seek commands = %v, want one relative seek", commands)
					}
					if request, ok := commands[0].(seekPlayback); !ok || request.delta != delta {
						t.Fatalf("seek command = %#v", commands[0])
					}
					confirmed, _ := Reduce(got, PlaybackPositionChanged{Position: 20*time.Second + delta})
					if confirmed.Settings != state.Settings || !slices.Equal(confirmed.playbackOrder, state.playbackOrder) || confirmed.Player.Status != status {
						t.Fatalf("seek confirmation reset playback modes/order: %+v", confirmed)
					}
				}
			}
		}
	}
}
