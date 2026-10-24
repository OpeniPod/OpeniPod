package library

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"

	"github.com/abema/go-mp4"
)

func recognizeM4A(ctx context.Context, file fs.File) error {
	seeker, ok := file.(io.ReadSeeker)
	if !ok {
		return fmt.Errorf("inspect MP4 audio: file is not seekable")
	}
	info, err := file.Stat()
	if err != nil {
		return err
	}
	reader := &audioReadSeeker{ctx: ctx, source: seeker}
	var haveType, haveAudio, haveData bool
	found := errors.New("audio track recognized")
	_, err = mp4.ReadBoxStructure(reader, func(handle *mp4.ReadHandle) (interface{}, error) {
		box := handle.BoxInfo
		if box.Offset > uint64(info.Size()) || box.Size > uint64(info.Size())-box.Offset {
			return nil, errUnrecognizedAudio
		}
		switch {
		case len(handle.Path) == 1 && box.Type == mp4.BoxTypeFtyp():
			haveType = true
		case len(handle.Path) == 1 && box.Type == mp4.BoxTypeMdat():
			haveData = haveData || box.Size > box.HeaderSize
		case len(handle.Path) == 1 && box.Type == mp4.BoxTypeMoov():
			return handle.Expand()
		case len(handle.Path) == 2 && handle.Path[0] == mp4.BoxTypeMoov() && box.Type == mp4.BoxTypeTrak():
			return handle.Expand()
		case len(handle.Path) == 3 && handle.Path[1] == mp4.BoxTypeTrak() && box.Type == mp4.BoxTypeMdia():
			return handle.Expand()
		case len(handle.Path) == 4 && handle.Path[2] == mp4.BoxTypeMdia() && box.Type == mp4.BoxTypeHdlr():
			payload, _, err := handle.ReadPayload()
			if err != nil {
				return nil, err
			}
			haveAudio = haveAudio || payload.(*mp4.Hdlr).HandlerType == [4]byte{'s', 'o', 'u', 'n'}
		}
		if haveType && haveAudio && haveData {
			return nil, found
		}
		return nil, nil
	})
	if reader.failure != nil {
		return reader.failure
	}
	if errors.Is(err, found) {
		return ctx.Err()
	}
	if err != nil {
		return fmt.Errorf("%w: MP4 container: %v", errUnrecognizedAudio, err)
	}
	return fmt.Errorf("%w: MP4 audio track or media data missing", errUnrecognizedAudio)
}

type audioReadSeeker struct {
	ctx     context.Context
	source  io.ReadSeeker
	failure error
}

func (reader *audioReadSeeker) Read(buffer []byte) (int, error) {
	if err := reader.ctx.Err(); err != nil {
		reader.failure = err
		return 0, err
	}
	n, err := reader.source.Read(buffer)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		reader.failure = err
	}
	return n, err
}

func (reader *audioReadSeeker) Seek(offset int64, whence int) (int64, error) {
	if err := reader.ctx.Err(); err != nil {
		reader.failure = err
		return 0, err
	}
	position, err := reader.source.Seek(offset, whence)
	if err != nil {
		reader.failure = err
	}
	return position, err
}
