package player

import (
	"errors"
	"math"
	"testing"

	"github.com/OpeniPod/OpeniPod/internal/app"
	"github.com/gopxl/beep"
)

type sliceStreamer struct {
	samples [][2]float64
	pos     int
	err     error
}

func (s *sliceStreamer) Stream(dst [][2]float64) (int, bool) {
	n := copy(dst, s.samples[s.pos:])
	s.pos += n
	if s.pos == len(s.samples) {
		return n, false
	}
	return n, true
}

func (s *sliceStreamer) Err() error { return s.err }

type scriptedStreamer struct {
	err   error
	calls int
}

func (s *scriptedStreamer) Stream(dst [][2]float64) (int, bool) {
	s.calls++
	if s.calls == 1 {
		dst[0] = [2]float64{0.25, -0.25}
		dst[1] = [2]float64{0.5, -0.5}
		return 2, true
	}
	s.err = errors.New("source failed")
	return 0, false
}

func (s *scriptedStreamer) Err() error { return s.err }

func TestEqualizerOffIsExactBypass(t *testing.T) {
	want := [][2]float64{{0.25, -0.75}, {1.2, -1.3}, {0, 0}}
	source := &sliceStreamer{samples: append([][2]float64(nil), want...)}
	eq := newEqualizer(source, app.EqualizerOff)
	got := make([][2]float64, len(want))
	n, ok := eq.Stream(got)
	if n != len(want) || ok {
		t.Fatalf("Stream() = (%d, %t), want (%d, false)", n, ok, len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("sample %d = %v, want exact bypass %v", i, got[i], want[i])
		}
	}
}

func TestEqualizerPinnedToneGains(t *testing.T) {
	const rate = 48000
	tests := []struct {
		name      string
		preset    app.EqualizerPreset
		strongHz  float64
		weakHz    float64
		wantRatio float64
		tolerance float64
	}{
		{"bass boost low versus high", app.EqualizerBassBoost, 40, 10000, 2.0, 0.08},
		{"vocal mid versus low", app.EqualizerVocal, 2000, 100, 1.58, 0.08},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			strong := toneRMS(t, tt.preset, tt.strongHz, rate)
			weak := toneRMS(t, tt.preset, tt.weakHz, rate)
			ratio := strong / weak
			if math.Abs(ratio-tt.wantRatio) > tt.tolerance {
				t.Fatalf("strong/weak RMS ratio = %.3f, want %.3f ± %.3f", ratio, tt.wantRatio, tt.tolerance)
			}
		})
	}
}

func toneRMS(t *testing.T, preset app.EqualizerPreset, frequency float64, rate int) float64 {
	t.Helper()
	count := rate
	samples := make([][2]float64, count)
	for i := range samples {
		x := 0.25 * math.Sin(2*math.Pi*frequency*float64(i)/float64(rate))
		samples[i] = [2]float64{x, x}
	}
	eq := newEqualizer(&sliceStreamer{samples: samples}, preset)
	output := make([][2]float64, count)
	if n, _ := eq.Stream(output); n != count {
		t.Fatalf("tone streamer returned %d samples, want %d", n, count)
	}
	// Ignore startup response and measure a steady-state second half.
	var sum float64
	for _, sample := range output[count/2:] {
		sum += sample[0] * sample[0]
	}
	return math.Sqrt(sum / float64(count/2))
}

func TestEqualizerStereoChannelsAreIndependent(t *testing.T) {
	samples := make([][2]float64, 512)
	for i := range samples {
		samples[i][0] = 0.5 * math.Sin(2*math.Pi*1000*float64(i)/float64(sampleRate))
	}
	eq := newEqualizer(&sliceStreamer{samples: samples}, app.EqualizerVocal)
	output := make([][2]float64, len(samples))
	eq.Stream(output)
	for i, sample := range output {
		if sample[1] != 0 {
			t.Fatalf("right channel sample %d = %g, want silence", i, sample[1])
		}
	}
}

func TestEqualizerSwitchBackToOffContinuesSameSource(t *testing.T) {
	want := [][2]float64{{0.1, -0.1}, {0.2, -0.2}, {0.3, -0.3}, {0.4, -0.4}}
	source := &sliceStreamer{samples: append([][2]float64(nil), want...)}
	eq := newEqualizer(source, app.EqualizerBassBoost)
	first := make([][2]float64, 2)
	if n, _ := eq.Stream(first); n != 2 {
		t.Fatalf("first Stream() = %d samples, want 2", n)
	}
	if source.pos != 2 {
		t.Fatalf("source position after first Stream() = %d, want 2", source.pos)
	}

	eq.setPreset(app.EqualizerOff)
	remaining := make([][2]float64, 2)
	if n, _ := eq.Stream(remaining); n != 2 {
		t.Fatalf("second Stream() = %d samples, want 2", n)
	}
	if remaining[0] != want[2] || remaining[1] != want[3] {
		t.Fatalf("samples after switch = %v, want source continuation %v", remaining, want[2:])
	}
}

