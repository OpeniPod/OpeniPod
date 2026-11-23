package app_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/OpeniPod/OpeniPod/internal/app"
	"github.com/OpeniPod/OpeniPod/internal/library"
	"github.com/OpeniPod/OpeniPod/internal/music"
	"github.com/OpeniPod/OpeniPod/internal/player"
	"github.com/OpeniPod/OpeniPod/internal/storage"
)

// Extend the existing fake with deterministic end-of-track events. Commands and
// confirmations still traverse the real App loop; no real audio is required.
type finishingPlayer struct {
	*player.Fake
	events   chan app.Event
	played   []music.TrackID
	finishes int
}

func (p *finishingPlayer) Events() <-chan app.Event { return p.events }

func (p *finishingPlayer) Play(ctx context.Context, track music.Track) error {
	if err := p.Fake.Play(ctx, track); err != nil {
		return err
	}
	p.played = append(p.played, track.ID)
	p.events <- <-p.Fake.Events()
	if p.finishes > 0 {
		p.finishes--
		p.Playing = false
		p.events <- app.TrackFinished{}
	}
	return nil
}

func TestAppAppliesPlaybackModesThroughPlayerEvents(t *testing.T) {
	for _, shuffle := range []bool{false, true} {
		for _, mode := range []app.RepeatMode{app.RepeatOff, app.RepeatAll, app.RepeatOne} {
			for _, size := range []int{1, 4} {
				t.Run(fmt.Sprintf("shuffle=%t/repeat=%s/tracks=%d", shuffle, mode, size), func(t *testing.T) {
					ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
					defer cancel()
					lib := library.NewFake()
					lib.Tracks = lib.Tracks[:size]
					settings := storage.NewMemory(50)
					if err := settings.SaveSettings(ctx, app.SettingsState{Volume: 50, Repeat: mode, Shuffle: shuffle}); err != nil {
						t.Fatal(err)
					}
					wantPlays := size
					finishes := size
					if mode != app.RepeatOff {
						wantPlays, finishes = size*2+1, size*2
					}
					p := &finishingPlayer{Fake: player.NewFake(), events: make(chan app.Event, 16), finishes: finishes}
					completed := false
					view := renderFunc(func(state app.AppState) error {
						if len(p.played) == wantPlays &&
							((mode == app.RepeatOff && state.Player.Status == app.StatusStopped) ||
								(mode != app.RepeatOff && state.Player.Status == app.StatusPlaying)) {
							completed = true
							cancel()
						}
						return nil
					})
					input := inputFunc(func(ctx context.Context, events chan<- app.Event) error {
						if err := (eventInput{events: []app.Event{app.SelectPressed{}, app.SelectPressed{}}}).Run(ctx, events); err != nil {
							return err
						}
						<-ctx.Done()
						return ctx.Err()
					})
					application := app.New(p, lib, settings, input, view)
					if err := application.Run(ctx); err != nil {
						t.Fatal(err)
					}
					if !completed || len(p.played) != wantPlays {
						t.Fatalf("automatic transitions incomplete: completed=%t played=%v", completed, p.played)
					}
					for i, id := range p.played {
						if mode == app.RepeatOne {
							if id != p.played[0] {
								t.Fatalf("repeat-one switched tracks: %v", p.played)
							}
							continue
						}
						for j := (i / size) * size; j < i; j++ {
							if p.played[j] == id {
								t.Fatalf("track repeated in the same pass: %v", p.played)
							}
						}
					}
				})
			}
		}
	}
}
