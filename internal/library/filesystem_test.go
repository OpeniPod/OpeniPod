package library

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/OpeniPod/OpeniPod/internal/music"
)

func TestFilesystemLoadDiscoversSupportedFiles(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "nested", "deeper"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"track.mp3",
		"nested.mp3",
		"nested/deeper/album.FLAC",
		"nested/live.WaV",
		"nested/live.ogg",
		"nested/live.m4a",
		"nested/live.AAC",
		"ignored.txt",
		"nested/live.mp3.txt",
	} {
		writeTestFile(t, root, name)
	}
	if err := os.Mkdir(filepath.Join(root, "dir.mp3"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, root, "dir.mp3/inside.mp3")
	if err := os.Symlink(filepath.Join(root, "track.mp3"), filepath.Join(root, "nested", "link.mp3")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "nested"), filepath.Join(root, "nested-link")); err != nil {
		t.Fatal(err)
	}

	lib := NewFilesystem(root)
	got, err := lib.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	wantPaths := []string{
		filepath.Join(root, "dir.mp3", "inside.mp3"),
		filepath.Join(root, "nested.mp3"),
		filepath.Join(root, "nested", "deeper", "album.FLAC"),
		filepath.Join(root, "nested", "live.AAC"),
		filepath.Join(root, "nested", "live.WaV"),
		filepath.Join(root, "nested", "live.m4a"),
		filepath.Join(root, "nested", "live.ogg"),
		filepath.Join(root, "track.mp3"),
	}
	if len(got) != len(wantPaths) {
		t.Fatalf("got %d tracks, want %d: %+v", len(got), len(wantPaths), got)
	}
	seenIDs := make(map[music.TrackID]bool, len(got))
	for i, track := range got {
		if track.Path != filepath.Clean(wantPaths[i]) {
			t.Errorf("track %d path = %q, want %q", i, track.Path, wantPaths[i])
		}
		if track.Title != filepath.Base(wantPaths[i]) {
			t.Errorf("track %d title = %q, want %q", i, track.Title, filepath.Base(wantPaths[i]))
		}
		if track.ID == "" || seenIDs[track.ID] {
			t.Errorf("track %d ID = %q, want non-empty unique ID", i, track.ID)
		}
		seenIDs[track.ID] = true
		if track.Artist != "" || track.Album != "" || track.TrackNum != 0 || track.Duration != 0 {
			t.Errorf("track %d has unexpected metadata: %+v", i, track)
		}
	}
}

func TestFilesystemLoadValidatesMP3ContentInMixedDirectory(t *testing.T) {
	root := t.TempDir()
	writeTestBytes(t, root, "text-renamed.mp3", []byte("this is not an audio file"))
	writeTestBytes(t, root, "tags-only.mp3", []byte{
		'I', 'D', '3', 0x04, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	})
	writeTestFile(t, root, "valid.mp3")
	valid := readTestMP3(t)
	damagedTags := append([]byte{
		'I', 'D', '3', 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
	}, valid...)
	writeTestBytes(t, root, "damaged-tags.mp3", damagedTags)
	corruptedTail := append(append([]byte(nil), valid...), []byte("not an MP3 frame")...)
	writeTestBytes(t, root, "corrupted-tail.mp3", corruptedTail)
	writeTestFile(t, root, filepath.Join("nested", "other.flac"))

	tracks, err := NewFilesystem(root).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	wantTitles := []string{
		"corrupted-tail.mp3",
		"damaged-tags.mp3",
		"other.flac",
		"valid.mp3",
	}
	if got := trackTitles(tracks); !reflect.DeepEqual(got, wantTitles) {
		t.Fatalf("tracks = %v, want %v", got, wantTitles)
	}
	for _, track := range tracks {
		if track.Title == "valid.mp3" && (track.Artist != "" || track.Album != "" || track.TrackNum != 0 || track.Duration != 0) {
			t.Fatalf("untagged MP3 has unexpected metadata: %+v", track)
		}
	}
}

