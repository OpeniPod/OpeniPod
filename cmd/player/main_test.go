package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OpeniPod/OpeniPod/internal/app"
	"github.com/OpeniPod/OpeniPod/internal/player"
)

func TestRunSelectsLibrary(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "Album"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"01.flac", "Album/02.MP3", "Daft Punk - Get Lucky (feat. Pharrell Williams).mp3", "notes.txt"} {
		writePlayerTestFile(t, root, name)
	}

	for _, tc := range []struct {
		name    string
		args    []string
		want    []string
		missing []string
	}{
		{
			name:    "sample library by default",
			want:    []string{"> One More Time\n", "  Digital Love\n", "Playing"},
			missing: []string{"01.flac", "02.MP3"},
		},
		{
			name:    "filesystem library from flag",
			args:    []string{"--music-dir", root},
			want:    []string{"> 01.flac\n", "  02.MP3\n", "  Daft Punk - Get Lucky (feat. Pharrell Williams).mp3\n", "Playing"},
			missing: []string{"One More Time", "Digital Love", "notes.txt"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			output := runWithInput(t, tc.args, "\n\n")
			for _, want := range tc.want {
				if !strings.Contains(output, want) {
					t.Errorf("output does not contain %q:\n%s", want, output)
				}
			}
			for _, missing := range tc.missing {
				if strings.Contains(output, missing) {
					t.Errorf("output unexpectedly contains %q:\n%s", missing, output)
				}
			}
		})
	}
}

func TestRunEmptyMusicDirectory(t *testing.T) {
	output := runWithInput(t, []string{"--music-dir", t.TempDir()}, "\n")
	if !strings.Contains(output, "No tracks loaded") || strings.Contains(output, "Error:") {
		t.Fatalf("expected an empty library without an error:\n%s", output)
	}
}

func TestRunShowsLibraryError(t *testing.T) {
	root := filepath.Join(t.TempDir(), "missing")
	output := runWithInput(t, []string{"--music-dir", root}, "\n")
	if !strings.Contains(output, "Error: load library:") || !strings.Contains(output, root) {
		t.Fatalf("expected the library error and directory path:\n%s", output)
	}
	if !strings.Contains(output, "No tracks loaded") || strings.Contains(output, "One More Time") {
		t.Fatalf("expected an empty library after a load failure:\n%s", output)
	}
}

func TestRunUsesSavedLibraryUntilMenuUpdate(t *testing.T) {
	root := t.TempDir()
	oldPath := filepath.Join(root, "old.mp3")
	writePlayerTestFile(t, root, filepath.Base(oldPath))
	args := []string{"--music-dir", root}
	first := runWithInput(t, args, "\n")
	if !strings.Contains(first, "> old.mp3\n") {
		t.Fatalf("first run did not discover music:\n%s", first)
	}
	if strings.Contains(first, "Update Library") {
		t.Fatalf("first run showed Update Library on Home:\n%s", first)
	}
	if err := os.Remove(oldPath); err != nil {
		t.Fatal(err)
	}
	writePlayerTestFile(t, root, "new.flac")
	second := runWithInput(t, args, "\n")
	if !strings.Contains(second, "> old.mp3\n") || strings.Contains(second, "new.flac") {
		t.Fatalf("second run did not use the saved library:\n%s", second)
	}

	// Enter Settings, select Update Library, then return to Tracks.
	updated := runWithInput(t, args, "jj\n\nbkk\n")
	if !strings.Contains(updated, "> Update Library\n") || strings.Contains(updated, "Updating library...") || !strings.Contains(updated, "> new.flac\n") || strings.Contains(updated, "old.mp3") || strings.Contains(updated, "Error:") {
		t.Fatalf("menu update did not show the new library:\n%s", updated)
	}
	third := runWithInput(t, args, "\n")
	if !strings.Contains(third, "> new.flac\n") || strings.Contains(third, "old.mp3") {
		t.Fatalf("menu update did not persist across runs:\n%s", third)
	}
}

