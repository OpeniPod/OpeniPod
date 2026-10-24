package library

import (
	"bytes"
	"errors"
	"os"
	"testing"
)

func TestRecognizeFLACAllowsLastStreamInfoBlock(t *testing.T) {
	data, err := os.ReadFile("testdata/valid.flac")
	if err != nil {
		t.Fatal(err)
	}
	data[4] |= 0x80
	if err := recognizeFLAC(bytes.NewReader(data)); err != nil {
		t.Fatalf("recognizeFLAC = %v, want success", err)
	}
}

func TestRecognizeFLACRejectsMissingSampleRate(t *testing.T) {
	data, err := os.ReadFile("testdata/valid.flac")
	if err != nil {
		t.Fatal(err)
	}
	clear(data[18:21])
	if err := recognizeFLAC(bytes.NewReader(data)); !errors.Is(err, errUnrecognizedAudio) {
		t.Fatalf("recognizeFLAC = %v, want unrecognized audio", err)
	}
}
