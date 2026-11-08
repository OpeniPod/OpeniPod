# ST7789V display adapter

`internal/display/st7789` sends complete 240x320 frames to the 2-inch, 3.3 V
SPI ST7789V module selected in [issue #5](https://github.com/OpeniPod/OpeniPod/issues/5).
Hardware has not been validated; `cmd/displaytest` is a separate diagnostic that does not require the desktop window or an audio device.

## Image contract

`Draw(image.Image) error` accepts exactly 240x320 pixels, sent top to bottom
and left to right; alpha is composited over black. The `*image.RGBA` from
`internal/ui.Display.Render` (#20) is valid without changing `AppState`; nonzero
origins and subimages work, with big-endian RGB565 output (153,600 bytes/frame).

`Draw` is synchronous, retains only its RGB565 buffer, and requires the caller
to serialize `Draw` and `Close`. `New(transport, panelConfig)` owns its
transport even on initialization failure; `Open(linuxConfig)` owns Linux
devices. Both return transport errors; `Close` releases resources once.

## Driver choice

The adapter uses native Go libraries:

- [`periph.io/x/host/v3/sysfs`](https://github.com/periph/host) for Linux
  `/dev/spidevB.C`, SPI mode, frequency and synchronous transfers;
- [`go-gpiocdev`](https://github.com/warthog618/go-gpiocdev) for GPIO
  character-device D/C and RESET lines.

The small ST7789 command layer is local because the considered
[`periph.io-st7789`](https://github.com/nwhirschfeld/periph.io-st7789) driver
swallows errors from configuration and command writes. Reset, initialization,
RGB565, memory order, inversion and frame-window errors must reach the caller.

## Linux setup

Use Linux 5.10 or newer with SPI and GPIO character-device drivers. On an
Orange Pi Zero 3 vendor image, enable `spi1-cs0-spidev` with
`sudo orangepi-config` under **System -> Hardware**, then reboot; other images
may use another overlay name.

Check the actual device nodes and GPIO offsets before wiring or launching:

```sh
ls -l /dev/spidev* /dev/gpiochip*
# Debian/Ubuntu: sudo apt install gpiod
gpiodetect
gpioinfo
cat /sys/module/spidev/parameters/bufsiz
```

`/dev/spidev1.0` means `--bus=1 --cs=0`. GPIO offsets are not header pin
numbers, wiringOP numbers or legacy global GPIO numbers. Select free D/C and
active-low RESET lines from the board schematic and `gpioinfo`; do not guess.
Give the launching user node access through device groups or udev rules; no other process may claim the SPI device or GPIO lines, and no `/dev/mem` is used.

With power disconnected, connect common GND, the module's documented 3.3 V
supply, MOSI (header pin 19), CLK (23), CS0 (24), and the selected D/C and
RESET lines. MISO is unused. Wire the backlight separately according to the
module's documentation; this adapter does not control it.

## Build and launch

The dry run generates RGB bars with one white corner marker and traces the real
initialization and frame-transfer path without opening devices:

```sh
go run ./cmd/displaytest --dry-run
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o /tmp/displaytest ./cmd/displaytest
```

On the board, replace the placeholders with values confirmed above:

```sh
./displaytest --bus=1 --cs=0 --speed-hz=8000000 --mode=0 \
  --dc-chip=gpiochip0 --dc-line=<dc-offset> \
  --reset-chip=gpiochip0 --reset-line=<reset-offset>
```

`LinuxConfig` exposes SPI bus, chip select, speed, mode, even transfer
`chunk-size` (2..4096), D/C and RESET pins, and panel options. Use
`--bgr`, `--invert`, `--mirror-x`, `--mirror-y`, `--x-offset` and `--y-offset`
to match the panel. The fixed image contract remains portrait 240x320;
offsets must fit the 16-bit controller address window.

## Deferred hardware check

Software tests cover initialization, RGB565 conversion, frame transfer and
error propagation without device nodes. Hardware validation still needs the
received module: verify pin mapping, RESET, external backlight, RGB/BGR,
inversion, orientation, offsets and clipping. Missing nodes, permissions,
claims or unsupported SPI settings fail at startup; passing tests does not
confirm electrical wiring.
