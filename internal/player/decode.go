package player

import (
	"os"

	"github.com/gopxl/beep"
	"github.com/gopxl/beep/mp3"
)

// Match the contents, as the library also accepts audio with a different extension.
// On success the decoder owns file; on failure the caller closes it.
func decodeTrack(file *os.File) (beep.StreamSeekCloser, beep.Format, error) {
	var signature [4]byte
	if _, err := file.ReadAt(signature[:], 0); err != nil {
		return nil, beep.Format{}, err
	}
	if string(signature[:]) == "fLaC" {
		return decodeFLAC(file)
	}
	return mp3.Decode(file)
}
