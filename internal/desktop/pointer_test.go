package desktop

import (
	"context"
	"fmt"
	"image"
	"reflect"
	"testing"
	"time"

	"github.com/OpeniPod/OpeniPod/internal/app"
	"github.com/OpeniPod/OpeniPod/internal/library"
	"github.com/OpeniPod/OpeniPod/internal/music"
	"github.com/OpeniPod/OpeniPod/internal/player"
	"github.com/OpeniPod/OpeniPod/internal/storage"
	"github.com/OpeniPod/OpeniPod/internal/ui"
	"golang.org/x/mobile/event/key"
	"golang.org/x/mobile/event/lifecycle"
	"golang.org/x/mobile/event/mouse"
	"golang.org/x/mobile/event/size"
)

func clickControl(t *testing.T, state app.AppState, want app.Event, size image.Point) mouse.Event {
	t.Helper()
	for _, control := range ui.Controls(state) {
		if !reflect.DeepEqual(control.Event, want) {
			continue
		}
		center := control.Bounds.Min.Add(control.Bounds.Size().Div(2))
		viewport := fit(size)
		return mouse.Event{
			X:      float32(viewport.Min.X) + (float32(center.X)+0.5)*float32(viewport.Dx())/ui.ScreenWidth,
			Y:      float32(viewport.Min.Y) + (float32(center.Y)+0.5)*float32(viewport.Dy())/ui.ScreenHeight,
			Button: mouse.ButtonLeft, Direction: mouse.DirPress,
		}
	}
	t.Fatalf("no control for %#v in screen %v", want, state.Screen)
	return mouse.Event{}
}

func TestPointerFollowsScaledAndResizedDisplay(t *testing.T) {
	for _, size := range []image.Point{
		image.Pt(240, 320), image.Pt(480, 640), image.Pt(720, 960), image.Pt(960, 1280),
		image.Pt(640, 320), image.Pt(240, 640), image.Pt(357, 481), image.Pt(120, 160),
	} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			window, native := newTestWindow(t)
			state := app.AppState{Screen: app.ScreenHome}
			if err := window.Render(state); err != nil {
				t.Fatal(err)
			}
			// Exercise the native event loop's resize and mouse paths together.
			native.Send(sizeEvent(size))
			want := app.ItemActivated{Screen: app.ScreenHome, Index: 2}
			native.Send(clickControl(t, state, want, size))
			native.Send(lifecycle.Event{To: lifecycle.StageDead})
			events := make(chan app.Event, 2)
			if err := window.Run(context.Background(), events); err != nil {
				t.Fatal(err)
			}
			if got := receive(t, events); !reflect.DeepEqual(got, want) {
				t.Fatalf("click = %#v, want %#v", got, want)
			}
			if got := receive(t, events); got != (app.QuitRequested{}) {
				t.Fatalf("close = %#v", got)
			}
		})
	}
}

func sizeEvent(point image.Point) size.Event {
	return size.Event{WidthPx: point.X, HeightPx: point.Y}
}

func TestPointerIgnoresBordersAndNonPresses(t *testing.T) {
	window, _ := newTestWindow(t)
	if got := window.pointerEvent(mouse.Event{X: 30, Y: 70, Button: mouse.ButtonLeft, Direction: mouse.DirPress}); got != nil {
		t.Fatalf("click before rendering = %#v", got)
	}
	state := app.AppState{Screen: app.ScreenHome}
	if err := window.Render(state); err != nil {
		t.Fatal(err)
	}
	valid := clickControl(t, state, app.ItemActivated{Screen: app.ScreenHome, Index: 0}, window.size)
	for _, event := range []mouse.Event{
		{X: valid.X, Y: valid.Y, Button: mouse.ButtonLeft, Direction: mouse.DirRelease},
		{X: valid.X, Y: valid.Y, Button: mouse.ButtonRight, Direction: mouse.DirPress},
		{X: valid.X, Y: valid.Y, Button: mouse.ButtonMiddle, Direction: mouse.DirPress},
		{X: valid.X, Y: valid.Y, Direction: mouse.DirNone},
		{X: -0.1, Y: valid.Y, Button: mouse.ButtonLeft, Direction: mouse.DirPress},
		{X: 240, Y: valid.Y, Button: mouse.ButtonLeft, Direction: mouse.DirPress},
		{X: valid.X, Y: 320, Button: mouse.ButtonLeft, Direction: mouse.DirPress},
		{X: 120, Y: 10, Button: mouse.ButtonLeft, Direction: mouse.DirPress},
	} {
		if got := window.pointerEvent(event); got != nil {
			t.Errorf("event %+v activated %#v", event, got)
		}
	}
	for _, size := range []image.Point{image.Pt(640, 320), image.Pt(240, 640), image.Pt(0, 0), image.Pt(1, 1)} {
		if err := window.resize(size); err != nil {
			t.Fatal(err)
		}
		if got := window.pointerEvent(mouse.Event{X: 0, Y: 0, Button: mouse.ButtonLeft, Direction: mouse.DirPress}); got != nil {
			t.Errorf("border click at size %v activated %#v", size, got)
		}
	}
}

func TestPointerRegionsFollowLatestRenderedScreen(t *testing.T) {
	window, _ := newTestWindow(t)
	state := app.AppState{Screen: app.ScreenHome}
	if err := window.Render(state); err != nil {
		t.Fatal(err)
	}
	click := clickControl(t, state, app.ItemActivated{Screen: app.ScreenHome, Index: 0}, window.size)
	if err := window.Render(app.AppState{Screen: app.ScreenTracks}); err != nil {
		t.Fatal(err)
	}
	if got := window.pointerEvent(click); got != nil {
		t.Fatalf("empty library used stale Home hit region: %#v", got)
	}
}

