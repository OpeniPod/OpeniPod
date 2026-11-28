# Repeat and shuffle (#13)

## Research and decision

A pseudo-random generator produces a reproducible sequence from its initial
state. It does not, by itself, prevent the same track from being selected twice.
For this player, randomness determines a **permutation of the library**, not an
independent random choice after each song.

Use the standard `math/rand/v2` PCG source with `Rand.Shuffle` (Fisher–Yates).
Go's [implementation](https://go.dev/src/math/rand/v2/rand.go) visits each
remaining position and swaps it with a uniformly selected position. Preparing
an order takes linear time and our saved order takes linear space. With unique
library track IDs, each track occurs exactly once per pass.

| Option | Decision |
| --- | --- |
| Independently pick the next random track | Repeats are possible. Tracking visited IDs adds retries and another mechanism for detecting the end of a pass. |
| Shuffle once per pass | Selected: finite, inspectable order; no duplicates within a pass; Next and Previous can follow the same order. |
| Cryptographic randomness | Not required for a music order. It would not replace the need for a permutation. |

The [Go documentation](https://pkg.go.dev/math/rand/v2) provides both PCG and
ChaCha8 sources, explicit seeds, and Shuffle. We choose PCG because playlist
ordering is not security-sensitive and explicit seeds make tests reproducible.
This is a suitability choice, not a benchmark on a target phone. Do not reuse
this generator for secrets; use `crypto/rand` for security-sensitive values.
See also the [Go rand/v2 design explanation](https://go.dev/blog/randv2) and
the [PCG author's research](https://www.pcg-random.org/paper.html).

App chooses a non-fixed session seed with `rand.Uint64()` during construction.
Reducer state holds the seed and a generation counter. A new local PCG source
is constructed from those values for each permutation; the counter advances
in the returned state. Consequently, identical reducer inputs give identical
outputs, with no clock reads, shared mutable RNG, or I/O inside Reduce.
Use the same Go toolchain when reproducing a particular order; tests check
behavioral invariants rather than promising identical permutations across
future standard-library versions.

## Behavior

- Repeat **Off**: advance automatically in library order, or in the shuffled
  order, then stop at the end. Defaults are Off and Shuffle Off.
- Repeat **All**: advance automatically and wrap at the end. With Shuffle On,
  prepare a new permutation for each pass.
- Repeat **One**: replay the current library track when it finishes, even if
  Shuffle is on. Manual Next and Previous still change tracks.
- Manual Next and Previous wrap in the current order regardless of repeat mode,
  preserving existing manual wrap behavior. In shuffle, Previous follows the
  current permutation backwards; it is not an unlimited playback history.
- At a new automatic shuffled pass, avoid starting with the just-finished track
  if another track exists. Choose the first track uniformly from the other
  tracks, then shuffle the remainder. Do not use a fixed swap as a correction:
  that can bias the distribution. One-track libraries necessarily replay it.
- Selecting a track explicitly, or restarting a stopped track with Play, begins
  a new shuffled pass anchored at that track. A newly generated permutation
  can coincidentally match an earlier one; new does not mean guaranteed distinct.
- Turning Shuffle on anchors the new order at the current track, if present,
  without restarting or resuming it. Turning it off restores library order.
- A successful library refresh begins a new order using the updated IDs while
  preserving current playback. If the current ID was removed, its completion
  advances into the new library. Failed refreshes preserve the existing order.
- Automatic completion preserves the current screen and browsing selection.
  Empty libraries do not start playback; inactive/duplicate finish events do
  not start another track while stopped, paused, or awaiting confirmation.

## Settings and controls

Settings keeps the existing Update Library and Equalizer items, followed by
Volume, Repeat and Shuffle. Keyboard movement plus Enter and on-screen row
activation use the same reducer. Enter increases Volume by 10 percentage
points; the existing global minus/plus controls still allow both directions.
Repeat cycles Off → All → One → Off; Shuffle toggles Off/On.

`SettingsState` adds `repeat` (`all`/`one`) and `shuffle` (boolean). Off values
are zero values and omitted from saved JSON, keeping old files compatible.
Missing fields default to Off; unknown repeat strings normalize to Off during
settings application. Existing volume and equalizer values are preserved.
Storage still saves the complete settings using the existing debounce and
shutdown flush. The queue, seed and cursor are session state, not persisted
settings; restarting restores the modes, not the previous track or position.

## Verification

```sh
go test ./internal/app -run 'Test.*(Shuffle|Repeat|Finished|Playback|Settings)'
go test ./internal/storage ./internal/ui ./internal/desktop ./cmd/player
make all
```

Tests use fixed seeds and explicit completion events. They cover all six
repeat/shuffle combinations, multiple passes and seeds, manual navigation,
empty and one-track libraries, library refresh, immutable reducer snapshots,
keyboard and pointer controls, old settings, preservation of equalizer, and
restoration after restarting App. A real audio device is not required for
these automated checks.

For a listening check, use a short MP3 library in the normal desktop app.
Verify Off stops after the last track; All restarts; One replays the current
track; Shuffle visits each track once per pass. Repeat the combinations and
use Next/Previous, pause, change settings, and restart. Phone listening and
touch checks remain part of the project's manual runtime test matrix.
