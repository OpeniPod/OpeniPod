package app

import (
	"math/rand/v2"
	"slices"

	"github.com/OpeniPod/OpeniPod/internal/music"
)

func validRepeat(mode RepeatMode) bool {
	return mode == RepeatOff || mode == RepeatAll || mode == RepeatOne
}

func nextRepeat(mode RepeatMode) RepeatMode {
	switch mode {
	case RepeatAll:
		return RepeatOne
	case RepeatOne:
		return RepeatOff
	default:
		return RepeatAll
	}
}

func (mode RepeatMode) String() string {
	switch mode {
	case RepeatAll:
		return "All"
	case RepeatOne:
		return "One"
	default:
		return "Off"
	}
}

func currentTrackID(state AppState) music.TrackID {
	if state.Player.HasTrack {
		return state.Player.Track.ID
	}
	return ""
}

func trackIndex(tracks []music.Track, id music.TrackID) int {
	return slices.IndexFunc(tracks, func(track music.Track) bool { return track.ID == id })
}

// Rebuild into a new slice, so Reduce never mutates the incoming state. The
// initial seed is chosen by App; each permutation is a pure function of state.
// An explicit selection/current track starts the pass, followed by every other
// track exactly once. The queue is session state, not a persisted setting.
func rebuildOrder(state AppState, first, avoidFirst music.TrackID) AppState {
	state.playbackOrder = nil
	if !state.Settings.Shuffle {
		return state
	}
	state.shuffleGeneration++
	rng := rand.New(rand.NewPCG(state.shuffleSeed, state.shuffleGeneration))
	order := make([]music.TrackID, 0, len(state.Library.Tracks))
	anchor := trackIndex(state.Library.Tracks, first)
	if avoided := trackIndex(state.Library.Tracks, avoidFirst); anchor < 0 && avoided >= 0 && len(state.Library.Tracks) > 1 {
		// Pick the first track uniformly from the allowed tracks, then shuffle
		// the remainder. A fixed swap after shuffling would bias the order.
		anchor = rng.IntN(len(state.Library.Tracks) - 1)
		if anchor >= avoided {
			anchor++
		}
		first = state.Library.Tracks[anchor].ID
	}
	if anchor >= 0 {
		order = append(order, first)
	}
	for i, track := range state.Library.Tracks {
		if i != anchor {
			order = append(order, track.ID)
		}
	}
	offset := 0
	if anchor >= 0 {
		offset = 1
	}
	rng.Shuffle(len(order)-offset, func(i, j int) {
		order[i+offset], order[j+offset] = order[j+offset], order[i+offset]
	})
	state.playbackOrder = order
	return state
}

// Manual navigation wraps in the current order, regardless of repeat mode.
// In particular RepeatOne must not trap the user on the current track.
func stepShuffledTrack(state AppState, delta int) (AppState, []command) {
	if len(state.Library.Tracks) == 0 {
		return state, nil
	}
	if len(state.playbackOrder) != len(state.Library.Tracks) {
		state = rebuildOrder(state, currentTrackID(state), "")
	}
	index := slices.Index(state.playbackOrder, currentTrackID(state))
	if index < 0 {
		index = 0
	} else {
		index = (index + delta + len(state.playbackOrder)) % len(state.playbackOrder)
	}
	return startTrack(state, state.Library.Tracks[trackIndex(state.Library.Tracks, state.playbackOrder[index])])
}

func finishTrack(state AppState) (AppState, []command) {
	if !state.Player.HasTrack || state.Player.Status != StatusPlaying {
		return state, nil
	}
	state.Player.Status = StatusStopped
	tracks := state.Library.Tracks
	if len(tracks) == 0 {
		return state, nil
	}
	index := trackIndex(tracks, state.Player.Track.ID)
	if state.Settings.Repeat == RepeatOne && index >= 0 {
		return startAutomaticTrack(state, tracks[index])
	}
	if !state.Settings.Shuffle {
		index++
		if index == len(tracks) {
			if state.Settings.Repeat != RepeatAll {
				return state, nil
			}
			index = 0
		}
		return startAutomaticTrack(state, tracks[index])
	}
	if len(state.playbackOrder) != len(tracks) {
		state = rebuildOrder(state, currentTrackID(state), "")
	}
	index = slices.Index(state.playbackOrder, state.Player.Track.ID) + 1
	if index == len(state.playbackOrder) {
		if state.Settings.Repeat != RepeatAll {
			return state, nil
		}
		state = rebuildOrder(state, "", state.Player.Track.ID)
		index = 0
	}
	return startAutomaticTrack(state, tracks[trackIndex(tracks, state.playbackOrder[index])])
}

// Automatic transitions must not pull the user out of Settings or Tracks.
func startAutomaticTrack(state AppState, track music.Track) (AppState, []command) {
	screen, navigation := state.Screen, state.Navigation
	state, commands := startTrack(state, track)
	state.Screen, state.Navigation = screen, navigation
	return state, commands
}
