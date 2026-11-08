// displaytest sends a test pattern through the ST7789V adapter.
package main

import (
	"errors"
	"flag"
	"fmt"
	"image"
	"image/color"
	"io"
	"os"

	"github.com/OpeniPod/OpeniPod/internal/display/st7789"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) (err error) {
	var config st7789.LinuxConfig
	var dryRun bool
	flags := flag.NewFlagSet("displaytest", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.BoolVar(&dryRun, "dry-run", false, "trace controller operations without accessing devices")
	flags.IntVar(&config.Bus, "bus", -1, "Linux SPI bus number (required on hardware)")
	flags.IntVar(&config.ChipSelect, "cs", -1, "SPI chip select number (required on hardware)")
	flags.Int64Var(&config.SpeedHz, "speed-hz", 8000000, "SPI clock in Hz")
	flags.IntVar(&config.Mode, "mode", 0, "SPI mode (0..3)")
	flags.IntVar(&config.ChunkSize, "chunk-size", 4096, "maximum SPI data transfer bytes (even, 2..4096)")
	flags.StringVar(&config.DC.Chip, "dc-chip", "", "D/C GPIO chip, e.g. gpiochip0")
	flags.IntVar(&config.DC.Offset, "dc-line", -1, "D/C line offset within chip")
	flags.StringVar(&config.Reset.Chip, "reset-chip", "", "RESET GPIO chip")
	flags.IntVar(&config.Reset.Offset, "reset-line", -1, "RESET line offset within chip")
	flags.BoolVar(&config.Panel.MirrorX, "mirror-x", false, "mirror portrait columns (MADCTL MX)")
	flags.BoolVar(&config.Panel.MirrorY, "mirror-y", false, "mirror portrait rows (MADCTL MY)")
	flags.BoolVar(&config.Panel.BGR, "bgr", false, "select BGR panel color order")
	flags.BoolVar(&config.Panel.Invert, "invert", false, "enable panel inversion")
	flags.IntVar(&config.Panel.XOffset, "x-offset", 0, "controller column offset")
	flags.IntVar(&config.Panel.YOffset, "y-offset", 0, "controller row offset")
	if err := flags.Parse(args); errors.Is(err, flag.ErrHelp) {
		return nil
	} else if err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected arguments: %v", flags.Args())
	}
	frame := testPattern()
	var display *st7789.Display
	if dryRun {
		display, err = st7789.New(&traceTransport{out: stdout}, config.Panel)
	} else {
		display, err = st7789.Open(config)
	}
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, display.Close()) }()
	return display.Draw(frame)
}

func testPattern() *image.RGBA {
	frame := image.NewRGBA(image.Rect(0, 0, st7789.Width, st7789.Height))
	colors := [...]color.RGBA{{R: 255, A: 255}, {G: 255, A: 255}, {B: 255, A: 255}}
	for y := 0; y < st7789.Height; y++ {
		for x := 0; x < st7789.Width; x++ {
			c := colors[x/(st7789.Width/len(colors))]
			if x < 16 && y < 16 {
				c = color.RGBA{R: 255, G: 255, B: 255, A: 255}
			}
			frame.SetRGBA(x, y, c)
		}
	}
	return frame
}

type traceTransport struct{ out io.Writer }

func (t *traceTransport) Reset() error {
	_, err := fmt.Fprintln(t.out, "RESET")
	return err
}

func (t *traceTransport) Command(command byte) error {
	_, err := fmt.Fprintf(t.out, "CMD %02x\n", command)
	return err
}

func (t *traceTransport) Data(data []byte) error {
	if len(data) <= 4 {
		_, err := fmt.Fprintf(t.out, "DATA %x\n", data)
		return err
	}
	_, err := fmt.Fprintf(t.out, "DATA %d bytes (RGB565 frame)\n", len(data))
	return err
}

func (t *traceTransport) Close() error {
	_, err := fmt.Fprintln(t.out, "CLOSE")
	return err
}
