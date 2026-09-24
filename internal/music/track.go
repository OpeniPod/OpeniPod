package music

import "time"

type TrackID string

type Track struct {
	ID       TrackID
	Path     string
	Title    string
	Artist   string
	Album    string
	TrackNum int
	Duration time.Duration
}
