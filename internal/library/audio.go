package library

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
)

var errUnrecognizedAudio = errors.New("supported audio format not recognized")

// checkAudio selects a recognizer from the contents, not the filename.
// Recognizers inspect required format headers, without decoding the song.
func checkAudio(ctx context.Context, files fs.FS, name string) error {
	file, err := files.Open(name)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer file.Close()
	reader := bufio.NewReader(contextReader{ctx: ctx, source: file})
	prefix, err := reader.Peek(12)
	if err != nil {
		return invalidAudioRead(err)
	}
	switch {
	case bytes.HasPrefix(prefix, []byte("fLaC")):
		err = recognizeFLAC(reader)
	case bytes.HasPrefix(prefix, []byte("RIFF")):
		err = recognizeWAV(reader)
	case bytes.HasPrefix(prefix, []byte("OggS")):
		err = recognizeOGG(reader)
	case bytes.Equal(prefix[4:8], []byte("ftyp")):
		err = recognizeM4A(ctx, file)
	case prefix[0] == 0xff && prefix[1]&0xf6 == 0xf0:
		err = recognizeAAC(reader)
	default:
		// Both MP3 and ADTS can have leading ID3 tags or damaged metadata.
		// Skip tags before frame recognition; retry the original bytes if a
		// damaged tag size hid the audio. Reopening also supports non-seekable files.
		var skippedID3 bool
		skippedID3, err = skipID3(reader)
		if err == nil {
			err = recognizeMP3(ctx, reader)
		} else if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			err = fmt.Errorf("%w: %w", errUnrecognizedMP3, err)
		} else {
			return err
		}
		if skippedID3 && errors.Is(err, errUnrecognizedMP3) {
			fallback, openErr := files.Open(name)
			if openErr != nil {
				return fmt.Errorf("reopen file after ID3 recognition: %w", openErr)
			}
			defer fallback.Close()
			err = recognizeMP3(ctx, fallback)
		}
		if errors.Is(err, errUnrecognizedMP3) {
			fallback, openErr := files.Open(name)
			if openErr != nil {
				return fmt.Errorf("reopen file for ADTS recognition: %w", openErr)
			}
			defer fallback.Close()
			err = recognizeAAC(contextReader{ctx: ctx, source: fallback})
		}
	}
	if contextErr := ctx.Err(); contextErr != nil {
		return contextErr
	}
	return err
}

func invalidAudioRead(err error) error {
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return fmt.Errorf("%w: %v", errUnrecognizedAudio, err)
	}
	return err
}

func readAudioHeader(source io.Reader, buffer []byte) error {
	_, err := io.ReadFull(source, buffer)
	return err
}

func skipAudioBytes(source io.Reader, count int64) error {
	_, err := io.CopyN(io.Discard, source, count)
	return err
}

type contextReader struct {
	ctx    context.Context
	source io.Reader
}

func (r contextReader) Read(buffer []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.source.Read(buffer)
}
