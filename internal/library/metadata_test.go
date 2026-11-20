package library

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestFilesystemReadsSongTags(t *testing.T) {
	fixtures := []string{"tagged.mp3", "tagged.flac", "tagged.wav", "tagged.ogg", "tagged.m4a", "tagged.aac", "tagged-opus.ogg", "tagged-flac.ogg"}
	for _, fixture := range fixtures {
		t.Run(fixture, func(t *testing.T) {
			root := t.TempDir()
			// Deliberately use the same extension for different contents.
			writeTestBytes(t, root, "track01.mp3", readFilesystemFixture(t, fixture))
			tracks, err := NewFilesystem(root).Load(context.Background())
			if err != nil || len(tracks) != 1 {
				t.Fatalf("load = %+v, %v; want one tagged track", tracks, err)
			}
			track := tracks[0]
			if track.Title != "Digital Love" || track.Artist != "Daft Punk" || track.Album != "Discovery" || track.TrackNum != 3 {
				t.Fatalf("song tags = %+v", track)
			}
			if track.ID == "" || track.Path != filepath.Join(root, "track01.mp3") {
				t.Fatalf("track identity = %+v", track)
			}
			// An ordinary load restores every field, even when the file is gone.
			if err := os.Remove(track.Path); err != nil {
				t.Fatal(err)
			}
			cached, err := NewFilesystem(root).Load(context.Background())
			if err != nil || !reflect.DeepEqual(cached, tracks) {
				t.Fatalf("cached tracks = %+v, %v; want %+v", cached, err, tracks)
			}
		})
	}
}
