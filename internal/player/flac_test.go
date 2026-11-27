package player

import (
	"bytes"
	"encoding/binary"
	"io"
	"math"
	"testing"

	"github.com/gopxl/beep"
	"github.com/mewkiz/flac"
	"github.com/mewkiz/flac/frame"
	"github.com/mewkiz/flac/meta"
)

type flacDecodeCase struct {
	name     string
	channels int
	bits     uint8
}

func TestDecodeFLACFormatsAndSamples(t *testing.T) {
	for _, test := range flacDecodeCases() {
		t.Run(test.name, func(t *testing.T) {
			const sampleRate = 48000
			const sampleCount = 128
			want := flacTestSamples(test.channels, test.bits, sampleCount)
			decoder, format := newTestFLACDecoder(t, test, sampleRate, want)
			defer decoder.Close()

			if format.SampleRate != sampleRate || format.NumChannels != test.channels ||
				format.Precision != int((test.bits+7)/8) {
				t.Fatalf("format = %+v, want %d Hz, %d channels, %d-byte precision",
					format, sampleRate, test.channels, (test.bits+7)/8)
			}
			if decoder.Len() != sampleCount {
				t.Fatalf("Len() = %d, want %d", decoder.Len(), sampleCount)
			}

			got := make([][2]float64, sampleCount)
			n, ok := decoder.Stream(got)
			if n != sampleCount || !ok {
				t.Fatalf("Stream() = (%d, %t), want (%d, true)", n, ok, sampleCount)
			}
			assertFLACSamples(t, got, want, test.bits)
			if decoder.Position() != sampleCount {
				t.Fatalf("Position() = %d after decode, want %d", decoder.Position(), sampleCount)
			}

			n, ok = decoder.Stream(make([][2]float64, 1))
			if n != 0 || ok {
				t.Fatalf("stream after EOF = (%d, %t), want (0, false)", n, ok)
			}
			if err := decoder.Err(); err != nil {
				t.Fatalf("Err() after clean EOF = %v, want nil", err)
			}
		})
	}
}

func TestDecodeFLACSeekAfterPartialBufferedReads(t *testing.T) {
	const sampleRate = 48000
	const sampleCount = 128
	const blockSize = 32

	for _, test := range flacDecodeCases() {
		t.Run(test.name, func(t *testing.T) {
			want := flacTestSamples(test.channels, test.bits, sampleCount)
			decoder, _ := newTestFLACDecoder(t, test, sampleRate, want)
			defer decoder.Close()

			// Leave samples in the current frame's decode buffer before seeking.
			partial := make([][2]float64, 3)
			if n, ok := decoder.Stream(partial); n != len(partial) || !ok {
				t.Fatalf("partial Stream() = (%d, %t), want (%d, true)", n, ok, len(partial))
			}

			for _, target := range []int{blockSize + 7, 2*blockSize + 11, 9, sampleCount - 1, sampleCount} {
				if err := decoder.Seek(target); err != nil {
					t.Fatalf("Seek(%d) = %v", target, err)
				}
				if got := decoder.Position(); got != target {
					t.Fatalf("Position() after Seek(%d) = %d, want %d", target, got, target)
				}

				if target == sampleCount {
					n, ok := decoder.Stream(make([][2]float64, 1))
					if n != 0 || ok {
						t.Fatalf("stream at end = (%d, %t), want (0, false)", n, ok)
					}
					continue
				}

				wantCount := min(2, sampleCount-target)
				got := make([][2]float64, wantCount)
				n, ok := decoder.Stream(got)
				if n != len(got) || !ok {
					t.Fatalf("Stream() after Seek(%d) = (%d, %t), want (%d, true)", target, n, ok, len(got))
				}
				assertFLACSamples(t, got, want[target:target+len(got)], test.bits)
				if decoder.Position() != target+len(got) {
					t.Fatalf("Position() after Seek(%d) and stream = %d, want %d", target, decoder.Position(), target+len(got))
				}
			}

			for _, target := range []int{-1, sampleCount + 1} {
				if err := decoder.Seek(target); err == nil {
					t.Fatalf("Seek(%d) succeeded, want an invalid-position error", target)
				}
			}
		})
	}
}

