package library

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"testing"
	"testing/fstest"
)

func TestRecognizeMP3ReadFailure(t *testing.T) {
	sentinel := errors.New("injected read failure")
	err := recognizeMP3(context.Background(), readerFunc(func([]byte) (int, error) {
		return 0, sentinel
	}))
	if !errors.Is(err, sentinel) || errors.Is(err, errUnrecognizedMP3) {
		t.Fatalf("recognition = %v, want read failure distinct from unrecognized format", err)
	}
}

func TestRecognizeMP3Cancellation(t *testing.T) {
	for _, alreadyCanceled := range []bool{false, true} {
		name := "during recognition"
		if alreadyCanceled {
			name = "before recognition"
		}
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if alreadyCanceled {
				cancel()
			}
			reads := 0
			err := recognizeMP3(ctx, readerFunc(func(buffer []byte) (int, error) {
				reads++
				cancel()
				clear(buffer)
				return len(buffer), nil
			}))
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("recognition = %v, want context cancellation", err)
			}
			wantReads := 1
			if alreadyCanceled {
				wantReads = 0
			}
			if reads != wantReads {
				t.Fatalf("read calls = %d, want %d", reads, wantReads)
			}
		})
	}
}

func TestMP3FileIOFailureReturnsNoPartialLibrary(t *testing.T) {
	valid, err := os.ReadFile("testdata/valid.mp3")
	if err != nil {
		t.Fatal(err)
	}
	for _, failRead := range []bool{false, true} {
		name := "open failure"
		if failRead {
			name = "read failure"
		}
		t.Run(name, func(t *testing.T) {
			sentinel := errors.New("injected MP3 file I/O failure")
			files := failingMP3FS{
				FS: fstest.MapFS{
					"a.mp3": &fstest.MapFile{Data: valid},
					"b.mp3": &fstest.MapFile{Data: valid},
				},
				failure: sentinel, failRead: failRead,
			}
			tracks, err := load(context.Background(), t.TempDir(), files)
			if tracks != nil || !errors.Is(err, sentinel) {
				t.Fatalf("load = tracks=%+v err=%v, want no partial list and I/O failure", tracks, err)
			}
		})
	}
}

type readerFunc func([]byte) (int, error)

func (read readerFunc) Read(buffer []byte) (int, error) { return read(buffer) }

type failingMP3FS struct {
	fs.FS
	failure  error
	failRead bool
}

func (f failingMP3FS) Open(name string) (fs.File, error) {
	if name == "b.mp3" && !f.failRead {
		return nil, f.failure
	}
	file, err := f.FS.Open(name)
	if err == nil && name == "b.mp3" {
		return failingMP3File{File: file, failure: f.failure}, nil
	}
	return file, err
}

type failingMP3File struct {
	fs.File
	failure error
}

func (f failingMP3File) Read([]byte) (int, error) { return 0, f.failure }