func TestRunMenuUpdateRecoversInvalidIndex(t *testing.T) {
	root := t.TempDir()
	writePlayerTestFile(t, root, "track.mp3")
	if err := os.WriteFile(filepath.Join(root, ".openipod-library.json"), []byte("invalid JSON"), 0o600); err != nil {
		t.Fatal(err)
	}
	output := runWithInput(t, []string{"--music-dir", root}, "jj\n\nbkk\n")
	if !strings.Contains(output, "Error: load library:") {
		t.Fatalf("invalid index did not show a startup error:\n%s", output)
	}
	lastView := output[strings.LastIndex(output, "\x1b[H\x1b[2J"):]
	if !strings.Contains(lastView, "> track.mp3\n") || strings.Contains(lastView, "Error:") {
		t.Fatalf("menu update did not recover the library:\n%s", lastView)
	}
}

func TestRunPersistsEqualizerPresetAcrossRuns(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	args := []string{"--settings-file", settingsPath}

	bassBoost := runWithInput(t, args, "jj\nj\n")
	if !strings.Contains(bassBoost, "> Equalizer: Bass boost\n") {
		t.Fatalf("selecting the equalizer did not choose Bass boost:\n%s", bassBoost)
	}
	stored, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(stored), `"equalizer":"bass_boost"`) {
		t.Fatalf("settings file did not store Bass boost: %s", stored)
	}

	restored := runWithInput(t, args, "jj\n")
	if !strings.Contains(restored, "Equalizer: Bass boost\n") {
		t.Fatalf("next run did not restore Bass boost:\n%s", restored)
	}

	vocal := runWithInput(t, args, "jj\nj\n")
	if !strings.Contains(vocal, "> Equalizer: Vocal\n") {
		t.Fatalf("cycling from Bass boost did not choose Vocal:\n%s", vocal)
	}

	off := runWithInput(t, args, "jj\nj\n")
	if !strings.Contains(off, "> Equalizer: Off\n") {
		t.Fatalf("cycling from Vocal did not choose Off:\n%s", off)
	}
}

func runWithInput(t *testing.T, args []string, keys string) string {
	t.Helper()
	testDir := t.TempDir()
	args = append([]string{
		"--log-file", filepath.Join(testDir, "player.log"),
	}, args...)
	in, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = in.Close() })
	t.Cleanup(func() { _ = writer.Close() })
	if _, err := writer.WriteString(keys); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	var out, stderr bytes.Buffer
	if err := runWith(t, args, in, &out, &stderr); err != nil {
		t.Fatal(err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("diagnostics reached stderr in file mode: %s", &stderr)
	}
	return out.String()
}

func runWith(t *testing.T, args []string, stdin *os.File, stdout, stderr io.Writer) error {
	t.Helper()
	testDir := t.TempDir()
	args = append([]string{
		"--settings-file", filepath.Join(testDir, "settings.json"),
	}, args...)
	return runWithPlayerFactory(args, stdin, stdout, stderr, func() (app.Player, error) {
		return player.NewFake(), nil
	})
}

