package input

import (
	"fmt"
	"time"

	"github.com/OpeniPod/OpeniPod/internal/app"
)

// Signals is a snapshot of the encoder and three buttons. A and B are electrical
// levels; button values are logical presses (true = pressed). The caller owns
// GPIO polarity conversion. Both encoder contacts high is the cycle boundary.
type Signals struct {
	A, B                    bool
	Select, Back, PlayPause bool
}

// Controls translates ordered signal snapshots into application events. It owns
// no clock, goroutines, channels, or application state. Use it from one goroutine.
type Controls struct {
	last     time.Time
	debounce time.Duration
	encoder  uint8
	synced   bool
	steps    int
	buttons  [3]button
}

type button struct {
	raw, stable bool
	since       time.Time
}

// NewControls seeds the decoder without generating events. Buttons held at
// startup must be released before their next press can generate an event.
// debounce applies to both press and release; zero disables button debouncing.
func NewControls(initial Signals, at time.Time, debounce time.Duration) (*Controls, error) {
	if debounce < 0 {
		return nil, fmt.Errorf("button debounce must not be negative: %s", debounce)
	}
	c := &Controls{last: at, debounce: debounce, encoder: encoderBits(initial)}
	c.synced = c.encoder == 3
	for i, pressed := range initial.presses() {
		c.buttons[i] = button{raw: pressed, stable: pressed, since: at}
	}
	return c, nil
}

// Update consumes a snapshot at a nondecreasing time and returns any events.
// Older snapshots are ignored entirely. Call again with unchanged signals to
// settle pending button changes, including when no new electrical edge arrives.
// A press shorter than debounce is ignored; holding never repeats a press.
func (c *Controls) Update(at time.Time, signals Signals) []app.Event {
	if at.Before(c.last) {
		return nil
	}
	c.last = at
	var events []app.Event
	if delta := c.rotate(encoderBits(signals)); delta != 0 {
		events = append(events, app.Scroll{Delta: delta})
	}
	pressEvents := [3]app.Event{app.SelectPressed{}, app.BackPressed{}, app.PlayPausePressed{}}
	for i, pressed := range signals.presses() {
		b := &c.buttons[i]
		// Settle the preceding level before replacing it: a sufficiently long
		// press is still recognized if the next sample is its release edge.
		if b.settle(at, c.debounce) {
			events = append(events, pressEvents[i])
		}
		if pressed != b.raw {
			b.raw, b.since = pressed, at
		}
		if b.settle(at, c.debounce) {
			events = append(events, pressEvents[i])
		}
	}
	return events
}

func (s Signals) presses() [3]bool { return [3]bool{s.Select, s.Back, s.PlayPause} }

func encoderBits(s Signals) uint8 {
	var bits uint8
	if s.A {
		bits |= 2
	}
	if s.B {
		bits |= 1
	}
	return bits
}

func (b *button) settle(at time.Time, debounce time.Duration) bool {
	if b.raw == b.stable || at.Sub(b.since) < debounce {
		return false
	}
	b.stable = b.raw
	return b.stable
}

func (c *Controls) rotate(next uint8) int {
	previous := c.encoder
	c.encoder = next
	if previous == next {
		return 0
	}
	if !c.synced || previous^next == 3 {
		// Two contacts changing together means at least one edge was lost.
		// Discard the partial cycle and resume only at a known boundary.
		c.steps = 0
		c.synced = next == 3
		return 0
	}
	// Adjacent Gray-code transitions. Bounces retrace an edge and cancel
	// progress, allowing fast rotation without a time-based lockout.
	switch previous<<2 | next {
	case 3<<2 | 1, 1<<2 | 0, 0<<2 | 2, 2<<2 | 3:
		c.steps++
	default:
		c.steps--
	}
	if next != 3 {
		return 0
	}
	delta := 0
	if c.steps == 4 {
		delta = 1
	} else if c.steps == -4 {
		delta = -1
	}
	c.steps = 0
	return delta
}