func TestFilesystemLoadRejectsRenamedTextForEverySupportedFormat(t *testing.T) {
	root := t.TempDir()
	for _, extension := range supportedTestExtensions() {
		writeTestFile(t, root, "valid"+extension)
		writeTestBytes(t, root, "text"+extension, []byte("this is not an audio file"))
	}

	wantTitles := []string{
		"valid.aac",
		"valid.flac",
		"valid.m4a",
		"valid.mp3",
		"valid.ogg",
		"valid.wav",
	}
	lib := NewFilesystem(root)
	first, err := lib.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := trackTitles(first); !reflect.DeepEqual(got, wantTitles) {
		t.Fatalf("first scan tracks = %v, want %v", got, wantTitles)
	}

	refreshed, err := lib.Refresh(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := trackTitles(refreshed); !reflect.DeepEqual(got, wantTitles) {
		t.Fatalf("refresh tracks = %v, want %v", got, wantTitles)
	}
}

func TestFilesystemLoadRejectsVideoOnlyM4A(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "valid.m4a")
	writeTestBytes(t, root, "video-only.m4a", readFilesystemFixture(t, "video-only.mp4"))

	lib := NewFilesystem(root)
	tracks, err := lib.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	wantTitles := []string{"valid.m4a"}
	if got := trackTitles(tracks); !reflect.DeepEqual(got, wantTitles) {
		t.Fatalf("first scan tracks = %v, want %v", got, wantTitles)
	}

	refreshed, err := lib.Refresh(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := trackTitles(refreshed); !reflect.DeepEqual(got, wantTitles) {
		t.Fatalf("refresh tracks = %v, want %v", got, wantTitles)
	}
}

func TestFilesystemLoadAcceptsOggOpus(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "valid.ogg")
	writeTestBytes(t, root, "opus.ogg", readFilesystemFixture(t, "opus.ogg"))

	tracks, err := NewFilesystem(root).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	wantTitles := []string{"opus.ogg", "valid.ogg"}
	if got := trackTitles(tracks); !reflect.DeepEqual(got, wantTitles) {
		t.Fatalf("tracks = %v, want %v", got, wantTitles)
	}
}

func TestFilesystemLoadCacheDoesNotRevalidateMP3UntilRefresh(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "track.mp3")
	lib := NewFilesystem(root)
	tracks, err := lib.Load(context.Background())
	if err != nil || len(tracks) != 1 {
		t.Fatalf("initial load = tracks=%+v err=%v, want one track", tracks, err)
	}

	writeTestBytes(t, root, "track.mp3", []byte("the cached MP3 is now damaged"))
	cached, err := lib.Load(context.Background())
	if err != nil || len(cached) != 1 || cached[0].Title != "track.mp3" {
		t.Fatalf("cached load = tracks=%+v err=%v, want cached track", cached, err)
	}

	refreshed, err := lib.Refresh(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if refreshed == nil || len(refreshed) != 0 {
		t.Fatalf("refresh = tracks=%+v, want a non-nil empty library", refreshed)
	}
}

func TestFilesystemLoadUsesCacheAndRefreshReturnsFreshSlices(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "b.mp3")

	configuredRoot := root + "/nested/.."
	lib := NewFilesystem(configuredRoot)
	first, err := lib.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 1 {
		t.Fatalf("first load got %d tracks, want 1", len(first))
	}

	writeTestFile(t, root, "a.mp3")
	second, err := lib.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(second) != 1 || second[0].Title != "b.mp3" {
		t.Fatalf("second cached load = %+v, want b.mp3", second)
	}

	refreshed, err := lib.Refresh(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := []string{refreshed[0].Title, refreshed[1].Title}; !reflect.DeepEqual(got, []string{"a.mp3", "b.mp3"}) {
		t.Fatalf("sorted titles = %v", got)
	}
	if refreshed[1].ID != first[0].ID || refreshed[1].Path != first[0].Path {
		t.Fatalf("existing track changed after adding an earlier file: before=%+v after=%+v", first[0], refreshed[1])
	}
	first[0].Title = "changed outside library"
	third, err := lib.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(third) != 2 {
		t.Fatalf("third load got %d tracks, want 2", len(third))
	}
	if third[1].Title != "b.mp3" {
		t.Fatalf("load reused mutable track data: %+v", third)
	}
}

