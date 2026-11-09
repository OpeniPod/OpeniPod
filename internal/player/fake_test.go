package player

import (
	"context"
	"testing"
	"time"

	"github.com/OpeniPod/OpeniPod/internal/app"
	"github.com/OpeniPod/OpeniPod/internal/music"
	"github.com/gopxl/beep"
)

func TestSeekTargetMovesAndClamps(t *testing.T) {
	rate := beep.SampleRate(48000)
	length := rate.N(10 * time.Second)
	for _, tc := range []struct {
		name     string
		position int
		delta    time.Duration
		want     int
	}{
		{name: "backward", position: rate.N(5 * time.Second), delta: -app.SeekStep, want: rate.N(0)},
		{name: "forward", position: rate.N(2 * time.Second), delta: app.SeekStep, want: rate.N(7 * time.Second)},
		{name: "clamp at start", position: rate.N(2 * time.Second), delta: -app.SeekStep, want: rate.N(0)},
		{name: "clamp at end", position: rate.N(8 * time.Second), delta: app.SeekStep, want: length},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := seekTarget(rate, tc.position, length, 10*time.Second, tc.delta)
			if got != tc.want {
				t.Fatalf("seek target = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestFakeSeekClampsToTrackBounds(t *testing.T) {
	for _, tc := range []struct {
		name     string
		position time.Duration
		delta    time.Duration
		want     time.Duration
	}{
		{name: "backward to zero", position: 2 * time.Second, delta: -5 * time.Second, want: 0},
		{name: "forward to duration", position: 8 * time.Second, delta: 5 * time.Second, want: 10 * time.Second},
		{name: "seek inside track", position: 2 * time.Second, delta: 5 * time.Second, want: 7 * time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := NewFake()
			fake.Track = music.Track{ID: "track", Duration: 10 * time.Second}
			fake.HasTrack = true
			fake.Playing = true
			fake.Position = tc.position

			if err := fake.Seek(context.Background(), tc.delta); err != nil {
				t.Fatal(err)
			}
			if fake.Position != tc.want {
				t.Fatalf("position = %s, want %s", fake.Position, tc.want)
			}
			event := <-fake.Events()
			position, ok := event.(app.PlaybackPositionChanged)
			if !ok || position.Position != tc.want {
				t.Fatalf("event = %#v, want position %s", event, tc.want)
			}
		})
	}
}

func TestFakeSeekRequiresTrack(t *testing.T) {
	fake := NewFake()
	if err := fake.Seek(context.Background(), app.SeekStep); err == nil {
		t.Fatal("Seek without a track succeeded")
	}
}
