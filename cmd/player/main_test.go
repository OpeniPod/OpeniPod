package main

import (
	"bytes"
	"os"
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
	var out bytes.Buffer
	if err := run(args, in, &out); err != nil {
		t.Fatal(err)
	}
	return out.String()
}
