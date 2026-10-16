package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
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
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatal(err)
		}
		if i < 2 && (record["level"] != "INFO" || record["component"] != "app") {
			t.Fatalf("unexpected record: %v", record)
		}
		if i == 2 && record["level"] != "DEBUG" {
			t.Fatalf("expected DEBUG: %v", record)
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
	for _, line := range lines {
		if len(line) < 3 {
			t.Fatalf("short record: %q", line)
		}
		var record map[string]any
		if err := json.Unmarshal([]byte(line[3:]), &record); err != nil {
			t.Fatal(err)
		}
		if line[:3] != priorities[fmt.Sprint(record["level"])] || record["component"] != "player" {
			t.Fatalf("wrong journal priority or fields: %s", line)
		}
		track, ok := record["track"].(map[string]any)
		if !ok || track["operation"] != "play" || track["id"] == nil {
			t.Fatalf("group attributes lost: %s", line)
		}
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
