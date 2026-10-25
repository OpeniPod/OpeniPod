// Package logging configures the application's diagnostic output.
package logging

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"unicode"
)

// Open uses a private append-only file by default. Journal mode requires stderr
// to be redirected to journald; it never redirects the terminal UI's stdout.
// The returned cleanup reports the first write failure as well as close errors:
// slog.Logger itself deliberately discards errors returned by Handler.Handle.
func Open(target, path, level string, stderr io.Writer) (*slog.Logger, func() error, error) {
	var minimum slog.Level
	if err := minimum.UnmarshalText([]byte(strings.ToUpper(level))); err != nil {
		return nil, nil, fmt.Errorf("invalid log level %q: %w", level, err)
	}
	output := &logOutput{}
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
		output.writer, output.close = file, file.Close
	case "journal":
		if path != "" {
			return nil, nil, fmt.Errorf("--log-file applies only to --log-target=file")
		}
		output.writer = stderr
	default:
		return nil, nil, fmt.Errorf("unsupported log target %q (available: file, journal)", target)
	}
	handler := &textHandler{output: output, minimum: minimum, journal: target == "journal"}
	return slog.New(handler), output.finish, nil
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

type textHandler struct {
	output  *logOutput
	minimum slog.Level
	journal bool
	attrs   []slog.Attr
	group   string
}

func (h *textHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.minimum
}

func (h *textHandler) Handle(_ context.Context, record slog.Record) error {
	attrs := append([]slog.Attr(nil), h.attrs...)
	record.Attrs(func(attr slog.Attr) bool {
		attrs = appendAttrs(attrs, h.group, attr)
		return true
	})
	component, operation := "-", "-"
	var details []string
	for _, attr := range attrs {
		switch attr.Key {
		case "component":
			component = attr.Value.String()
		case "operation":
			operation = attr.Value.String()
		default:
			details = append(details, readable(attr.Key)+": "+readable(attr.Value.String()))
		}
	}
	var line strings.Builder
	if h.journal {
		// systemd-cat consumes the prefix and sets journald's native PRIORITY.
		fmt.Fprintf(&line, "<%d>", journalPriority(record.Level))
	}
	fmt.Fprintf(&line, "%s %s [%s/%s] %s", record.Time.Format("2006-01-02 15:04:05"),
		record.Level, readable(component), readable(operation), readable(record.Message))
	if len(details) != 0 {
		fmt.Fprintf(&line, " (%s)", strings.Join(details, ", "))
	}
	line.WriteByte('\n')
	return h.output.write([]byte(line.String()))
}

func (h *textHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	clone := *h
	clone.attrs = append([]slog.Attr(nil), h.attrs...)
	for _, attr := range attrs {
		clone.attrs = appendAttrs(clone.attrs, h.group, attr)
	}
	return &clone
}

func (h *textHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	clone := *h
	clone.group += name + "."
	return &clone
}

// Resolve values when attributes are bound, retaining their group at that time.
func appendAttrs(dst []slog.Attr, group string, attr slog.Attr) []slog.Attr {
	attr.Value = attr.Value.Resolve()
	if attr.Equal(slog.Attr{}) {
		return dst
	}
	if attr.Value.Kind() == slog.KindGroup {
		if attr.Key != "" {
			group += attr.Key + "."
		}
		for _, child := range attr.Value.Group() {
			dst = appendAttrs(dst, group, child)
		}
		return dst
	}
	attr.Key = group + attr.Key
	return append(dst, attr)
}

// Keep each record on one line, even if an error contains newlines or terminal
// control sequences. Quote delimiters too so additional fields remain readable.
func readable(value string) string {
	if value == "" || strings.ContainsAny(value, ",()[]\"") || strings.ContainsFunc(value, unicode.IsControl) {
		return strconv.Quote(value)
	}
	return value
}

func journalPriority(level slog.Level) int {
	switch {
	case level >= slog.LevelError:
		return 3
	case level >= slog.LevelWarn:
		return 4
	case level >= slog.LevelInfo:
		return 6
	default:
		return 7
	}
}

// Derived handlers share output, its lock and its first write error. Retain only
// one write error so a repeatedly failing destination cannot grow memory usage.
type logOutput struct {
	mu       sync.Mutex
	writer   io.Writer
	close    func() error
	writeErr error
}

func (o *logOutput) write(line []byte) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	n, err := o.writer.Write(line)
	if err == nil && n != len(line) {
		err = io.ErrShortWrite
	}
	if err != nil && o.writeErr == nil {
		o.writeErr = fmt.Errorf("write log: %w", err)
	}
	return err
}

func (o *logOutput) finish() error {
	o.mu.Lock()
	defer o.mu.Unlock()
	var closeErr error
	if o.close != nil {
		if err := o.close(); err != nil {
			closeErr = fmt.Errorf("close log: %w", err)
		}
	}
	return errors.Join(o.writeErr, closeErr)
}
