package library

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// WAVE stores song tags in RIFF INFO lists or an embedded ID3 chunk.
func readWAVTags(source io.ReadSeeker) (songTags, error) {
	var tags songTags
	var header [12]byte
	if _, err := io.ReadFull(source, header[:]); err != nil {
		return tags, err
	}
	end := int64(binary.LittleEndian.Uint32(header[4:8])) + 8
	for position := int64(12); position+8 <= end; {
		var chunk [8]byte
		if _, err := io.ReadFull(source, chunk[:]); err != nil {
			return tags, err
		}
		size := int64(binary.LittleEndian.Uint32(chunk[4:]))
		next := position + 8 + size + size%2
		if next > end {
			return tags, fmt.Errorf("WAVE tag chunk exceeds RIFF size")
		}
		switch string(chunk[:4]) {
		case "LIST", "id3 ", "ID3 ":
			if size > maxMetadataBytes {
				return tags, fmt.Errorf("WAVE tags exceed %d bytes", maxMetadataBytes)
			}
			data := make([]byte, int(size))
			if _, err := io.ReadFull(source, data); err != nil {
				return tags, err
			}
			if string(chunk[:4]) == "LIST" {
				if err := readWAVInfo(data, &tags); err != nil {
					return tags, err
				}
			} else {
				id3, err := readSongTags(bytes.NewReader(data))
				if err != nil {
					return tags, err
				}
				return id3, nil
			}
		}
		if _, err := source.Seek(next, io.SeekStart); err != nil {
			return tags, err
		}
		position = next
	}
	return tags, nil
}

func readWAVInfo(data []byte, tags *songTags) error {
	if len(data) < 4 || !bytes.Equal(data[:4], []byte("INFO")) {
		return nil
	}
	for data = data[4:]; len(data) > 0; {
		if len(data) < 8 {
			return fmt.Errorf("truncated WAVE INFO header")
		}
		size := uint64(binary.LittleEndian.Uint32(data[4:8]))
		if size+size%2 > uint64(len(data)-8) {
			return fmt.Errorf("truncated WAVE INFO value")
		}
		value := cleanTagText(string(data[8 : 8+size]))
		switch string(data[:4]) {
		case "INAM":
			tags.title = value
		case "IART":
			tags.artist = value
		case "IPRD":
			tags.album = value
		case "ITRK", "IPRT":
			tags.track = parseTrackNumber(value)
		}
		data = data[8+size+size%2:]
	}
	return nil
}

func parseTrackNumber(value string) int {
	number, _, _ := strings.Cut(value, "/")
	result, err := strconv.Atoi(strings.TrimSpace(number))
	if err != nil || result < 0 {
		return 0
	}
	return result
}
