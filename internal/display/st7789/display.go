// Package st7789 transfers full 240×320 frames to an ST7789V over four-wire SPI.
// It has no dependency on application state or the UI renderer.
package st7789

import (
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"time"
)

const (
	Width  = 240
	Height = 320
)

// Config describes the panel, independently of its Linux connection. Mirrors
// preserve portrait dimensions; a 90° axis swap is deliberately not exposed.
// The zero value selects RGB, no inversion, no mirroring and zero offsets.
type Config struct {
	MirrorX, MirrorY bool
	BGR              bool
	Invert           bool
	XOffset, YOffset int
}

func (c Config) validate() error {
	if c.XOffset < 0 || c.XOffset > 65536-Width || c.YOffset < 0 || c.YOffset > 65536-Height {
		return fmt.Errorf("panel offsets exceed the 16-bit address window")
	}
	return nil
}

// Transport supplies synchronous device operations. Command uses D/C low;
// Data uses D/C high and must send every byte in order, or return an error.
// Reset pulses the active-low hardware reset and waits for it to settle.
// Data may split a payload to respect the SPI driver's transfer limit.
type Transport interface {
	Reset() error
	Command(byte) error
	Data([]byte) error
	Close() error
}

// Display owns the transport and a frame buffer. The caller must serialize
// Draw and Close, and must not modify an image while Draw is running.
type Display struct {
	transport Transport
	config    Config
	pixels    []byte
	closed    bool
	closeErr  error
}

// New initializes the panel. It takes ownership of transport, also closing it
// if validation or initialization fails. Use Open for the Linux SPI transport.
func New(transport Transport, config Config) (*Display, error) {
	return newDisplay(transport, config, time.Sleep)
}

func newDisplay(transport Transport, config Config, sleep func(time.Duration)) (_ *Display, err error) {
	if transport == nil {
		return nil, errors.New("st7789: nil transport")
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, transport.Close())
		}
	}()
	if err := config.validate(); err != nil {
		return nil, fmt.Errorf("st7789: %w", err)
	}
	d := &Display{transport: transport, config: config, pixels: make([]byte, Width*Height*2)}
	if err := transport.Reset(); err != nil {
		return nil, fmt.Errorf("st7789: hardware reset: %w", err)
	}
	var madctl byte
	if config.MirrorY {
		madctl |= 0x80
	}
	if config.MirrorX {
		madctl |= 0x40
	}
	if config.BGR {
		madctl |= 0x08
	}
	inversion := byte(0x20) // INVOFF
	if config.Invert {
		inversion = 0x21 // INVON
	}
	// ST7789V reset, sleep-out, RGB565, memory order, inversion, normal mode,
	// display-on. Delays allow reset/sleep/power state transitions to settle.
	for _, step := range []struct {
		command byte
		data    []byte
		delay   time.Duration
	}{
		{0x01, nil, 150 * time.Millisecond},
		{0x11, nil, 120 * time.Millisecond},
		{0x3a, []byte{0x55}, 10 * time.Millisecond},
		{0x36, []byte{madctl}, 0},
		{inversion, nil, 0},
		{0x13, nil, 10 * time.Millisecond},
		{0x29, nil, 120 * time.Millisecond},
	} {
		if err := d.command(step.command, step.data); err != nil {
			return nil, fmt.Errorf("st7789: initialize: %w", err)
		}
		if step.delay > 0 {
			sleep(step.delay)
		}
	}
	return d, nil
}

// Draw sends exactly one 240×320 image, row-major, as big-endian RGB565.
// Nonzero image origins and subimages are supported. Alpha is composited over
// black using color.Color's premultiplied RGBA values. The caller must not
// modify the image until Draw returns. A failed transfer can leave a partial
// frame; the next Draw always restores the complete address window.
func (d *Display) Draw(img image.Image) error {
	if d.closed {
		return errors.New("st7789: display is closed")
	}
	if img == nil {
		return errors.New("st7789: nil image")
	}
	bounds := img.Bounds()
	if bounds.Dx() != Width || bounds.Dy() != Height {
		return fmt.Errorf("st7789: image size %dx%d; want %dx%d", bounds.Dx(), bounds.Dy(), Width, Height)
	}
	i := 0
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			pixel := uint16((r>>11)<<11 | (g>>10)<<5 | b>>11)
			binary.BigEndian.PutUint16(d.pixels[i:i+2], pixel)
			i += 2
		}
	}
	var columns, rows [4]byte
	binary.BigEndian.PutUint16(columns[:2], uint16(d.config.XOffset))
	binary.BigEndian.PutUint16(columns[2:], uint16(d.config.XOffset+Width-1))
	binary.BigEndian.PutUint16(rows[:2], uint16(d.config.YOffset))
	binary.BigEndian.PutUint16(rows[2:], uint16(d.config.YOffset+Height-1))
	if err := d.command(0x2a, columns[:]); err != nil {
		return fmt.Errorf("st7789: set columns: %w", err)
	}
	if err := d.command(0x2b, rows[:]); err != nil {
		return fmt.Errorf("st7789: set rows: %w", err)
	}
	if err := d.command(0x2c, d.pixels); err != nil {
		return fmt.Errorf("st7789: write frame: %w", err)
	}
	return nil
}

func (d *Display) command(command byte, data []byte) error {
	if err := d.transport.Command(command); err != nil {
		return fmt.Errorf("command 0x%02x: %w", command, err)
	}
	if len(data) > 0 {
		if err := d.transport.Data(data); err != nil {
			return fmt.Errorf("command 0x%02x data: %w", command, err)
		}
	}
	return nil
}

// Close releases the transport once and returns any shutdown error.
func (d *Display) Close() error {
	if !d.closed {
		d.closed = true
		d.closeErr = d.transport.Close()
	}
	return d.closeErr
}
