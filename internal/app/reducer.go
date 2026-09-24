package app

import "github.com/OpeniPod/OpeniPod/internal/music"

type command interface{ isCommand() }

type playTrack struct{ track music.Track }
type pausePlayback struct{}
type resumePlayback struct{}
type setVolume struct{ volume int }
type saveSettings struct{ settings SettingsState }

func (playTrack) isCommand()      {}
func (pausePlayback) isCommand()  {}
func (resumePlayback) isCommand() {}
func (setVolume) isCommand()      {}
func (saveSettings) isCommand()   {}

// Reduce computes a state transition and its side effects without performing I/O.
func Reduce(state AppState, event Event) (AppState, []command) {
	switch e := event.(type) {
	case Scroll:
		switch state.Screen {
		case ScreenHome:
			state.Navigation.HomeIndex = clamp(state.Navigation.HomeIndex+e.Delta, 0, 2)
		case ScreenTracks:
			state.Navigation.TrackIndex = clamp(state.Navigation.TrackIndex+e.Delta, 0, max(0, len(state.Library.Tracks)-1))
		}
	case SelectPressed:
		switch state.Screen {
		case ScreenHome:
			switch state.Navigation.HomeIndex {
			case 0:
				state.Screen = ScreenTracks
			case 1:
				state.Screen = ScreenNowPlaying
			case 2:
				state.Screen = ScreenSettings
			}
		case ScreenTracks:
			if len(state.Library.Tracks) > 0 {
				track := state.Library.Tracks[state.Navigation.TrackIndex]
				return startTrack(state, track)
			}
		}
	case BackPressed:
		state.Screen = ScreenHome
	case PlayPausePressed:
		switch state.Player.Status {
		case StatusPlaying:
			return state, []command{pausePlayback{}}
		case StatusPaused:
			return state, []command{resumePlayback{}}
		case StatusStopped:
			if state.Player.HasTrack {
				return startTrack(state, state.Player.Track)
			}
		}
	case NextPressed:
		return stepTrack(state, 1)
	case PreviousPressed:
		return stepTrack(state, -1)
	case VolumeChanged:
		volume := clamp(state.Settings.Volume+e.Delta, 0, 100)
		if volume != state.Settings.Volume {
			state.Settings.Volume = volume
			return state, []command{setVolume{volume}, saveSettings{state.Settings}}
		}
	case SettingsLoaded:
		state.Settings.Volume = clamp(e.Settings.Volume, 0, 100)
	case LibraryLoaded:
		state.Library.Tracks = append([]music.Track(nil), e.Tracks...)
		state.Navigation.TrackIndex = clamp(state.Navigation.TrackIndex, 0, max(0, len(e.Tracks)-1))
		state.Error = ""
	case LibraryLoadFailed:
		state.Error = e.Err.Error()
	case PlaybackStarted:
		state.Player.Track = e.Track
		state.Player.HasTrack = true
		state.Player.Status = StatusPlaying
		state.Player.Position = 0
		state.Error = ""
	case PlaybackPaused:
		state.Player.Status = StatusPaused
	case PlaybackResumed:
		state.Player.Status = StatusPlaying
	case PlaybackPositionChanged:
		state.Player.Position = e.Position
	case TrackFinished:
		state.Player.Status = StatusStopped
	case PlaybackError:
		state.Player.Status = StatusStopped
		state.Error = e.Err.Error()
	}
	return state, nil
}

func startTrack(state AppState, track music.Track) (AppState, []command) {
	state.Screen = ScreenNowPlaying
	state.Player.Track = track
	state.Player.HasTrack = true
	state.Player.Status = StatusStarting
	state.Player.Position = 0
	state.Error = ""
	return state, []command{playTrack{track}}
}

func stepTrack(state AppState, delta int) (AppState, []command) {
	tracks := state.Library.Tracks
	if len(tracks) == 0 {
		return state, nil
	}
	index := -1
	if state.Player.HasTrack {
		for i, track := range tracks {
			if track.ID == state.Player.Track.ID {
				index = i
				break
			}
		}
	}
	if index == -1 {
		index = 0
	} else {
		index = (index + delta + len(tracks)) % len(tracks)
	}
	state.Navigation.TrackIndex = index
	return startTrack(state, tracks[index])
}

func clamp(value, low, high int) int {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}
