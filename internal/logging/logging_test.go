package logging

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestFileAppendsAndFiltersLevels(t *testing.T) {
	path := filepath.Join(t.TempDir(), "logs", "player.log")
	for _, level := range []string{"info", "debug"} {
		logger, closeLog, err := Open("file", path, level, io.Discard)
		if err != nil {
			t.Fatal(err)
		}
		logger.Info("started", "component", "app")
		logger.Debug("details")
		if err := closeLog(); err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected two INFO records and one DEBUG record: %s", data)
	}
	for i, line := range lines {
		if len(line) < 19 {
			t.Fatalf("short record: %q", line)
		}
		if _, err := time.Parse("2006-01-02 15:04:05", line[:19]); err != nil {
			t.Fatalf("invalid timestamp: %s", line)
		}
		want := " INFO [app/-] started"
		if i == 2 {
			want = " DEBUG [-/-] details"
		}
		if line[19:] != want {
			t.Fatalf("unexpected record: %s", line)
		}
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("log file permissions: %v", info.Mode())
	}
}

func TestDefaultPath(t *testing.T) {
	state := t.TempDir()
	t.Setenv("XDG_STATE_HOME", state)
	path, err := DefaultPath()
	if err != nil || path != filepath.Join(state, "openipod", "player.log") {
		t.Fatalf("default path: %q, %v", path, err)
	}
	t.Setenv("XDG_STATE_HOME", "")
	t.Setenv("HOME", state)
	path, err = DefaultPath()
	if err != nil || path != filepath.Join(state, ".local", "state", "openipod", "player.log") {
		t.Fatalf("home fallback: %q, %v", path, err)
	}
	t.Setenv("XDG_STATE_HOME", "relative")
	if _, err := DefaultPath(); err == nil {
		t.Fatal("relative XDG_STATE_HOME was accepted")
	}
}

func TestInvalidConfiguration(t *testing.T) {
	for _, c := range []struct{ target, path, level string }{
		{"file", "", "verbose"},
		{"console", "", "info"},
		{"journal", "player.log", "info"},
		{"file", t.TempDir(), "info"},
	} {
		if _, _, err := Open(c.target, c.path, c.level, io.Discard); err == nil {
			t.Fatalf("invalid configuration accepted: %+v", c)
		}
	}
}

func TestJournalPrioritiesAndConcurrentDerivedLoggers(t *testing.T) {
	var out bytes.Buffer
	logger, closeLog, err := Open("journal", "", "debug", &out)
	if err != nil {
		t.Fatal(err)
	}
	defer closeLog()
	levels := []slog.Level{slog.LevelDebug, slog.LevelInfo, slog.LevelWarn, slog.LevelError}
	var workers sync.WaitGroup
	for i := 0; i < 40; i++ {
		workers.Add(1)
		go func(i int) {
			defer workers.Done()
			logger.With("component", "player").WithGroup("track").With("id", i).
				Log(context.Background(), levels[i%4], "event", "operation", "play")
		}(i)
	}
	workers.Wait()
	priorities := map[string]string{"DEBUG": "<7>", "INFO": "<6>", "WARN": "<4>", "ERROR": "<3>"}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 40 {
		t.Fatalf("lost records: %d", len(lines))
	}
	pattern := regexp.MustCompile(`^(<\d>)\d{4}-\d\d-\d\d \d\d:\d\d:\d\d (DEBUG|INFO|WARN|ERROR) \[player/-\] event \(track.id: (\d+), track.operation: play\)$`)
	seen := make(map[string]bool)
	for _, line := range lines {
		fields := pattern.FindStringSubmatch(line)
		if fields == nil || fields[1] != priorities[fields[2]] || seen[fields[3]] {
			t.Fatalf("wrong priority, fields or duplicate record: %s", line)
		}
		seen[fields[3]] = true
	}
}

type shortWriter struct{}

func (shortWriter) Write(data []byte) (int, error) { return len(data) - 1, nil }

func TestJournalReportsShortWrites(t *testing.T) {
	logger, _, err := Open("journal", "", "info", shortWriter{})
	if err != nil {
		t.Fatal(err)
	}
	record := slog.Record{Level: slog.LevelInfo, Message: "event"}
	if err := logger.Handler().Handle(context.Background(), record); err != io.ErrShortWrite {
		t.Fatalf("short write error = %v", err)
	}
}

