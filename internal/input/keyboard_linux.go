package input

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"syscall"
	"time"
	"unsafe"

	"github.com/OpeniPod/OpeniPod/internal/app"
)

// Keyboard reads single keys from a Linux terminal, or bytes from a pipe for tests.
type Keyboard struct {
	file *os.File
}

func NewKeyboard(file *os.File) *Keyboard { return &Keyboard{file: file} }

func (k *Keyboard) Run(ctx context.Context, events chan<- app.Event) (err error) {
	fd := int(k.file.Fd())
	restore, err := rawMode(fd)
	if err != nil {
		return fmt.Errorf("configure terminal: %w", err)
	}
	defer func() {
		if restoreErr := restore(); restoreErr != nil && err == nil {
			err = fmt.Errorf("restore terminal: %w", restoreErr)
		}
	}()

	for {
		key, ready, readErr := readByte(ctx, fd, 100*time.Millisecond)
		if errors.Is(readErr, io.EOF) || ctx.Err() != nil {
			return nil
		}
		if readErr != nil {
			return fmt.Errorf("read keyboard: %w", readErr)
		}
		if !ready {
			continue
		}
		event, recognized, parseErr := keyEvent(ctx, fd, key)
		if parseErr != nil {
			return fmt.Errorf("read key sequence: %w", parseErr)
		}
		if recognized {
			select {
			case <-ctx.Done():
				return nil
			case events <- event:
			}
		}
	}
}

func keyEvent(ctx context.Context, fd int, key byte) (app.Event, bool, error) {
	switch key {
	case 'j':
		return app.Scroll{Delta: 1}, true, nil
	case 'k':
		return app.Scroll{Delta: -1}, true, nil
	case '\r', '\n':
		return app.SelectPressed{}, true, nil
	case 'b':
		return app.BackPressed{}, true, nil
	case 'p', ' ':
		return app.PlayPausePressed{}, true, nil
	case 'n':
		return app.NextPressed{}, true, nil
	case 'h':
		return app.PreviousPressed{}, true, nil
	case '+', '=':
		return app.VolumeChanged{Delta: 10}, true, nil
	case '-':
		return app.VolumeChanged{Delta: -10}, true, nil
	case 'q':
		return app.QuitRequested{}, true, nil
	case 0x1b:
		second, ready, err := readByte(ctx, fd, 30*time.Millisecond)
		if err != nil && !errors.Is(err, io.EOF) {
			return nil, false, err
		}
		if !ready || second != '[' {
			return app.BackPressed{}, true, nil
		}
		third, ready, err := readByte(ctx, fd, 30*time.Millisecond)
		if err != nil && !errors.Is(err, io.EOF) {
			return nil, false, err
		}
		if !ready {
			return app.BackPressed{}, true, nil
		}
		switch third {
		case 'A':
			return app.Scroll{Delta: -1}, true, nil
		case 'B':
			return app.Scroll{Delta: 1}, true, nil
		case 'C':
			return app.NextPressed{}, true, nil
		case 'D':
			return app.PreviousPressed{}, true, nil
		}
		return nil, false, nil
	}
	return nil, false, nil
}

func readByte(ctx context.Context, fd int, wait time.Duration) (byte, bool, error) {
	if err := ctx.Err(); err != nil {
		return 0, false, err
	}
	var fds syscall.FdSet
	wordBits := int(unsafe.Sizeof(fds.Bits[0]) * 8)
	if fd < 0 || fd/wordBits >= len(fds.Bits) {
		return 0, false, fmt.Errorf("file descriptor %d exceeds select limit", fd)
	}
	fds.Bits[fd/wordBits] |= 1 << (fd % wordBits)
	timeout := syscall.NsecToTimeval(wait.Nanoseconds())
	n, err := syscall.Select(fd+1, &fds, nil, nil, &timeout)
	if err == syscall.EINTR {
		return 0, false, nil
	}
	if err != nil || n == 0 {
		return 0, false, err
	}
	var buffer [1]byte
	n, err = syscall.Read(fd, buffer[:])
	if err == syscall.EINTR {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	if n == 0 {
		return 0, false, io.EOF
	}
	return buffer[0], true, nil
}

func rawMode(fd int) (func() error, error) {
	var original syscall.Termios
	if err := ioctl(fd, syscall.TCGETS, &original); err != nil {
		if errors.Is(err, syscall.ENOTTY) {
			return func() error { return nil }, nil
		}
		return nil, err
	}
	raw := original
	raw.Lflag &^= syscall.ICANON | syscall.ECHO
	raw.Iflag &^= syscall.ICRNL
	raw.Cc[syscall.VMIN] = 1
	raw.Cc[syscall.VTIME] = 0
	if err := ioctl(fd, syscall.TCSETS, &raw); err != nil {
		return nil, err
	}
	return func() error { return ioctl(fd, syscall.TCSETS, &original) }, nil
}

func ioctl(fd int, request uint, termios *syscall.Termios) error {
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), uintptr(request), uintptr(unsafe.Pointer(termios)))
	if errno != 0 {
		return errno
	}
	return nil
}
