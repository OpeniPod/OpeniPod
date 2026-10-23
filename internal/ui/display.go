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

	margin       = 16
	contentRight = ScreenWidth - margin
	headerRuleY  = 44
	errorTop     = 249
	footerRuleY  = 286
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

	d.drawText(img, d.titleFace, screenTitle(state.Screen), margin, 30, colorText)
	d.drawText(img, d.smallFace, "OPENIPOD", contentRight-57, 27, colorAccent)
	d.fillRect(img, image.Rect(margin, headerRuleY, contentRight, headerRuleY+1), colorPanel)

	switch state.Screen {
	case app.ScreenHome:
		d.renderHome(img, state)
	case app.ScreenTracks:
		d.renderTracks(img, state)
	case app.ScreenNowPlaying:
		d.renderNowPlaying(img, state)
	case app.ScreenSettings:
		d.renderSettings(img, state)
	}

	if state.Error != "" {
		d.renderError(img, state.Error)
	}
	d.renderFooter(img)
	return img
}

func (d *Display) renderHome(img *image.RGBA, state app.AppState) {
	for i, label := range []string{"Tracks", "Now Playing", "Settings"} {
		d.drawMenuRow(img, label, 68+i*49, i == state.Navigation.HomeIndex)
	}
	count := len(state.Library.Tracks)
	label := "tracks"
	if count == 1 {
		label = "track"
	}
	d.drawText(img, d.smallFace, fmt.Sprintf("%d %s in library", count, label), margin, 237, colorMuted)
}

func (d *Display) renderTracks(img *image.RGBA, state app.AppState) {
	tracks := state.Library.Tracks
	if len(tracks) == 0 {
		d.drawText(img, d.bodyFace, "No tracks in library", margin, 92, colorText)
		d.drawWrapped(img, "Add music, then update the library.", d.smallFace,
			margin, 115, contentRight-margin, 2, colorMuted, 14)
		return
	}

	const (
		firstY     = 57
		rowHeight  = 45
		maxRows    = 4
		titleWidth = contentRight - margin - 20
	)
	selected := clamp(state.Navigation.TrackIndex, 0, len(tracks)-1)
	start := max(0, min(selected-maxRows+1, len(tracks)-maxRows))
	end := min(start+maxRows, len(tracks))
	for i := start; i < end; i++ {
		y := firstY + (i-start)*rowHeight
		row := image.Rect(margin, y, contentRight, y+rowHeight-4)
		if i == selected {
			d.drawSelection(img, row)
		}
		d.drawText(img, d.bodyFace,
			d.truncate(tracks[i].Title, titleWidth, d.bodyFace),
			margin+10, y+17, colorText)
		artist := tracks[i].Artist
		if artist == "" {
			artist = "Unknown artist"
		}
		d.drawText(img, d.smallFace,
			d.truncate(artist, titleWidth, d.smallFace),
			margin+10, y+33, colorMuted)
	}

	d.drawText(img, d.smallFace, fmt.Sprintf("%d / %d", selected+1, len(tracks)),
		contentRight-37, 244, colorMuted)
}

func (d *Display) renderNowPlaying(img *image.RGBA, state app.AppState) {
	if !state.Player.HasTrack {
		d.drawText(img, d.bodyFace, "Select a track to start", margin, 91, colorText)
		d.drawText(img, d.smallFace, "Choose one from Tracks.", margin, 113, colorMuted)
		d.renderVolume(img, state.Settings.Volume, 174)
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
	d.renderVolume(img, state.Settings.Volume, 239)
}

func (d *Display) renderSettings(img *image.RGBA, state app.AppState) {
	d.renderVolume(img, state.Settings.Volume, 86)

	rows := []image.Rectangle{
		image.Rect(margin, 112, contentRight, 154),
		image.Rect(margin, 162, contentRight, 210),
	}
	selected := clamp(state.Navigation.SettingsIndex, 0, len(rows)-1)
	for i, row := range rows {
		if i == selected {
			d.drawSelection(img, row)
		}
	}
	d.drawText(img, d.bodyFace, "Update Library", margin+11, 138, colorText)
	d.drawText(img, d.bodyFace, "Equalizer", margin+11, 184, colorText)
	d.drawText(img, d.smallFace, state.Settings.Equalizer.String(), margin+11, 201, colorAccent)

	hint := "Scan music folder for changes"
	if selected == 1 {
		hint = "Cycle sound preset"
	}
	d.drawText(img, d.smallFace, hint, margin, 233, colorMuted)
}

func (d *Display) renderVolume(img *image.RGBA, volume, baseline int) {
	volume = clamp(volume, 0, 100)
	d.drawText(img, d.smallFace, "VOLUME", margin, baseline, colorMuted)
	bar := image.Rect(margin+52, baseline-9, contentRight-36, baseline-2)
	d.fillRect(img, bar, colorProgress)
	if filled := bar.Dx() * volume / 100; filled > 0 {
		d.fillRect(img, image.Rect(bar.Min.X, bar.Min.Y, bar.Min.X+filled, bar.Max.Y), colorAccent)
	}
	d.drawText(img, d.smallFace, fmt.Sprintf("%d%%", volume), bar.Max.X+7, baseline, colorText)
}

func (d *Display) renderError(img *image.RGBA, message string) {
	box := image.Rect(margin, errorTop, contentRight, footerRuleY-5)
	d.fillRect(img, box, colorErrorPanel)
	d.drawText(img, d.smallFace, "ERROR", box.Min.X+8, box.Min.Y+13, colorError)
	d.drawWrapped(img, message, d.smallFace, box.Min.X+8, box.Min.Y+20,
		box.Dx()-16, 2, colorText, 10)
}

func (d *Display) renderFooter(img *image.RGBA) {
	d.fillRect(img, image.Rect(margin, footerRuleY, contentRight, footerRuleY+1), colorPanel)
	d.drawText(img, d.smallFace, "ENTER SELECT", margin, 307, colorMuted)
	d.drawText(img, d.smallFace, "B BACK", contentRight-43, 307, colorMuted)
}

func (d *Display) drawMenuRow(img *image.RGBA, label string, y int, selected bool) {
	row := image.Rect(margin, y, contentRight, y+37)
	if selected {
		d.drawSelection(img, row)
	}
	d.drawText(img, d.bodyFace, label, margin+12, y+24, colorText)
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
