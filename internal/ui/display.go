package ui

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"strings"
	"time"

	"github.com/OpeniPod/OpeniPod/internal/app"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

const (
	ScreenWidth  = 240
	ScreenHeight = 320

	margin          = 16
	contentRight    = ScreenWidth - margin
	headerRuleY     = 44
	contentBottom   = 219
	errorTop        = 220
	errorBottom     = 250
	footerRuleY     = 254
	transportTop    = 254
	transportBottom = 284
	volumeTop       = 288
	volumeBottom    = 318
)

var (
	colorBackground = color.RGBA{R: 10, G: 18, B: 32, A: 255}
	colorPanel      = color.RGBA{R: 22, G: 34, B: 53, A: 255}
	colorSelected   = color.RGBA{R: 31, G: 62, B: 83, A: 255}
	colorText       = color.RGBA{R: 239, G: 245, B: 250, A: 255}
	colorMuted      = color.RGBA{R: 149, G: 167, B: 186, A: 255}
	colorAccent     = color.RGBA{R: 79, G: 209, B: 197, A: 255}
	colorError      = color.RGBA{R: 255, G: 132, B: 132, A: 255}
	colorErrorPanel = color.RGBA{R: 59, G: 35, B: 45, A: 255}
	colorProgress   = color.RGBA{R: 51, G: 69, B: 88, A: 255}
	colorPaused     = color.RGBA{R: 255, G: 201, B: 87, A: 255}
)

type Display struct {
	titleFace font.Face
	bodyFace  font.Face
	smallFace font.Face
}

// Control describes an actionable region in the 240x320 display coordinate
// system. Desktop adapters can scale Bounds with the rendered image and send
// Event when a pointer press lands inside it.
type Control struct {
	Bounds image.Rectangle
	Event  app.Event
}

type itemRegion struct {
	Bounds image.Rectangle
	Index  int
}

// screenLayout owns the geometry shared by rendering and pointer hit testing.
// Keeping the row rectangles here prevents a visual row and its hit region from
// drifting apart as screens are reflowed.
type screenLayout struct {
	homeRows      []itemRegion
	settingsRows  []itemRegion
	trackRows     []itemRegion
	trackPrevious image.Rectangle
	trackNext     image.Rectangle
	canPageUp     bool
	canPageDown   bool

	back       image.Rectangle
	previous   image.Rectangle
	playPause  image.Rectangle
	next       image.Rectangle
	volumeDown image.Rectangle
	volumeUp   image.Rectangle
}

const (
	trackFirstY    = 53
	trackRowStep   = 35
	trackRowHeight = 33
	trackMaxRows   = 4
)

func layoutFor(state app.AppState) screenLayout {
	layout := screenLayout{
		homeRows: []itemRegion{
			{Bounds: image.Rect(margin, 57, contentRight, 91), Index: 0},
			{Bounds: image.Rect(margin, 96, contentRight, 130), Index: 1},
			{Bounds: image.Rect(margin, 135, contentRight, 169), Index: 2},
		},
		back:       image.Rect(margin, transportTop, 61, transportBottom),
		previous:   image.Rect(63, transportTop, 110, transportBottom),
		playPause:  image.Rect(112, transportTop, 166, transportBottom),
		next:       image.Rect(168, transportTop, contentRight, transportBottom),
		volumeDown: image.Rect(margin, volumeTop, 53, volumeBottom),
		volumeUp:   image.Rect(187, volumeTop, contentRight, volumeBottom),
	}
	for i := 0; i <= app.SettingsShuffle; i++ {
		y := 53 + i*31
		layout.settingsRows = append(layout.settingsRows, itemRegion{
			Bounds: image.Rect(margin, y, contentRight, y+29), Index: i,
		})
	}

	tracks := state.Library.Tracks
	if len(tracks) == 0 {
		return layout
	}
	selected := clamp(state.Navigation.TrackIndex, 0, len(tracks)-1)
	start := selected / trackMaxRows * trackMaxRows
	end := min(start+trackMaxRows, len(tracks))
	for i := start; i < end; i++ {
		row := i - start
		layout.trackRows = append(layout.trackRows, itemRegion{
			Bounds: image.Rect(margin, trackFirstY+row*trackRowStep,
				contentRight, trackFirstY+row*trackRowStep+trackRowHeight),
			Index: i,
		})
	}
	layout.trackPrevious = image.Rect(margin, 194, 82, contentBottom)
	layout.trackNext = image.Rect(158, 194, contentRight, contentBottom)
	layout.canPageUp = start > 0
	layout.canPageDown = end < len(tracks)
	return layout
}

