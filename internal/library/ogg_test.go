package library

import (
	"bytes"
	"encoding/binary"
	"errors"
	"os"
	"testing"
)

func TestRecognizeOGGCodecIdentificationHeaders(t *testing.T) {
	opus := make([]byte, 19)
	copy(opus, "OpusHead")
	opus[8] = 1
	opus[9] = 2
	binary.LittleEndian.PutUint32(opus[12:16], 48000)
	if err := recognizeOGG(bytes.NewReader(containerOggPage(opus))); err != nil {
		t.Fatalf("recognizeOGG(Opus) = %v, want success", err)
	}

	theora := append([]byte{0x80}, []byte("theora")...)
	if err := recognizeOGG(bytes.NewReader(containerOggPage(theora))); !errors.Is(err, errUnrecognizedAudio) {
		t.Fatalf("recognizeOGG(Theora) = %v, want unrecognized audio", err)
	}
}

func TestRecognizeOGGFirstSegment255(t *testing.T) {
	valid := make([]byte, 255)
	copy(valid, "OpusHead")
	valid[8] = 1
	valid[9] = 2
	if err := recognizeOGG(bytes.NewReader(containerOggPage(valid))); err != nil {
		t.Fatalf("recognizeOGG(Opus in 255-byte segment) = %v, want success", err)
	}

	invalid := bytes.Repeat([]byte{0xa5}, 255)
	if err := recognizeOGG(bytes.NewReader(containerOggPage(invalid))); !errors.Is(err, errUnrecognizedAudio) {
		t.Fatalf("recognizeOGG(invalid 51-byte prefix) = %v, want unrecognized audio", err)
	}
}

func TestRecognizeOGGRejectsTruncatedFirstSegmentTail(t *testing.T) {
	packet := make([]byte, 255)
	copy(packet, "OpusHead")
	packet[8] = 1
	packet[9] = 2

	page := containerOggPage(packet)
	page = page[:28+51]
	if err := recognizeOGG(bytes.NewReader(page)); !errors.Is(err, errUnrecognizedAudio) {
		t.Fatalf("recognizeOGG(truncated 255-byte segment) = %v, want unrecognized audio", err)
	}
}

func TestRecognizeOggFLACAllowsUnknownHeaderCount(t *testing.T) {
	flac, err := os.ReadFile("testdata/valid.flac")
	if err != nil {
		t.Fatal(err)
	}
	// Mapping 1.0 permits an unknown (zero) header packet count.
	packet := append([]byte{'\x7f', 'F', 'L', 'A', 'C', 1, 0, 0, 0}, flac[:42]...)
	if err := recognizeOGG(bytes.NewReader(containerOggPage(packet))); err != nil {
		t.Fatalf("recognizeOGG(FLAC, unknown header count) = %v", err)
	}
}

func containerOggPage(packet []byte) []byte {
	page := make([]byte, 28+len(packet))
	copy(page, "OggS")
	page[5] = 2 // BOS
	page[26] = 1
	page[27] = byte(len(packet))
	copy(page[28:], packet)
	return page
}
