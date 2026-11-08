//go:build !linux

package st7789

import "errors"

// Open is unavailable on non-Linux targets.
func Open(LinuxConfig) (*Display, error) {
	return nil, errors.New("st7789: Linux SPI display is unavailable on this platform")
}