func TestFilesystemLoadCacheSurvivesFilesystemChanges(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "kept.mp3")
	writeTestFile(t, root, "removed.mp3")
	if err := os.Mkdir(filepath.Join(root, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, root, filepath.Join("nested", "hidden.mp3"))

	lib := NewFilesystem(root)
	if _, err := lib.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "removed.mp3")); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(root, "nested")); err != nil {
		t.Fatal(err)
	}

	tracks, err := NewFilesystem(root).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := []string{tracks[0].Title, tracks[1].Title, tracks[2].Title}; !reflect.DeepEqual(got, []string{"kept.mp3", "hidden.mp3", "removed.mp3"}) {
		t.Fatalf("cached tracks = %v, want original scan", got)
	}
	tracks, err = NewFilesystem(root).Refresh(context.Background())
	if err != nil || len(tracks) != 1 || tracks[0].Title != "kept.mp3" {
		t.Fatalf("refreshed tracks = %+v err=%v, want kept.mp3", tracks, err)
	}
}

func TestFilesystemIndexPayloadAndPermissions(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "track.mp3")
	configuredRoot := filepath.Join(root, ".", "nested", "..")
	if _, err := NewFilesystem(configuredRoot).Load(context.Background()); err != nil {
		t.Fatal(err)
	}

	indexPath := filepath.Join(root, ".openipod-library.json")
	data, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Version int           `json:"version"`
		Root    string        `json:"root"`
		Tracks  []music.Track `json:"tracks"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatal(err)
	}
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	if payload.Version != 1 || payload.Root != filepath.Clean(absoluteRoot) || len(payload.Tracks) != 1 {
		t.Fatalf("index payload = %+v, want version 1, cleaned root, one track", payload)
	}
	mode, err := os.Stat(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	if mode.Mode().Perm() != 0o600 {
		t.Fatalf("index mode = %o, want 600", mode.Mode().Perm())
	}
}

func TestFilesystemCachedLoadDoesNotReadDirectory(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "track.mp3")
	if _, err := NewFilesystem(root).Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(root, 0o700); err != nil {
			t.Errorf("restore root permissions: %v", err)
		}
	})
	// A known file remains readable with directory search permission, but
	// enumerating that directory requires read permission.
	if err := os.Chmod(root, 0o100); err != nil {
		t.Fatal(err)
	}
	if _, err := os.ReadDir(root); err == nil {
		t.Skip("directory is readable despite its permissions (for example, running as root)")
	} else if !errors.Is(err, fs.ErrPermission) {
		t.Fatal(err)
	}
	tracks, err := NewFilesystem(root).Load(context.Background())
	if err != nil || len(tracks) != 1 || tracks[0].Title != "track.mp3" {
		t.Fatalf("cached load enumerated directory: tracks=%+v err=%v", tracks, err)
	}
}

func TestFilesystemLoadRejectsCorruptIndexAndRefreshRecovers(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "track.mp3")
	lib := NewFilesystem(root)
	if _, err := lib.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	indexPath := filepath.Join(root, ".openipod-library.json")
	if err := os.WriteFile(indexPath, []byte(`{"version":1,"root":"wrong","tracks":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}

	tracks, err := lib.Load(context.Background())
	if tracks != nil || err == nil || !strings.Contains(err.Error(), "root") {
		t.Fatalf("corrupt load = tracks=%#v err=%v, want root validation error", tracks, err)
	}
	tracks, err = lib.Refresh(context.Background())
	if err != nil || len(tracks) != 1 {
		t.Fatalf("recovery refresh = tracks=%#v err=%v", tracks, err)
	}
	tracks, err = NewFilesystem(root).Load(context.Background())
	if err != nil || len(tracks) != 1 || tracks[0].Title != "track.mp3" {
		t.Fatalf("recovered load = tracks=%#v err=%v", tracks, err)
	}
}