// Controls returns the actionable regions visible for state. Regions that are
// not useful at the current boundary (such as paging beyond the library) are
// intentionally omitted.
func Controls(state app.AppState) []Control {
	layout := layoutFor(state)
	controls := make([]Control, 0, 12)
	appendItems := func(rows []itemRegion, screen app.Screen) {
		for _, row := range rows {
			controls = append(controls, Control{
				Bounds: row.Bounds,
				Event:  app.ItemActivated{Screen: screen, Index: row.Index},
			})
		}
	}
	switch state.Screen {
	case app.ScreenHome:
		appendItems(layout.homeRows, app.ScreenHome)
	case app.ScreenTracks:
		appendItems(layout.trackRows, app.ScreenTracks)
		if layout.canPageUp {
			controls = append(controls, Control{Bounds: layout.trackPrevious,
				Event: app.Scroll{Delta: -trackMaxRows}})
		}
		if layout.canPageDown {
			controls = append(controls, Control{Bounds: layout.trackNext,
				Event: app.Scroll{Delta: trackMaxRows}})
		}
	case app.ScreenSettings:
		appendItems(layout.settingsRows, app.ScreenSettings)
	}

	controls = append(controls,
		Control{Bounds: layout.back, Event: app.BackPressed{}},
		Control{Bounds: layout.previous, Event: app.PreviousPressed{}},
		Control{Bounds: layout.playPause, Event: app.PlayPausePressed{}},
		Control{Bounds: layout.next, Event: app.NextPressed{}},
		Control{Bounds: layout.volumeDown, Event: app.VolumeChanged{Delta: -10}},
		Control{Bounds: layout.volumeUp, Event: app.VolumeChanged{Delta: 10}},
	)
	return controls
}

func NewDisplay() (*Display, error) {
	parsed, err := opentype.Parse(goregular.TTF)
	if err != nil {
		return nil, fmt.Errorf("parse embedded display font: %w", err)
	}

	newFace := func(size float64) (font.Face, error) {
		face, err := opentype.NewFace(parsed, &opentype.FaceOptions{
			Size:    size,
			DPI:     72,
			Hinting: font.HintingFull,
		})
		if err != nil {
			return nil, fmt.Errorf("create display font face at %g points: %w", size, err)
		}
		return face, nil
	}

	titleFace, err := newFace(19)
	if err != nil {
		return nil, err
	}
	bodyFace, err := newFace(13)
	if err != nil {
		titleFace.Close()
		return nil, err
	}
	smallFace, err := newFace(10)
	if err != nil {
		titleFace.Close()
		bodyFace.Close()
		return nil, err
	}

	return &Display{titleFace: titleFace, bodyFace: bodyFace, smallFace: smallFace}, nil
}

func (d *Display) Close() error {
	var closeErrs []error
	for _, face := range []font.Face{d.titleFace, d.bodyFace, d.smallFace} {
		if err := face.Close(); err != nil {
			closeErrs = append(closeErrs, err)
		}
	}
	return errors.Join(closeErrs...)
}

func (d *Display) Render(state app.AppState) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, ScreenWidth, ScreenHeight))
	d.fillRect(img, img.Bounds(), colorBackground)
	layout := layoutFor(state)

	d.drawText(img, d.titleFace, screenTitle(state.Screen), margin, 30, colorText)
	d.drawText(img, d.smallFace, "OPENIPOD", contentRight-57, 27, colorAccent)
	d.fillRect(img, image.Rect(margin, headerRuleY, contentRight, headerRuleY+1), colorPanel)

	switch state.Screen {
	case app.ScreenHome:
		d.renderHome(img, state, layout)
	case app.ScreenTracks:
		d.renderTracks(img, state, layout)
	case app.ScreenNowPlaying:
		d.renderNowPlaying(img, state)
	case app.ScreenSettings:
		d.renderSettings(img, state, layout)
	}

	if state.Error != "" {
		d.renderError(img, state.Error)
	}
	d.renderGlobalControls(img, state, layout)
	return img
}