func TestDecodeFLACStreamInfoAndShortFinalFrame(t *testing.T) {
	for _, test := range flacDecodeCases() {
		for _, variant := range []string{"short-tail", "unknown-length", "inherited-format"} {
			t.Run(test.name+"/"+variant, func(t *testing.T) {
				want := flacTestSamples(test.channels, test.bits, 112)
				encoded := encodeTestFLAC(t, test, 48000, want)
				switch variant {
				case "unknown-length":
					// Clear the 36-bit total-samples field in STREAMINFO.
					encoded[21] &= 0xf0
					clear(encoded[22:26])
				case "inherited-format":
					inheritFLACFrameFormat(t, encoded)
				}
				decoder, _, err := decodeFLAC(&readSeekCloser{Reader: bytes.NewReader(encoded)})
				if err != nil {
					t.Fatal(err)
				}
				defer decoder.Close()
				if decoder.Len() != len(want) {
					t.Fatalf("length = %d, want %d", decoder.Len(), len(want))
				}
				for _, target := range []int{111, 33, 96, 0} {
					if err := decoder.Seek(target); err != nil {
						t.Fatalf("Seek(%d): %v", target, err)
					}
					got := make([][2]float64, 1)
					if n, ok := decoder.Stream(got); n != 1 || !ok {
						t.Fatalf("stream at %d = %d, %v, err=%v", target, n, ok, decoder.Err())
					}
					assertFLACSamples(t, got, want[target:target+1], test.bits)
				}
			})
		}
	}
}

func TestDecodeFLACCorruptFrameDrainsStream(t *testing.T) {
	test := flacDecodeCase{channels: 2, bits: 16}
	want := flacTestSamples(test.channels, test.bits, 64)
	encoded := encodeTestFLAC(t, test, 48000, want)
	encoded[len(encoded)-1] ^= 1 // Damage the second frame's CRC.
	decoder, _, err := decodeFLAC(&readSeekCloser{Reader: bytes.NewReader(encoded)})
	if err != nil {
		t.Fatal(err)
	}
	defer decoder.Close()
	if n, ok := decoder.Stream(make([][2]float64, 64)); n != 0 || ok || decoder.Err() == nil {
		t.Fatalf("corrupt stream = %d, %v, err=%v", n, ok, decoder.Err())
	}
	if n, ok := decoder.Stream(make([][2]float64, 1)); n != 0 || ok {
		t.Fatalf("stream after error = %d, %v", n, ok)
	}
}

func TestDecodeFLACFailedSeekPreservesPlayback(t *testing.T) {
	test := flacDecodeCase{channels: 1, bits: 16}
	want := flacTestSamples(test.channels, test.bits, 64)
	encoded := encodeTestFLAC(t, test, 48000, want)
	encoded[len(encoded)-1] ^= 1
	decoder, _, err := decodeFLAC(&readSeekCloser{Reader: bytes.NewReader(encoded)})
	if err != nil {
		t.Fatal(err)
	}
	defer decoder.Close()
	decoder.Stream(make([][2]float64, 3))
	if err := decoder.Seek(40); err == nil {
		t.Fatal("seek into corrupt frame succeeded")
	}
	if decoder.Position() != 3 || decoder.Err() != nil {
		t.Fatalf("failed seek changed playback: position=%d err=%v", decoder.Position(), decoder.Err())
	}
	got := make([][2]float64, 29)
	if n, ok := decoder.Stream(got); n != len(got) || !ok {
		t.Fatalf("stream after failed seek = %d, %v", n, ok)
	}
	assertFLACSamples(t, got, want[3:32], test.bits)
}

// Replace explicit frame format fields with STREAMINFO inheritance and repair
// both checksums. Parse each original frame first to locate its boundaries.
func inheritFLACFrameFormat(t *testing.T, encoded []byte) {
	t.Helper()
	reader := bytes.NewReader(encoded)
	stream, err := flac.NewSeek(reader)
	if err != nil {
		t.Fatal(err)
	}
	for {
		start, _ := reader.Seek(0, io.SeekCurrent)
		f, err := stream.Next()
		if err == io.EOF {
			return
		}
		if err != nil {
			t.Fatal(err)
		}
		headerEnd, _ := reader.Seek(0, io.SeekCurrent)
		if err := f.Parse(); err != nil {
			t.Fatal(err)
		}
		end, _ := reader.Seek(0, io.SeekCurrent)
		encoded[start+2] &= 0xf0  // Sample rate 0: use STREAMINFO.
		encoded[start+3] &^= 0x0e // Sample size 0: use STREAMINFO.
		encoded[headerEnd-1] = byte(flacTestCRC(encoded[start:headerEnd-1], 8, 0x07))
		binary.BigEndian.PutUint16(encoded[end-2:end], flacTestCRC(encoded[start:end-2], 16, 0x8005))
	}
}