func TestFilesystemLoadRejectsInvalidIndexWithoutRescan(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "existing.mp3")
	if _, err := NewFilesystem(root).Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, root, "new.mp3")
	indexPath := filepath.Join(root, ".openipod-library.json")

	tests := []struct {
		name string
		data string
	}{
		{name: "malformed JSON", data: "{"},
		{name: "unsupported version", data: fmt.Sprintf(`{"version":2,"root":%q,"tracks":[]}`, root)},
		{name: "missing tracks", data: fmt.Sprintf(`{"version":1,"root":%q}`, root)},
		{name: "null tracks", data: fmt.Sprintf(`{"version":1,"root":%q,"tracks":null}`, root)},
		{name: "malformed track object", data: fmt.Sprintf(`{"version":1,"root":%q,"tracks":[{"Path":1}]}`, root)},
		{name: "null track object", data: fmt.Sprintf(`{"version":1,"root":%q,"tracks":[null]}`, root)},
		{name: "missing track fields", data: fmt.Sprintf(`{"version":1,"root":%q,"tracks":[{}]}`, root)},
		{name: "outside root", data: fmt.Sprintf(`{"version":1,"root":%q,"tracks":[{"ID":"outside","Path":%q,"Title":"outside.mp3"}]}`, root, filepath.Join(filepath.Dir(root), "outside.mp3"))},
		{name: "duplicate paths", data: fmt.Sprintf(`{"version":1,"root":%q,"tracks":[{"ID":"first","Path":%q,"Title":"existing.mp3"},{"ID":"second","Path":%q,"Title":"existing.mp3"}]}`, root, filepath.Join(root, "existing.mp3"), filepath.Join(root, "existing.mp3"))},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := os.WriteFile(indexPath, []byte(test.data), 0o600); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(indexPath)
			if err != nil {
				t.Fatal(err)
			}

			tracks, err := NewFilesystem(root).Load(context.Background())
			if tracks != nil || err == nil {
				t.Fatalf("load = tracks=%#v err=%v, want validation error", tracks, err)
			}
			after, err := os.ReadFile(indexPath)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(after, before) {
				t.Fatalf("invalid index was changed: before=%q after=%q", before, after)
			}
		})
	}
}

func TestFilesystemLoadValidatesCachedTrackIDs(t *testing.T) {
	tests := []struct {
		name      string
		mutate    func([]music.Track)
		wantError string
	}{
		{name: "valid index"},
		{
			name: "duplicate IDs",
			mutate: func(tracks []music.Track) {
				tracks[1].ID = tracks[0].ID
			},
			wantError: "track 1 has duplicate ID",
		},
		{
			name: "ID does not match path",
			mutate: func(tracks []music.Track) {
				tracks[1].ID = music.TrackID(fmt.Sprintf("%x", sha256.Sum256([]byte(tracks[1].Path+".other"))))
			},
			wantError: "track 1 ID does not match path",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			writeTestFile(t, root, "a.mp3")
			writeTestFile(t, root, "b.mp3")
			original, err := NewFilesystem(root).Load(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if len(original) != 2 {
				t.Fatalf("initial load = %+v, want two tracks", original)
			}
			for _, track := range original {
				wantID := music.TrackID(fmt.Sprintf("%x", sha256.Sum256([]byte(track.Path))))
				if track.ID != wantID {
					t.Fatalf("track %q ID = %q, want SHA-256 %q", track.Path, track.ID, wantID)
				}
			}
			if test.mutate != nil {
				test.mutate(original)
			}
			data, err := json.Marshal(libraryIndex{Version: libraryIndexVersion, Root: root, Tracks: original})
			if err != nil {
				t.Fatal(err)
			}
			indexPath := filepath.Join(root, ".openipod-library.json")
			if err := os.WriteFile(indexPath, data, 0o600); err != nil {
				t.Fatal(err)
			}

			tracks, err := NewFilesystem(root).Load(context.Background())
			if test.wantError != "" {
				if tracks != nil || err == nil || !strings.Contains(err.Error(), "decode library index: "+test.wantError) {
					t.Fatalf("load = tracks=%#v err=%v, want nil tracks and %q", tracks, err, test.wantError)
				}
			} else if err != nil || !reflect.DeepEqual(tracks, original) {
				t.Fatalf("cached load = tracks=%#v err=%v, want original tracks with stable IDs %+v", tracks, err, original)
			}
			after, err := os.ReadFile(indexPath)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(after, data) {
				t.Fatalf("load changed index: before=%q after=%q", data, after)
			}
		})
	}
}

