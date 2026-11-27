package player

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"sort"

	"github.com/gopxl/beep"
	"github.com/mewkiz/flac"
	"github.com/mewkiz/flac/frame"
)

// flacDecoder adapts FLAC frames to the player's stereo sample stream. It owns
// the frame buffer so seeking can discard old samples and land within a frame.
type flacDecoder struct {
	file      io.Closer
	reader    *flacReader
	stream    *flac.Stream
	frame     *frame.Frame
	offset    int
	position  int
	scale     float64
	err       error
	dataStart int64
	points    []flacSeekPoint
}

func decodeFLAC(file io.ReadSeekCloser) (beep.StreamSeekCloser, beep.Format, error) {
	reader := &flacReader{Reader: bufio.NewReader(file), source: file}
	stream, err := flac.NewSeek(reader)
	if err != nil {
		return nil, beep.Format{}, fmt.Errorf("flac: %w", err)
	}
	info := stream.Info
	if info.NChannels > 2 {
		return nil, beep.Format{}, fmt.Errorf("flac: unsupported channel count %d", info.NChannels)
	}
	format := beep.Format{
		SampleRate:  beep.SampleRate(info.SampleRate),
		NumChannels: int(info.NChannels),
		Precision:   (int(info.BitsPerSample) + 7) / 8,
	}
	start, err := reader.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, beep.Format{}, err
	}
	d := &flacDecoder{
		file: file, reader: reader, stream: stream, dataStart: start,
		scale: 1 / float64(uint64(1)<<(info.BitsPerSample-1)),
	}
	if info.NSamples == 0 {
		if err := d.indexFrames(); err != nil {
			return nil, beep.Format{}, err
		}
		if _, err := reader.Seek(start, io.SeekStart); err != nil {
			return nil, beep.Format{}, err
		}
	}
	return d, format, nil
}

func (d *flacDecoder) Stream(samples [][2]float64) (n int, ok bool) {
	if d.err != nil {
		return 0, false
	}
	for i := range samples {
		if d.frame == nil || d.offset == len(d.frame.Subframes[0].Samples) {
			f, err := d.parseFrame()
			if err != nil {
				if err != io.EOF {
					d.err = fmt.Errorf("flac: %w", err)
					return 0, false
				}
				return n, n > 0
			}
			d.frame, d.offset = f, 0
		}
		left := float64(d.frame.Subframes[0].Samples[d.offset]) * d.scale
		right := left
		if len(d.frame.Subframes) == 2 {
			right = float64(d.frame.Subframes[1].Samples[d.offset]) * d.scale
		}
		samples[i] = [2]float64{left, right}
		d.offset++
		d.position++
		n++
	}
	return n, n > 0
}

func (d *flacDecoder) Len() int      { return int(d.stream.Info.NSamples) }
func (d *flacDecoder) Position() int { return d.position }
func (d *flacDecoder) Err() error    { return d.err }
func (d *flacDecoder) Close() error  { return d.file.Close() }

func (d *flacDecoder) parseFrame() (*frame.Frame, error) {
	f, err := d.stream.Next()
	if err != nil {
		return nil, err
	}
	// A zero field in a frame header inherits the value from STREAMINFO.
	info := d.stream.Info
	if f.BitsPerSample == 0 {
		f.BitsPerSample = info.BitsPerSample
	}
	if f.SampleRate == 0 {
		f.SampleRate = info.SampleRate
	}
	if f.Channels.Count() != int(info.NChannels) || f.BitsPerSample != info.BitsPerSample || f.SampleRate != info.SampleRate {
		return nil, fmt.Errorf("frame format differs from FLAC stream info")
	}
	if err := f.Parse(); err != nil {
		return nil, err
	}
	return f, nil
}

func (d *flacDecoder) Seek(target int) (err error) {
	if target < 0 || target > d.Len() {
		return fmt.Errorf("flac: seek position %d out of range [0, %d]", target, d.Len())
	}
	previousOffset, err := d.reader.Seek(0, io.SeekCurrent)
	if err != nil {
		return fmt.Errorf("flac: seek: %w", err)
	}
	defer func() {
		if err != nil {
			_, restoreErr := d.reader.Seek(previousOffset, io.SeekStart)
			err = errors.Join(err, restoreErr)
		}
	}()
	if d.points == nil {
		if err := d.indexFrames(); err != nil {
			return err
		}
	}
	// Use counted samples rather than frame numbers: the last fixed-block
	// frame can be shorter than the others. Include the EOF point for Seek(Len()).
	i := sort.Search(len(d.points), func(i int) bool { return d.points[i].sample > target }) - 1
	point := d.points[i]
	if _, err := d.reader.Seek(point.offset, io.SeekStart); err != nil {
		return fmt.Errorf("flac: seek: %w", err)
	}
	var next *frame.Frame
	offset := target - point.sample
	if offset > 0 {
		next, err = d.parseFrame()
		if err != nil {
			return fmt.Errorf("flac: seek frame: %w", err)
		}
		if offset > len(next.Subframes[0].Samples) {
			return io.ErrUnexpectedEOF
		}
	}
	d.frame, d.offset, d.err = next, offset, nil
	d.position = target
	return nil
}

type flacSeekPoint struct {
	sample int
	offset int64
}

// Build once on first seek, as files need not contain a seek table. Decode one
// frame at a time; retain only offsets, never the whole track's PCM samples.
func (d *flacDecoder) indexFrames() error {
	if _, err := d.reader.Seek(d.dataStart, io.SeekStart); err != nil {
		return fmt.Errorf("flac: index: %w", err)
	}
	var points []flacSeekPoint
	position := 0
	for {
		offset, err := d.reader.Seek(0, io.SeekCurrent)
		if err != nil {
			return fmt.Errorf("flac: index: %w", err)
		}
		points = append(points, flacSeekPoint{sample: position, offset: offset})
		f, err := d.parseFrame()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("flac: index: %w", err)
		}
		position += int(f.BlockSize)
	}
	d.points = points
	if d.stream.Info.NSamples == 0 {
		d.stream.Info.NSamples = uint64(position)
	}
	return nil
}

// Keep byte-wise frame parsing buffered while reporting logical file offsets.
type flacReader struct {
	*bufio.Reader
	source io.ReadSeeker
}

func (r *flacReader) Seek(offset int64, whence int) (int64, error) {
	if whence == io.SeekCurrent {
		offset -= int64(r.Buffered())
	}
	position, err := r.source.Seek(offset, whence)
	if err == nil {
		r.Reader.Reset(r.source)
	}
	return position, err
}
