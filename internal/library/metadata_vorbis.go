package library

import (
	"bytes"
	"fmt"
	"io"

	"github.com/dhowden/tag"
)

func readOggSongTags(source io.ReadSeeker) (songTags, error) {
	// The tag parser supports Vorbis and Opus directly. Ogg FLAC carries
	// native FLAC metadata in packets following a nine-byte mapping header.
	var header [27]byte
	if _, err := io.ReadFull(source, header[:]); err != nil {
		return songTags{}, err
	}
	if _, err := source.Seek(int64(header[26]), io.SeekCurrent); err != nil {
		return songTags{}, err
	}
	var prefix [5]byte
	if _, err := io.ReadFull(source, prefix[:]); err != nil {
		return songTags{}, err
	}
	if _, err := source.Seek(0, io.SeekStart); err != nil {
		return songTags{}, err
	}
	if string(prefix[:]) != "\x7fFLAC" {
		metadata, err := tag.ReadFrom(&metadataReadSeeker{source: source, remaining: maxMetadataBytes})
		if err != nil {
			return songTags{}, err
		}
		return tagsFromMetadata(metadata), nil
	}

	reader := io.LimitReader(source, maxMetadataBytes)
	var packet, native bytes.Buffer
	var serial [4]byte
	first := true
	for {
		if _, err := io.ReadFull(reader, header[:]); err != nil {
			return songTags{}, err
		}
		if string(header[:4]) != "OggS" {
			return songTags{}, fmt.Errorf("invalid Ogg FLAC metadata page")
		}
		if first {
			copy(serial[:], header[14:18])
			first = false
		}
		var lacing [255]byte
		segments := lacing[:int(header[26])]
		if _, err := io.ReadFull(reader, segments); err != nil {
			return songTags{}, err
		}
		for _, size := range segments {
			if !bytes.Equal(header[14:18], serial[:]) {
				if _, err := io.CopyN(io.Discard, reader, int64(size)); err != nil {
					return songTags{}, err
				}
				continue
			}
			if _, err := io.CopyN(&packet, reader, int64(size)); err != nil {
				return songTags{}, err
			}
			if size == 255 {
				continue
			}
			data := packet.Bytes()
			if native.Len() == 0 {
				if len(data) < 51 || string(data[:5]) != "\x7fFLAC" {
					return songTags{}, fmt.Errorf("invalid Ogg FLAC mapping packet")
				}
				native.Write(data[9:13]) // fLaC marker
				data = data[13:]
			}
			if len(data) < 4 {
				return songTags{}, fmt.Errorf("truncated Ogg FLAC metadata block")
			}
			native.Write(data)
			if data[0]&0x80 != 0 {
				metadata, err := tag.ReadFLACTags(bytes.NewReader(native.Bytes()))
				if err != nil {
					return songTags{}, err
				}
				return tagsFromMetadata(metadata), nil
			}
			packet.Reset()
		}
	}
}
