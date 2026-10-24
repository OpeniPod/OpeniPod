package library

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/tcolgate/mp3"
)

var errUnrecognizedMP3 = errors.New("MP3 audio frames not found")

// recognizeMP3 looks for two consecutive complete MPEG Layer III frames.
// Tags are not interpreted: absent or damaged metadata alone cannot reject
// audio. This recognizes the format; it does not decode or validate the song.
func recognizeMP3(ctx context.Context, source io.Reader) error {
	decoder := mp3.NewDecoder(bufio.NewReader(contextReader{ctx: ctx, source: source}))
	var frame mp3.Frame
	var previous [4]byte
	havePrevious := false
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		var skipped int
		if err := decoder.Decode(&frame, &skipped); err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				return fmt.Errorf("%w: %w", errUnrecognizedMP3, err)
			}
			return fmt.Errorf("read MP3 frames: %w", err)
		}
		header := frame.Header()
		if header.Layer() != mp3.Layer3 {
			havePrevious = false
			continue
		}
		if havePrevious && skipped == 0 &&
			header.Version() == mp3.FrameHeader(previous[:]).Version() &&
			header.SampleRate() == mp3.FrameHeader(previous[:]).SampleRate() {
			return ctx.Err()
		}
		copy(previous[:], header)
		havePrevious = true
	}
}
