package library

import (
	"bytes"
	"encoding/binary"
	"errors"
	"testing"
)

func TestRecognizeWAVRequiresFirstDataByte(t *testing.T) {
	data := containerWAV(1, nil)
	if err := recognizeWAV(bytes.NewReader(data)); !errors.Is(err, errUnrecognizedAudio) {
		t.Fatalf("recognizeWAV = %v, want unrecognized audio", err)
	}
}

func TestRecognizeWAVSkipsPaddedOptionalChunk(t *testing.T) {
	data := containerWAVWithJunk()
	if err := recognizeWAV(bytes.NewReader(data)); err != nil {
		t.Fatalf("recognizeWAV = %v, want success", err)
	}
}

func containerWAV(dataSize uint32, data []byte) []byte {
	body := make([]byte, 0, 4+24+8+len(data))
	body = append(body, []byte("WAVE")...)
	body = append(body, []byte("fmt ")...)
	var size [4]byte
	binary.LittleEndian.PutUint32(size[:], 16)
	body = append(body, size[:]...)
	var format [16]byte
	binary.LittleEndian.PutUint16(format[0:2], 1)
	binary.LittleEndian.PutUint16(format[2:4], 1)
	binary.LittleEndian.PutUint32(format[4:8], 44100)
	binary.LittleEndian.PutUint16(format[12:14], 2)
	body = append(body, format[:]...)
	body = append(body, []byte("data")...)
	binary.LittleEndian.PutUint32(size[:], dataSize)
	body = append(body, size[:]...)
	body = append(body, data...)
	result := make([]byte, 12, 12+len(body))
	copy(result, "RIFF")
	riffSize := len(body)
	if int(dataSize) > len(data) {
		riffSize += int(dataSize) - len(data)
	}
	binary.LittleEndian.PutUint32(result[4:8], uint32(riffSize))
	copy(result[8:12], body[:4])
	return append(result, body[4:]...)
}

func containerWAVWithJunk() []byte {
	data := containerWAV(2, []byte{1, 2})
	chunk := make([]byte, 0, 5)
	chunk = append(chunk, []byte("JUNK")...)
	chunk = append(chunk, 1, 0, 0, 0, 0, 0)
	insertAt := 12 + 8 + 16
	data = append(data[:insertAt], append(chunk, data[insertAt:]...)...)
	binary.LittleEndian.PutUint32(data[4:8], binary.LittleEndian.Uint32(data[4:8])+10)
	return data
}
