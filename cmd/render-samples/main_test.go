package main

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/OpeniPod/OpeniPod/internal/ui"
)

func TestRenderSamplesCreatesAllScreens(t *testing.T) {
	outputDir := t.TempDir()
	if err := renderSamples(outputDir); err != nil {
		t.Fatal(err)
	}

	names := []string{
		"home.png",
		"tracks.png",
		"now-playing.png",
		"settings.png",
		"empty-library.png",
		"library-error.png",
		"playback-error.png",
	}
	for _, name := range names {
		file, err := os.Open(filepath.Join(outputDir, name))
		if err != nil {
			t.Fatalf("open %s: %v", name, err)
		}
		img, decodeErr := png.Decode(file)
		closeErr := file.Close()
		if decodeErr != nil {
			t.Fatalf("decode %s: %v", name, decodeErr)
		}
		if closeErr != nil {
			t.Fatalf("close %s: %v", name, closeErr)
		}
		if got := img.Bounds().Size(); got != image.Pt(ui.ScreenWidth, ui.ScreenHeight) {
			t.Errorf("%s size = %v, want %dx%d", name, got, ui.ScreenWidth, ui.ScreenHeight)
		}
	}
}
