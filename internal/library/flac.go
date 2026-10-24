package library

import (
	"bytes"
	"encoding/binary"
	"io"
)

// recognizeFLAC checks the FLAC marker and the mandatory STREAMINFO block.
// Metadata after STREAMINFO is intentionally left uninterpreted.
func recognizeFLAC(source io.Reader) error {
	var signature [4]byte
	if err := readAudioHeader(source, signature[:]); err != nil {
		return invalidAudioRead(err)
	}
	if !bytes.Equal(signature[:], []byte("fLaC")) {
		return errUnrecognizedAudio
	}

	var blockHeader [4]byte
	if err := readAudioHeader(source, blockHeader[:]); err != nil {
		return invalidAudioRead(err)
	}
	blockLength := uint32(blockHeader[1])<<16 |
		uint32(blockHeader[2])<<8 |
		uint32(blockHeader[3])
	if blockHeader[0]&0x7f != 0 || blockLength != 34 {
		return errUnrecognizedAudio
	}

	var streamInfo [34]byte
	if err := readAudioHeader(source, streamInfo[:]); err != nil {
		return invalidAudioRead(err)
	}

	if !validFLACStreamInfo(streamInfo[:]) {
		return errUnrecognizedAudio
	}

	return nil
}

func validFLACStreamInfo(streamInfo []byte) bool {
	if len(streamInfo) < 34 {
		return false
	}
	minBlockSize := binary.BigEndian.Uint16(streamInfo[0:2])
	maxBlockSize := binary.BigEndian.Uint16(streamInfo[2:4])
	if minBlockSize < 16 || maxBlockSize < minBlockSize {
		return false
	}

	sampleRate := uint32(streamInfo[10])<<12 |
		uint32(streamInfo[11])<<4 |
		uint32(streamInfo[12])>>4
	channels := ((streamInfo[12] >> 1) & 0x07) + 1
	bitsPerSample := ((streamInfo[12] & 0x01) << 4) | (streamInfo[13] >> 4)
	bitsPerSample++
	return sampleRate != 0 && channels != 0 && bitsPerSample >= 4 && bitsPerSample <= 32
}
