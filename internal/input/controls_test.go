package input

import (
	"reflect"
	"testing"
	"time"

	"github.com/OpeniPod/OpeniPod/internal/app"
)

func TestControlsRotationBothDirections(t *testing.T) {
	base := time.Unix(100, 0)
	tests := []struct {
		name   string
		states []Signals
		want   []app.Event
	}{
		{
			name: "clockwise",
			states: []Signals{
				{A: true, B: true},
				{A: false, B: true},
				{A: false, B: false},
				{A: true, B: false},
				{A: true, B: true},
			},
			want: []app.Event{app.Scroll{Delta: 1}},
		},
		{
			name: "counterclockwise",
			states: []Signals{
				{A: true, B: true},
				{A: true, B: false},
				{A: false, B: false},
				{A: false, B: true},
				{A: true, B: true},
			},
			want: []app.Event{app.Scroll{Delta: -1}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			controls := newTestControls(t, tt.states[0], base, 0)
			var got []app.Event
			for i, state := range tt.states[1:] {
				got = append(got, controls.Update(base.Add(time.Duration(i+1)), state)...)
			}
			assertEvents(t, got, tt.want...)
		})
	}
}

func TestControlsRotationBounceAndReversalCancelPartialProgress(t *testing.T) {
	base := time.Unix(100, 0)
	controls := newTestControls(t, Signals{A: true, B: true}, base, 0)

	// Returning to the 11 boundary and retracing an edge cancel the partial
	// cycle. Only the final complete clockwise cycle is emitted.
	states := []Signals{
		{A: false, B: true}, // 11 -> 01
		{A: true, B: true},  // bounce back to boundary
		{A: false, B: true}, // begin again
		{A: false, B: false},
		{A: false, B: true}, // reversal: cancel progress
		{A: false, B: false},
		{A: true, B: false},
		{A: true, B: true},
	}
	var got []app.Event
	for i, state := range states {
		got = append(got, controls.Update(base.Add(time.Duration(i+1)), state)...)
	}
	assertEvents(t, got, app.Scroll{Delta: 1})
}

func TestControlsRotationIncompleteCycleEmitsOnlyOnBoundary(t *testing.T) {
	base := time.Unix(100, 0)
	controls := newTestControls(t, Signals{A: true, B: true}, base, 0)

	for i, state := range []Signals{
		{A: false, B: true},
		{A: false, B: false},
		{A: true, B: false},
	} {
		assertEvents(t, controls.Update(base.Add(time.Duration(i+1)), state))
	}
	assertEvents(t, controls.Update(base.Add(4), Signals{A: true, B: true}), app.Scroll{Delta: 1})
}

func TestControlsRotationInvalidJumpRequiresResynchronization(t *testing.T) {
	base := time.Unix(100, 0)
	controls := newTestControls(t, Signals{A: true, B: true}, base, 0)

	// 11 -> 00 changes both contacts. Adjacent-looking transitions remain
	// ignored until 11 is observed again, after which a new cycle is needed.
	states := []Signals{
		{A: false, B: false}, // invalid jump, lose synchronization
		{A: true, B: false},
		{A: true, B: true}, // resynchronize, no event
		{A: false, B: true},
		{A: false, B: false},
		{A: true, B: false},
		{A: true, B: true},
	}
	var got []app.Event
	for i, state := range states {
		got = append(got, controls.Update(base.Add(time.Duration(i+1)), state)...)
	}
	assertEvents(t, got, app.Scroll{Delta: 1})
}

func TestControlsRotationStartupMidTurnWaitsForSynchronization(t *testing.T) {
	base := time.Unix(100, 0)
	controls := newTestControls(t, Signals{A: false, B: true}, base, 0)

	// A partial turn present at startup cannot establish direction. Reaching
	// 11 synchronizes the decoder; only a subsequent complete cycle counts.
	states := []Signals{
		{A: false, B: false},
		{A: true, B: false},
		{A: true, B: true},
		{A: false, B: true},
		{A: false, B: false},
		{A: true, B: false},
		{A: true, B: true},
	}
	var got []app.Event
	for i, state := range states {
		got = append(got, controls.Update(base.Add(time.Duration(i+1)), state)...)
	}
	assertEvents(t, got, app.Scroll{Delta: 1})
}

func TestControlsRotationFastCyclesHaveNoTimeRateLimit(t *testing.T) {
	base := time.Unix(100, 0)
	controls := newTestControls(t, Signals{A: true, B: true}, base, 0)
	cycle := []Signals{
		{A: false, B: true},
		{A: false, B: false},
		{A: true, B: false},
		{A: true, B: true},
	}

	var got []app.Event
	for i, state := range append(append([]Signals{}, cycle...), cycle...) {
		// Equal timestamps are accepted; tiny increments are also valid.
		at := base.Add(time.Duration(i / 2))
		got = append(got, controls.Update(at, state)...)
	}
	assertEvents(t, got, app.Scroll{Delta: 1}, app.Scroll{Delta: 1})
}

