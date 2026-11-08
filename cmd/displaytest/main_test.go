package main

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestDryRun(t *testing.T) {
	var out, stderr bytes.Buffer
	if err := run([]string{"--dry-run", "--bgr", "--invert"}, &out, &stderr); err != nil {
		t.Fatal(err)
	}
	trace := out.String()
	for _, want := range []string{
		"CMD 36\nDATA 08\nCMD 21\n",
		"CMD 2c\nDATA 153600 bytes (RGB565 frame)\nCLOSE\n",
	} {
		if !strings.Contains(trace, want) {
			t.Fatalf("trace missing %q:\n%s", want, trace)
		}
	}
}

func TestCLIValidationAndErrors(t *testing.T) {
	for _, args := range [][]string{
		{"unexpected"}, {"--unknown"},
		{"--dry-run", "--x-offset=-1"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			if err := run(args, io.Discard, io.Discard); err == nil {
				t.Fatal("want input error")
			}
		})
	}

	want := errors.New("trace writer failed")
	if err := run([]string{"--dry-run"}, failingWriter{want}, io.Discard); !errors.Is(err, want) {
		t.Fatalf("got %v, want writer error", err)
	}
}

type failingWriter struct{ err error }

func (w failingWriter) Write([]byte) (int, error) { return 0, w.err }
