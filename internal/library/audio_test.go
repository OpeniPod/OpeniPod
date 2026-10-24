package library

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestAudioRecognitionUsesContentInsteadOfExtension(t *testing.T) {
	extensions := []string{".mp3", ".flac", ".wav", ".ogg", ".m4a", ".aac"}
	for _, extension := range extensions {
		t.Run(extension, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("testdata", "valid"+extension))
			if err != nil {
				t.Fatal(err)
			}
			for _, filenameExtension := range extensions {
				root := t.TempDir()
				writeTestBytes(t, root, "different-extension"+filenameExtension, data)
				tracks, err := NewFilesystem(root).Load(context.Background())
				if err != nil || len(tracks) != 1 {
					t.Fatalf("load %s content as %s = tracks=%+v err=%v", extension, filenameExtension, tracks, err)
				}
			}
		})
	}
}