func TestFilesystemRefreshCancellationPreservesIndex(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "track.mp3")
	lib := NewFilesystem(root)
	if _, err := lib.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, root, "new.mp3")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	tracks, err := lib.Refresh(ctx)
	if tracks != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled refresh = tracks=%#v err=%v", tracks, err)
	}
	tracks, err = NewFilesystem(root).Load(context.Background())
	if err != nil || len(tracks) != 1 || tracks[0].Title != "track.mp3" {
		t.Fatalf("load after canceled refresh = tracks=%#v err=%v", tracks, err)
	}
}

func TestFilesystemRefreshFailurePreservesIndex(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "track.mp3")
	lib := NewFilesystem(root)
	if _, err := lib.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	blocked := filepath.Join(root, "blocked")
	if err := os.Mkdir(blocked, 0o700); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, blocked, "hidden.mp3")
	t.Cleanup(func() {
		if err := os.Chmod(blocked, 0o700); err != nil {
			t.Errorf("restore directory permissions: %v", err)
		}
	})
	if err := os.Chmod(blocked, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := os.ReadDir(blocked); err == nil {
		t.Skip("directory is readable despite its permissions (for example, running as root)")
	} else if !errors.Is(err, fs.ErrPermission) {
		t.Fatalf("read directory: %v", err)
	}

	tracks, err := lib.Refresh(context.Background())
	if tracks != nil || !errors.Is(err, fs.ErrPermission) {
		t.Fatalf("failed refresh = tracks=%#v err=%v, want permission error", tracks, err)
	}
	tracks, err = NewFilesystem(root).Load(context.Background())
	if err != nil || len(tracks) != 1 || tracks[0].Title != "track.mp3" {
		t.Fatalf("load after failed refresh = tracks=%#v err=%v", tracks, err)
	}
}

func TestFilesystemRefreshWriteFailurePreservesIndex(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "existing.mp3")
	lib := NewFilesystem(root)
	if _, err := lib.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	indexPath := filepath.Join(root, ".openipod-library.json")
	previousIndex, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, root, "new.mp3")

	if err := os.Chmod(root, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(root, 0o700); err != nil {
			t.Errorf("restore root permissions: %v", err)
		}
	})
	probe, err := os.CreateTemp(root, ".write-permission-probe-*")
	if err == nil {
		probePath := probe.Name()
		if closeErr := probe.Close(); closeErr != nil {
			t.Fatal(closeErr)
		}
		if removeErr := os.Remove(probePath); removeErr != nil {
			t.Fatal(removeErr)
		}
		t.Skip("process can write despite root permissions (for example, running as root)")
	}
	if !errors.Is(err, fs.ErrPermission) {
		t.Fatalf("write permission probe: %v", err)
	}

	tracks, err := lib.Refresh(context.Background())
	if tracks != nil || err == nil {
		t.Fatalf("failed refresh = tracks=%#v err=%v, want persistence error", tracks, err)
	}
	after, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, previousIndex) {
		t.Fatalf("failed refresh changed index: before=%q after=%q", previousIndex, after)
	}
	tracks, err = NewFilesystem(root).Load(context.Background())
	if err != nil || len(tracks) != 1 || tracks[0].Title != "existing.mp3" {
		t.Fatalf("load after failed persistence = tracks=%#v err=%v", tracks, err)
	}
}

