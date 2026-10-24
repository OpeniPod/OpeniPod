package library

import (
	"bytes"
	"encoding/binary"
	"io"
)

// recognizeWAV checks a RIFF/WAVE stream until it has seen a valid fmt chunk
// followed by a non-empty data chunk. Only its first data byte is required.
func recognizeWAV(source io.Reader) error {
	var riffHeader [12]byte
	if err := readAudioHeader(source, riffHeader[:]); err != nil {
		return invalidAudioRead(err)
	}
	if !bytes.Equal(riffHeader[0:4], []byte("RIFF")) ||
		!bytes.Equal(riffHeader[8:12], []byte("WAVE")) {
		return errUnrecognizedAudio
	}

	riffSize := binary.LittleEndian.Uint32(riffHeader[4:8])
	if riffSize < 4 {
		return errUnrecognizedAudio
	}
	remaining := int64(riffSize) - 4 // The WAVE marker is part of RIFF's size.
	haveFormat := false

	for remaining > 0 {
		if remaining < 8 {
			return errUnrecognizedAudio
		}

		var chunkHeader [8]byte
		if err := readAudioHeader(source, chunkHeader[:]); err != nil {
			return invalidAudioRead(err)
		}
		remaining -= 8
		chunkSize := binary.LittleEndian.Uint32(chunkHeader[4:8])
		chunkTotal := int64(chunkSize)
		if chunkSize&1 != 0 {
			chunkTotal++
		}
		if chunkTotal > remaining {
			return errUnrecognizedAudio
		}

		switch {
		case bytes.Equal(chunkHeader[0:4], []byte("fmt ")):
			if chunkSize < 16 {
				return errUnrecognizedAudio
			}
			var format [16]byte
			if err := readAudioHeader(source, format[:]); err != nil {
				return invalidAudioRead(err)
			}
			remaining -= 16
			if binary.LittleEndian.Uint16(format[0:2]) == 0 ||
				binary.LittleEndian.Uint16(format[2:4]) == 0 ||
				binary.LittleEndian.Uint32(format[4:8]) == 0 ||
				binary.LittleEndian.Uint16(format[12:14]) == 0 {
				return errUnrecognizedAudio
			}
			if err := skipAudioBytes(source, int64(chunkSize)-16); err != nil {
				return invalidAudioRead(err)
			}
			remaining -= int64(chunkSize) - 16
			haveFormat = true

		case bytes.Equal(chunkHeader[0:4], []byte("data")):
			if chunkSize == 0 || !haveFormat {
				return errUnrecognizedAudio
			}
			var firstSample [1]byte
			if err := readAudioHeader(source, firstSample[:]); err != nil {
				return invalidAudioRead(err)
			}
			return nil

		default:
			if err := skipAudioBytes(source, int64(chunkSize)); err != nil {
				return invalidAudioRead(err)
			}
			remaining -= int64(chunkSize)
		}

		if chunkSize&1 != 0 {
			var padding [1]byte
			if err := readAudioHeader(source, padding[:]); err != nil {
				return invalidAudioRead(err)
			}
			remaining--
		}
	}

	return errUnrecognizedAudio
}
