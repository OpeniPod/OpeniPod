package desktop

import (
	"testing"

	"github.com/OpeniPod/OpeniPod/internal/app"
	"golang.org/x/mobile/event/key"
)

func TestKeyEventMapping(t *testing.T) {
	tests := []struct {
		name string
		code key.Code
		want app.Event
	}{
		{name: "J scrolls down", code: key.CodeJ, want: app.Scroll{Delta: 1}},
		{name: "down arrow scrolls down", code: key.CodeDownArrow, want: app.Scroll{Delta: 1}},
		{name: "K scrolls up", code: key.CodeK, want: app.Scroll{Delta: -1}},
		{name: "up arrow scrolls up", code: key.CodeUpArrow, want: app.Scroll{Delta: -1}},
		{name: "enter selects", code: key.CodeReturnEnter, want: app.SelectPressed{}},
		{name: "keypad enter selects", code: key.CodeKeypadEnter, want: app.SelectPressed{}},
		{name: "B goes back", code: key.CodeB, want: app.BackPressed{}},
		{name: "escape goes back", code: key.CodeEscape, want: app.BackPressed{}},
		{name: "P toggles playback", code: key.CodeP, want: app.PlayPausePressed{}},
		{name: "space toggles playback", code: key.CodeSpacebar, want: app.PlayPausePressed{}},
		{name: "N advances track", code: key.CodeN, want: app.NextPressed{}},
		{name: "right arrow advances track", code: key.CodeRightArrow, want: app.NextPressed{}},
		{name: "H returns to previous track", code: key.CodeH, want: app.PreviousPressed{}},
		{name: "left arrow returns to previous track", code: key.CodeLeftArrow, want: app.PreviousPressed{}},
		{name: "equals increases volume", code: key.CodeEqualSign, want: app.VolumeChanged{Delta: 10}},
		{name: "keypad plus increases volume", code: key.CodeKeypadPlusSign, want: app.VolumeChanged{Delta: 10}},
		{name: "hyphen decreases volume", code: key.CodeHyphenMinus, want: app.VolumeChanged{Delta: -10}},
		{name: "keypad hyphen decreases volume", code: key.CodeKeypadHyphenMinus, want: app.VolumeChanged{Delta: -10}},
		{name: "Q quits", code: key.CodeQ, want: app.QuitRequested{}},
		{name: "Ctrl+C quits", code: key.CodeC, want: app.QuitRequested{}},
	}

	for _, tc := range tests {
		for _, direction := range []key.Direction{key.DirPress, key.DirNone} {
			t.Run(tc.name+"/"+direction.String(), func(t *testing.T) {
				modifiers := key.Modifiers(0)
				if tc.code == key.CodeC {
					modifiers = key.ModControl
				}
				got, ok := keyEvent(key.Event{Code: tc.code, Direction: direction, Modifiers: modifiers})
				if !ok {
					t.Fatal("keyEvent() did not map the key")
				}
				if got != tc.want {
					t.Fatalf("keyEvent() = %#v, want %#v", got, tc.want)
				}
			})
		}
	}
}

func TestKeyEventIgnoresUnsupportedInput(t *testing.T) {
	tests := []struct {
		name  string
		event key.Event
	}{
		{name: "unknown code", event: key.Event{Code: key.CodeUnknown, Direction: key.DirPress}},
		{name: "unmapped code", event: key.Event{Code: key.CodeA, Direction: key.DirPress}},
		{name: "release", event: key.Event{Code: key.CodeJ, Direction: key.DirRelease}},
		{name: "Ctrl+J", event: key.Event{Code: key.CodeJ, Direction: key.DirPress, Modifiers: key.ModControl}},
		{name: "Ctrl+Shift+C", event: key.Event{Code: key.CodeC, Direction: key.DirPress, Modifiers: key.ModControl | key.ModShift}},
		{name: "Alt+P", event: key.Event{Code: key.CodeP, Direction: key.DirPress, Modifiers: key.ModAlt}},
		{name: "Meta+Q", event: key.Event{Code: key.CodeQ, Direction: key.DirPress, Modifiers: key.ModMeta}},
		{name: "Ctrl+Alt+C", event: key.Event{Code: key.CodeC, Direction: key.DirPress, Modifiers: key.ModControl | key.ModAlt}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := keyEvent(tc.event)
			if ok || got != nil {
				t.Fatalf("keyEvent() = (%#v, %t), want (nil, false)", got, ok)
			}
		})
	}
}
