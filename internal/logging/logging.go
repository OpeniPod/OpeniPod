// Package logging configures the application's diagnostic output.
package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Open uses a private append-only file by default. Journal mode requires stderr
// to be redirected to journald; the graphical UI uses a separate window.
func Open(target, path, level string, stderr io.Writer) (*slog.Logger, func() error, error) {
	var minimum slog.Level
	if err := minimum.UnmarshalText([]byte(strings.ToUpper(level))); err != nil {
		return nil, nil, fmt.Errorf("invalid log level %q: %w", level, err)
	}
	options := &slog.HandlerOptions{Level: minimum}
	switch target {
	case "file":
		if path == "" {
			var err error
			path, err = DefaultPath()
			if err != nil {
				return nil, nil, err
			}
		}
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return nil, nil, fmt.Errorf("create log directory: %w", err)
		}
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0600)
		if err != nil {
			return nil, nil, fmt.Errorf("open log file: %w", err)
		}
		return slog.New(slog.NewJSONHandler(file, options)), file.Close, nil
	case "journal":
		if path != "" {
			return nil, nil, fmt.Errorf("--log-file applies only to --log-target=file")
		}
		out := &priorityWriter{out: stderr}
		handler := &journalHandler{base: slog.NewJSONHandler(out, options), out: out}
		return slog.New(handler), func() error { return nil }, nil
	default:
		return nil, nil, fmt.Errorf("unsupported log target %q (available: file, journal)", target)
	}
}

func DefaultPath() (string, error) {
	state := os.Getenv("XDG_STATE_HOME")
	if state != "" && !filepath.IsAbs(state) {
		return "", fmt.Errorf("XDG_STATE_HOME must be an absolute path")
	}
	if state == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("find home directory for logs: %w", err)
		}
		state = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(state, "openipod", "player.log"), nil
}

// journald's stream protocol understands <priority> prefixes, not JSON's level
// field. All derived handlers share this lock so prefix and record stay atomic.
type journalHandler struct {
	base slog.Handler
	out  *priorityWriter
}

func (h *journalHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.base.Enabled(ctx, level)
}

func (h *journalHandler) Handle(ctx context.Context, record slog.Record) error {
	h.out.mu.Lock()
	defer h.out.mu.Unlock()
	switch {
	case record.Level >= slog.LevelError:
		h.out.priority = 3
	case record.Level >= slog.LevelWarn:
		h.out.priority = 4
	case record.Level >= slog.LevelInfo:
		h.out.priority = 6
	default:
		h.out.priority = 7
	}
	return h.base.Handle(ctx, record)
}

func (h *journalHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &journalHandler{base: h.base.WithAttrs(attrs), out: h.out}
}

func (h *journalHandler) WithGroup(name string) slog.Handler {
	return &journalHandler{base: h.base.WithGroup(name), out: h.out}
}

type priorityWriter struct {
	mu       sync.Mutex
	out      io.Writer
	priority int
}

func (w *priorityWriter) Write(record []byte) (int, error) {
	line := append(fmt.Appendf(nil, "<%d>", w.priority), record...)
	n, err := w.out.Write(line)
	if err == nil && n != len(line) {
		err = io.ErrShortWrite
	}
	if err != nil {
		return 0, err
	}
	return len(record), nil
}
