package st7789

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"reflect"
	"testing"
	"time"

	"github.com/OpeniPod/OpeniPod/internal/app"
	"github.com/OpeniPod/OpeniPod/internal/ui"
)

type operation struct {
	kind string
	data []byte
}

type fakeTransport struct {
	ops           []operation
	failAt        int
	err, closeErr error
	closes        int
}

func (f *fakeTransport) record(kind string, data []byte) error {
	f.ops = append(f.ops, operation{kind, bytes.Clone(data)})
	if len(f.ops) == f.failAt {
		return f.err
	}
	return nil
}
func (f *fakeTransport) Reset() error           { return f.record("reset", nil) }
func (f *fakeTransport) Command(b byte) error   { return f.record("command", []byte{b}) }
func (f *fakeTransport) Data(data []byte) error { return f.record("data", data) }
func (f *fakeTransport) Close() error           { f.closes++; return f.closeErr }

func ready(t *testing.T, f *fakeTransport, config Config) *Display {
	t.Helper()
	d, err := newDisplay(f, config, func(time.Duration) {})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	f.ops = nil
	return d
}

func TestInitialization(t *testing.T) {
	for _, config := range []Config{{}, {MirrorX: true, MirrorY: true, BGR: true, Invert: true}} {
		f := &fakeTransport{}
		var delays []time.Duration
		d, err := newDisplay(f, config, func(delay time.Duration) { delays = append(delays, delay) })
		if err != nil {
			t.Fatal(err)
		}
		madctl, inversion := byte(0), byte(0x20)
		if config.BGR {
			madctl, inversion = 0xc8, 0x21
		}
		want := []operation{
			{"reset", nil}, {"command", []byte{0x01}}, {"command", []byte{0x11}},
			{"command", []byte{0x3a}}, {"data", []byte{0x55}},
			{"command", []byte{0x36}}, {"data", []byte{madctl}},
			{"command", []byte{inversion}}, {"command", []byte{0x13}}, {"command", []byte{0x29}},
		}
		if !reflect.DeepEqual(f.ops, want) {
			t.Fatalf("initialization = %v, want %v", f.ops, want)
		}
		wantDelays := []time.Duration{150 * time.Millisecond, 120 * time.Millisecond, 10 * time.Millisecond, 10 * time.Millisecond, 120 * time.Millisecond}
		if !reflect.DeepEqual(delays, wantDelays) {
			t.Fatalf("delays = %v", delays)
		}
		if err := d.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestInitializationErrors(t *testing.T) {
	failure, closeFailure := errors.New("device failed"), errors.New("close failed")
	for step := 1; step <= 10; step++ {
		f := &fakeTransport{failAt: step, err: failure, closeErr: closeFailure}
		d, err := newDisplay(f, Config{}, func(time.Duration) {})
		if d != nil || !errors.Is(err, failure) || !errors.Is(err, closeFailure) || f.closes != 1 || len(f.ops) != step {
			t.Fatalf("step %d: display=%v, err=%v, operations=%d, closes=%d", step, d, err, len(f.ops), f.closes)
		}
	}
	if _, err := New(nil, Config{}); err == nil {
		t.Fatal("nil transport accepted")
	}
}

func TestFrame(t *testing.T) {
	f := &fakeTransport{}
	d := ready(t, f, Config{XOffset: 2, YOffset: 3})
	// Nonzero-origin image with transparency and markers across row boundaries.
	img := image.NewNRGBA(image.Rect(5, 7, 5+Width, 7+Height))
	colors := []color.NRGBA{{R: 255, A: 255}, {G: 255, A: 255}, {B: 255, A: 255}, {R: 255, G: 255, B: 255, A: 255}, {R: 255, A: 0}, {R: 255, A: 128}}
	for i, c := range colors {
		img.SetNRGBA(5+i, 7, c)
	}
	img.SetNRGBA(5, 8, color.NRGBA{R: 255, G: 255, A: 255})
	img.SetNRGBA(5+Width-1, 7+Height-1, color.NRGBA{B: 255, A: 255})
	if err := d.Draw(img); err != nil {
		t.Fatal(err)
	}
	wantWindow := []operation{
		{"command", []byte{0x2a}}, {"data", []byte{0, 2, 0, 241}},
		{"command", []byte{0x2b}}, {"data", []byte{0, 3, 1, 66}}, {"command", []byte{0x2c}},
	}
	if len(f.ops) != 6 || !reflect.DeepEqual(f.ops[:5], wantWindow) {
		t.Fatalf("frame operations = %v", f.ops)
	}
	data := f.ops[5].data
	wantPixels := []byte{0xf8, 0, 0x07, 0xe0, 0, 0x1f, 0xff, 0xff, 0, 0, 0x80, 0}
	if len(data) != Width*Height*2 || !bytes.Equal(data[:12], wantPixels) {
		t.Fatalf("RGB565 frame length=%d, first pixels=%x", len(data), data[:min(12, len(data))])
	}
	if !bytes.Equal(data[Width*2:Width*2+2], []byte{0xff, 0xe0}) || !bytes.Equal(data[len(data)-2:], []byte{0, 0x1f}) {
		t.Fatal("row order or final pixel incorrect")
	}
}

func TestDrawErrorsAndRetry(t *testing.T) {
	failure := errors.New("transfer failed")
	img := image.NewRGBA(image.Rect(0, 0, Width, Height))
	for step := 1; step <= 6; step++ {
		f := &fakeTransport{}
		d := ready(t, f, Config{})
		f.failAt, f.err = step, failure
		if err := d.Draw(img); !errors.Is(err, failure) || len(f.ops) != step {
			t.Fatalf("step %d: err=%v, operations=%d", step, err, len(f.ops))
		}
		f.failAt, f.ops = 0, nil
		if err := d.Draw(img); err != nil || len(f.ops) != 6 || len(f.ops[5].data) != Width*Height*2 {
			t.Fatalf("retry: err=%v, operations=%d", err, len(f.ops))
		}
	}
}

func TestInvalidInputAndClose(t *testing.T) {
	f := &fakeTransport{}
	d := ready(t, f, Config{})
	for _, img := range []image.Image{nil, image.NewRGBA(image.Rect(0, 0, 320, 240))} {
		if err := d.Draw(img); err == nil || len(f.ops) != 0 {
			t.Fatal("invalid image reached transport")
		}
	}
	f.closeErr = errors.New("close failed")
	for i := 0; i < 2; i++ {
		if err := d.Close(); !errors.Is(err, f.closeErr) {
			t.Fatal(err)
		}
	}
	if f.closes != 1 {
		t.Fatal("transport closed more than once")
	}
	if err := d.Draw(image.NewRGBA(image.Rect(0, 0, Width, Height))); err == nil || len(f.ops) != 0 {
		t.Fatal("draw after close accepted")
	}
	for _, config := range []Config{{XOffset: -1}, {YOffset: 65536 - Height + 1}} {
		f := &fakeTransport{}
		if _, err := newDisplay(f, config, func(time.Duration) {}); err == nil || len(f.ops) != 0 || f.closes != 1 {
			t.Fatal("invalid offsets accepted or transport leaked")
		}
	}
}

func TestUIFrame(t *testing.T) {
	renderer, err := ui.NewDisplay()
	if err != nil {
		t.Fatal(err)
	}
	defer renderer.Close()
	f := &fakeTransport{}
	d := ready(t, f, Config{})
	if err := d.Draw(renderer.Render(app.AppState{})); err != nil {
		t.Fatal(err)
	}
	if len(f.ops[5].data) != Width*Height*2 {
		t.Fatal("incomplete UI frame")
	}
}
