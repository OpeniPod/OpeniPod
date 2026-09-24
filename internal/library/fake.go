package library

import (
	"context"
	"time"

	"github.com/OpeniPod/OpeniPod/internal/music"
)

// Fake is a stable sample library for development without audio files.
type Fake struct {
	Tracks []music.Track
}

func NewFake() *Fake {
	return &Fake{Tracks: []music.Track{
		{ID: "one-more-time", Title: "One More Time", Artist: "Daft Punk", Album: "Discovery", TrackNum: 1, Duration: 5*time.Minute + 20*time.Second},
		{ID: "digital-love", Title: "Digital Love", Artist: "Daft Punk", Album: "Discovery", TrackNum: 3, Duration: 4*time.Minute + 58*time.Second},
		{ID: "battery", Title: "Battery", Artist: "Metallica", Album: "Master of Puppets", TrackNum: 1, Duration: 5*time.Minute + 12*time.Second},
		{ID: "everything-in-its-right-place", Title: "Everything in Its Right Place", Artist: "Radiohead", Album: "Kid A", TrackNum: 1, Duration: 4*time.Minute + 11*time.Second},
	}}
}

func (f *Fake) Load(ctx context.Context) ([]music.Track, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return append([]music.Track(nil), f.Tracks...), nil
}
