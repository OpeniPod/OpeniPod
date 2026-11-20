package library

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"time"

	"github.com/abema/go-mp4"
	"github.com/tcolgate/mp3"
)

var errDuration = errors.New("audio duration is unavailable")

// Called after recognition; count frames or read timing headers, without
// decoding samples. An unavailable duration does not invalidate the track.
func readDuration(ctx context.Context, source io.ReadSeeker) (time.Duration, error) {
	r := &audioReadSeeker{ctx: ctx, source: source}
	var prefix [12]byte
	if err := readAudioHeader(r, prefix[:]); err != nil {
		return 0, err
	}
	if _, err := r.Seek(0, io.SeekStart); err != nil {
		return 0, err
	}
	switch {
	case string(prefix[:4]) == "fLaC":
		if _, err := r.Seek(8, io.SeekStart); err != nil {
			return 0, err
		}
		var info [34]byte
		if err := readAudioHeader(r, info[:]); err != nil {
			return 0, err
		}
		return flacDuration(info[:])
	case string(prefix[:4]) == "RIFF":
		return wavDuration(r, int64(binary.LittleEndian.Uint32(prefix[4:8]))+8)
	case string(prefix[:4]) == "OggS":
		return oggDuration(r)
	case string(prefix[4:8]) == "ftyp":
		return mp4Duration(r)
	default:
		return mpegDuration(ctx, r)
	}
}

func mpegDuration(ctx context.Context, source io.Reader) (time.Duration, error) {
	r := bufio.NewReader(source)
	if _, err := skipID3(r); err != nil {
		return 0, err
	}
	header, err := r.Peek(2)
	if err != nil {
		return 0, err
	}
	if header[0] == 0xff && header[1]&0xf6 == 0xf0 {
		return aacDuration(ctx, r)
	}
	decoder := mp3.NewDecoder(r)
	var frame mp3.Frame
	var total time.Duration
	for {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		var skipped int
		if err := decoder.Decode(&frame, &skipped); err != nil {
			if total > 0 && (errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF)) {
				return total, nil
			}
			return 0, err
		}
		total += frame.Duration()
	}
}

func aacDuration(ctx context.Context, r io.Reader) (time.Duration, error) {
	rates := [...]uint32{96000, 88200, 64000, 48000, 44100, 32000, 24000, 22050, 16000, 12000, 11025, 8000, 7350}
	var total time.Duration
	for {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		var h [7]byte
		if err := readAudioHeader(r, h[:]); err != nil {
			if total > 0 && (errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF)) {
				return total, nil
			}
			return 0, err
		}
		rate := int(h[2] >> 2 & 15)
		length := int(h[3]&3)<<11 | int(h[4])<<3 | int(h[5]>>5)
		if h[0] != 0xff || h[1]&0xf6 != 0xf0 || rate >= len(rates) || length < 7 {
			return 0, errDuration
		}
		// The seven bytes read above exclude an optional two-byte CRC.
		if err := skipAudioBytes(r, int64(length-7)); err != nil {
			return 0, err
		}
		d, err := durationRatio(uint64(h[6]&3+1)*1024, rates[rate])
		if err != nil {
			return 0, err
		}
		total += d
	}
}

func flacDuration(info []byte) (time.Duration, error) {
	rate := uint32(info[10])<<12 | uint32(info[11])<<4 | uint32(info[12])>>4
	samples := binary.BigEndian.Uint64(info[10:18]) & ((1 << 36) - 1)
	return durationRatio(samples, rate)
}

func wavDuration(r io.ReadSeeker, end int64) (time.Duration, error) {
	var byteRate uint32
	for position := int64(12); position+8 <= end; {
		if _, err := r.Seek(position, io.SeekStart); err != nil {
			return 0, err
		}
		var h [8]byte
		if err := readAudioHeader(r, h[:]); err != nil {
			return 0, err
		}
		size := int64(binary.LittleEndian.Uint32(h[4:]))
		position += 8 + size + size%2
		if position > end {
			return 0, errDuration
		}
		switch string(h[:4]) {
		case "fmt ":
			var format [16]byte
			if size < 16 {
				return 0, errDuration
			}
			if err := readAudioHeader(r, format[:]); err != nil {
				return 0, err
			}
			code := binary.LittleEndian.Uint16(format[:2])
			if code != 1 && code != 3 { // PCM or IEEE float; compressed WAV may be VBR.
				return 0, nil
			}
			byteRate = binary.LittleEndian.Uint32(format[8:12])
		case "data":
			return durationRatio(uint64(size), byteRate)
		}
	}
	return 0, errDuration
}

