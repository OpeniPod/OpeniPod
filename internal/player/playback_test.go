package player

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/OpeniPod/OpeniPod/internal/app"
	"github.com/OpeniPod/OpeniPod/internal/library"
	"github.com/gopxl/beep/speaker"
)

func TestLibraryTrackPlaybackControls(t *testing.T) {
	for _, test := range []struct{ fixture, name string }{
		{"tagged.mp3", "track.MP3"},
		{"tagged.flac", "track.FLAC"},
		{"valid.flac", "flac-with-mp3-extension.mp3"},
		{"valid.mp3", "mp3-with-flac-extension.flac"},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			root := t.TempDir()
			data, err := os.ReadFile(filepath.Join("../library/testdata", test.fixture))
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, test.name), data, 0o600); err != nil {
				t.Fatal(err)
			}
			tracks, err := library.NewFilesystem(root).Load(ctx)
			if err != nil || len(tracks) != 1 {
				t.Fatalf("library tracks = %v, err = %v", tracks, err)
			}

			// Drive real decoding through Play without initializing an audio device.
			// Pull samples synchronously; no speaker worker consumes the stream.
			p := &Player{
				events: make(chan app.Event, 16), eventReady: make(chan struct{}, 1),
				shutdown: make(chan struct{}), closeDone: make(chan struct{}), volumeLevel: 50,
			}
			p.eventWG.Add(1)
			go p.dispatchEvents()
			t.Cleanup(func() {
				if err := p.Close(); err != nil {
					t.Error(err)
				}
			})
			if err := p.Play(ctx, tracks[0]); err != nil {
				t.Fatal(err)
			}
			started := awaitPlaybackEvent[app.PlaybackStarted](t, p)
			if started.Track.ID != tracks[0].ID || started.Track.Path != tracks[0].Path ||
				started.Track.Title != tracks[0].Title || started.Track.Duration <= 0 {
				t.Fatalf("started track = %+v, library track = %+v", started.Track, tracks[0])
			}
			pull := func() {
				t.Helper()
				speaker.Lock()
				defer speaker.Unlock()
				if n, ok := p.volume.Stream(make([][2]float64, 128)); !ok || n != 128 || p.streamer.Err() != nil {
					t.Fatalf("stream = %d, %v, err = %v", n, ok, p.streamer.Err())
				}
			}
			pull()
			if p.streamer.Position() <= 0 {
				t.Fatal("playback did not advance")
			}
			if err := p.Pause(ctx); err != nil {
				t.Fatal(err)
			}
			awaitPlaybackEvent[app.PlaybackPaused](t, p)
			position := p.streamer.Position()
			pull()
			if p.streamer.Position() != position {
				t.Fatal("paused playback consumed samples")
			}

			for _, delta := range []time.Duration{100 * time.Millisecond, -50 * time.Millisecond} {
				before := p.streamer.Position()
				if err := p.Seek(ctx, delta); err != nil {
					t.Fatal(err)
				}
				position = p.streamer.Position()
				moved := p.streamRate.D(position - before)
				if diff := moved - delta; diff < -time.Millisecond || diff > time.Millisecond {
					t.Fatalf("seek moved %v, want %v", moved, delta)
				}
				pull()
				if !p.ctrl.Paused || p.streamer.Position() != position {
					t.Fatal("seek changed pause state or paused playback advanced")
				}
			}
			if err := p.Resume(ctx); err != nil {
				t.Fatal(err)
			}
			awaitPlaybackEvent[app.PlaybackResumed](t, p)
			pull()
			if p.streamer.Position() <= position {
				t.Fatal("resumed playback did not advance")
			}
			if err := p.Seek(ctx, -time.Hour); err != nil || p.streamer.Position() != 0 || p.ctrl.Paused {
				t.Fatalf("seek to start: position=%d paused=%v err=%v", p.streamer.Position(), p.ctrl.Paused, err)
			}
			pull()
			if err := p.Seek(ctx, time.Hour); err != nil || p.streamer.Position() != p.streamer.Len()-1 {
				t.Fatalf("seek to end: position=%d length=%d err=%v", p.streamer.Position(), p.streamer.Len(), err)
			}
			speaker.Lock()
			n, _ := p.streamer.Stream(make([][2]float64, 1))
			speaker.Unlock()
			if n != 1 || p.streamer.Err() != nil {
				t.Fatalf("last sample: n=%d err=%v", n, p.streamer.Err())
			}
		})
	}
}

func awaitPlaybackEvent[T app.Event](t *testing.T, p *Player) T {
	t.Helper()
	timeout := time.NewTimer(2 * time.Second)
	defer timeout.Stop()
	for {
		select {
		case event := <-p.Events():
			if wanted, ok := event.(T); ok {
				return wanted
			}
			if _, ok := event.(app.PlaybackPositionChanged); !ok {
				t.Fatalf("unexpected playback event: %#v", event)
			}
		case <-timeout.C:
			var wanted T
			t.Fatalf("timed out waiting for %T", wanted)
		}
	}
}
