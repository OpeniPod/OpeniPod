package player

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/OpeniPod/OpeniPod/internal/app"
	"github.com/OpeniPod/OpeniPod/internal/music"
	"github.com/gopxl/beep"
	"github.com/gopxl/beep/speaker"
)

func TestPlayerSetEqualizerPreservesPlayback(t *testing.T) {
	position := 0
	source := beep.StreamerFunc(func(samples [][2]float64) (int, bool) {
		for i := range samples {
			samples[i] = [2]float64{0.25, -0.25}
		}
		position += len(samples)
		return len(samples), true
	})
	eq := newEqualizer(source, app.EqualizerOff)
	ctrl := &beep.Ctrl{Streamer: eq, Paused: true}
	p := &Player{equalizer: eq, ctrl: ctrl, track: music.Track{ID: "current"}, volumeLevel: 70}
	for _, preset := range []app.EqualizerPreset{app.EqualizerBassBoost, app.EqualizerVocal, app.EqualizerOff} {
		if err := p.SetEqualizer(context.Background(), preset); err != nil {
			t.Fatal(err)
		}
		if p.equalizerPreset != preset || p.equalizer != eq || p.ctrl != ctrl || !ctrl.Paused || p.track.ID != "current" || p.volumeLevel != 70 {
			t.Fatalf("equalizer change altered playback: %+v", p)
		}
		samples := make([][2]float64, 16)
		ctrl.Stream(samples)
		if position != 0 {
			t.Fatalf("paused player consumed %d source samples", position)
		}
	}
	ctrl.Paused = false
	samples := make([][2]float64, 16)
	ctrl.Stream(samples)
	if position != len(samples) || samples[0] != [2]float64{0.25, -0.25} {
		t.Fatalf("resumed off stream = %v, position = %d", samples, position)
	}
}

func TestPlayerSetEqualizerBeforePlaybackAndInvalidCommands(t *testing.T) {
	p := &Player{}
	if err := p.SetEqualizer(context.Background(), app.EqualizerVocal); err != nil || p.equalizerPreset != app.EqualizerVocal {
		t.Fatalf("set preset before playing: preset=%q err=%v", p.equalizerPreset, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := p.SetEqualizer(ctx, app.EqualizerOff); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled command error = %v", err)
	}
	if err := p.SetEqualizer(context.Background(), "unknown"); err == nil || !strings.Contains(err.Error(), `"unknown"`) {
		t.Fatalf("invalid preset error = %v", err)
	}
	if p.equalizerPreset != app.EqualizerVocal {
		t.Fatalf("invalid command changed preset to %q", p.equalizerPreset)
	}
	p.closed = true
	if err := p.SetEqualizer(context.Background(), app.EqualizerOff); !errors.Is(err, ErrClosed) {
		t.Fatalf("closed player command error = %v", err)
	}
}

func TestPlayerSetEqualizerSynchronizesWithStreaming(t *testing.T) {
	source := beep.StreamerFunc(func(samples [][2]float64) (int, bool) {
		for i := range samples {
			samples[i] = [2]float64{0.25, -0.25}
		}
		return len(samples), true
	})
	p := &Player{equalizer: newEqualizer(source, app.EqualizerOff)}
	var worker sync.WaitGroup
	worker.Add(1)
	go func() {
		defer worker.Done()
		samples := make([][2]float64, 64)
		for i := 0; i < 300; i++ {
			speaker.Lock()
			p.equalizer.Stream(samples)
			speaker.Unlock()
		}
	}()
	for i := 0; i < 100; i++ {
		for _, preset := range []app.EqualizerPreset{app.EqualizerBassBoost, app.EqualizerVocal, app.EqualizerOff} {
			if err := p.SetEqualizer(context.Background(), preset); err != nil {
				t.Error(err)
			}
		}
	}
	worker.Wait()
}
