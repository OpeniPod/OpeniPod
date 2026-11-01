package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/OpeniPod/OpeniPod/internal/app"
	"github.com/OpeniPod/OpeniPod/internal/player"
)

func TestParseOptionsDefaultsAndFlags(t *testing.T) {
	home := t.TempDir()
	config := filepath.Join(t.TempDir(), "config")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", config)

	got, err := parseOptions(nil, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if got.musicDir != filepath.Join(home, "Music") {
		t.Errorf("default musicDir = %q, want %q", got.musicDir, filepath.Join(home, "Music"))
	}
	if got.settingsFile != filepath.Join(config, "openipod", "settings.json") {
		t.Errorf("default settingsFile = %q, want %q", got.settingsFile, filepath.Join(config, "openipod", "settings.json"))
	}
	if got.platform != "desktop" || got.logTarget != "file" || got.logLevel != "info" || got.scale != 2 {
		t.Errorf("default platform/logging/scale = %+v", got)
	}
	if _, err := parseOptions([]string{"--help"}, io.Discard); !errors.Is(err, flag.ErrHelp) {
		t.Errorf("parseOptions(--help) error = %v, want flag.ErrHelp", err)
	}

	got, err = parseOptions([]string{
		"--music-dir", "/tmp/music",
		"--settings-file", "/tmp/preferences.json",
		"--log-target", "journal",
		"--log-file", "/tmp/openipod.log",
		"--log-level", "debug",
		"--platform", "desktop",
		"--scale", "4",
	}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if got.musicDir != "/tmp/music" || got.settingsFile != "/tmp/preferences.json" ||
		got.logTarget != "journal" || got.logFile != "/tmp/openipod.log" ||
		got.logLevel != "debug" || got.platform != "desktop" || got.scale != 4 {
		t.Errorf("explicit options = %+v", got)
	}
}

func TestParseOptionsScaleBoundariesAndRange(t *testing.T) {
	for _, scale := range []string{"1", "4"} {
		t.Run("boundary "+scale, func(t *testing.T) {
			got, err := parseOptions([]string{"--scale", scale}, io.Discard)
			if err != nil {
				t.Fatal(err)
			}
			want := 1
			if scale == "4" {
				want = 4
			}
			if got.scale != want {
				t.Errorf("scale = %d, want %d", got.scale, want)
			}
		})
	}
	for _, scale := range []string{"0", "5"} {
		t.Run("out of range "+scale, func(t *testing.T) {
			_, err := parseOptions([]string{"--scale", scale}, io.Discard)
			if err == nil || !strings.Contains(err.Error(), "scale must be between 1 and 4") {
				t.Errorf("parseOptions() error = %v, want scale range error", err)
			}
		})
	}
}

func TestRunDoesNotLaunchForHelpOrInvalidOptions(t *testing.T) {
	for _, args := range [][]string{
		{"--help"},
		{"--log-level", "verbose"},
		{"--log-target", "console"},
		{"--log-target", "journal", "--log-file", "/tmp/openipod.log"},
		{"--platform", "unknown"},
		{"--scale", "5"},
		{"unexpected"},
	} {
		var stderr bytes.Buffer
		launchCalls := 0
		err := run(args, &stderr, func(context.Context, options, *slog.Logger) error {
			launchCalls++
			return nil
		})
		if launchCalls != 0 {
			t.Errorf("run(%v) launched %d times", args, launchCalls)
		}
		if args[0] == "--help" {
			if err != nil {
				t.Errorf("run(--help) error = %v, want nil", err)
			}
		} else if err == nil {
			t.Errorf("run(%v) error = nil, want validation error", args)
		}
	}
}

func TestRunDiscoversAndSortsMusicDirectoryTracks(t *testing.T) {
	musicDir := t.TempDir()
	writePlayerTestFile(t, musicDir, "02 Second Song.mp3")
	writePlayerTestFile(t, musicDir, "01 First Song.mp3")

	run := runPlayer(t, []string{"--music-dir", musicDir}, eventInput{})
	if run.err != nil {
		t.Fatal(run.err)
	}
	tracks := lastState(t, run.states).Library.Tracks
	if len(tracks) != 2 || tracks[0].Title != "01 First Song.mp3" || tracks[1].Title != "02 Second Song.mp3" {
		t.Fatalf("discovered tracks = %+v, want files in sorted order", tracks)
	}
	if run.opts.musicDir != musicDir {
		t.Errorf("run musicDir = %q, want %q", run.opts.musicDir, musicDir)
	}
}

func TestRunHandlesEmptyAndMissingMusicDirectories(t *testing.T) {
	emptyDir := t.TempDir()
	empty := runPlayer(t, []string{"--music-dir", emptyDir}, eventInput{})
	if empty.err != nil {
		t.Fatal(empty.err)
	}
	if state := lastState(t, empty.states); len(state.Library.Tracks) != 0 || state.Error != "" {
		t.Fatalf("empty library state = %+v, want no tracks and no error", state)
	}

	missingDir := filepath.Join(t.TempDir(), "missing")
	missing := runPlayer(t, []string{"--music-dir", missingDir}, eventInput{})
	if missing.err != nil {
		t.Fatal(missing.err)
	}
	if state := lastState(t, missing.states); len(state.Library.Tracks) != 0 || !strings.Contains(state.Error, missingDir) {
		t.Fatalf("missing library state = %+v, want empty tracks and path error", state)
	}
}

func TestRunUsesSavedLibraryUntilMenuRefresh(t *testing.T) {
	musicDir := t.TempDir()
	oldPath := filepath.Join(musicDir, "old.mp3")
	writePlayerTestFile(t, musicDir, filepath.Base(oldPath))
	args := []string{"--music-dir", musicDir}

	first := runPlayer(t, args, eventInput{})
	if first.err != nil {
		t.Fatal(first.err)
	}
	if got := trackTitles(lastState(t, first.states)); !slices.Equal(got, []string{"old.mp3"}) {
		t.Fatalf("first library = %v, want [old.mp3]", got)
	}
	if err := os.Remove(oldPath); err != nil {
		t.Fatal(err)
	}
	writePlayerTestFile(t, musicDir, "new.mp3")

	second := runPlayer(t, args, eventInput{})
	if second.err != nil {
		t.Fatal(second.err)
	}
	if got := trackTitles(lastState(t, second.states)); !slices.Equal(got, []string{"old.mp3"}) {
		t.Fatalf("cached library = %v, want saved [old.mp3]", got)
	}

	refresh := runPlayer(t, args, eventInput{events: []app.Event{
		app.Scroll{Delta: 2},
		app.SelectPressed{},
		app.SelectPressed{},
		app.BackPressed{},
		app.SelectPressed{},
	}})
	if refresh.err != nil {
		t.Fatal(refresh.err)
	}
	if state := lastState(t, refresh.states); !slices.Equal(trackTitles(state), []string{"new.mp3"}) || state.Error != "" {
		t.Fatalf("refreshed library state = %+v, want new.mp3 without an error", state)
	}
	third := runPlayer(t, args, eventInput{})
	if third.err != nil {
		t.Fatal(third.err)
	}
	if got := trackTitles(lastState(t, third.states)); !slices.Equal(got, []string{"new.mp3"}) {
		t.Fatalf("persisted refreshed library = %v, want [new.mp3]", got)
	}
}

func TestRunMenuRefreshRecoversInvalidLibraryIndex(t *testing.T) {
	musicDir := t.TempDir()
	writePlayerTestFile(t, musicDir, "track.mp3")
	if err := os.WriteFile(filepath.Join(musicDir, ".openipod-library.json"), []byte("invalid JSON"), 0o600); err != nil {
		t.Fatal(err)
	}

	got := runPlayer(t, []string{"--music-dir", musicDir, "--log-level", "debug"}, eventInput{events: []app.Event{
		app.Scroll{Delta: 2},
		app.SelectPressed{},
		app.SelectPressed{},
		app.BackPressed{},
		app.SelectPressed{},
	}})
	if got.err != nil {
		t.Fatal(got.err)
	}
	if state := lastState(t, got.states); !slices.Equal(trackTitles(state), []string{"track.mp3"}) || state.Error != "" {
		t.Fatalf("recovered library state = %+v, want track.mp3 without an error", state)
	}
	counts := make(map[string]int)
	for _, record := range readJSONLog(t, got.logFile) {
		counts[record["msg"].(string)]++
		if record["level"] == "ERROR" {
			t.Errorf("recoverable index failure became fatal: %v", record)
		}
	}
	if counts["library load failed"] != 1 || counts["library loaded"] != 1 {
		t.Errorf("index recovery diagnostics = %v, want one warning and one successful load", counts)
	}
}

func TestRunPersistsEqualizerAndRestoresVolume(t *testing.T) {
	settingsFile := filepath.Join(t.TempDir(), "settings.json")
	args := []string{"--settings-file", settingsFile, "--music-dir", t.TempDir()}
	for i, preset := range []app.EqualizerPreset{app.EqualizerBassBoost, app.EqualizerVocal, app.EqualizerOff} {
		events := []app.Event{
			app.Scroll{Delta: 2},
			app.SelectPressed{},
			app.Scroll{Delta: 1},
			app.SelectPressed{},
		}
		if i == 0 {
			events = append(events, app.VolumeChanged{Delta: 20})
		}
		events = append(events, app.QuitRequested{})
		got := runPlayer(t, args, eventInput{events: events})
		if got.err != nil {
			t.Fatal(got.err)
		}
		if got.audio.Equalizer != preset || got.audio.Volume != 70 {
			t.Fatalf("player settings after selecting %q = volume %d, equalizer %q", preset, got.audio.Volume, got.audio.Equalizer)
		}

		settings, err := os.ReadFile(settingsFile)
		if err != nil {
			t.Fatal(err)
		}
		var stored map[string]any
		if err := json.Unmarshal(settings, &stored); err != nil {
			t.Fatal(err)
		}
		storedPreset := stored["equalizer"]
		if preset == app.EqualizerOff && storedPreset == nil {
			storedPreset = string(app.EqualizerOff)
		}
		if storedPreset != string(preset) || stored["volume"] != float64(70) {
			t.Fatalf("stored settings = %v, want equalizer %q at volume 70", stored, preset)
		}
	}

	restored := runPlayer(t, args, eventInput{})
	if restored.err != nil {
		t.Fatal(restored.err)
	}
	if restored.audio.Equalizer != app.EqualizerOff || restored.audio.Volume != 70 {
		t.Fatalf("restored player settings = volume %d, equalizer %q", restored.audio.Volume, restored.audio.Equalizer)
	}
}

func TestRunFileLoggingRecordsFilesystemPlayback(t *testing.T) {
	musicDir := t.TempDir()
	writePlayerTestFile(t, musicDir, "track.mp3")
	got := runPlayer(t, []string{"--music-dir", musicDir, "--log-level", "debug"}, eventInput{events: []app.Event{
		app.SelectPressed{},
		app.SelectPressed{},
	}})
	if got.err != nil {
		t.Fatal(got.err)
	}
	if got.stderr.Len() != 0 {
		t.Fatalf("file logging wrote diagnostics to stderr: %s", &got.stderr)
	}
	if state := lastState(t, got.states); state.Screen != app.ScreenNowPlaying || state.Player.Status != app.StatusPlaying {
		t.Fatalf("playback state = %+v, want active track", state)
	}

	counts := make(map[string]int)
	for _, record := range readJSONLog(t, got.logFile) {
		message, _ := record["msg"].(string)
		counts[message]++
		if message == "library loaded" && record["track_count"] != float64(1) {
			t.Errorf("library loaded record = %v, want one track", record)
		}
		if message == "playback started" {
			if trackID, ok := record["track_id"].(string); !ok || trackID == "" {
				t.Errorf("playback started record has no track ID: %v", record)
			}
		}
		if record["level"] == "ERROR" {
			t.Errorf("successful playback logged a fatal error: %v", record)
		}
	}
	if counts["application started"] != 1 || counts["library loaded"] != 1 || counts["playback started"] != 1 {
		t.Fatalf("filesystem diagnostics = %v", counts)
	}
}

func TestRunJournalLogsInputFailureOnce(t *testing.T) {
	inputErr := errors.New("input adapter failed")
	got := runPlayer(t, []string{"--music-dir", t.TempDir(), "--log-target", "journal", "--log-file", ""}, eventInput{err: inputErr})
	var recorded *loggedError
	if !errors.As(got.err, &recorded) || !errors.Is(got.err, inputErr) {
		t.Fatalf("runtime failure error = %v, want loggedError", got.err)
	}

	fatalCount := 0
	for _, line := range strings.Split(strings.TrimSpace(got.stderr.String()), "\n") {
		if len(line) < 4 {
			t.Fatalf("invalid journal record: %q", line)
		}
		var record map[string]any
		if err := json.Unmarshal([]byte(line[3:]), &record); err != nil {
			t.Fatal(err)
		}
		if record["level"] == "ERROR" {
			fatalCount++
			if line[:3] != "<3>" || record["component"] != "input" || record["operation"] != "read" {
				t.Errorf("unexpected journal error record: %q", line)
			}
		}
	}
	if fatalCount != 1 {
		t.Fatalf("journal fatal record count = %d, want one; stderr=%s", fatalCount, &got.stderr)
	}
}

type playerRun struct {
	err     error
	opts    options
	audio   *player.Fake
	states  []app.AppState
	stderr  bytes.Buffer
	logFile string
}

func runPlayer(t *testing.T, args []string, input eventInput) playerRun {
	t.Helper()
	testDir := t.TempDir()
	settingsFile := filepath.Join(testDir, "settings.json")
	logFile := filepath.Join(testDir, "player.log")
	baseArgs := []string{
		"--settings-file", settingsFile,
		"--music-dir", filepath.Join(testDir, "Music"),
		"--log-file", logFile,
	}
	args = append(baseArgs, args...)

	result := playerRun{audio: player.NewFake(), logFile: logFile}
	renderer := &stateRenderer{}
	result.err = run(args, &result.stderr, func(ctx context.Context, opts options, logger *slog.Logger) error {
		result.opts = opts
		return runApplication(ctx, opts, result.audio, input, renderer, logger)
	})
	result.states = renderer.states
	return result
}

type eventInput struct {
	events []app.Event
	err    error
}

func (in eventInput) Run(ctx context.Context, out chan<- app.Event) error {
	for _, event := range in.events {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case out <- event:
		}
	}
	return in.err
}

type stateRenderer struct {
	states []app.AppState
}

func (r *stateRenderer) Render(state app.AppState) error {
	r.states = append(r.states, state)
	return nil
}

func lastState(t *testing.T, states []app.AppState) app.AppState {
	t.Helper()
	if len(states) == 0 {
		t.Fatal("renderer received no states")
	}
	return states[len(states)-1]
}

func trackTitles(state app.AppState) []string {
	titles := make([]string, len(state.Library.Tracks))
	for i, track := range state.Library.Tracks {
		titles[i] = track.Title
	}
	return titles
}

func writePlayerTestFile(t *testing.T, root, name string) {
	t.Helper()
	extension := strings.ToLower(filepath.Ext(name))
	fixture, err := os.ReadFile(filepath.Join("..", "..", "internal", "library", "testdata", "valid"+extension))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, fixture, 0o600); err != nil {
		t.Fatal(err)
	}
}

func readJSONLog(t *testing.T, path string) []map[string]any {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	var records []map[string]any
	for {
		var record map[string]any
		if err := decoder.Decode(&record); err != nil {
			if errors.Is(err, io.EOF) {
				return records
			}
			t.Fatal(err)
		}
		records = append(records, record)
	}
}