type observingWindow struct {
	*Window
	states chan app.AppState
}

func (w observingWindow) Render(state app.AppState) error {
	if err := w.Window.Render(state); err != nil {
		return err
	}
	w.states <- state
	return nil
}

// Use the real window input, renderer, App loop, and fake backends: no display
// server or audio hardware is required for this acceptance flow.
func TestMouseControlsReachAppAndPlayer(t *testing.T) {
	window, native := newTestWindow(t)
	window.size = image.Pt(480, 640)
	tracks := make([]music.Track, 10)
	for i := range tracks {
		tracks[i] = music.Track{ID: music.TrackID(fmt.Sprintf("track-%d", i)), Title: fmt.Sprintf("Track %d", i)}
	}
	fakePlayer := player.NewFake()
	settings := storage.NewMemory(50)
	states := make(chan app.AppState, 64)
	application := app.New(fakePlayer, &library.Fake{Tracks: tracks}, settings, window, observingWindow{window, states})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	done := make(chan error, 1)
	go func() { done <- application.Run(ctx) }()
	finished := false
	t.Cleanup(func() {
		cancel()
		if !finished {
			if err := receive(t, done); err != nil {
				t.Errorf("application: %v", err)
			}
		}
	})
	var state app.AppState
	waitFor := func(description string, accept func(app.AppState) bool) {
		t.Helper()
		for {
			state = receive(t, states)
			if accept(state) {
				return
			}
			if ctx.Err() != nil {
				t.Fatalf("waiting for %s: %v", description, ctx.Err())
			}
		}
	}
	click := func(event app.Event) {
		t.Helper()
		e := clickControl(t, state, event, window.size)
		native.Send(e)
		e.Direction = mouse.DirRelease
		native.Send(e)
	}
	waitFor("loaded library", func(s app.AppState) bool { return len(s.Library.Tracks) == len(tracks) })
	click(app.ItemActivated{Screen: app.ScreenHome, Index: 0})
	waitFor("track list", func(s app.AppState) bool { return s.Screen == app.ScreenTracks })
	click(app.Scroll{Delta: 4})
	waitFor("second screen of tracks", func(s app.AppState) bool { return s.Navigation.TrackIndex == 4 })
	click(app.ItemActivated{Screen: app.ScreenTracks, Index: 4})
	waitFor("play clicked track", func(s app.AppState) bool {
		return s.Screen == app.ScreenNowPlaying && s.Player.Status == app.StatusPlaying && s.Player.Track.ID == tracks[4].ID
	})
	click(app.PlayPausePressed{})
	waitFor("pause", func(s app.AppState) bool { return s.Player.Status == app.StatusPaused })
	click(app.PlayPausePressed{})
	waitFor("resume", func(s app.AppState) bool { return s.Player.Status == app.StatusPlaying })
	click(app.NextPressed{})
	waitFor("next", func(s app.AppState) bool {
		return s.Player.Track.ID == tracks[5].ID && s.Player.Status == app.StatusPlaying
	})
	click(app.PreviousPressed{})
	waitFor("previous", func(s app.AppState) bool {
		return s.Player.Track.ID == tracks[4].ID && s.Player.Status == app.StatusPlaying
	})
	click(app.VolumeChanged{Delta: 10})
	waitFor("volume up", func(s app.AppState) bool { return s.Settings.Volume == 60 })
	click(app.VolumeChanged{Delta: -10})
	waitFor("volume down", func(s app.AppState) bool { return s.Settings.Volume == 50 })
	click(app.BackPressed{})
	waitFor("return to menu", func(s app.AppState) bool { return s.Screen == app.ScreenHome })
	// Keyboard navigation, selection, pause, volume and Back still work after
	// the mouse has driven the complete playback flow.
	native.Send(key.Event{Code: key.CodeDownArrow, Direction: key.DirPress})
	native.Send(key.Event{Code: key.CodeReturnEnter, Direction: key.DirPress})
	waitFor("keyboard Now Playing", func(s app.AppState) bool { return s.Screen == app.ScreenNowPlaying })
	native.Send(key.Event{Code: key.CodeSpacebar, Direction: key.DirPress})
	waitFor("keyboard pause", func(s app.AppState) bool { return s.Player.Status == app.StatusPaused })
	native.Send(key.Event{Code: key.CodeEqualSign, Direction: key.DirPress})
	waitFor("keyboard volume", func(s app.AppState) bool { return s.Settings.Volume == 60 })
	native.Send(key.Event{Code: key.CodeEscape, Direction: key.DirPress})
	waitFor("keyboard Back", func(s app.AppState) bool { return s.Screen == app.ScreenHome })
	native.Send(lifecycle.Event{To: lifecycle.StageDead})
	err := receive(t, done)
	finished = true
	if err != nil {
		t.Fatal(err)
	}
	if fakePlayer.Track.ID != tracks[4].ID || fakePlayer.Playing || fakePlayer.Volume != 60 {
		t.Fatalf("player did not receive controls: track=%s playing=%t volume=%d", fakePlayer.Track.ID, fakePlayer.Playing, fakePlayer.Volume)
	}
	saved, err := settings.LoadSettings(context.Background())
	if err != nil || saved.Volume != 60 {
		t.Fatalf("saved settings = %+v, error = %v", saved, err)
	}
}
