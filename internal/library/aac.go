package library

import (
	"bufio"
	"io"
)

// recognizeAAC recognizes two adjacent ADTS frames, allowing leading tags.
func recognizeAAC(source io.Reader) error {
	reader := bufio.NewReader(source)
	previousRate, previousChannels := -1, -1
	for {
		header, err := reader.Peek(7)
		if err != nil {
			return invalidAudioRead(err)
		}
		rate := int(header[2] >> 2 & 0x0f)
		channels := int(header[2]&1)<<2 | int(header[3]>>6)
		size := int(header[3]&3)<<11 | int(header[4])<<3 | int(header[5]>>5)
		headerSize := 7
		if header[1]&1 == 0 {
			headerSize = 9 // header with CRC
		}
		if header[0] != 0xff || header[1]&0xf6 != 0xf0 || rate > 12 || size <= headerSize {
			if _, err := reader.Discard(1); err != nil {
				return invalidAudioRead(err)
			}
			previousRate, previousChannels = -1, -1
			continue
		}
		if _, err := io.CopyN(io.Discard, reader, int64(size)); err != nil {
			return invalidAudioRead(err)
		}
		if rate == previousRate && channels == previousChannels {
			return nil
		}
		previousRate, previousChannels = rate, channels
	}
}
