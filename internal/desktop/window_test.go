package desktop

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/OpeniPod/OpeniPod/internal/app"
	"github.com/OpeniPod/OpeniPod/internal/ui"
	"golang.org/x/exp/shiny/screen"
	"golang.org/x/mobile/event/key"
	"golang.org/x/mobile/event/lifecycle"
	"golang.org/x/mobile/event/paint"
	"golang.org/x/mobile/event/size"
)

// Only the driver boundary is replaced: tests exercise actual display rendering
// and window input without requiring an X server or an audio device in CI.
type testNative struct {
	screen.Window
	events  chan interface{}
	frames  chan *image.RGBA
	pending *image.RGBA
	waiting chan struct{}
}

func (n *testNative) Send(event interface{}) { n.events <- event }
func (n *testNative) NextEvent() interface{} {
	select {
	case n.waiting <- struct{}{}:
	default:
	}
	return <-n.events
}
func (n *testNative) Upload(_ image.Point, buffer screen.Buffer, _ image.Rectangle) {
	n.pending = image.NewRGBA(buffer.Bounds())
	draw.Draw(n.pending, n.pending.Bounds(), buffer.RGBA(), image.Point{}, draw.Src)
}
func (n *testNative) Publish() screen.PublishResult {
	n.frames <- n.pending
	return screen.PublishResult{}
}

type testScreen struct {
	screen.Screen
	err error
}

func (s *testScreen) NewBuffer(size image.Point) (screen.Buffer, error) {
	if s.err != nil {
		return nil, s.err
	}
	return &testBuffer{image: image.NewRGBA(image.Rectangle{Max: size})}, nil
}

type testBuffer struct{ image *image.RGBA }

func (b *testBuffer) Release()                {}
func (b *testBuffer) Size() image.Point       { return b.image.Bounds().Size() }
func (b *testBuffer) Bounds() image.Rectangle { return b.image.Bounds() }
func (b *testBuffer) RGBA() *image.RGBA       { return b.image }

func newTestWindow(t *testing.T) (*Window, *testNative) {
	t.Helper()
	display, err := ui.NewDisplay()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := display.Close(); err != nil {
			t.Error(err)
		}
	})
	native := &testNative{events: make(chan interface{}, 64), frames: make(chan *image.RGBA, 128)}
	return &Window{screen: &testScreen{}, native: native, display: display,
		size: image.Pt(ui.ScreenWidth, ui.ScreenHeight)}, native
}

func receive[T any](t *testing.T, values <-chan T) T {
	t.Helper()
	select {
	case value := <-values:
		return value
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for window")
		var zero T
		return zero
	}
}

func TestWindowPresentsAndRepaintsLatestState(t *testing.T) {
	window, native := newTestWindow(t)
	state := app.AppState{Screen: app.ScreenHome, Settings: app.DefaultSettings()}
	if err := window.Render(state); err != nil {
		t.Fatal(err)
	}
	first := receive(t, native.frames)
	if !bytes.Equal(first.Pix, window.display.Render(state).Pix) {
		t.Fatal("window did not show the existing display renderer's image")
	}
	state.Screen = app.ScreenSettings
	if err := window.Render(state); err != nil {
		t.Fatal(err)
	}
	latest := receive(t, native.frames)
	if bytes.Equal(first.Pix, latest.Pix) {
		t.Fatal("screen change did not update the window")
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- window.Run(ctx, make(chan app.Event)) }()
	native.Send(paint.Event{})
	repainted := receive(t, native.frames)
	cancel()
	if err := receive(t, done); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(latest.Pix, repainted.Pix) {
		t.Fatal("expose did not restore the last frame")
	}
}