func oggDuration(r io.ReadSeeker) (time.Duration, error) {
	var rate, serial uint32
	var preSkip uint64
	first := true
	for {
		var header [27]byte
		if err := readAudioHeader(r, header[:]); err != nil {
			return 0, err
		}
		if string(header[:4]) != "OggS" {
			return 0, errDuration
		}
		var lacing [255]byte
		segments := lacing[:int(header[26])]
		if err := readAudioHeader(r, segments); err != nil {
			return 0, err
		}
		size := 0
		for _, n := range segments {
			size += int(n)
		}
		if first {
			serial = binary.LittleEndian.Uint32(header[14:18])
			var prefix [51]byte
			n := min(size, len(prefix))
			if err := readAudioHeader(r, prefix[:n]); err != nil {
				return 0, err
			}
			size -= n
			switch {
			case n >= 30 && bytes.HasPrefix(prefix[:], []byte("\x01vorbis")):
				rate = binary.LittleEndian.Uint32(prefix[12:16])
			case n >= 19 && bytes.HasPrefix(prefix[:], []byte("OpusHead")):
				rate = 48000
				preSkip = uint64(binary.LittleEndian.Uint16(prefix[10:12]))
			case n >= 51 && bytes.HasPrefix(prefix[:], []byte("\x7fFLAC")):
				return flacDuration(prefix[17:51])
			default:
				return 0, errDuration
			}
			first = false
		}
		if _, err := r.Seek(int64(size), io.SeekCurrent); err != nil {
			return 0, err
		}
		if binary.LittleEndian.Uint32(header[14:18]) == serial && header[5]&4 != 0 {
			granule := binary.LittleEndian.Uint64(header[6:14])
			if granule == ^uint64(0) || granule < preSkip {
				return 0, errDuration
			}
			return durationRatio(granule-preSkip, rate)
		}
	}
}

func mp4Duration(r io.ReadSeeker) (time.Duration, error) {
	type timing struct {
		audio bool
		rate  uint32
		count uint64
	}
	var result time.Duration
	_, err := mp4.ReadBoxStructure(r, func(h *mp4.ReadHandle) (interface{}, error) {
		switch h.BoxInfo.Type {
		case mp4.BoxTypeMoov(), mp4.BoxTypeMdia():
			return h.Expand(h.Params...)
		case mp4.BoxTypeTrak():
			track := new(timing)
			if _, err := h.Expand(track); err != nil {
				return nil, err
			}
			if track.audio && result == 0 {
				var err error
				result, err = durationRatio(track.count, track.rate)
				return nil, err
			}
		case mp4.BoxTypeHdlr(), mp4.BoxTypeMdhd():
			if len(h.Params) == 0 {
				return nil, nil
			}
			track := h.Params[0].(*timing)
			box, _, err := h.ReadPayload()
			if err != nil {
				return nil, err
			}
			switch box := box.(type) {
			case *mp4.Hdlr:
				track.audio = box.HandlerType == [4]byte{'s', 'o', 'u', 'n'}
			case *mp4.Mdhd:
				track.rate, track.count = box.Timescale, box.GetDuration()
			}
		}
		return nil, nil
	})
	return result, err
}

func durationRatio(samples uint64, rate uint32) (time.Duration, error) {
	if rate == 0 {
		return 0, errDuration
	}
	seconds, remainder := samples/uint64(rate), samples%uint64(rate)
	if seconds > uint64((1<<63-1)/time.Second) {
		return 0, errDuration
	}
	nanos := seconds*uint64(time.Second) + remainder*uint64(time.Second)/uint64(rate)
	if nanos > 1<<63-1 {
		return 0, errDuration
	}
	return time.Duration(nanos), nil
}
