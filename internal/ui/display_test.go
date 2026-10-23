package ui

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
	"time"

	"github.com/OpeniPod/OpeniPod/internal/app"
	"github.com/OpeniPod/OpeniPod/internal/music"
	"golang.org/x/image/font"
)

func newTestDisplay(t *testing.T) *Display {
	t.Helper()
	display, err := NewDisplay()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := display.Close(); err != nil {
			t.Errorf("close display: %v", err)
		}
	})
	return display
}

func TestRenderAllScreens(t *testing.T) {
	display := newTestDisplay(t)
	track := music.Track{
		ID:       "russian-track",
		Title:    "Ночные огни с очень длинным названием для проверки переноса",
		Artist:   "Исполнитель",
		Duration: 4 * time.Minute,
	}
	states := []app.AppState{
		{Screen: app.ScreenHome, Navigation: app.NavigationState{HomeIndex: 2}},
		{Screen: app.ScreenTracks, Library: app.LibraryState{Tracks: []music.Track{track}}},
		{
			Screen: app.ScreenNowPlaying,
			Player: app.PlayerState{
				Track: track, HasTrack: true, Status: app.StatusPlaying, Position: time.Minute,
			},
			Settings: app.SettingsState{Volume: 70},
		},
		{
			Screen:     app.ScreenSettings,
			Navigation: app.NavigationState{SettingsIndex: 0},
			Settings:   app.SettingsState{Volume: 45},
		},
		{Screen: app.ScreenTracks},
		{Screen: app.ScreenSettings, Error: "Ошибка обновления библиотеки"},
		{Screen: app.ScreenNowPlaying, Error: "Ошибка воспроизведения"},
	}

	for i, state := range states {
		img := display.Render(state)
		if got := img.Bounds().Size(); got != image.Pt(ScreenWidth, ScreenHeight) {
			t.Errorf("state %d image size = %v, want %dx%d", i, got, ScreenWidth, ScreenHeight)
		}
		if countPixels(img, colorBackground) == ScreenWidth*ScreenHeight {
			t.Errorf("state %d rendered a blank image", i)
		}
	}
}

func TestRenderDoesNotMutateState(t *testing.T) {
	display := newTestDisplay(t)
	state := app.AppState{
		Screen: app.ScreenTracks,
		Library: app.LibraryState{Tracks: []music.Track{
			{ID: "one", Title: "Один"},
		}},
	}
	before := state.Library.Tracks[0]
	display.Render(state)
	if state.Library.Tracks[0] != before {
		t.Fatalf("render modified track: got %+v, want %+v", state.Library.Tracks[0], before)
	}
}

func TestRenderPNG(t *testing.T) {
	display := newTestDisplay(t)
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, display.Render(app.AppState{Screen: app.ScreenHome})); err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(&encoded)
	if err != nil {
		t.Fatal(err)
	}
	if got := img.Bounds().Size(); got != image.Pt(ScreenWidth, ScreenHeight) {
		t.Fatalf("decoded image size = %v, want %dx%d", got, ScreenWidth, ScreenHeight)
	}
}

func TestDisplayFontHasCyrillicGlyphs(t *testing.T) {
	display := newTestDisplay(t)
	advance, ok := display.bodyFace.GlyphAdvance('Я')
	if !ok || advance <= 0 {
		t.Fatal("embedded display font does not contain a visible Cyrillic glyph")
	}
}

func TestSettingsSelectionFollowsSelectedItem(t *testing.T) {
	display := newTestDisplay(t)
	for _, test := range []struct {
		name     string
		selected int
	}{
		{name: "update library", selected: 0},
		{name: "equalizer", selected: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			img := display.Render(app.AppState{
				Screen:     app.ScreenSettings,
				Navigation: app.NavigationState{SettingsIndex: test.selected},
			})
			firstWant, secondWant := colorBackground, colorAccent
			if test.selected == 0 {
				firstWant, secondWant = colorAccent, colorBackground
			}
			if got := img.RGBAAt(margin+1, 124); got != firstWant {
				t.Errorf("Update Library marker = %v, want %v", got, firstWant)
			}
			if got := img.RGBAAt(margin+1, 174); got != secondWant {
				t.Errorf("Equalizer marker = %v, want %v", got, secondWant)
			}
		})
	}
}

