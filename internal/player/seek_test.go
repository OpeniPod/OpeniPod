package player

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/OpeniPod/OpeniPod/internal/app"
	"github.com/OpeniPod/OpeniPod/internal/music"
	"github.com/gopxl/beep"
	"github.com/gopxl/beep/mp3"
)

func TestRealMP3SeekNearEndClampsToLastSample(t *testing.T) {
	file, err := os.Open("../library/testdata/valid_48k.mp3")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	streamer, format, err := mp3.Decode(file)
	if err != nil {
		t.Fatal(err)
	}
	defer streamer.Close()
	if format.SampleRate != sampleRate {
		t.Fatalf("fixture sample rate = %d, want %d", format.SampleRate, sampleRate)
	}

	resampled := beep.Resample(4, format.SampleRate, sampleRate, streamer)
	equalizer := newEqualizer(resampled, app.EqualizerOff)
	p := &Player{
		events:      make(chan app.Event, 16),
		eventReady:  make(chan struct{}, 1),
		shutdown:    make(chan struct{}),
		streamer:    streamer,
		streamRate:  format.SampleRate,
		resampler:   resampled,
		ctrl:        &beep.Ctrl{Streamer: equalizer, Paused: false},
		equalizer:   equalizer,
		track:       music.Track{Duration: format.SampleRate.D(streamer.Len())},
		volumeLevel: 50,
	}

	nearEnd := streamer.Len() - format.SampleRate.N(time.Second)
	if nearEnd < 0 {
		nearEnd = 0
	}
	if err := streamer.Seek(nearEnd); err != nil {
		t.Fatal(err)
	}
	if err := p.Seek(context.Background(), 5*time.Second); err != nil {
		t.Fatal(err)
	}
	if got := streamer.Position(); got >= streamer.Len() {
		t.Fatalf("seek position = %d, len = %d; target reached end", got, streamer.Len())
	}
	if got, want := streamer.Position(), streamer.Len()-1; got != want {
		t.Fatalf("seek position = %d, want last sample %d", got, want)
	}
	streamer.Stream(make([][2]float64, 1))
	if err := streamer.Err(); err != nil {
		t.Fatalf("streaming from last sample: %v", err)
	}
}

func TestSeekWhileTrackFinishedIsIgnored(t *testing.T) {
	file, err := os.Open("../library/testdata/valid.mp3")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	streamer, _, err := mp3.Decode(file)
	if err != nil {
		t.Fatal(err)
	}

	p := &Player{
		events:       make(chan app.Event, 16),
		eventReady:   make(chan struct{}, 1),
		shutdown:     make(chan struct{}),
		playbackID:   1,
		streamer:     streamer,
		playbackStop: make(chan struct{}),
	}
	p.eventWG.Add(1)
	go p.dispatchEvents()
	defer func() {
		close(p.shutdown)
		p.eventWG.Wait()
	}()

	p.finishPlayback(1, streamer)
	select {
	case event := <-p.Events():
		if _, ok := event.(app.TrackFinished); !ok {
			t.Fatalf("event = %#v, want TrackFinished", event)
		}
	case <-time.After(time.Second):
		t.Fatal("TrackFinished was not queued")
	}

	if err := p.Seek(context.Background(), 5*time.Second); err != nil {
		t.Fatalf("Seek before handling TrackFinished = %v, want ignored", err)
	}
}
