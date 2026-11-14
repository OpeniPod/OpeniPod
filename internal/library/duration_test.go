package library

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestReadDurationFixtures(t *testing.T) {
	tests := []struct {
		name string
		min  time.Duration
		max  time.Duration
	}{
		{"valid.mp3", 500 * time.Millisecond, 600 * time.Millisecond},
		{"valid.flac", 390 * time.Millisecond, 410 * time.Millisecond},
		{"valid.wav", 390 * time.Millisecond, 410 * time.Millisecond},
		{"valid.ogg", 390 * time.Millisecond, 410 * time.Millisecond},
		{"valid.m4a", 410 * time.Millisecond, 440 * time.Millisecond},
		{"valid.aac", 430 * time.Millisecond, 450 * time.Millisecond},
		{"opus.ogg", 90 * time.Millisecond, 120 * time.Millisecond},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			file, err := os.Open(filepath.Join("testdata", test.name))
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()
			got, err := readDuration(context.Background(), file)
			if err != nil {
				t.Fatal(err)
			}
			if got < test.min || got > test.max {
				t.Fatalf("duration = %v, want %v..%v", got, test.min, test.max)
			}
		})
	}
}

func TestReadDurationCancellationAndInvalidData(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := readDuration(ctx, bytes.NewReader([]byte("not audio"))); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled duration = %v, want context.Canceled", err)
	}
	if _, err := readDuration(context.Background(), bytes.NewReader([]byte("not audio"))); err == nil {
		t.Fatal("invalid audio duration succeeded")
	}
}