func TestControlsButtonsDebounceBounceHoldAndRelease(t *testing.T) {
	base := time.Unix(200, 0)
	controls := newTestControls(t, buttonSignals(false, false, false), base, 10*time.Millisecond)

	// A press candidate must remain stable through the exact threshold, while
	// a bounce resets its candidate timer and does not produce an event.
	assertEvents(t, controls.Update(base.Add(1*time.Millisecond), buttonSignals(true, false, false)))
	assertEvents(t, controls.Update(base.Add(2*time.Millisecond), buttonSignals(false, false, false)))
	assertEvents(t, controls.Update(base.Add(3*time.Millisecond), buttonSignals(true, false, false)))
	assertEvents(t, controls.Update(base.Add(12*time.Millisecond), buttonSignals(true, false, false)))
	assertEvents(t, controls.Update(base.Add(13*time.Millisecond), buttonSignals(true, false, false)), app.SelectPressed{})
	assertEvents(t, controls.Update(base.Add(100*time.Millisecond), buttonSignals(true, false, false)))

	// Release is debounced too and is what rearms the button. A new press
	// after the settled release emits exactly once.
	assertEvents(t, controls.Update(base.Add(101*time.Millisecond), buttonSignals(false, false, false)))
	assertEvents(t, controls.Update(base.Add(111*time.Millisecond), buttonSignals(false, false, false)))
	assertEvents(t, controls.Update(base.Add(112*time.Millisecond), buttonSignals(true, false, false)))
	assertEvents(t, controls.Update(base.Add(122*time.Millisecond), buttonSignals(true, false, false)), app.SelectPressed{})
	assertEvents(t, controls.Update(base.Add(200*time.Millisecond), buttonSignals(true, false, false)))
}

func TestControlsButtonsAreIndependent(t *testing.T) {
	base := time.Unix(200, 0)
	controls := newTestControls(t, buttonSignals(false, false, false), base, 10*time.Millisecond)

	assertEvents(t, controls.Update(base.Add(1*time.Millisecond), buttonSignals(true, false, false)))
	assertEvents(t, controls.Update(base.Add(2*time.Millisecond), buttonSignals(true, true, false)))
	assertEvents(t, controls.Update(base.Add(3*time.Millisecond), buttonSignals(true, true, true)))
	assertEvents(t, controls.Update(base.Add(11*time.Millisecond), buttonSignals(true, true, true)), app.SelectPressed{})
	assertEvents(t, controls.Update(base.Add(12*time.Millisecond), buttonSignals(true, true, true)), app.BackPressed{})
	assertEvents(t, controls.Update(base.Add(13*time.Millisecond), buttonSignals(true, true, true)), app.PlayPausePressed{})
	// Holding any combination does not repeat any button.
	assertEvents(t, controls.Update(base.Add(100*time.Millisecond), buttonSignals(true, true, true)))
}

func TestControlsSparseUpdateSettlesPressBeforeRelease(t *testing.T) {
	base := time.Unix(200, 0)
	controls := newTestControls(t, buttonSignals(false, false, false), base, 10*time.Millisecond)

	// The release sample arrives only after the press was held long enough.
	// The previous candidate must still be settled before recording release.
	assertEvents(t, controls.Update(base.Add(1*time.Millisecond), buttonSignals(true, false, false)))
	assertEvents(t, controls.Update(base.Add(11*time.Millisecond), buttonSignals(false, false, false)), app.SelectPressed{})
	assertEvents(t, controls.Update(base.Add(21*time.Millisecond), buttonSignals(false, false, false)))
	assertEvents(t, controls.Update(base.Add(22*time.Millisecond), buttonSignals(true, false, false)))
	assertEvents(t, controls.Update(base.Add(32*time.Millisecond), buttonSignals(true, false, false)), app.SelectPressed{})
}

func TestControlsStartupHeldButtonsAreSuppressedUntilReleased(t *testing.T) {
	base := time.Unix(200, 0)
	initial := buttonSignals(true, true, true)
	controls := newTestControls(t, initial, base, 10*time.Millisecond)

	assertEvents(t, controls.Update(base.Add(100*time.Millisecond), initial))
	assertEvents(t, controls.Update(base.Add(101*time.Millisecond), buttonSignals(false, false, false)))
	assertEvents(t, controls.Update(base.Add(111*time.Millisecond), buttonSignals(false, false, false)))
	assertEvents(t, controls.Update(base.Add(112*time.Millisecond), buttonSignals(true, true, true)))
	assertEvents(t, controls.Update(base.Add(122*time.Millisecond), buttonSignals(true, true, true)),
		app.SelectPressed{}, app.BackPressed{}, app.PlayPausePressed{})
}

