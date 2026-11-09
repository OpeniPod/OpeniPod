package player

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/OpeniPod/OpeniPod/internal/app"
	"github.com/OpeniPod/OpeniPod/internal/music"
)

// Fake accepts playback commands and reports transitions; it produces no sound.
type Fake struct {
	events    chan app.Event
	Track     music.Track
	Position  time.Duration
	HasTrack  bool
	Playing   bool
	Paused    bool
	Volume    int
	Equalizer app.EqualizerPreset
}

func NewFake() *Fake {
	return &Fake{events: make(chan app.Event, 16), Volume: 50}
}

func (f *Fake) Events() <-chan app.Event { return f.events }

func (f *Fake) Close() error { return nil }

func (f *Fake) Play(ctx context.Context, track music.Track) error {
	if err := f.emit(ctx, app.PlaybackStarted{Track: track}); err != nil {
		return err
	}
	f.Track = track
	f.Position = 0
	f.HasTrack = true
	f.Playing = true
	f.Paused = false
	return nil
}

func (f *Fake) Pause(ctx context.Context) error {
	if err := f.emit(ctx, app.PlaybackPaused{}); err != nil {
		return err
	}
	f.Playing = false
	f.Paused = true
	return nil
}

func (f *Fake) Resume(ctx context.Context) error {
	if err := f.emit(ctx, app.PlaybackResumed{}); err != nil {
		return err
	}
	f.Playing = true
	f.Paused = false
	return nil
}

func (f *Fake) Seek(ctx context.Context, delta time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !f.HasTrack || (!f.Playing && !f.Paused) {
		return errors.New("no track is playing")
	}
	target := f.Position
	switch {
	case delta > 0 && f.Track.Duration > 0 &&
		(f.Position >= f.Track.Duration || delta >= f.Track.Duration-f.Position):
		target = f.Track.Duration
	case delta > 0 && f.Position > time.Duration(1<<63-1)-delta:
		target = time.Duration(1<<63 - 1)
	case delta < 0 && delta <= -f.Position:
		target = 0
	default:
		target += delta
	}
	if err := f.emit(ctx, app.PlaybackPositionChanged{Position: target}); err != nil {
		return err
	}
	f.Position = target
	return nil
}

func (f *Fake) SetVolume(ctx context.Context, volume int) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f.Volume = volume
	return nil
}

func (f *Fake) SetEqualizer(ctx context.Context, preset app.EqualizerPreset) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !preset.Valid() {
		return fmt.Errorf("unknown equalizer preset %q", string(preset))
	}
	f.Equalizer = preset
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
