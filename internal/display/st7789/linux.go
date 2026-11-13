//go:build linux

package st7789

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/warthog618/go-gpiocdev"
	"periph.io/x/conn/v3/physic"
	"periph.io/x/conn/v3/spi"
	"periph.io/x/host/v3/sysfs"
)

// Open opens the configured devices and initializes the panel.
func Open(config LinuxConfig) (_ *Display, err error) {
	if err := config.validate(); err != nil {
		return nil, fmt.Errorf("st7789: %w", err)
	}
	port, err := sysfs.NewSPI(config.Bus, config.ChipSelect)
	if err != nil {
		return nil, fmt.Errorf("st7789: open SPI: %w", err)
	}
	t := &linuxTransport{port: port, chunkSize: config.ChunkSize}
	defer func() {
		if err != nil {
			err = errors.Join(err, t.Close())
		}
	}()
	t.spi, err = port.Connect(physic.Frequency(config.SpeedHz)*physic.Hertz, spi.Mode(config.Mode), 8)
	if err != nil {
		return nil, fmt.Errorf("st7789: configure SPI: %w", err)
	}
	dc, err := gpiocdev.RequestLine(config.DC.Chip, config.DC.Offset, gpiocdev.AsOutput(1))
	if err != nil {
		return nil, fmt.Errorf("st7789: request D/C: %w", err)
	}
	t.dc = dc
	reset, err := gpiocdev.RequestLine(config.Reset.Chip, config.Reset.Offset, gpiocdev.AsOutput(1))
	if err != nil {
		return nil, fmt.Errorf("st7789: request RESET: %w", err)
	}
	t.reset = reset
	return New(t, config.Panel)
}

func (c LinuxConfig) validate() error {
	if c.Bus < 0 || c.Bus > 65535 || c.ChipSelect < 0 || c.ChipSelect > 255 {
		return errors.New("invalid SPI bus or chip select")
	}
	if c.SpeedHz < 100 || c.SpeedHz > 1_000_000_000 || c.Mode < 0 || c.Mode > 3 {
		return errors.New("invalid SPI speed or mode")
	}
	if c.ChunkSize < 2 || c.ChunkSize > 4096 || c.ChunkSize%2 != 0 {
		return errors.New("SPI chunk size must be even, between 2 and 4096")
	}
	for _, pin := range []Pin{c.DC, c.Reset} {
		if pin.Chip == "" || pin.Offset < 0 {
			return errors.New("GPIO chip and non-negative line offset are required")
		}
	}
	if strings.TrimPrefix(c.DC.Chip, "/dev/") == strings.TrimPrefix(c.Reset.Chip, "/dev/") && c.DC.Offset == c.Reset.Offset {
		return errors.New("D/C and RESET must use different GPIO lines")
	}
	return c.Panel.validate()
}

type gpioLine interface {
	SetValue(int) error
	Close() error
}

type linuxTransport struct {
	spi       interface{ Tx(w, r []byte) error }
	port      io.Closer
	dc, reset gpioLine
	chunkSize int
}

func (t *linuxTransport) Reset() error {
	for _, step := range []struct {
		level int
		delay time.Duration
	}{{1, 10 * time.Millisecond}, {0, 10 * time.Millisecond}, {1, 120 * time.Millisecond}} {
		if err := t.reset.SetValue(step.level); err != nil {
			return fmt.Errorf("reset GPIO: %w", err)
		}
		time.Sleep(step.delay)
	}
	return nil
}

func (t *linuxTransport) Command(command byte) error {
	if err := t.dc.SetValue(0); err != nil {
		return fmt.Errorf("D/C command: %w", err)
	}
	return t.spi.Tx([]byte{command}, nil)
}

func (t *linuxTransport) Data(data []byte) error {
	if err := t.dc.SetValue(1); err != nil {
		return fmt.Errorf("D/C data: %w", err)
	}
	for len(data) > 0 {
		n := min(t.chunkSize, len(data))
		if err := t.spi.Tx(data[:n], nil); err != nil {
			return fmt.Errorf("SPI data: %w", err)
		}
		data = data[n:]
	}
	return nil
}

func (t *linuxTransport) Close() error {
	resources := []io.Closer{t.reset, t.dc, t.port}
	t.reset, t.dc, t.port = nil, nil, nil
	var err error
	for _, resource := range resources {
		if resource != nil {
			err = errors.Join(err, resource.Close())
		}
	}
	return err
}