func TestControlsZeroDebounceEmitsPressImmediately(t *testing.T) {
	base := time.Unix(200, 0)
	controls := newTestControls(t, buttonSignals(false, false, false), base, 0)

	assertEvents(t, controls.Update(base, buttonSignals(true, true, true)),
		app.SelectPressed{}, app.BackPressed{}, app.PlayPausePressed{})
	assertEvents(t, controls.Update(base, buttonSignals(true, true, true)))
}

func TestControlsSimultaneousEventsOrderRotationThenButtons(t *testing.T) {
	base := time.Unix(200, 0)
	controls := newTestControls(t, buttonSignals(false, false, false), base, 0)

	// All button presses happen on the same sample that closes the encoder
	// cycle. Rotation must be emitted first, followed by button order.
	assertEvents(t, controls.Update(base.Add(1), Signals{A: false, B: true}))
	assertEvents(t, controls.Update(base.Add(2), Signals{A: false, B: false}))
	assertEvents(t, controls.Update(base.Add(3), Signals{A: true, B: false}))
	assertEvents(t, controls.Update(base.Add(4), Signals{
		A: true, B: true, Select: true, Back: true, PlayPause: true,
	}), app.Scroll{Delta: 1}, app.SelectPressed{}, app.BackPressed{}, app.PlayPausePressed{})
}

func TestControlsStaleTimestampIsIgnoredEntirely(t *testing.T) {
	base := time.Unix(200, 0)
	controls := newTestControls(t, buttonSignals(false, false, false), base, 10*time.Millisecond)

	assertEvents(t, controls.Update(base.Add(10*time.Millisecond), buttonSignals(true, false, false)))
	// This older sample must not alter either rotation or button state.
	assertEvents(t, controls.Update(base.Add(5*time.Millisecond), Signals{
		A: false, B: true, Select: false,
	}))
	assertEvents(t, controls.Update(base.Add(20*time.Millisecond), buttonSignals(true, false, false)), app.SelectPressed{})
}

func TestNewControlsRejectsNegativeDebounce(t *testing.T) {
	_, err := NewControls(Signals{}, time.Unix(0, 0), -time.Nanosecond)
	if err == nil {
		t.Fatal("NewControls accepted a negative debounce")
	}
}

func TestControlsDirectionChangeAfterAbandonedTurn(t *testing.T) {
	base := time.Unix(100, 0)
	controls := newTestControls(t, Signals{A: true, B: true}, base, time.Millisecond)
	states := []Signals{
		{B: true}, {}, {A: true}, {A: true, B: true}, // +1
		{B: true}, {}, {B: true}, {A: true, B: true}, // abandon + turn
		{A: true}, {}, {B: true}, {A: true, B: true}, // -1
	}
	var got []app.Event
	for _, state := range states {
		got = append(got, controls.Update(base, state)...)
	}
	assertEvents(t, got, app.Scroll{Delta: 1}, app.Scroll{Delta: -1})
}

func TestControlsShortPressAndReleaseBounce(t *testing.T) {
	base := time.Unix(100, 0)
	controls := newTestControls(t, buttonSignals(false, false, false), base, 5*time.Millisecond)
	type sample struct {
		ms      int
		pressed bool
		want    []app.Event
	}
	for _, s := range []sample{
		{1, true, nil}, {5, false, nil}, // short pulse rejected
		{20, false, nil}, {21, true, nil},
		{26, true, []app.Event{app.SelectPressed{}}},
		{27, false, nil}, {28, true, nil}, // release bounce: no rearm
		{40, true, nil}, {41, false, nil},
		{46, true, nil}, // stable release settles on next press edge
		{51, true, []app.Event{app.SelectPressed{}}},
	} {
		assertEvents(t, controls.Update(base.Add(time.Duration(s.ms)*time.Millisecond),
			buttonSignals(s.pressed, false, false)), s.want...)
	}
}

func newTestControls(t *testing.T, initial Signals, at time.Time, debounce time.Duration) *Controls {
	t.Helper()
	controls, err := NewControls(initial, at, debounce)
	if err != nil {
		t.Fatalf("NewControls() error = %v", err)
	}
	return controls
}

func buttonSignals(selectPressed, backPressed, playPausePressed bool) Signals {
	return Signals{
		A: true, B: true,
		Select: selectPressed, Back: backPressed, PlayPause: playPausePressed,
	}
}

func assertEvents(t *testing.T, got []app.Event, want ...app.Event) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("events = %#v, want %#v", got, want)
	}
}
