package ui

import (
	"image"
	"image/color"
	"reflect"
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
			firstWant, secondWant := colorPanel, colorAccent
			if test.selected == 0 {
				firstWant, secondWant = colorAccent, colorPanel
			}
			if got := img.RGBAAt(margin+1, 70); got != firstWant {
				t.Errorf("Update Library marker = %v, want %v", got, firstWant)
			}
			if got := img.RGBAAt(margin+1, 120); got != secondWant {
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
	region := image.Rect(margin+11, 132, contentRight, 153)
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

func TestTrackPagerShowsConsecutivePages(t *testing.T) {
	state := app.AppState{
		Screen:     app.ScreenTracks,
		Navigation: app.NavigationState{TrackIndex: 3},
		Library:    app.LibraryState{Tracks: make([]music.Track, 10)},
	}
	steps := []struct {
		indexes  []int
		up, down bool
		delta    int
	}{
		{[]int{0, 1, 2, 3}, false, true, 4},
		{[]int{4, 5, 6, 7}, true, true, 4},
		{[]int{8, 9}, true, false, -4},
		{[]int{4, 5, 6, 7}, true, true, -4},
		{[]int{0, 1, 2, 3}, false, true, 0},
	}
	for step, want := range steps {
		controls := Controls(state)
		var indexes []int
		for _, control := range controls {
			if activated, ok := control.Event.(app.ItemActivated); ok {
				if activated.Screen != app.ScreenTracks {
					t.Errorf("track control screen = %v, want Tracks", activated.Screen)
				}
				indexes = append(indexes, activated.Index)
			}
		}
		if !reflect.DeepEqual(indexes, want.indexes) {
			t.Errorf("step %d: visible track indexes = %v, want %v", step, indexes, want.indexes)
		}
		up := hasControlEvent(controls, app.Scroll{Delta: -4})
		down := hasControlEvent(controls, app.Scroll{Delta: 4})
		if up != want.up || down != want.down {
			t.Errorf("step %d: pager buttons up/down = %t/%t, want %t/%t", step, up, down, want.up, want.down)
		}
		state, _ = app.Reduce(state, app.Scroll{Delta: want.delta})
	}
}

func TestControlsOmitTrackActivationForEmptyLibrary(t *testing.T) {
	controls := Controls(app.AppState{Screen: app.ScreenTracks})
	for _, control := range controls {
		switch control.Event.(type) {
		case app.ItemActivated, app.Scroll:
			t.Fatalf("empty library exposed track action: %#v", control.Event)
		}
	}
	if len(controls) != 6 {
		t.Fatalf("empty library controls = %d, want six global controls", len(controls))
	}
}

func TestControlsExposeTransportAndVolumeOnEveryScreen(t *testing.T) {
	for _, screen := range []app.Screen{app.ScreenHome, app.ScreenTracks, app.ScreenNowPlaying, app.ScreenSettings} {
		controls := Controls(app.AppState{Screen: screen})
		for _, event := range []app.Event{
			app.BackPressed{}, app.PreviousPressed{}, app.PlayPausePressed{}, app.NextPressed{},
			app.VolumeChanged{Delta: -10}, app.VolumeChanged{Delta: 10},
		} {
			if !hasControlEvent(controls, event) {
				t.Errorf("screen %v missing control event %#v", screen, event)
			}
		}
	}
}

func TestControlsDoNotOverlapErrorPanel(t *testing.T) {
	state := app.AppState{Screen: app.ScreenSettings, Error: "library failed"}
	for _, control := range Controls(state) {
		if control.Bounds.Overlaps(image.Rect(margin, errorTop, contentRight, errorBottom)) {
			t.Fatalf("control %v overlaps error panel", control.Bounds)
		}
	}
	display := newTestDisplay(t)
	img := display.Render(state)
	if got := img.RGBAAt(margin, errorTop); got != colorErrorPanel {
		t.Fatalf("error panel pixel = %v, want %v", got, colorErrorPanel)
	}
}

func TestRenderShowsUnselectedRowsAndPlaybackStatus(t *testing.T) {
	display := newTestDisplay(t)
	state := app.AppState{
		Screen:     app.ScreenTracks,
		Navigation: app.NavigationState{TrackIndex: 1},
		Library:    app.LibraryState{Tracks: []music.Track{{Title: "one"}, {Title: "two"}}},
		Player:     app.PlayerState{Status: app.StatusPlaying, HasTrack: true},
	}
	img := display.Render(state)
	if got := img.RGBAAt(margin+1, trackFirstY); got != colorPanel {
		t.Fatalf("unselected track row pixel = %v, want %v", got, colorPanel)
	}
	if got := img.RGBAAt(margin+1, trackFirstY+trackRowStep); got != colorAccent {
		t.Fatalf("selected track marker = %v, want %v", got, colorAccent)
	}
	if countPixelsInRect(img, image.Rect(112, transportTop, 166, transportBottom), colorPanel) == 0 {
		t.Fatal("playback control was not rendered")
	}
}

func hasControlEvent(controls []Control, want app.Event) bool {
	for _, control := range controls {
		if reflect.DeepEqual(control.Event, want) {
			return true
		}
	}
	return false
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