func (d *Display) renderHome(img *image.RGBA, state app.AppState, layout screenLayout) {
	for i, label := range []string{"Tracks", "Now Playing", "Settings"} {
		row := layout.homeRows[i].Bounds
		d.drawClickableRow(img, row, i == clamp(state.Navigation.HomeIndex, 0, 2))
		d.drawText(img, d.bodyFace, label, row.Min.X+12, row.Min.Y+23, colorText)
	}
	count := len(state.Library.Tracks)
	label := "tracks"
	if count == 1 {
		label = "track"
	}
	d.drawText(img, d.smallFace, fmt.Sprintf("%d %s in library", count, label), margin, 209, colorMuted)
}

func (d *Display) renderTracks(img *image.RGBA, state app.AppState, layout screenLayout) {
	tracks := state.Library.Tracks
	if len(tracks) == 0 {
		d.drawText(img, d.bodyFace, "No tracks in library", margin, 92, colorText)
		d.drawWrapped(img, "Add music, then update the library.", d.smallFace,
			margin, 115, contentRight-margin, 2, colorMuted, 14)
		return
	}

	const titleWidth = contentRight - margin - 20
	selected := clamp(state.Navigation.TrackIndex, 0, len(tracks)-1)
	for _, region := range layout.trackRows {
		i := region.Index
		row := region.Bounds
		d.drawClickableRow(img, row, i == selected)
		d.drawText(img, d.bodyFace,
			d.truncate(tracks[i].Title, titleWidth, d.bodyFace),
			row.Min.X+10, row.Min.Y+16, colorText)
		artist := tracks[i].Artist
		if artist == "" {
			artist = "Unknown artist"
		}
		d.drawText(img, d.smallFace,
			d.truncate(artist, titleWidth, d.smallFace),
			row.Min.X+10, row.Min.Y+29, colorMuted)
	}

	// The pager shares its rectangles with Controls. Disabled buttons remain
	// visible as feedback, but are omitted from Controls at the boundaries.
	if layout.canPageUp {
		d.drawButton(img, layout.trackPrevious, "UP", colorText)
	} else {
		d.drawButton(img, layout.trackPrevious, "UP", colorMuted)
	}
	if layout.canPageDown {
		d.drawButton(img, layout.trackNext, "DOWN", colorText)
	} else {
		d.drawButton(img, layout.trackNext, "DOWN", colorMuted)
	}
	d.drawText(img, d.smallFace, fmt.Sprintf("%d / %d", selected+1, len(tracks)),
		97, 210, colorMuted)
}

func (d *Display) renderNowPlaying(img *image.RGBA, state app.AppState) {
	if !state.Player.HasTrack {
		d.drawText(img, d.bodyFace, "Select a track to start", margin, 91, colorText)
		d.drawText(img, d.smallFace, "Choose one from Tracks.", margin, 113, colorMuted)
		return
	}

	track := state.Player.Track
	titleLines := d.drawWrapped(img, track.Title, d.bodyFace, margin, 69,
		contentRight-margin, 2, colorText, 18)
	artist := track.Artist
	if artist == "" {
		artist = "Unknown artist"
	}
	artistBaseline := 76 + max(1, titleLines)*20
	d.drawWrapped(img, artist, d.smallFace, margin, artistBaseline,
		contentRight-margin, 1, colorMuted, 13)

	status, statusColor := playbackStatus(state.Player.Status)
	statusRect := image.Rect(margin, 137, margin+97, 160)
	d.fillRect(img, statusRect, colorPanel)
	d.drawText(img, d.smallFace, status, statusRect.Min.X+8, 152, statusColor)

	bar := image.Rect(margin, 190, contentRight, 198)
	d.fillRect(img, bar, colorProgress)
	progress := 0.0
	if track.Duration > 0 {
		progress = float64(state.Player.Position) / float64(track.Duration)
	}
	progress = max(0, min(progress, 1))
	if filled := int(float64(bar.Dx()) * progress); filled > 0 {
		d.fillRect(img, image.Rect(bar.Min.X, bar.Min.Y, bar.Min.X+filled, bar.Max.Y), colorAccent)
	}
	d.drawText(img, d.smallFace, displayClock(state.Player.Position), bar.Min.X, 215, colorText)
	d.drawText(img, d.smallFace, displayClock(track.Duration), bar.Max.X-31, 215, colorMuted)
}

