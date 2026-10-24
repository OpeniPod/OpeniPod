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

// skipID3 skips leading ID3v2 tags by their declared synchsafe size. It does not
// interpret tag contents, which may contain bytes resembling MPEG headers.
// The caller can reopen the source for a raw scan if the tag size was damaged.
func skipID3(reader *bufio.Reader) (bool, error) {
	skipped := false
	for {
		header, err := reader.Peek(10)
		if err != nil {
			return skipped, err
		}
		if string(header[:3]) != "ID3" || header[3] < 2 || header[3] > 4 || header[4] == 0xff {
			return skipped, nil
		}
		var size int64
		for _, part := range header[6:10] {
			if part&0x80 != 0 {
				return skipped, nil
			}
			size = size<<7 | int64(part)
		}
		size += 10 // The tag size excludes its header and optional v2.4 footer.
		if header[3] == 4 && header[5]&0x10 != 0 {
			size += 10
		}
		skipped = true
		if err := skipAudioBytes(reader, size); err != nil {
			return skipped, err
		}
	}
}

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
