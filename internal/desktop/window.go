// Package desktop presents the display renderer in an interactive native window.
package desktop

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"sync"

	"github.com/OpeniPod/OpeniPod/internal/app"
	"github.com/OpeniPod/OpeniPod/internal/ui"
	"golang.org/x/exp/shiny/driver"
	"golang.org/x/exp/shiny/screen"
	"golang.org/x/image/draw"
	"golang.org/x/mobile/event/key"
	"golang.org/x/mobile/event/lifecycle"
	"golang.org/x/mobile/event/paint"
	"golang.org/x/mobile/event/size"
)

// Window implements App's input and renderer interfaces. App owns its state;
// this adapter only retains the last rendered image for expose and resize events.
type Window struct {
	screen  screen.Screen
	native  screen.Window
	display *ui.Display

	// Render runs in App's loop; resize and expose run in its input worker.
	mu     sync.Mutex
	buffer screen.Buffer
	frame  *image.RGBA
	size   image.Point
}

var (
	_ app.Input    = (*Window)(nil)
	_ app.Renderer = (*Window)(nil)
)

// Run opens a window and calls run with its input/renderer adapter. Call it from
// main: native drivers can require the main OS thread. The callback must stop
// its input worker before returning (App.Run already does this).
func Run(scale int, run func(*Window) error) (runErr error) {
	if scale < 1 || scale > 4 {
		return fmt.Errorf("window scale must be between 1 and 4")
	}
	driver.Main(func(s screen.Screen) {
		native, err := s.NewWindow(&screen.NewWindowOptions{
			Title:  "OpeniPod",
			Width:  ui.ScreenWidth * scale,
			Height: ui.ScreenHeight * scale,
		})
		if err != nil {
			runErr = fmt.Errorf("open window (Linux requires an X11 display): %w", err)
			return
		}
		defer native.Release()
		display, err := ui.NewDisplay()
		if err != nil {
			runErr = err
			return
		}
		defer func() { runErr = errors.Join(runErr, display.Close()) }()
		w := &Window{screen: s, native: native, display: display,
			size: image.Pt(ui.ScreenWidth*scale, ui.ScreenHeight*scale)}
		defer func() {
			if w.buffer != nil {
				w.buffer.Release()
			}
		}()
		runErr = run(w)
	})
	return runErr
}

// Render draws a snapshot without retaining or modifying application state.
func (w *Window) Render(state app.AppState) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.frame = w.display.Render(state)
	return w.present()
}

type wakeup struct{}

// Run translates window events to typed application events.
func (w *Window) Run(ctx context.Context, events chan<- app.Event) error {
	// NextEvent blocks without a context. Wake it on cancellation, and wait for
	// that wakeup to finish before the owner can release the native window.
	wakeDone := make(chan struct{})
	stop := context.AfterFunc(ctx, func() {
		defer close(wakeDone)
		w.native.Send(wakeup{})
	})
	defer func() {
		if !stop() {
			<-wakeDone
		}
	}()

	for ctx.Err() == nil {
		event := w.native.NextEvent()
		if ctx.Err() != nil {
			return nil
		}
		var input app.Event
		switch e := event.(type) {
		case key.Event:
			input, _ = keyEvent(e)
		case lifecycle.Event:
			if e.To == lifecycle.StageDead {
				input = app.QuitRequested{}
			}
		case size.Event:
			if err := w.resize(image.Pt(e.WidthPx, e.HeightPx)); err != nil {
				return err
			}
		case paint.Event:
			if err := w.repaint(); err != nil {
				return err
			}
		case error:
			return fmt.Errorf("window: %w", e)
		}
		if input != nil {
			select {
			case <-ctx.Done():
				return nil
			case events <- input:
			}
			if _, quit := input.(app.QuitRequested); quit {
				return nil
			}
		}
	}
	return nil
}

func (w *Window) resize(size image.Point) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.size = size
	return w.present()
}

func (w *Window) repaint() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.present()
}

// present is called with mu held. Buffer memory is never modified during upload.
func (w *Window) present() error {
	if w.frame == nil || w.size.X <= 0 || w.size.Y <= 0 {
		return nil
	}
	if w.buffer == nil || w.buffer.Size() != w.size {
		buffer, err := w.screen.NewBuffer(w.size)
		if err != nil {
			return fmt.Errorf("allocate window frame: %w", err)
		}
		if w.buffer != nil {
			w.buffer.Release()
		}
		w.buffer = buffer
	}
	dst := w.buffer.RGBA()
	draw.Draw(dst, dst.Bounds(), image.NewUniform(color.Black), image.Point{}, draw.Src)
	draw.NearestNeighbor.Scale(dst, fit(w.size), w.frame, w.frame.Bounds(), draw.Src, nil)
	w.native.Upload(image.Point{}, w.buffer, w.buffer.Bounds())
	w.native.Publish()
	return nil
}

// fit preserves the 240:320 aspect ratio, centering the image after a resize.
func fit(size image.Point) image.Rectangle {
	width, height := size.X, size.Y
	if width*ui.ScreenHeight > height*ui.ScreenWidth {
		width = height * ui.ScreenWidth / ui.ScreenHeight
	} else {
		height = width * ui.ScreenHeight / ui.ScreenWidth
	}
	left, top := (size.X-width)/2, (size.Y-height)/2
	return image.Rect(left, top, left+width, top+height)
}