func TestWindowResizePreservesAspectRatio(t *testing.T) {
	window, native := newTestWindow(t)
	if err := window.Render(app.AppState{}); err != nil {
		t.Fatal(err)
	}
	original := receive(t, native.frames)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- window.Run(ctx, make(chan app.Event)) }()
	native.Send(size.Event{WidthPx: 640, HeightPx: 320})
	resized := receive(t, native.frames)
	cancel()
	if err := receive(t, done); err != nil {
		t.Fatal(err)
	}
	if resized.Bounds() != image.Rect(0, 0, 640, 320) {
		t.Fatalf("resized bounds = %v", resized.Bounds())
	}
	if resized.RGBAAt(0, 0) != (color.RGBA{A: 255}) || resized.RGBAAt(639, 319) != (color.RGBA{A: 255}) {
		t.Fatal("resize did not clear letterbox borders")
	}
	for y := 0; y < ui.ScreenHeight; y++ {
		for x := 0; x < ui.ScreenWidth; x++ {
			if resized.RGBAAt(x+200, y) != original.RGBAAt(x, y) {
				t.Fatal("resize distorted the display")
			}
		}
	}
}

func TestWindowInputAndClose(t *testing.T) {
	window, native := newTestWindow(t)
	native.Send(key.Event{Code: key.CodeDownArrow, Direction: key.DirPress})
	native.Send(key.Event{Code: key.CodeReturnEnter, Direction: key.DirPress})
	native.Send(lifecycle.Event{To: lifecycle.StageDead})
	events := make(chan app.Event, 3)
	if err := window.Run(context.Background(), events); err != nil {
		t.Fatal(err)
	}
	for _, want := range []app.Event{app.Scroll{Delta: 1}, app.SelectPressed{}, app.QuitRequested{}} {
		if got := receive(t, events); !reflect.DeepEqual(got, want) {
			t.Fatalf("event = %#v, want %#v", got, want)
		}
	}
}

func TestWindowCancellation(t *testing.T) {
	for _, blockedSend := range []bool{false, true} {
		t.Run(map[bool]string{false: "waiting for native event", true: "waiting for app"}[blockedSend], func(t *testing.T) {
			window, native := newTestWindow(t)
			native.waiting = make(chan struct{}, 1)
			if blockedSend {
				native.Send(key.Event{Code: key.CodeJ, Direction: key.DirPress})
			}
			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan error, 1)
			go func() { done <- window.Run(ctx, make(chan app.Event)) }()
			receive(t, native.waiting)
			cancel()
			if err := receive(t, done); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestWindowReportsDriverErrors(t *testing.T) {
	for _, source := range []string{"input", "render", "resize"} {
		t.Run(source, func(t *testing.T) {
			window, native := newTestWindow(t)
			failure := errors.New("driver failure")
			var err error
			switch source {
			case "input":
				native.Send(failure)
				err = window.Run(context.Background(), make(chan app.Event))
			case "render":
				window.screen = &testScreen{err: failure}
				err = window.Render(app.AppState{})
			case "resize":
				if err := window.Render(app.AppState{}); err != nil {
					t.Fatal(err)
				}
				window.screen = &testScreen{err: failure}
				native.Send(size.Event{WidthPx: 480, HeightPx: 640})
				err = window.Run(context.Background(), make(chan app.Event))
			}
			if !errors.Is(err, failure) {
				t.Fatalf("got %v, want driver failure", err)
			}
		})
	}
}

func TestWindowConcurrentRenderingAndResize(t *testing.T) {
	window, native := newTestWindow(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- window.Run(ctx, make(chan app.Event)) }()
	var worker sync.WaitGroup
	worker.Add(1)
	go func() {
		defer worker.Done()
		for i := 0; i < 10; i++ {
			native.Send(size.Event{WidthPx: 240 + i, HeightPx: 320 + i})
			native.Send(paint.Event{})
		}
	}()
	for i := 0; i < 10; i++ {
		if err := window.Render(app.AppState{Screen: app.Screen(i % 4)}); err != nil {
			t.Error(err)
		}
	}
	worker.Wait()
	cancel()
	if err := receive(t, done); err != nil {
		t.Fatal(err)
	}
}
