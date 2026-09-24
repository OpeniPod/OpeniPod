package app

import (
	"time"

	"github.com/OpeniPod/OpeniPod/internal/music"
)

// Event is closed to this package so adapters can send only known application events.
type Event interface{ isEvent() }

type Scroll struct{ Delta int }
type SelectPressed struct{}
type BackPressed struct{}
type PlayPausePressed struct{}
type NextPressed struct{}
type PreviousPressed struct{}
type VolumeChanged struct{ Delta int }
type QuitRequested struct{}

type PlaybackStarted struct{ Track music.Track }
type PlaybackPaused struct{}
type PlaybackResumed struct{}
type PlaybackPositionChanged struct{ Position time.Duration }
type TrackFinished struct{}
type PlaybackError struct{ Err error }

type LibraryLoaded struct{ Tracks []music.Track }
type LibraryLoadFailed struct{ Err error }
type SettingsLoaded struct{ Settings SettingsState }

func (Scroll) isEvent()                  {}
func (SelectPressed) isEvent()           {}
func (BackPressed) isEvent()             {}
func (PlayPausePressed) isEvent()        {}
func (NextPressed) isEvent()             {}
func (PreviousPressed) isEvent()         {}
func (VolumeChanged) isEvent()           {}
func (QuitRequested) isEvent()           {}
func (PlaybackStarted) isEvent()         {}
func (PlaybackPaused) isEvent()          {}
func (PlaybackResumed) isEvent()         {}
func (PlaybackPositionChanged) isEvent() {}
func (TrackFinished) isEvent()           {}
func (PlaybackError) isEvent()           {}
func (LibraryLoaded) isEvent()           {}
func (LibraryLoadFailed) isEvent()       {}
func (SettingsLoaded) isEvent()          {}