func TestSettingsRendersEqualizerPreset(t *testing.T) {
	display := newTestDisplay(t)
	settingsState := func(preset app.EqualizerPreset) app.AppState {
		return app.AppState{
			Screen:     app.ScreenSettings,
			Navigation: app.NavigationState{SettingsIndex: 1},
			Settings:   app.SettingsState{Equalizer: preset},
		}
	}
	region := image.Rect(margin+11, 187, contentRight, 207)
	off := display.Render(settingsState(app.EqualizerOff))
	bassBoost := display.Render(settingsState(app.EqualizerBassBoost))
	vocal := display.Render(settingsState(app.EqualizerVocal))
	if imagesEqualInRect(off, bassBoost, region) {
		t.Fatal("Equalizer row did not change when preset changed from Off to Bass boost")
	}
	if imagesEqualInRect(bassBoost, vocal, region) {
		t.Fatal("Equalizer row did not change when preset changed from Bass boost to Vocal")
	}
}

func TestLongCyrillicWordsWrapWithinWidth(t *testing.T) {
	display := newTestDisplay(t)
	const maxWidth = 70
	lines := wrapText(strings.Repeat("д", 30), display.bodyFace, maxWidth)
	if len(lines) < 2 {
		t.Fatalf("wrapped lines = %v, want multiple lines", lines)
	}
	for _, line := range lines {
		if width := font.MeasureString(display.bodyFace, line).Ceil(); width > maxWidth {
			t.Errorf("line width = %d, exceeds %d: %q", width, maxWidth, line)
		}
	}
}

func TestEllipsizeHandlesWidthSmallerThanEllipsis(t *testing.T) {
	display := newTestDisplay(t)
	const maxWidth = 1
	got := display.ellipsize("длинный текст", maxWidth, display.smallFace)
	if width := font.MeasureString(display.smallFace, got).Ceil(); width > maxWidth {
		t.Fatalf("ellipsized width = %d, exceeds %d", width, maxWidth)
	}
}

func TestLongErrorKeepsFooterVisible(t *testing.T) {
	display := newTestDisplay(t)
	img := display.Render(app.AppState{
		Screen: app.ScreenSettings,
		Error:  "Не удалось обновить библиотеку: очень длинная ошибка, которая не должна перекрыть футер",
	})
	for y := footerRuleY - 4; y < footerRuleY; y++ {
		for x := margin; x < contentRight; x++ {
			if got := img.RGBAAt(x, y); got != colorBackground {
				t.Fatalf("pixel (%d, %d) = %v, want gap before footer", x, y, got)
			}
		}
	}
	if countPixelsInRect(img, image.Rect(margin, errorTop, contentRight, footerRuleY-5), colorErrorPanel) == 0 {
		t.Fatal("error message panel was not rendered")
	}
}

func countPixels(img *image.RGBA, target color.Color) int {
	return countPixelsInRect(img, img.Bounds(), target)
}

func countPixelsInRect(img *image.RGBA, rect image.Rectangle, target color.Color) int {
	count := 0
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		for x := rect.Min.X; x < rect.Max.X; x++ {
			if img.At(x, y) == target {
				count++
			}
		}
	}
	return count
}

func imagesEqualInRect(first, second *image.RGBA, rect image.Rectangle) bool {
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		for x := rect.Min.X; x < rect.Max.X; x++ {
			if first.RGBAAt(x, y) != second.RGBAAt(x, y) {
				return false
			}
		}
	}
	return true
}
