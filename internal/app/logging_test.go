package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/OpeniPod/OpeniPod/internal/app"
	"github.com/OpeniPod/OpeniPod/internal/library"
	"github.com/OpeniPod/OpeniPod/internal/music"
	"github.com/OpeniPod/OpeniPod/internal/player"
	"github.com/OpeniPod/OpeniPod/internal/storage"
	"github.com/OpeniPod/OpeniPod/internal/ui"
)

func records(t *testing.T, out *bytes.Buffer) []map[string]any {
	t.Helper()
	decoder := json.NewDecoder(out)
	var result []map[string]any
	for {
		var record map[string]any
		err := decoder.Decode(&record)
		if errors.Is(err, io.EOF) {
			return result
		}
		if err != nil {
			t.Fatal(err)
		}
		result = append(result, record)
	}
}

func diagnosticLogger(out *bytes.Buffer) *slog.Logger {
	return slog.New(slog.NewJSONHandler(out, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

func TestLogsLifecycleAndPlayback(t *testing.T) {
	var out bytes.Buffer
	view := &observingUI{playing: make(chan struct{})}
	application := app.New(player.NewFake(), library.NewFake(), storage.NewMemory(50),
		scriptedInput{playing: view.playing}, view, app.WithLogger(diagnosticLogger(&out)))
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := application.Run(ctx); err != nil {
		t.Fatal(err)
	}
	logs := records(t, &out)
	for _, message := range []string{"application started", "application stopped", "library loaded", "play requested", "playback started"} {
		count := 0
		for _, record := range logs {
			if record["msg"] != message {
				continue
			}
			count++
			if record["component"] == nil || record["operation"] == nil {
				t.Fatalf("missing diagnostic fields: %v", record)
			}
			if message == "library loaded" && record["track_count"] != float64(4) {
				t.Fatalf("missing library size: %v", record)
			}
			if (message == "play requested" || message == "playback started") && record["track_id"] != "digital-love" {
				t.Fatalf("missing track ID: %v", record)
			}
			if message == "application stopped" && record["reason"] != "quit" {
				t.Fatalf("missing quit reason: %v", record)
			}
		}
		if count != 1 {
			t.Fatalf("%q occurred %d times: %v", message, count, logs)
		}
	}
}

type failedLibrary struct{ err error }

func (f failedLibrary) Load(context.Context) ([]music.Track, error) { return nil, f.err }

func (f failedLibrary) Refresh(context.Context) ([]music.Track, error) { return nil, f.err }

func TestLibraryRefreshDiagnostics(t *testing.T) {
	for _, tc := range []struct {
		name       string
		refreshErr error
		cancel     bool
		wantLoads  int
		wantWarns  int
	}{
		{name: "success", wantLoads: 2},
		{name: "recoverable failure", refreshErr: errors.New("scan failed"), wantLoads: 1, wantWarns: 1},
		{name: "cancellation", cancel: true, wantLoads: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			refreshes := 0
			lib := refreshTestLibrary{
				load: func(context.Context) ([]music.Track, error) {
					return []music.Track{{ID: "old"}}, nil
				},
				refresh: func(ctx context.Context) ([]music.Track, error) {
					refreshes++
					if tc.cancel {
						cancel()
						return nil, ctx.Err()
					}
					return []music.Track{{ID: "new"}}, tc.refreshErr
				},
			}
			var out bytes.Buffer
			view := &recordingUI{}
			application := app.New(player.NewFake(), lib, storage.NewMemory(50),
				eventInput{events: []app.Event{app.Scroll{Delta: 2}, app.SelectPressed{}, app.SelectPressed{}}},
				view, app.WithLogger(diagnosticLogger(&out)))
			if err := application.Run(ctx); err != nil {
				t.Fatal(err)
			}
			counts := make(map[string]int)
			for _, record := range records(t, &out) {
				message := record["msg"].(string)
				counts[message]++
				if record["level"] == "ERROR" {
					t.Fatalf("refresh failure or cancellation became fatal: %v", record)
				}
				if message == "library load failed" && (record["level"] != "WARN" || record["error"] != "update library: scan failed") {
					t.Fatalf("wrong recoverable refresh diagnostic: %v", record)
				}
				if message == "refreshing library" && (record["component"] != "library" || record["operation"] != "refresh") {
					t.Fatalf("missing refresh context: %v", record)
				}
				if message == "application stopped" && tc.cancel && record["reason"] != "cancelled" {
					t.Fatalf("cancellation lost at shutdown: %v", record)
				}
			}
			if refreshes != 1 || counts["refreshing library"] != 1 || counts["library loaded"] != tc.wantLoads || counts["library load failed"] != tc.wantWarns || counts["application stopped"] != 1 {
				t.Fatalf("refresh calls = %d, diagnostics = %v", refreshes, counts)
			}
			wantID := music.TrackID("new")
			if tc.refreshErr != nil || tc.cancel {
				wantID = "old"
			}
			last := view.states[len(view.states)-1]
			if len(last.Library.Tracks) != 1 || last.Library.Tracks[0].ID != wantID {
				t.Fatalf("refresh changed the wrong state: %+v", last.Library)
			}
		})
	}
}

func TestRecoverableErrorsAndNoRenderNoise(t *testing.T) {
	var out, screen bytes.Buffer
	track := music.Track{ID: "broken-track", Title: "Broken track"}
	events := []app.Event{app.PlaybackStarted{Track: track}}
	for i := 0; i < 100; i++ {
		events = append(events, app.PlaybackPositionChanged{Position: time.Duration(i) * time.Second}, app.Scroll{Delta: 1})
	}
	events = append(events, app.PlaybackError{Err: errors.New("decoder failed")}, app.QuitRequested{})
	application := app.New(player.NewFake(), failedLibrary{errors.New("library unavailable")}, storage.NewMemory(50),
		eventInput{events: events}, ui.NewTerminal(&screen), app.WithLogger(diagnosticLogger(&out)))
	if err := application.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	logs := records(t, &out)
	failures := 0
	for _, record := range logs {
		if record["error"] == nil {
			continue
		}
		failures++
		switch record["component"] {
		case "library":
			if record["level"] != "WARN" || record["operation"] != "load" {
				t.Fatalf("wrong library error: %v", record)
			}
		case "player":
			if record["level"] != "ERROR" || record["track_id"] != string(track.ID) || record["operation"] != "playback" {
				t.Fatalf("wrong playback error: %v", record)
			}
		default:
			t.Fatalf("unexpected failure: %v", record)
		}
	}
	if failures != 2 || len(logs) != 7 {
		t.Fatalf("errors duplicated or redraw/progress logged: %v", logs)
	}
	if bytes.Contains(screen.Bytes(), []byte("\"component\"")) || bytes.Contains(screen.Bytes(), []byte("application started")) {
		t.Fatal("diagnostics leaked into terminal UI")
	}
}

type failingPlayer struct {
	*player.Fake
	operation string
	err       error
}

func (p failingPlayer) Events() <-chan app.Event {
	if p.operation == "events" {
		events := make(chan app.Event)
		close(events)
		return events
	}
	return p.Fake.Events()
}

func (p failingPlayer) Play(ctx context.Context, track music.Track) error {
	if p.operation == "play" {
		return p.err
	}
	return p.Fake.Play(ctx, track)
}

func (p failingPlayer) Pause(ctx context.Context) error {
	if p.operation == "pause" {
		return p.err
	}
	return p.Fake.Pause(ctx)
}

func (p failingPlayer) Resume(ctx context.Context) error {
	if p.operation == "resume" {
		return p.err
	}
	return p.Fake.Resume(ctx)
}

func (p failingPlayer) SetVolume(ctx context.Context, volume int) error {
	if p.operation == "set_volume" {
		return p.err
	}
	return p.Fake.SetVolume(ctx, volume)
}

type failingStorage struct {
	app.Storage
	operation string
	err       error
}

func (s failingStorage) LoadSettings(ctx context.Context) (app.SettingsState, error) {
	if s.operation == "load_settings" {
		return app.SettingsState{}, s.err
	}
	return s.Storage.LoadSettings(ctx)
}

func (s failingStorage) SaveSettings(ctx context.Context, settings app.SettingsState) error {
	if s.operation == "save_settings" {
		return s.err
	}
	return s.Storage.SaveSettings(ctx, settings)
}

type failedUI struct{ err error }

func (u failedUI) Render(app.AppState) error { return u.err }

func TestFatalErrorsAreLoggedOnceWithContext(t *testing.T) {
	root := errors.New("backend unavailable")
	track := music.Track{ID: "test-track"}
	for _, c := range []struct {
		component, operation string
		events               []app.Event
		trackID              bool
	}{
		{"storage", "load_settings", nil, false},
		{"storage", "save_settings", []app.Event{app.VolumeChanged{Delta: 10}}, false},
		{"player", "set_volume", nil, false},
		{"player", "events", nil, false},
		{"player", "play", []app.Event{app.SelectPressed{}, app.SelectPressed{}}, true},
		{"player", "pause", []app.Event{app.PlaybackStarted{Track: track}, app.PlayPausePressed{}}, true},
		{"player", "resume", []app.Event{app.PlaybackStarted{Track: track}, app.PlaybackPaused{}, app.PlayPausePressed{}}, true},
		{"input", "read", nil, false},
		{"ui", "render", nil, false},
	} {
		t.Run(c.component+"/"+c.operation, func(t *testing.T) {
			var out bytes.Buffer
			p := failingPlayer{Fake: player.NewFake(), err: root}
			s := failingStorage{Storage: storage.NewMemory(50), err: root}
			in := eventInput{events: c.events}
			var view app.Renderer = silentUI{}
			switch c.component {
			case "player":
				p.operation = c.operation
			case "storage":
				s.operation = c.operation
			case "input":
				in.err = root
			case "ui":
				view = failedUI{root}
			}
			application := app.New(p, library.NewFake(), s, in, view, app.WithLogger(diagnosticLogger(&out)))
			if err := application.Run(context.Background()); err == nil || (c.operation != "events" && !errors.Is(err, root)) {
				t.Fatalf("root error was lost: %v", err)
			}
			count := 0
			for _, record := range records(t, &out) {
				if record["level"] == "ERROR" {
					count++
					if record["component"] != c.component || record["operation"] != c.operation || record["error"] == nil {
						t.Fatalf("wrong fatal error: %v", record)
					}
					if c.trackID && record["track_id"] == nil {
						t.Fatalf("track context lost: %v", record)
					}
				}
				if record["msg"] == "application stopped" && record["reason"] != "error" {
					t.Fatalf("wrong shutdown reason: %v", record)
				}
			}
			if count != 1 {
				t.Fatalf("expected one error record, got %d", count)
			}
		})
	}
}

func TestCancellationIsNotAnError(t *testing.T) {
	var out bytes.Buffer
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	application := app.New(player.NewFake(), library.NewFake(), storage.NewMemory(50),
		eventInput{}, silentUI{}, app.WithLogger(diagnosticLogger(&out)))
	if err := application.Run(ctx); err != nil {
		t.Fatal(err)
	}
	for _, record := range records(t, &out) {
		if record["level"] == "ERROR" || (record["msg"] == "application stopped" && record["reason"] != "cancelled") {
			t.Fatalf("cancellation logged as failure: %v", record)
		}
	}
}
