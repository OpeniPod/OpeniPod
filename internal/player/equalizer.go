package player

import (
	"math"

	"github.com/OpeniPod/OpeniPod/internal/app"
	"github.com/gopxl/beep"
)

// equalizer wraps a stream with one fixed RBJ biquad profile. The streamer is
// read only by Beep's speaker goroutine; callers changing the preset must hold
// speaker.Lock.
type equalizer struct {
	source    beep.Streamer
	preset    app.EqualizerPreset
	filter    biquad
	state     [2]biquadState
	inputGain float64
}

type biquad struct {
	b0, b1, b2 float64
	a1, a2     float64
}

type biquadState struct {
	z1, z2 float64
}

func newEqualizer(source beep.Streamer, preset app.EqualizerPreset) *equalizer {
	eq := &equalizer{source: source, preset: app.EqualizerOff}
	eq.setPreset(preset)
	return eq
}

// setPreset replaces the filter profile and clears its delay state. Reapplying
// the current preset is a no-op so a repeated setting doesn't reset playback.
// The caller must pass a valid preset and hold speaker.Lock while streaming.
func (eq *equalizer) setPreset(preset app.EqualizerPreset) {
	if preset == eq.preset || !preset.Valid() {
		return
	}

	eq.preset = preset
	eq.state = [2]biquadState{}
	switch preset {
	case app.EqualizerOff:
		eq.filter = biquad{}
		eq.inputGain = 1
	case app.EqualizerBassBoost:
		eq.filter = lowShelf(sampleRate, 150, 6, 1)
		eq.inputGain = dbToGain(-6)
	case app.EqualizerVocal:
		eq.filter = peaking(sampleRate, 2000, 0.8, 4)
		eq.inputGain = dbToGain(-4)
	}
}

func (eq *equalizer) Stream(samples [][2]float64) (int, bool) {
	if eq.preset == app.EqualizerOff {
		return eq.source.Stream(samples)
	}

	n, ok := eq.source.Stream(samples)
	for i := 0; i < n; i++ {
		for channel := 0; channel < 2; channel++ {
			y := eq.filter.process(samples[i][channel]*eq.inputGain, &eq.state[channel])
			if y > 1 {
				y = 1
			} else if y < -1 {
				y = -1
			}
			samples[i][channel] = y
		}
	}
	return n, ok
}

func (eq *equalizer) Err() error { return eq.source.Err() }

func (f biquad) process(x float64, state *biquadState) float64 {
	y := f.b0*x + state.z1
	state.z1 = f.b1*x - f.a1*y + state.z2
	state.z2 = f.b2*x - f.a2*y
	return y
}

// Biquad coefficients below follow the W3C Audio EQ Cookbook equations:
// https://www.w3.org/TR/audio-eq-cookbook/
func lowShelf(sampleRate beep.SampleRate, frequency, gainDB, slope float64) biquad {
	a := math.Pow(10, gainDB/40)
	omega := 2 * math.Pi * frequency / float64(sampleRate)
	cos, sin := math.Cos(omega), math.Sin(omega)
	alpha := sin / 2 * math.Sqrt((a+1/a)*(1/slope-1)+2)
	beta := 2 * math.Sqrt(a) * alpha

	a0 := (a + 1) + (a-1)*cos + beta
	return biquad{
		b0: (a * ((a + 1) - (a-1)*cos + beta)) / a0,
		b1: (2 * a * ((a - 1) - (a+1)*cos)) / a0,
		b2: (a * ((a + 1) - (a-1)*cos - beta)) / a0,
		a1: (-2 * ((a - 1) + (a+1)*cos)) / a0,
		a2: ((a + 1) + (a-1)*cos - beta) / a0,
	}
}

func peaking(sampleRate beep.SampleRate, frequency, q, gainDB float64) biquad {
	a := math.Pow(10, gainDB/40)
	omega := 2 * math.Pi * frequency / float64(sampleRate)
	alpha := math.Sin(omega) / (2 * q)
	cos := math.Cos(omega)

	a0 := 1 + alpha/a
	return biquad{
		b0: (1 + alpha*a) / a0,
		b1: (-2 * cos) / a0,
		b2: (1 - alpha*a) / a0,
		a1: (-2 * cos) / a0,
		a2: (1 - alpha/a) / a0,
	}
}

func dbToGain(db float64) float64 { return math.Pow(10, db/20) }