func flacTestCRC(data []byte, width uint, polynomial uint16) uint16 {
	var crc uint16
	for _, b := range data {
		crc ^= uint16(b) << (width - 8)
		for bit := 0; bit < 8; bit++ {
			if crc&(1<<(width-1)) != 0 {
				crc = crc<<1 ^ polynomial
			} else {
				crc <<= 1
			}
		}
	}
	return crc & uint16((uint32(1)<<width)-1)
}

func flacDecodeCases() []flacDecodeCase {
	return []flacDecodeCase{
		{name: "mono-16", channels: 1, bits: 16},
		{name: "stereo-16", channels: 2, bits: 16},
		{name: "mono-24", channels: 1, bits: 24},
		{name: "stereo-24", channels: 2, bits: 24},
	}
}

func newTestFLACDecoder(t *testing.T, test flacDecodeCase, sampleRate int, samples [][]int32) (beep.StreamSeekCloser, beep.Format) {
	t.Helper()
	encoded := encodeTestFLAC(t, test, sampleRate, samples)
	decoder, format, err := decodeFLAC(&readSeekCloser{Reader: bytes.NewReader(encoded)})
	if err != nil {
		t.Fatalf("decodeFLAC() = %v", err)
	}
	return decoder, format
}

func encodeTestFLAC(t *testing.T, test flacDecodeCase, sampleRate int, samples [][]int32) []byte {
	t.Helper()
	const blockSize = 32
	if len(samples) < 16 || (len(samples)%blockSize != 0 && len(samples)%blockSize < 16) {
		t.Fatalf("test samples = %d, each frame must have at least 16", len(samples))
	}

	info := &meta.StreamInfo{
		BlockSizeMin:  blockSize,
		BlockSizeMax:  blockSize,
		SampleRate:    uint32(sampleRate),
		NChannels:     uint8(test.channels),
		BitsPerSample: test.bits,
		NSamples:      uint64(len(samples)),
	}
	var encoded bytes.Buffer
	encoder, err := flac.NewEncoder(&encoded, info)
	if err != nil {
		t.Fatalf("flac.NewEncoder() = %v", err)
	}
	channels := frame.ChannelsMono
	if test.channels == 2 {
		channels = frame.ChannelsLR
	}
	for start := 0; start < len(samples); start += blockSize {
		size := min(blockSize, len(samples)-start)
		subframes := make([]*frame.Subframe, test.channels)
		for channel := range subframes {
			values := make([]int32, size)
			for i := range values {
				values[i] = samples[start+i][channel]
			}
			subframes[channel] = &frame.Subframe{
				SubHeader: frame.SubHeader{Pred: frame.PredVerbatim},
				NSamples:  size,
				Samples:   values,
			}
		}
		if err := encoder.WriteFrame(&frame.Frame{
			Header: frame.Header{
				HasFixedBlockSize: true,
				BlockSize:         uint16(size),
				SampleRate:        uint32(sampleRate),
				Channels:          channels,
				BitsPerSample:     test.bits,
			},
			Subframes: subframes,
		}); err != nil {
			t.Fatalf("WriteFrame() = %v", err)
		}
	}
	if err := encoder.Close(); err != nil {
		t.Fatalf("encoder.Close() = %v", err)
	}
	return encoded.Bytes()
}

func flacTestSamples(channels int, bits uint8, count int) [][]int32 {
	patterns := map[uint8][]int32{
		16: {-32768, -16384, -1, 0, 1, 16384, 32767},
		24: {-8388608, -4194304, -257, 0, 257, 4194304, 8388607},
	}
	pattern := patterns[bits]
	samples := make([][]int32, count)
	for i := range samples {
		samples[i] = make([]int32, channels)
		for channel := range samples[i] {
			samples[i][channel] = pattern[(i+2*channel)%len(pattern)]
		}
	}
	return samples
}

func assertFLACSamples(t *testing.T, got [][2]float64, want [][]int32, bits uint8) {
	t.Helper()
	scale := float64(uint64(1) << (bits - 1))
	for i := range got {
		for channel := 0; channel < 2; channel++ {
			wantChannel := channel
			if wantChannel >= len(want[i]) {
				wantChannel = 0
			}
			wantValue := float64(want[i][wantChannel]) / scale
			if math.Abs(got[i][channel]-wantValue) > 1e-12 {
				t.Fatalf("sample %d channel %d = %.12f, want %.12f", i, channel, got[i][channel], wantValue)
			}
		}
	}
}

type readSeekCloser struct {
	*bytes.Reader
}

func (readSeekCloser) Close() error { return nil }