func (d *Display) renderSettings(img *image.RGBA, state app.AppState, layout screenLayout) {
	selected := clamp(state.Navigation.SettingsIndex, 0, len(layout.settingsRows)-1)
	shuffle := "Off"
	if state.Settings.Shuffle {
		shuffle = "On"
	}
	labels := []string{
		"Update Library",
		"Equalizer: " + state.Settings.Equalizer.String(),
		fmt.Sprintf("Volume: %d%%", state.Settings.Volume),
		"Repeat: " + state.Settings.Repeat.String(),
		"Shuffle: " + shuffle,
	}
	for i, region := range layout.settingsRows {
		d.drawClickableRow(img, region.Bounds, i == selected)
		d.drawText(img, d.bodyFace, labels[i], region.Bounds.Min.X+11, region.Bounds.Min.Y+20, colorText)
	}
	hints := []string{
		"Scan music folder for changes",
		"Cycle sound preset",
		"Enter: +10; use +/- to adjust",
		"Cycle Off / All / One",
		"Toggle shuffled playback",
	}
	d.drawText(img, d.smallFace, hints[selected], margin, 217, colorMuted)
}

func (d *Display) renderError(img *image.RGBA, message string) {
	box := image.Rect(margin, errorTop, contentRight, errorBottom)
	d.fillRect(img, box, colorErrorPanel)
	d.drawText(img, d.smallFace, "ERROR", box.Min.X+8, box.Min.Y+9, colorError)
	d.drawWrapped(img, message, d.smallFace, box.Min.X+8, box.Min.Y+18,
		box.Dx()-16, 2, colorText, 10)
}

func (d *Display) renderGlobalControls(img *image.RGBA, state app.AppState, layout screenLayout) {
	for _, button := range []struct {
		bounds image.Rectangle
		label  string
	}{
		{layout.back, "BACK"},
		{layout.previous, "PREV"},
		{layout.playPause, playbackButtonLabel(state.Player.Status)},
		{layout.next, "NEXT"},
	} {
		d.drawButton(img, button.bounds, button.label, colorText)
	}

	d.drawButton(img, layout.volumeDown, "-", colorText)
	d.drawButton(img, layout.volumeUp, "+", colorText)
	volume := clamp(state.Settings.Volume, 0, 100)
	d.drawText(img, d.smallFace, "VOLUME", 58, volumeTop+15, colorMuted)
	volumeBar := image.Rect(106, volumeTop+10, 157, volumeTop+17)
	d.fillRect(img, volumeBar, colorProgress)
	if filled := volumeBar.Dx() * volume / 100; filled > 0 {
		d.fillRect(img, image.Rect(volumeBar.Min.X, volumeBar.Min.Y,
			volumeBar.Min.X+filled, volumeBar.Max.Y), colorAccent)
	}
	d.drawText(img, d.smallFace, fmt.Sprintf("%d%%", volume), 162, volumeTop+15, colorText)
}

func playbackButtonLabel(status app.PlaybackStatus) string {
	if status == app.StatusPlaying {
		return "PAUSE"
	}
	return "PLAY"
}

func (d *Display) drawClickableRow(img *image.RGBA, row image.Rectangle, selected bool) {
	d.fillRect(img, row, colorPanel)
	if selected {
		d.drawSelection(img, row)
	}
}

func (d *Display) drawButton(img *image.RGBA, bounds image.Rectangle, label string, c color.Color) {
	d.fillRect(img, bounds, colorPanel)
	width := measure(d.smallFace, label)
	d.drawText(img, d.smallFace, label,
		bounds.Min.X+(bounds.Dx()-width)/2, bounds.Min.Y+19, c)
}

func (d *Display) drawSelection(img *image.RGBA, row image.Rectangle) {
	d.fillRect(img, row, colorSelected)
	d.fillRect(img, image.Rect(row.Min.X, row.Min.Y, row.Min.X+3, row.Max.Y), colorAccent)
}