func TestFilesystemLoadEmptyDirectory(t *testing.T) {
	for _, tc := range []struct {
		name  string
		files []string
	}{
		{name: "empty"},
		{name: "unsupported only", files: []string{"notes.txt", "cover.jpg", "video.mp4", "no-extension"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			for _, name := range tc.files {
				writeTestFile(t, root, name)
			}
			tracks, err := NewFilesystem(root).Load(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if tracks == nil || len(tracks) != 0 {
				t.Fatalf("empty load = %#v, want non-nil empty slice", tracks)
			}
		})
	}
}

func TestFilesystemEmptyCacheSurvivesRestartUntilRefresh(t *testing.T) {
	root := t.TempDir()
	first, err := NewFilesystem(root).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if first == nil || len(first) != 0 {
		t.Fatalf("initial empty load = %#v, want non-nil empty slice", first)
	}
	writeTestFile(t, root, "new.mp3")

	cached, err := NewFilesystem(root).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if cached == nil || len(cached) != 0 {
		t.Fatalf("cached empty load = %#v, want non-nil empty slice", cached)
	}
	refreshed, err := NewFilesystem(root).Refresh(context.Background())
	if err != nil || len(refreshed) != 1 || refreshed[0].Title != "new.mp3" {
		t.Fatalf("refreshed empty cache = tracks=%#v err=%v", refreshed, err)
	}
}

func TestFilesystemLoadAcceptsRootSymlink(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "track.mp3")
	link := filepath.Join(t.TempDir(), "library-link")
	if err := os.Symlink(root, link); err != nil {
		t.Fatal(err)
	}

	tracks, err := NewFilesystem(link).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(tracks) != 1 || tracks[0].Path != filepath.Join(link, "track.mp3") {
		t.Fatalf("root symlink load = %+v", tracks)
	}
}

func TestFilesystemLoadRejectsInvalidRoots(t *testing.T) {
	testCases := []struct {
		name string
		root func(string) string
		want error
	}{
		{name: "missing", root: func(parent string) string { return filepath.Join(parent, "missing") }, want: fs.ErrNotExist},
		{name: "file", root: func(parent string) string {
			file := filepath.Join(parent, "root-file")
			writeTestFile(t, parent, "root-file")
			return file
		}},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			root := tc.root(t.TempDir())
			tracks, err := NewFilesystem(root).Load(context.Background())
			if err == nil || tracks != nil {
				t.Fatalf("load = tracks=%#v err=%v, want nil tracks and error", tracks, err)
			}
			if tc.want != nil && !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want errors.Is(%v)", err, tc.want)
			}
		})
	}
}

func TestFilesystemLoadReadErrorReturnsNoTracks(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "found.mp3")
	if err := os.Mkdir(filepath.Join(root, "later"), 0o755); err != nil {
		t.Fatal(err)
	}
	sentinel := errors.New("injected directory read error")
	files := testFS{FS: os.DirFS(root)}
	files.readDir = func(name string) ([]fs.DirEntry, error) {
		if name == "later" {
			return nil, sentinel
		}
		return fs.ReadDir(files.FS, name)
	}

	tracks, err := load(context.Background(), root, files)
	if tracks != nil {
		t.Fatalf("tracks = %#v, want nil on walk error", tracks)
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("error = %v, want wrapped sentinel", err)
	}
	if !strings.Contains(err.Error(), filepath.Join(root, "later")) {
		t.Fatalf("error = %v, want offending path", err)
	}
}

func TestFilesystemLoadContextCancellation(t *testing.T) {
	t.Run("already canceled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		tracks, err := NewFilesystem(t.TempDir()).Load(ctx)
		if tracks != nil || !errors.Is(err, context.Canceled) {
			t.Fatalf("load = tracks=%#v err=%v, want context cancellation", tracks, err)
		}
	})

	for _, tc := range []struct {
		name  string
		files []string
	}{
		{name: "during walk", files: []string{"a.mp3", "b.mp3"}},
		{name: "after final entry", files: []string{"only.mp3"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			for _, name := range tc.files {
				writeTestFile(t, root, name)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			files := testFS{FS: os.DirFS(root)}
			files.readDir = func(name string) ([]fs.DirEntry, error) {
				entries, err := fs.ReadDir(files.FS, name)
				if name == "." && err == nil && len(entries) > 0 {
					entries[0] = cancelTypeEntry{DirEntry: entries[0], cancel: cancel}
				}
				return entries, err
			}

			tracks, err := load(ctx, root, files)
			if tracks != nil || !errors.Is(err, context.Canceled) {
				t.Fatalf("load = tracks=%#v err=%v, want context cancellation", tracks, err)
			}
		})
	}
}

