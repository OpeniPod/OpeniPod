//go:build linux

package st7789

import (
	"bytes"
	"errors"
	"reflect"
	"testing"
)

type fakeLine struct {
	values        []int
	err, closeErr error
	closes        int
}

func (l *fakeLine) SetValue(v int) error { l.values = append(l.values, v); return l.err }
func (l *fakeLine) Close() error         { l.closes++; return l.closeErr }

type fakeSPI struct {
	writes [][]byte
	failAt int
	err    error
}

func (s *fakeSPI) Tx(w, _ []byte) error {
	s.writes = append(s.writes, bytes.Clone(w))
	if len(s.writes) == s.failAt {
		return s.err
	}
	return nil
}

func TestSPITransfer(t *testing.T) {
	bus, dc := &fakeSPI{}, &fakeLine{}
	transport := &linuxTransport{spi: bus, dc: dc, chunkSize: 4}
	if err := transport.Command(0x2c); err != nil {
		t.Fatal(err)
	}
	data := []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}
	if err := transport.Data(data); err != nil {
		t.Fatal(err)
	}
	want := [][]byte{{0x2c}, {0, 1, 2, 3}, {4, 5, 6, 7}, {8, 9}}
	if !reflect.DeepEqual(bus.writes, want) || !reflect.DeepEqual(dc.values, []int{0, 1}) {
		t.Fatalf("SPI=%x, D/C=%v", bus.writes, dc.values)
	}
	// Also exercise the actual full-frame payload at the normal kernel limit.
	bus.writes = nil
	transport.chunkSize = 4096
	frame := bytes.Repeat([]byte{0xf8, 0}, Width*Height)
	if err := transport.Data(frame); err != nil {
		t.Fatal(err)
	}
	if len(bus.writes) != 38 || len(bus.writes[37]) != 2048 || !bytes.Equal(bytes.Join(bus.writes, nil), frame) {
		t.Fatal("full frame was not split and transmitted intact")
	}
}

func TestSPIErrors(t *testing.T) {
	failure := errors.New("device failed")
	for _, command := range []bool{false, true} {
		bus, dc := &fakeSPI{}, &fakeLine{err: failure}
		transport := &linuxTransport{spi: bus, dc: dc, chunkSize: 4}
		send := func() error { return transport.Data(make([]byte, 10)) }
		if command {
			send = func() error { return transport.Command(0x2c) }
		}
		if err := send(); !errors.Is(err, failure) || len(bus.writes) != 0 {
			t.Fatal("D/C error not propagated before SPI")
		}
		dc.err = nil
		bus.err, bus.failAt = failure, 2
		if command {
			bus.failAt = 1
		}
		if err := send(); !errors.Is(err, failure) || len(bus.writes) != bus.failAt {
			t.Fatal("SPI error did not stop transmission")
		}
	}
}

func TestResetAndCleanup(t *testing.T) {
	failure := errors.New("close failed")
	reset, dc, port := &fakeLine{closeErr: failure}, &fakeLine{closeErr: failure}, &fakeLine{closeErr: failure}
	transport := &linuxTransport{reset: reset, dc: dc, port: port}
	if err := transport.Reset(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(reset.values, []int{1, 0, 1}) {
		t.Fatalf("reset=%v", reset.values)
	}
	if err := transport.Close(); !errors.Is(err, failure) {
		t.Fatal("close error lost")
	}
	if err := transport.Close(); err != nil {
		t.Fatal(err)
	}
	for _, line := range []*fakeLine{reset, dc, port} {
		if line.closes != 1 {
			t.Fatal("resource not closed exactly once")
		}
	}
	reset.err, reset.values = failure, nil
	transport.reset = reset
	if err := transport.Reset(); !errors.Is(err, failure) || len(reset.values) != 1 {
		t.Fatal("reset error lost")
	}
}

func TestLinuxConfig(t *testing.T) {
	valid := LinuxConfig{Bus: 1, ChipSelect: 0, SpeedHz: 8000000, ChunkSize: 4096, DC: Pin{"gpiochip0", 1}, Reset: Pin{"gpiochip0", 2}}
	if err := valid.validate(); err != nil {
		t.Fatal(err)
	}
	for i, change := range []func(*LinuxConfig){
		func(c *LinuxConfig) { c.Bus = -1 },
		func(c *LinuxConfig) { c.Mode = 4 },
		func(c *LinuxConfig) { c.SpeedHz = 0 },
		func(c *LinuxConfig) { c.ChunkSize = 0 },
		func(c *LinuxConfig) { c.ChunkSize = 3 },
		func(c *LinuxConfig) { c.ChunkSize = 4098 },
		func(c *LinuxConfig) { c.DC.Chip = "" },
		func(c *LinuxConfig) { c.Reset.Offset = -1 },
		func(c *LinuxConfig) { c.Reset = Pin{"/dev/gpiochip0", 1} },
	} {
		config := valid
		change(&config)
		if err := config.validate(); err == nil {
			t.Fatalf("invalid config %d accepted", i)
		}
	}
}
