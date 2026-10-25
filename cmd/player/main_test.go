package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunSelectsLibrary(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "Album"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"01.flac", "Album/02.MP3", "Daft Punk - Get Lucky (feat. Pharrell Williams).mp3", "notes.txt"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("fixture"), 0o644); err != nil {
			t.Fatal(err)
		}
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
	if err := os.WriteFile(oldPath, []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
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
	if err := os.WriteFile(filepath.Join(root, "new.flac"), []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
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
	if err := os.WriteFile(filepath.Join(root, "track.mp3"), []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
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

func runWithInput(t *testing.T, args []string, keys string) string {
	t.Helper()
	args = append([]string{"--settings-file", filepath.Join(t.TempDir(), "settings.json")}, args...)
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
	args = append([]string{"--log-file", filepath.Join(t.TempDir(), "player.log")}, args...)
	if err := runWith(args, in, &out, &stderr); err != nil {
		t.Fatal(err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("diagnostics reached stderr in file mode: %s", &stderr)
	}
	return out.String()
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
	if err := runWith([]string{"--log-file", path, "--log-level", "debug"}, stdin, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	if stderr.Len() != 0 || strings.Contains(stdout.String(), "application started") {
		t.Fatalf("logs reached terminal output: stderr=%s stdout=%s", &stderr, &stdout)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte(" INFO [player/started] playback started (track_id: one-more-time)")) || !bytes.Contains(data, []byte(" DEBUG [storage/load_settings] settings loaded")) {
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
			if err := os.WriteFile(filepath.Join(root, "track.mp3"), []byte("fixture"), 0o600); err != nil {
				t.Fatal(err)
			}
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
			counts := make(map[string]int)
			for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
				if strings.Contains(line, " ERROR ") {
					t.Fatalf("recoverable library failure became fatal: %s", line)
				}
				for _, message := range []string{"library loaded", "library load failed", "playback started"} {
					if strings.Contains(line, "] "+message+" (") {
						counts[message]++
					}
				}
				if strings.Contains(line, "] library loaded ") && !strings.HasSuffix(line, "(track_count: 1)") {
					t.Fatalf("missing filesystem track count: %s", line)
				}
				if strings.Contains(line, "] playback started ") && !strings.Contains(line, "(track_id: ") {
					t.Fatalf("missing filesystem track ID: %s", line)
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
	err = runWith([]string{"--log-target", "journal"}, stdin, &stdout, &stderr)
	var recorded *loggedError
	if !errors.As(err, &recorded) {
		t.Fatalf("runtime failure not marked as logged: %v", err)
	}
	count := 0
	for _, line := range strings.Split(strings.TrimSpace(stderr.String()), "\n") {
		if len(line) < 3 {
			t.Fatalf("invalid journal output: %q", line)
		}
		if strings.Contains(line, " ERROR ") {
			count++
			if line[:3] != "<3>" || !strings.Contains(line, "[input/read] operation failed (error: ") {
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
		err := runWith(args, nil, &stdout, &stderr)
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

type failingWriter struct{ err error }

func (w failingWriter) Write([]byte) (int, error) { return 0, w.err }

func TestRunPreservesApplicationAndLogWriteErrors(t *testing.T) {
	appErr := errors.New("display unavailable")
	logErr := errors.New("log disk full")
	for _, failApp := range []bool{false, true} {
		t.Run(fmt.Sprint(failApp), func(t *testing.T) {
			stdin, err := os.Open(os.DevNull)
			if err != nil {
				t.Fatal(err)
			}
			defer stdin.Close()
			var stdout io.Writer = io.Discard
			if failApp {
				stdout = failingWriter{appErr}
			}
			err = runWith([]string{"--log-target", "journal", "--settings-file", filepath.Join(t.TempDir(), "settings.json")}, stdin, stdout, failingWriter{logErr})
			if !errors.Is(err, logErr) || (failApp && !errors.Is(err, appErr)) {
				t.Fatalf("original failures were lost: %v", err)
			}
			var recorded *loggedError
			if errors.As(err, &recorded) {
				t.Fatalf("unwritten failure would be hidden from stderr: %v", err)
			}
		})
	}
}

func TestMainReportsFullLogDestination(t *testing.T) {
	if os.Getenv("OPENIPOD_LOG_FAILURE_HELPER") == "1" {
		os.Args = []string{"player", "--log-file=/dev/full", "--settings-file", os.Getenv("OPENIPOD_LOG_FAILURE_SETTINGS")}
		main()
		t.Fatal("main unexpectedly returned after a failed log write")
	}
	for _, failUI := range []bool{false, true} {
		t.Run(fmt.Sprint(failUI), func(t *testing.T) {
			command := exec.Command(os.Args[0], "-test.run=^TestMainReportsFullLogDestination$")
			command.Env = append(os.Environ(), "OPENIPOD_LOG_FAILURE_HELPER=1",
				"OPENIPOD_LOG_FAILURE_SETTINGS="+filepath.Join(t.TempDir(), "settings.json"))
			if failUI {
				full, err := os.OpenFile("/dev/full", os.O_WRONLY, 0)
				if err != nil {
					t.Fatal(err)
				}
				defer full.Close()
				command.Stdout = full
			}
			var stderr bytes.Buffer
			command.Stderr = &stderr
			err := command.Run() // Unset stdin is /dev/null, like the review example.
			var exit *exec.ExitError
			if !errors.As(err, &exit) || exit.ExitCode() != 1 {
				t.Fatalf("exit = %v, stderr = %s", err, &stderr)
			}
			if !strings.Contains(stderr.String(), "write log: write /dev/full:") ||
				(failUI && !strings.Contains(stderr.String(), "ui render: write /dev/stdout:")) {
				t.Fatalf("stderr lost an original failure: %s", &stderr)
			}
		})
	}
}
