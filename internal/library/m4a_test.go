package library

import (
	"context"
	"os"
	"testing"
)

func TestM4ARecognitionAcceptsGenericMP4Brand(t *testing.T) {
	data, err := os.ReadFile("testdata/valid.m4a")
	if err != nil {
		t.Fatal(err)
	}
	if string(data[4:8]) != "ftyp" {
		t.Fatal("fixture has no leading ftyp")
	}
	copy(data[8:12], "isom")
	root := t.TempDir()
	writeTestBytes(t, root, "generic.m4a", data)
	if err := checkAudio(context.Background(), os.DirFS(root), "generic.m4a"); err != nil {
		t.Fatalf("generic MP4 brand with audio track = %v", err)
	}
}
