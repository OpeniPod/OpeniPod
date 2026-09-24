package player

import (
	"context"
	"errors"

	"github.com/OpeniPod/OpeniPod/internal/app"
	"github.com/OpeniPod/OpeniPod/internal/music"
)

// Fake accepts playback commands and reports transitions; it produces no sound.
type Fake struct {
	events  chan app.Event
	Track   music.Track
	Playing bool
	Volume  int
}

func NewFake() *Fake {
	return &Fake{events: make(chan app.Event, 16), Volume: 50}
}

func (f *Fake) Events() <-chan app.Event { return f.events }

func (f *Fake) Play(ctx context.Context, track music.Track) error {
	if err := f.emit(ctx, app.PlaybackStarted{Track: track}); err != nil {
		return err
	}
	f.Track = track
	f.Playing = true
	return nil
}

func (f *Fake) Pause(ctx context.Context) error {
	if err := f.emit(ctx, app.PlaybackPaused{}); err != nil {
		return err
	}
	f.Playing = false
	return nil
}

func (f *Fake) Resume(ctx context.Context) error {
	if err := f.emit(ctx, app.PlaybackResumed{}); err != nil {
		return err
	}
	f.Playing = true
	return nil
}

func (f *Fake) SetVolume(ctx context.Context, volume int) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f.Volume = volume
	return nil
}

func (f *Fake) emit(ctx context.Context, event app.Event) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case f.events <- event:
		return nil
	default:
		return errors.New("fake player event queue is full")
	}
}
