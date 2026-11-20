package library

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"unicode/utf16"

	"github.com/abema/go-mp4"
)

// Read only the iTunes song fields; skip media, artwork and other boxes.
func readM4ATags(source io.ReadSeeker) (songTags, error) {
	var tags songTags
	_, err := mp4.ReadBoxStructure(source, func(handle *mp4.ReadHandle) (interface{}, error) {
		box := handle.BoxInfo
		switch box.Type {
		case mp4.BoxTypeMoov(), mp4.BoxTypeUdta(), mp4.BoxTypeMeta(), mp4.BoxTypeIlst():
			return handle.Expand()
		case mp4.StrToBoxType("\xa9nam"), mp4.StrToBoxType("\xa9ART"), mp4.StrToBoxType("\xa9alb"), mp4.StrToBoxType("trkn"):
			if len(handle.Path) >= 2 && handle.Path[len(handle.Path)-2] == mp4.BoxTypeIlst() {
				return handle.Expand()
			}
		case mp4.BoxTypeData():
			if !box.UnderIlstMeta {
				return nil, nil
			}
			if box.Size-box.HeaderSize > maxMetadataBytes {
				return nil, fmt.Errorf("MP4 song tag exceeds %d bytes", maxMetadataBytes)
			}
			var buffer bytes.Buffer
			if _, err := handle.ReadData(&buffer); err != nil {
				return nil, err
			}
			data := buffer.Bytes()
			if len(data) < 8 {
				return nil, fmt.Errorf("truncated MP4 song tag")
			}
			value := data[8:] // type indicator and locale precede the value
			field := handle.Path[len(handle.Path)-2]
			text := string(value)
			if field != mp4.StrToBoxType("trkn") {
				switch binary.BigEndian.Uint32(data[:4]) {
				case mp4.DataTypeStringUTF8:
				case mp4.DataTypeStringUTF16:
					if len(value)%2 != 0 {
						return nil, fmt.Errorf("truncated MP4 UTF-16 text")
					}
					units := make([]uint16, len(value)/2)
					for i := range units {
						units[i] = binary.BigEndian.Uint16(value[2*i:])
					}
					text = string(utf16.Decode(units))
				default:
					return nil, fmt.Errorf("unsupported MP4 song text encoding")
				}
			}
			switch field {
			case mp4.StrToBoxType("\xa9nam"):
				tags.title = text
			case mp4.StrToBoxType("\xa9ART"):
				tags.artist = text
			case mp4.StrToBoxType("\xa9alb"):
				tags.album = text
			case mp4.StrToBoxType("trkn"):
				if len(value) < 4 {
					return nil, fmt.Errorf("truncated MP4 track number")
				}
				tags.track = int(binary.BigEndian.Uint16(value[2:4]))
			}
		}
		return nil, nil
	})
	return tags, err
}
