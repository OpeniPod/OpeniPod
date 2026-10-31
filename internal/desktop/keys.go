package desktop

import (
	"github.com/OpeniPod/OpeniPod/internal/app"
	"golang.org/x/mobile/event/key"
)

func keyEvent(event key.Event) (app.Event, bool) {
	if event.Direction != key.DirPress && event.Direction != key.DirNone {
		return nil, false
	}
	if event.Modifiers&(key.ModAlt|key.ModMeta) != 0 {
		return nil, false
	}
	if event.Modifiers&key.ModControl != 0 {
		if event.Modifiers == key.ModControl && event.Code == key.CodeC {
			return app.QuitRequested{}, true
		}
		return nil, false
	}

	switch event.Code {
	case key.CodeJ, key.CodeDownArrow:
		return app.Scroll{Delta: 1}, true
	case key.CodeK, key.CodeUpArrow:
		return app.Scroll{Delta: -1}, true
	case key.CodeReturnEnter, key.CodeKeypadEnter:
		return app.SelectPressed{}, true
	case key.CodeB, key.CodeEscape:
		return app.BackPressed{}, true
	case key.CodeP, key.CodeSpacebar:
		return app.PlayPausePressed{}, true
	case key.CodeN, key.CodeRightArrow:
		return app.NextPressed{}, true
	case key.CodeH, key.CodeLeftArrow:
		return app.PreviousPressed{}, true
	case key.CodeEqualSign, key.CodeKeypadPlusSign:
		return app.VolumeChanged{Delta: 10}, true
	case key.CodeHyphenMinus, key.CodeKeypadHyphenMinus:
		return app.VolumeChanged{Delta: -10}, true
	case key.CodeQ:
		return app.QuitRequested{}, true
	default:
		return nil, false
	}
}
