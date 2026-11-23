package desktop

import (
	"context"
	"image"
	"testing"

	"github.com/OpeniPod/OpeniPod/internal/app"
	"golang.org/x/mobile/event/lifecycle"
	"golang.org/x/mobile/event/mouse"
)

func TestTapUsesRenderedPixelAtSmallSize(t *testing.T) {
	window, native := newTestWindow(t)
	state := app.AppState{Screen: app.ScreenHome}
	if err := window.Render(state); err != nil {
		t.Fatal(err)
	}
	if err := window.resize(image.Pt(120, 160)); err != nil {
		t.Fatal(err)
	}
	receive(t, native.frames)
	frame := receive(t, native.frames)
	if frame.RGBAAt(12, 28) == frame.RGBAAt(12, 27) || frame.RGBAAt(12, 28) != frame.RGBAAt(12, 29) {
		t.Fatal("expected the first visible row to start at y=28")
	}

	// The center of destination pixel (12, 28) maps to logical pixel (25, 57).
	// That is the visible top edge of the first Home control. Mapping the
	// destination edge instead would produce y=56 and miss it.
	press := mouse.Event{X: 12, Y: 28, Button: mouse.ButtonLeft, Direction: mouse.DirPress}
	if got := window.pointerEvent(press); got != nil {
		t.Fatalf("press emitted %#v before release", got)
	}
	press.Direction = mouse.DirRelease
	want := app.ItemActivated{Screen: app.ScreenHome, Index: 0}
	if got := window.pointerEvent(press); got != want {
		t.Fatalf("tap at visible edge = %#v, want %#v", got, want)
	}
}

func TestTapNeedsMatchingPressAndRelease(t *testing.T) {
	window, _ := newTestWindow(t)
	if err := window.Render(app.AppState{Screen: app.ScreenHome}); err != nil {
		t.Fatal(err)
	}
	first := logicalMouseEvent(image.Pt(50, 74), mouse.DirRelease)
	if got := window.pointerEvent(first); got != nil {
		t.Fatalf("unpaired release activated %#v", got)
	}

	press := logicalMouseEvent(image.Pt(50, 74), mouse.DirPress)
	window.pointerEvent(press)
	if got := window.pointerEvent(logicalMouseEvent(image.Pt(50, 110), mouse.DirRelease)); got != nil {
		t.Fatalf("release on another control activated %#v", got)
	}
}

func TestTapCancelsAfterDragEvenWhenPointerReturns(t *testing.T) {
	window, _ := newTestWindow(t)
	if err := window.Render(app.AppState{Screen: app.ScreenHome}); err != nil {
		t.Fatal(err)
	}
	window.pointerEvent(logicalMouseEvent(image.Pt(50, 74), mouse.DirPress))
	window.pointerEvent(logicalMouseEvent(image.Pt(70, 74), mouse.DirNone))
	window.pointerEvent(logicalMouseEvent(image.Pt(50, 74), mouse.DirNone))
	if got := window.pointerEvent(logicalMouseEvent(image.Pt(50, 74), mouse.DirRelease)); got != nil {
		t.Fatalf("drag back to the pressed control activated %#v", got)
	}
}

func TestTapSurvivesRepaintAndAllowsSmallPointerJitter(t *testing.T) {
	window, _ := newTestWindow(t)
	state := app.AppState{Screen: app.ScreenHome}
	if err := window.Render(state); err != nil {
		t.Fatal(err)
	}
	window.pointerEvent(logicalMouseEvent(image.Pt(50, 74), mouse.DirPress))
	if err := window.Render(state); err != nil {
		t.Fatal(err)
	}
	want := app.ItemActivated{Screen: app.ScreenHome, Index: 0}
	if got := window.pointerEvent(logicalMouseEvent(image.Pt(53, 76), mouse.DirRelease)); got != want {
		t.Fatalf("tap after repaint and small jitter = %#v, want %#v", got, want)
	}
}

func TestResizeCancelsPendingTap(t *testing.T) {
	window, _ := newTestWindow(t)
	if err := window.Render(app.AppState{Screen: app.ScreenHome}); err != nil {
		t.Fatal(err)
	}
	window.pointerEvent(logicalMouseEvent(image.Pt(50, 74), mouse.DirPress))
	if err := window.resize(image.Pt(480, 640)); err != nil {
		t.Fatal(err)
	}
	if got := window.pointerEvent(logicalMouseEvent(image.Pt(100, 148), mouse.DirRelease)); got != nil {
		t.Fatalf("tap after resize activated %#v", got)
	}
}

func TestFocusLossCancelsPendingTap(t *testing.T) {
	window, native := newTestWindow(t)
	if err := window.Render(app.AppState{Screen: app.ScreenHome}); err != nil {
		t.Fatal(err)
	}
	press := logicalMouseEvent(image.Pt(50, 74), mouse.DirPress)
	release := press
	release.Direction = mouse.DirRelease
	native.Send(press)
	native.Send(lifecycle.Event{From: lifecycle.StageFocused, To: lifecycle.StageVisible})
	native.Send(release)
	native.Send(lifecycle.Event{From: lifecycle.StageVisible, To: lifecycle.StageDead})
	events := make(chan app.Event, 2)
	if err := window.Run(context.Background(), events); err != nil {
		t.Fatal(err)
	}
	if got := receive(t, events); got != (app.QuitRequested{}) {
		t.Fatalf("event after focus loss = %#v, want quit only", got)
	}
	select {
	case got := <-events:
		t.Fatalf("unexpected event after focus loss: %#v", got)
	default:
	}
}

func logicalMouseEvent(point image.Point, direction mouse.Direction) mouse.Event {
	button := mouse.ButtonLeft
	if direction == mouse.DirNone {
		button = mouse.ButtonNone
	}
	return mouse.Event{X: float32(point.X), Y: float32(point.Y),
		Button: button, Direction: direction}
}
