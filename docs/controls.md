# EC11 and button signal processing

`internal/input.Controls` converts EC11 A/B levels and logical encoder, Back,
and Play/Pause button presses into the existing `app.Event` types. It neither
reads GPIO nor changes `AppState`. The keyboard backend remains available.
App continues to decide what scrolling means on each screen.

## Decoding policy

The encoder uses a full-cycle quadrature decoder. With A as the first bit,
`11 → 01 → 00 → 10 → 11` emits `Scroll{Delta: 1}`; the reverse emits
`Scroll{Delta: -1}`. Repeated levels do nothing. Retraced edges, including
contact bounce and mid-cycle direction changes, cancel partial progress.
Returning to `11` without a complete cycle emits nothing. A two-bit jump
discards the partial cycle; decoding resumes at `11`. Startup outside `11`
also waits for that boundary. Incomplete cycles have no timeout and cannot
emit by themselves. There is no rotation speed limit or time-based lockout.

This policy assumes one desired scroll step per complete electrical cycle,
with both contacts high at its boundary. It deliberately favors missing a step
over guessing when transitions are lost. Verify detent phase and cycles per
detent on the delivered EC11; variants may require a different policy. Swap A/B
in the future adapter if the physical direction is reversed. Tests establish
the software behavior; they do not establish the electrical behavior of the
undelivered component.

Buttons use a configurable stable-level debounce interval for both press and
release. A button must remain in its new state for at least that interval.
A debounced press emits exactly one `SelectPressed`, `BackPressed`, or
`PlayPausePressed`; a held button never repeats. A debounced release rearms
it. A press shorter than the interval is ignored. Buttons are independent.
A button held at startup must first be released. Zero debounce accepts edges
immediately; negative intervals are rejected by the constructor.

## Supplying signals and receiving events

Create the processor from an actual initial snapshot and a caller-supplied
timestamp. A/B values represent electrical high. Button values represent
logical **pressed**, so an active-low GPIO adapter must invert those levels.
The 5 ms interval below is a software starting point, configurable by the
caller; it must be checked on the actual switches.

```go
at := time.Unix(0, 0) // tests control time explicitly
signals := input.Signals{A: true, B: true}
controls, err := input.NewControls(signals, at, 5*time.Millisecond)
if err != nil {
    return err
}

signals.Select = true
events := controls.Update(at.Add(time.Millisecond), signals) // no event yet
events = append(events, controls.Update(at.Add(6*time.Millisecond), signals)...)
// events now contains app.SelectPressed{}; pass it to App's input channel.
```

Call `Update(timestamp, snapshot)` for **every** observed A/B transition and
button edge. Also call it with the last unchanged snapshot at debounce
deadlines (or poll regularly) so a held press is delivered without waiting for
another edge. The previous button level is settled before accepting a changed
level: a release after a sufficiently long press still reports that press
even if no intermediate update occurred. Missing short pulses cannot be
reconstructed. Encoder sampling must be fast enough to capture every A/B edge;
slow button polling alone is insufficient for rotation.

Times must be nondecreasing; equal timestamps are allowed, older snapshots
are ignored entirely. Production callers should use a monotonic time source
such as timestamps derived from `time.Now()`, normalized to the same timeline
if GPIO edge timestamps come from a different clock. No real-time reads,
timers, sleeps, or goroutines run inside Controls. Use one owning goroutine;
the processor is not safe for concurrent calls.

Returned events are ordered: rotation first, then encoder press, Back, and
Play/Pause for one snapshot. Forward each to the same input channel consumed
by `App.Run`, using a context-aware send. The adapter owns cancellation,
channel lifetime, sampling, and any timers. No GPIO runner is wired into
`cmd/player` at this stage. Linux GPIO setup, pin assignment, physical wiring,
display integration, and verification on delivered hardware remain separate.

## Verification

`go test ./internal/input` exercises signal traces with explicit timestamps
and no sleeps. The suite covers rotation, reversals, bounce, incomplete and
invalid sequences, rapid actions, button debounce boundaries, repeated
presses, held buttons, and independent buttons.