func TestEqualizerChunkSizesDoNotChangeOutput(t *testing.T) {
	input := make([][2]float64, 4097)
	for i := range input {
		input[i] = [2]float64{
			0.4 * math.Sin(2*math.Pi*1700*float64(i)/float64(sampleRate)),
			0.3 * math.Sin(2*math.Pi*370*float64(i)/float64(sampleRate)),
		}
	}
	whole := streamChunks(t, input, app.EqualizerVocal, len(input))
	chunked := streamChunks(t, input, app.EqualizerVocal, 73)
	for i := range whole {
		if math.Abs(whole[i][0]-chunked[i][0]) > 1e-12 || math.Abs(whole[i][1]-chunked[i][1]) > 1e-12 {
			t.Fatalf("sample %d differs by chunk size: whole=%v chunked=%v", i, whole[i], chunked[i])
		}
	}
}

func streamChunks(t *testing.T, input [][2]float64, preset app.EqualizerPreset, chunkSize int) [][2]float64 {
	t.Helper()
	eq := newEqualizer(&sliceStreamer{samples: append([][2]float64(nil), input...)}, preset)
	output := make([][2]float64, 0, len(input))
	for len(output) < len(input) {
		next := chunkSize
		if remaining := len(input) - len(output); next > remaining {
			next = remaining
		}
		buf := make([][2]float64, next)
		n, ok := eq.Stream(buf)
		if n != next || (len(output)+n < len(input) && !ok) {
			t.Fatalf("chunk Stream() = (%d, %t), want (%d, true) before drain", n, ok, next)
		}
		output = append(output, buf[:n]...)
	}
	return output
}

func TestEqualizerStreamContractAndErrorPropagation(t *testing.T) {
	streamErr := errors.New("source failed")
	source := &scriptedStreamer{}
	eq := newEqualizer(source, app.EqualizerBassBoost)
	buf := [][2]float64{{9, 9}, {9, 9}, {7, 7}, {8, 8}}
	n, ok := eq.Stream(buf)
	if n != 2 || !ok {
		t.Fatalf("first Stream() = (%d, %t), want (2, true)", n, ok)
	}
	if buf[2] != [2]float64{7, 7} || buf[3] != [2]float64{8, 8} {
		t.Fatalf("streamer touched samples beyond n: %v", buf[2:])
	}
	if eq.Err() != nil {
		t.Fatalf("Err() before source error = %v, want nil", eq.Err())
	}
	n, ok = eq.Stream(buf)
	if n != 0 || ok {
		t.Fatalf("drained Stream() = (%d, %t), want (0, false)", n, ok)
	}
	source.err = streamErr
	if !errors.Is(eq.Err(), streamErr) {
		t.Fatalf("Err() = %v, want source error %v", eq.Err(), streamErr)
	}
}

func TestEqualizerBoundsEnabledOutput(t *testing.T) {
	samples := make([][2]float64, 64)
	samples[0] = [2]float64{100, -100}
	for i := 1; i < len(samples); i++ {
		samples[i] = [2]float64{1, -1}
	}
	eq := newEqualizer(&sliceStreamer{samples: samples}, app.EqualizerBassBoost)
	output := make([][2]float64, len(samples))
	if n, _ := eq.Stream(output); n != len(samples) {
		t.Fatalf("Stream() returned %d samples, want %d", n, len(samples))
	}
	for i, sample := range output {
		for channel, value := range sample {
			if math.IsNaN(value) || math.IsInf(value, 0) || value < -1 || value > 1 {
				t.Fatalf("output[%d][%d] = %g, want finite value in [-1, 1]", i, channel, value)
			}
		}
	}
}

func TestEqualizerFreshAndChangedPresetResetHistory(t *testing.T) {
	input := [][2]float64{{0.75, -0.75}, {0.2, -0.2}}
	source := &sliceStreamer{samples: append([][2]float64(nil), input...)}
	eq := newEqualizer(source, app.EqualizerBassBoost)
	buf := make([][2]float64, 1)
	eq.Stream(buf)
	eq.setPreset(app.EqualizerVocal)
	actual := make([][2]float64, 1)
	if n, _ := eq.Stream(actual); n != 1 {
		t.Fatalf("Stream() after preset change = %d, want 1", n)
	}

	// The first sample after changing preset should match a fresh filter fed
	// that same source sample, regardless of the old filter's prior history.
	fresh := newEqualizer(&sliceStreamer{samples: [][2]float64{input[1]}}, app.EqualizerVocal)
	want := make([][2]float64, 1)
	fresh.Stream(want)
	if actual[0] != want[0] {
		t.Fatalf("first output after preset change = %v, want fresh filter output %v", actual[0], want[0])
	}

	// Reapplying the same preset must keep the state instead of restarting it.
	continued := newEqualizer(&sliceStreamer{samples: append([][2]float64(nil), input...)}, app.EqualizerVocal)
	all := make([][2]float64, len(input))
	reference := newEqualizer(&sliceStreamer{samples: append([][2]float64(nil), input...)}, app.EqualizerVocal)
	if n, _ := reference.Stream(all); n != len(input) {
		t.Fatalf("reference Stream() = %d samples, want %d", n, len(input))
	}
	continued.Stream(all[:1])
	continued.setPreset(app.EqualizerVocal)
	next := make([][2]float64, 1)
	continued.Stream(next)
	if next[0] != all[1] {
		t.Fatalf("output after reapplying unchanged preset = %v, want uninterrupted output %v", next[0], all[1])
	}
}

var _ beep.Streamer = (*equalizer)(nil)
