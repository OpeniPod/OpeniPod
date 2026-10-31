package ui_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/OpeniPod/OpeniPod/internal/app"
	"github.com/OpeniPod/OpeniPod/internal/ui"
)

func TestTerminalRendersSettingsAndEqualizerSelection(t *testing.T) {
	for _, tc := range []struct {
		name          string
		preset        app.EqualizerPreset
		selectedIndex int
		wantSelected  string
		wantUnmarked  string
	}{
		{"off on update library", app.EqualizerOff, 0, "> Update Library", "  Equalizer: Off"},
		{"bass boost on equalizer", app.EqualizerBassBoost, 1, "> Equalizer: Bass boost", "  Update Library"},
		{"vocal on equalizer", app.EqualizerVocal, 1, "> Equalizer: Vocal", "  Update Library"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var output bytes.Buffer
			terminal := ui.NewTerminal(&output)
			state := app.AppState{
				Screen:     app.ScreenSettings,
				Navigation: app.NavigationState{SettingsIndex: tc.selectedIndex},
				Settings:   app.SettingsState{Volume: 37, Equalizer: tc.preset},
			}
			if err := terminal.Render(state); err != nil {
				t.Fatal(err)
			}

			view := output.String()
			for _, want := range []string{
				"Volume: 37%",
				tc.wantSelected,
				tc.wantUnmarked,
				"Enter cycles equalizer presets",
			} {
				if !strings.Contains(view, want) {
					t.Errorf("settings view does not contain %q:\n%s", want, view)
				}
			}
			if strings.Count(view, "> ") != 1 {
				t.Errorf("settings view should mark exactly one item:\n%s", view)
			}
		})
	}
}