func TestCloseReportsWriteFailureFromDerivedLogger(t *testing.T) {
	logger, closeLog, err := Open("journal", "", "info", shortWriter{})
	if err != nil {
		t.Fatal(err)
	}
	logger.With("component", "player").WithGroup("track").Info("event", "id", 123)
	if err := closeLog(); !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("write failure was lost at shutdown: %v", err)
	}
}

func TestTextRecordFormat(t *testing.T) {
	stamp := time.Date(2026, 10, 19, 15, 30, 13, 0, time.UTC)
	for _, tc := range []struct {
		name, message, want string
		attrs               []slog.Attr
	}{
		{"playback", "Playback started", "Playback started (track: 123)", []slog.Attr{slog.Int("track", 123)}},
		{"error", "Playback failed", "Playback failed (error: decoder unavailable)", []slog.Attr{slog.Any("error", errors.New("decoder unavailable"))}},
		{"escaped", "bad\nmessage\x1b", `"bad\nmessage\x1b" (error: "disk\rfull", key: "a, b")`, []slog.Attr{slog.String("error", "disk\rfull"), slog.String("key", "a, b")}},
		{"groups", "event", "event (track.id: 123, inline: true)", []slog.Attr{slog.Group("track", slog.Int("id", 123)), slog.Group("", slog.Bool("inline", true)), slog.Group("empty"), {}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			logger, closeLog, err := Open("journal", "", "debug", &out)
			if err != nil {
				t.Fatal(err)
			}
			handler := logger.With("component", "player", "operation", "started").Handler()
			record := slog.NewRecord(stamp, slog.LevelInfo, tc.message, 0)
			record.AddAttrs(tc.attrs...)
			if err := handler.Handle(context.Background(), record); err != nil {
				t.Fatal(err)
			}
			if err := closeLog(); err != nil {
				t.Fatal(err)
			}
			want := "<6>2026-10-19 15:30:13 INFO [player/started] " + tc.want + "\n"
			if out.String() != want {
				t.Fatalf("record = %q, want %q", out.String(), want)
			}
		})
	}
}

type changingValue struct{ calls int }

func (v *changingValue) LogValue() slog.Value {
	v.calls++
	return slog.IntValue(v.calls)
}

func TestDerivedAttributesAreIndependentAndResolvedWhenBound(t *testing.T) {
	var out bytes.Buffer
	logger, closeLog, err := Open("journal", "", "debug", &out)
	if err != nil {
		t.Fatal(err)
	}
	value := &changingValue{}
	parent := logger.With("component", "player", "operation", "play", "value", value)
	child := parent.WithGroup("track").With("id", 123)
	child.Info("child")
	parent.Info("parent")
	logger.Info("root")
	if err := closeLog(); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if value.calls != 1 || len(lines) != 3 ||
		!strings.HasSuffix(lines[0], "[player/play] child (value: 1, track.id: 123)") ||
		!strings.HasSuffix(lines[1], "[player/play] parent (value: 1)") ||
		!strings.HasSuffix(lines[2], "[-/-] root") {
		t.Fatalf("attributes leaked or were re-evaluated: calls=%d, output=%s", value.calls, &out)
	}
}

func TestFinishPreservesWriteAndCloseErrors(t *testing.T) {
	writeErr := errors.New("write failed")
	closeErr := errors.New("close failed")
	out := &logOutput{writer: errorWriter{writeErr}, close: func() error { return closeErr }}
	if err := out.write([]byte("record\n")); !errors.Is(err, writeErr) {
		t.Fatalf("write error = %v", err)
	}
	// A later successful write must not erase an earlier failure.
	out.writer = io.Discard
	if err := out.write([]byte("record\n")); err != nil {
		t.Fatal(err)
	}
	if err := out.finish(); !errors.Is(err, writeErr) || !errors.Is(err, closeErr) {
		t.Fatalf("lost write or close failure: %v", err)
	}
}

type errorWriter struct{ err error }

func (w errorWriter) Write([]byte) (int, error) { return 0, w.err }