func (d *Display) drawText(img *image.RGBA, face font.Face, text string, x, baseline int, c color.Color) {
	drawer := font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(c),
		Face: face,
		Dot:  fixed.P(x, baseline),
	}
	drawer.DrawString(text)
}

func (d *Display) fillRect(img *image.RGBA, rect image.Rectangle, c color.Color) {
	draw.Draw(img, rect, &image.Uniform{C: c}, image.Point{}, draw.Src)
}

func (d *Display) drawWrapped(
	img *image.RGBA,
	text string,
	face font.Face,
	x, baseline, maxWidth, maxLines int,
	c color.Color,
	lineHeight int,
) int {
	lines := wrapText(text, face, maxWidth)
	if len(lines) > maxLines {
		lines = lines[:maxLines]
		lines[maxLines-1] = d.ellipsize(lines[maxLines-1], maxWidth, face)
	}
	for i, line := range lines {
		d.drawText(img, face, line, x, baseline+i*lineHeight, c)
	}
	return len(lines)
}

func (d *Display) ellipsize(text string, maxWidth int, face font.Face) string {
	if maxWidth <= 0 {
		return ""
	}
	const ellipsis = "..."
	if measure(face, ellipsis) > maxWidth {
		var fit strings.Builder
		for _, r := range text {
			candidate := fit.String() + string(r)
			if measure(face, candidate) > maxWidth {
				break
			}
			fit.WriteRune(r)
		}
		return fit.String()
	}
	runes := []rune(text)
	for len(runes) > 0 && measure(face, string(runes)+ellipsis) > maxWidth {
		runes = runes[:len(runes)-1]
	}
	return string(runes) + ellipsis
}

func (d *Display) truncate(text string, maxWidth int, face font.Face) string {
	if measure(face, text) <= maxWidth {
		return text
	}
	return d.ellipsize(text, maxWidth, face)
}

func wrapText(text string, face font.Face, maxWidth int) []string {
	if maxWidth <= 0 {
		return []string{""}
	}
	words := strings.Fields(text)
	if len(words) == 0 {
		return []string{""}
	}

	var lines []string
	line := ""
	for _, word := range words {
		if measure(face, word) > maxWidth {
			if line != "" {
				lines = append(lines, line)
				line = ""
			}
			chunks := splitWord(word, face, maxWidth)
			lines = append(lines, chunks[:len(chunks)-1]...)
			word = chunks[len(chunks)-1]
		}
		candidate := word
		if line != "" {
			candidate = line + " " + word
		}
		if measure(face, candidate) > maxWidth && line != "" {
			lines = append(lines, line)
			line = word
		} else {
			line = candidate
		}
	}
	if line != "" {
		lines = append(lines, line)
	}
	return lines
}

func splitWord(word string, face font.Face, maxWidth int) []string {
	var chunks []string
	var chunk strings.Builder
	for _, r := range word {
		candidate := chunk.String() + string(r)
		if chunk.Len() > 0 && measure(face, candidate) > maxWidth {
			chunks = append(chunks, chunk.String())
			chunk.Reset()
		}
		chunk.WriteRune(r)
	}
	if chunk.Len() > 0 {
		chunks = append(chunks, chunk.String())
	}
	return chunks
}

func measure(face font.Face, text string) int {
	return font.MeasureString(face, text).Ceil()
}

func screenTitle(screen app.Screen) string {
	switch screen {
	case app.ScreenTracks:
		return "Tracks"
	case app.ScreenNowPlaying:
		return "Now Playing"
	case app.ScreenSettings:
		return "Settings"
	default:
		return "Music"
	}
}

func playbackStatus(status app.PlaybackStatus) (string, color.Color) {
	switch status {
	case app.StatusStarting:
		return "STARTING", colorAccent
	case app.StatusPlaying:
		return "PLAYING", colorAccent
	case app.StatusPaused:
		return "PAUSED", colorPaused
	default:
		return "STOPPED", colorMuted
	}
}

func displayClock(value time.Duration) string {
	seconds := int(value / time.Second)
	if seconds < 0 {
		seconds = 0
	}
	return fmt.Sprintf("%02d:%02d", seconds/60, seconds%60)
}

func clamp(value, low, high int) int {
	return max(low, min(value, high))
}
