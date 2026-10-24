package library

import (
	"bytes"
	"errors"
	"os"
	"testing"
)

func TestAACRecognitionRejectsIncompleteFrames(t *testing.T) {
	data, err := os.ReadFile("testdata/valid.aac")
	if err != nil {
		t.Fatal(err)
	}
	for _, size := range []int{0, 2, 7, 10} {
		err := recognizeAAC(bytes.NewReader(data[:size]))
		if !errors.Is(err, errUnrecognizedAudio) {
			t.Fatalf("recognize %d-byte AAC = %v, want unrecognized audio", size, err)
		}
	}
}

func TestAACRecognitionPreservesReadFailures(t *testing.T) {
	sentinel := errors.New("injected ADTS read failure")
	err := recognizeAAC(readerFunc(func([]byte) (int, error) { return 0, sentinel }))
	if !errors.Is(err, sentinel) || errors.Is(err, errUnrecognizedAudio) {
		t.Fatalf("recognition = %v, want read failure", err)
	}
}
