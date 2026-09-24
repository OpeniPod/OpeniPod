package app_test

import (
	"context"
	"testing"
	"time"

	"github.com/OpeniPod/OpeniPod/internal/app"
	"github.com/OpeniPod/OpeniPod/internal/library"
	"github.com/OpeniPod/OpeniPod/internal/player"
	"github.com/OpeniPod/OpeniPod/internal/storage"
)

type scriptedInput struct {
	playing <-chan struct{}
}

func (s scriptedInput) Run(ctx context.Context, events chan<- app.Event) error {
	for _, event := range []app.Event{
		app.SelectPressed{},
		app.Scroll{Delta: 1},
		app.SelectPressed{},
	} {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case events <- event:
		}
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-s.playing:
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case events <- app.QuitRequested{}:
		return nil
	}
}

type observingUI struct {
	playing chan struct{}
	seen    bool
}

type waitingInput struct{ started chan<- struct{} }

func (w waitingInput) Run(ctx context.Context, _ chan<- app.Event) error {
	close(w.started)
	<-ctx.Done()
	return ctx.Err()
}

type silentUI struct{}

func (silentUI) Render(app.AppState) error { return nil }

type burstInput struct{}

func (burstInput) Run(ctx context.Context, events chan<- app.Event) error {
	for _, event := range []app.Event{app.SelectPressed{}, app.Scroll{Delta: 1}, app.SelectPressed{}} {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case events <- event:
		}
	}
	return nil
}

func (u *observingUI) Render(state app.AppState) error {
	if state.Screen == app.ScreenNowPlaying && state.Player.Status == app.StatusPlaying &&
		state.Player.Track.Title == "Digital Love" && !u.seen {
		u.seen = true
		close(u.playing)
	}
	return nil
}

func TestKeyboardStyleEventsReachPlayerAndUI(t *testing.T) {
	ui := &observingUI{playing: make(chan struct{})}
	fakePlayer := player.NewFake()
	application := app.New(fakePlayer, library.NewFake(), storage.NewMemory(50), scriptedInput{playing: ui.playing}, ui)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := application.Run(ctx); err != nil {
		t.Fatal(err)
	}
	if !ui.seen || fakePlayer.Track.Title != "Digital Love" || !fakePlayer.Playing {
		t.Fatalf("flow incomplete: UI saw playing=%t, player track=%q, player playing=%t", ui.seen, fakePlayer.Track.Title, fakePlayer.Playing)
	}
}

func TestRunWaitsForInputShutdown(t *testing.T) {
	started := make(chan struct{})
	application := app.New(player.NewFake(), library.NewFake(), storage.NewMemory(50), waitingInput{started}, silentUI{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() { result <- application.Run(ctx) }()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("input did not start")
	}
	cancel()
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("App did not stop after cancellation")
	}
}

func TestRunDrainsEventsWhenInputEnds(t *testing.T) {
	ui := &observingUI{playing: make(chan struct{})}
	fakePlayer := player.NewFake()
	application := app.New(fakePlayer, library.NewFake(), storage.NewMemory(50), burstInput{}, ui)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := application.Run(ctx); err != nil {
		t.Fatal(err)
	}
	if !ui.seen || fakePlayer.Track.Title != "Digital Love" {
		t.Fatalf("events were lost at input EOF: UI saw playing=%t, player track=%q", ui.seen, fakePlayer.Track.Title)
	}
}
