package app_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/OpeniPod/OpeniPod/internal/app"
	"github.com/OpeniPod/OpeniPod/internal/library"
	"github.com/OpeniPod/OpeniPod/internal/music"
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

type sequenceInput struct{ events []app.Event }

func (s sequenceInput) Run(ctx context.Context, events chan<- app.Event) error {
	for _, event := range s.events {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case events <- event:
		}
	}
	return nil
}

type recordingUI struct{ states []app.AppState }

func (u *recordingUI) Render(state app.AppState) error {
	u.states = append(u.states, state)
	return nil
}

type refreshTestLibrary struct {
	load    func(context.Context) ([]music.Track, error)
	refresh func(context.Context) ([]music.Track, error)
}

func (l refreshTestLibrary) Load(ctx context.Context) ([]music.Track, error) {
	return l.load(ctx)
}

func (l refreshTestLibrary) Refresh(ctx context.Context) ([]music.Track, error) {
	return l.refresh(ctx)
}

func TestUpdateLibraryMenuRunsRefresh(t *testing.T) {
	for _, tc := range []struct {
		name       string
		loadErr    error
		refreshErr error
		tracks     []music.Track
		wantID     music.TrackID
		wantError  string
	}{
		{name: "success", tracks: []music.Track{{ID: "new"}}, wantID: "new"},
		{name: "empty library"},
		{name: "failure preserves tracks", refreshErr: errors.New("scan failed"), wantID: "old", wantError: "update library: scan failed"},
		{name: "recovers from startup failure", loadErr: errors.New("bad index"), tracks: []music.Track{{ID: "recovered"}}, wantID: "recovered"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ui := &recordingUI{}
			loads, refreshes := 0, 0
			rendersBeforeRefresh := -1
			lib := refreshTestLibrary{
				load: func(context.Context) ([]music.Track, error) {
					loads++
					if tc.loadErr != nil {
						return nil, tc.loadErr
					}
					return []music.Track{{ID: "old"}}, nil
				},
				refresh: func(context.Context) ([]music.Track, error) {
					rendersBeforeRefresh = len(ui.states)
					refreshes++
					return tc.tracks, tc.refreshErr
				},
			}
			application := app.New(player.NewFake(), lib, storage.NewMemory(50), sequenceInput{[]app.Event{
				app.Scroll{Delta: 2}, app.SelectPressed{}, app.SelectPressed{},
			}}, ui)
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			if err := application.Run(ctx); err != nil {
				t.Fatal(err)
			}
			if loads != 1 || refreshes != 1 {
				t.Fatalf("load calls = %d, refresh calls = %d, want 1 each", loads, refreshes)
			}
			if len(ui.states) != rendersBeforeRefresh+1 {
				t.Fatalf("render count after refresh = %d, want %d", len(ui.states), rendersBeforeRefresh+1)
			}
			state := ui.states[len(ui.states)-1]
			if state.Error != tc.wantError {
				t.Fatalf("refresh result state = %+v", state)
			}
			if tc.wantID == "" {
				if len(state.Library.Tracks) != 0 {
					t.Fatalf("tracks = %+v, want empty library", state.Library.Tracks)
				}
			} else if len(state.Library.Tracks) != 1 || state.Library.Tracks[0].ID != tc.wantID {
				t.Fatalf("tracks = %+v, want track %q", state.Library.Tracks, tc.wantID)
			}
		})
	}
}

func TestUpdateLibraryCancellationStopsApp(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	refreshed := false
	lib := refreshTestLibrary{
		load: func(context.Context) ([]music.Track, error) { return nil, nil },
		refresh: func(refreshCtx context.Context) ([]music.Track, error) {
			refreshed = true
			cancel()
			return nil, refreshCtx.Err()
		},
	}
	ui := &recordingUI{}
	application := app.New(player.NewFake(), lib, storage.NewMemory(50), sequenceInput{[]app.Event{
		app.Scroll{Delta: 2}, app.SelectPressed{}, app.SelectPressed{},
	}}, ui)
	if err := application.Run(ctx); err != nil {
		t.Fatal(err)
	}
	if !refreshed {
		t.Fatal("refresh was not requested")
	}
	for _, state := range ui.states {
		if strings.Contains(state.Error, "context canceled") {
			t.Fatalf("cancellation shown as library error: %q", state.Error)
		}
	}
}