func TestFilesystemLoadDeadline(t *testing.T) {
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	tracks, err := NewFilesystem(t.TempDir()).Load(ctx)
	if tracks != nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("load = tracks=%#v err=%v, want deadline exceeded", tracks, err)
	}
}

func TestFilesystemLoadSkipsFIFO(t *testing.T) {
	root := t.TempDir()
	if err := syscall.Mkfifo(filepath.Join(root, "pipe.mp3"), 0o600); err != nil {
		t.Fatal(err)
	}
	tracks, err := NewFilesystem(root).Load(context.Background())
	if err != nil || len(tracks) != 0 {
		t.Fatalf("FIFO load = tracks=%+v err=%v, want empty library", tracks, err)
	}
}

func TestFilesystemLoadUnreadableDirectory(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "a.mp3")
	blocked := filepath.Join(root, "blocked")
	if err := os.Mkdir(blocked, 0o700); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, blocked, "hidden.mp3")
	t.Cleanup(func() {
		if err := os.Chmod(blocked, 0o700); err != nil {
			t.Errorf("restore directory permissions: %v", err)
		}
	})
	if err := os.Chmod(blocked, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := os.ReadDir(blocked); err == nil {
		t.Skip("directory is readable despite its permissions (for example, running as root)")
	} else if !errors.Is(err, fs.ErrPermission) {
		t.Fatalf("read directory: %v", err)
	}

	tracks, err := NewFilesystem(root).Load(context.Background())
	if tracks != nil || !errors.Is(err, fs.ErrPermission) {
		t.Fatalf("load = tracks=%+v err=%v, want nil tracks and permission error", tracks, err)
	}
	if !strings.Contains(err.Error(), blocked) {
		t.Fatalf("error = %v, want unreadable directory path", err)
	}
}

func writeTestFile(t *testing.T, root, name string) {
	t.Helper()
	data := []byte("test audio placeholder")
	if isSupportedTestExtension(filepath.Ext(name)) {
		data = readTestFixture(t, strings.ToLower(filepath.Ext(name)))
	}
	writeTestBytes(t, root, name, data)
}

func writeTestBytes(t *testing.T, root, name string, data []byte) {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func readTestMP3(t *testing.T) []byte {
	return readTestFixture(t, ".mp3")
}

func readTestFixture(t *testing.T, extension string) []byte {
	return readFilesystemFixture(t, "valid"+extension)
}

func readFilesystemFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Fatalf("testdata/%s is empty", name)
	}
	return data
}

func supportedTestExtensions() []string {
	return []string{".mp3", ".flac", ".wav", ".ogg", ".m4a", ".aac"}
}

func isSupportedTestExtension(extension string) bool {
	extension = strings.ToLower(extension)
	for _, supported := range supportedTestExtensions() {
		if extension == supported {
			return true
		}
	}
	return false
}

func trackTitles(tracks []music.Track) []string {
	titles := make([]string, len(tracks))
	for i, track := range tracks {
		titles[i] = track.Title
	}
	return titles
}

// testFS overrides directory reads for deterministic error and cancellation tests.
type testFS struct {
	fs.FS
	readDir func(string) ([]fs.DirEntry, error)
}

func (f testFS) ReadDir(name string) ([]fs.DirEntry, error) {
	return f.readDir(name)
}

type cancelTypeEntry struct {
	fs.DirEntry
	cancel context.CancelFunc
}

func (e cancelTypeEntry) Type() fs.FileMode {
	e.cancel()
	return e.DirEntry.Type()
}
