package library

import (
	"bytes"
	"encoding/binary"
	"io"
)

// recognizeOGG checks the first packet of the first (BOS) Ogg logical stream.
// The packet prefix is carried across pages when the first packet is split.
func recognizeOGG(source io.Reader) error {
	// The packet layouts checked below follow the Vorbis I identification
	// header, RFC 7845 section 5.1 (Opus), and the Ogg FLAC mapping. The
	// corresponding format references are kept here because Ogg's page
	// framing itself does not identify the codec.
	//
	// https://xiph.org/vorbis/doc/Vorbis_I_spec.html
	// https://www.rfc-editor.org/rfc/rfc7845#section-5.1
	// https://www.rfc-editor.org/rfc/rfc9639
	// https://xiph.org/flac/ogg_mapping.html
	const maxPacketHeader = 51
	var prefix [maxPacketHeader]byte
	prefixLength := 0
	firstPage := true

	for {
		var pageHeader [27]byte
		if err := readAudioHeader(source, pageHeader[:]); err != nil {
			return invalidAudioRead(err)
		}
		if !bytes.Equal(pageHeader[0:4], []byte("OggS")) || pageHeader[4] != 0 {
			return errUnrecognizedAudio
		}

		headerType := pageHeader[5]
		if firstPage {
			if headerType&0x02 == 0 || headerType&0x01 != 0 {
				return errUnrecognizedAudio
			}
		} else if headerType&0x01 == 0 {
			return errUnrecognizedAudio
		}

		segmentCount := int(pageHeader[26])
		if segmentCount == 0 {
			return errUnrecognizedAudio
		}
		var lacing [255]byte
		if err := readAudioHeader(source, lacing[:segmentCount]); err != nil {
			return invalidAudioRead(err)
		}

		for i := 0; i < segmentCount; i++ {
			segmentSize := int(lacing[i])
			if err := readOggSegment(source, segmentSize, prefix[:], &prefixLength); err != nil {
				return invalidAudioRead(err)
			}
			if oggAudioPacket(prefix[:prefixLength]) {
				return nil
			}
			if prefixLength == len(prefix) {
				return errUnrecognizedAudio
			}
			if lacing[i] < 255 {
				if prefixLength == 0 {
					return errUnrecognizedAudio
				}
				return errUnrecognizedAudio
			}
		}

		firstPage = false
	}
}

func readOggSegment(source io.Reader, segmentSize int, prefix []byte, prefixLength *int) error {
	if segmentSize == 0 {
		return nil
	}
	if *prefixLength < len(prefix) {
		prefixBytes := len(prefix) - *prefixLength
		if prefixBytes > segmentSize {
			prefixBytes = segmentSize
		}
		if err := readAudioHeader(source, prefix[*prefixLength:*prefixLength+prefixBytes]); err != nil {
			return err
		}
		*prefixLength += prefixBytes
		segmentSize -= prefixBytes
	}
	return skipAudioBytes(source, int64(segmentSize))
}

func oggAudioPacket(prefix []byte) bool {
	switch {
	case len(prefix) >= 7 && bytes.Equal(prefix[:7], []byte("\x01vorbis")):
		return validVorbisIdentification(prefix)
	case len(prefix) >= 8 && bytes.Equal(prefix[:8], []byte("OpusHead")):
		return validOpusIdentification(prefix)
	case len(prefix) >= 5 && bytes.Equal(prefix[:5], []byte("\x7fFLAC")):
		return validOggFLACMapping(prefix)
	default:
		return false
	}
}

func validVorbisIdentification(packet []byte) bool {
	if len(packet) < 30 {
		return false
	}
	return binary.LittleEndian.Uint32(packet[7:11]) == 0 &&
		packet[11] != 0 &&
		binary.LittleEndian.Uint32(packet[12:16]) != 0 &&
		packet[29]&1 != 0
}

func validOpusIdentification(packet []byte) bool {
	// RFC 7845 reserves versions 16 and above for incompatible changes;
	// versions through 15 remain compatible with this header layout.
	return len(packet) >= 19 && packet[8] <= 15 && packet[9] != 0
}

func validOggFLACMapping(packet []byte) bool {
	// Mapping 1.x is compatible across minor versions. The big-endian header
	// packet count at bytes 7..8 may be zero to mean unknown.
	if len(packet) < 51 || packet[5] != 1 ||
		!bytes.Equal(packet[9:13], []byte("fLaC")) {
		return false
	}
	blockLength := uint32(packet[14])<<16 |
		uint32(packet[15])<<8 |
		uint32(packet[16])
	return packet[13]&0x7f == 0 && blockLength == 34 && validFLACStreamInfo(packet[17:51])
}