func writePlayerTestFile(t *testing.T, root, name string) {
	t.Helper()
	data := []byte("fixture")
	extension := strings.ToLower(filepath.Ext(name))
	if isPlayerFixtureExtension(extension) {
		var err error
		data, err = os.ReadFile(filepath.Join("..", "..", "internal", "library", "testdata", "valid"+extension))
		if err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func isPlayerFixtureExtension(extension string) bool {
	switch extension {
	case ".mp3", ".flac", ".wav", ".ogg", ".m4a", ".aac":
		return true
	default:
		return false
	}
}

func TestRunFileLoggingKeepsUISeparate(t *testing.T) {
	stdin, err := os.CreateTemp(t.TempDir(), "input")
	if err != nil {
		t.Fatal(err)
	}
	defer stdin.Close()
	if _, err := stdin.WriteString("\n\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := stdin.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "player.log")
	var stdout, stderr bytes.Buffer
	if err := runWith(t, []string{"--log-file", path, "--log-level", "debug"}, stdin, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	if stderr.Len() != 0 || strings.Contains(stdout.String(), "application started") {
		t.Fatalf("logs reached terminal output: stderr=%s stdout=%s", &stderr, &stdout)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte("playback started")) || !bytes.Contains(data, []byte("\"level\":\"DEBUG\"")) {
		t.Fatalf("missing runtime diagnostics: %s", data)
	}
}

func TestFilesystemLibraryDiagnostics(t *testing.T) {
	for _, recoverIndex := range []bool{false, true} {
		name := "startup"
		if recoverIndex {
			name = "menu refresh after invalid index"
		}
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			writePlayerTestFile(t, root, "track.mp3")
			keys := "\n\n"
			if recoverIndex {
				if err := os.WriteFile(filepath.Join(root, ".openipod-library.json"), []byte("invalid JSON"), 0o600); err != nil {
					t.Fatal(err)
				}
				keys = "jj\n\nbkk\n\n"
			}
			path := filepath.Join(t.TempDir(), "player.log")
			output := runWithInput(t, []string{"--music-dir", root, "--log-file", path}, keys)
			if !strings.Contains(output, "Playing") || strings.Contains(output, "application started") {
				t.Fatalf("filesystem playback or UI/log separation failed: %s", output)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			decoder := json.NewDecoder(bytes.NewReader(data))
			counts := make(map[string]int)
			for decoder.More() {
				var record map[string]any
				if err := decoder.Decode(&record); err != nil {
					t.Fatal(err)
				}
				counts[record["msg"].(string)]++
				if record["level"] == "ERROR" {
					t.Fatalf("recoverable library failure became fatal: %v", record)
				}
				if record["msg"] == "library loaded" && record["track_count"] != float64(1) {
					t.Fatalf("missing filesystem track count: %v", record)
				}
				if record["msg"] == "playback started" {
					if id, ok := record["track_id"].(string); !ok || id == "" {
						t.Fatalf("missing filesystem track ID: %v", record)
					}
				}
			}
			wantFailures := 0
			if recoverIndex {
				wantFailures = 1
			}
			if counts["library loaded"] != 1 || counts["library load failed"] != wantFailures || counts["playback started"] != 1 {
				t.Fatalf("missing or duplicate filesystem diagnostics: %v", counts)
			}
		})
	}
}

func TestRunJournalFailureIsAlreadyRecorded(t *testing.T) {
	stdin, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	stdin.Close() // Force a real keyboard-adapter failure.
	var stdout, stderr bytes.Buffer
	err = runWith(t, []string{"--log-target", "journal"}, stdin, &stdout, &stderr)
	var recorded *loggedError
	if !errors.As(err, &recorded) {
		t.Fatalf("runtime failure not marked as logged: %v", err)
	}
	count := 0
	for _, line := range strings.Split(strings.TrimSpace(stderr.String()), "\n") {
		if len(line) < 3 {
			t.Fatalf("invalid journal output: %q", line)
		}
		var record map[string]any
		if err := json.Unmarshal([]byte(line[3:]), &record); err != nil {
			t.Fatal(err)
		}
		if record["level"] == "ERROR" {
			count++
			if line[:3] != "<3>" || record["component"] != "input" || record["operation"] != "read" {
				t.Fatalf("wrong journal error: %s", line)
			}
		}
	}
	if count != 1 || strings.Contains(stdout.String(), "operation failed") {
		t.Fatalf("error duplicated or leaked to UI: %s", &stderr)
	}
}

func TestHelpAndBadConfigurationDoNotStartUI(t *testing.T) {
	for _, args := range [][]string{
		{"--help"}, {"--log-level", "verbose"}, {"--log-target", "console"},
		{"--platform", "unknown"}, {"unexpected"},
	} {
		var stdout, stderr bytes.Buffer
		err := runWith(t, args, nil, &stdout, &stderr)
		if args[0] == "--help" {
			if err != nil || !strings.Contains(stderr.String(), "log-level") {
				t.Fatalf("help output: %v, %s", err, &stderr)
			}
		} else if err == nil {
			t.Fatalf("bad configuration accepted: %v", args)
		}
		if stdout.Len() != 0 {
			t.Fatalf("UI started before configuration validation: %v", args)
		}
	}
}
