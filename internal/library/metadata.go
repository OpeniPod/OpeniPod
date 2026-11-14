package library

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"strings"
	"unicode"

	"github.com/dhowden/tag"

	"github.com/OpeniPod/OpeniPod/internal/music"
)

const maxMetadataBytes = 16 << 20

type songTags struct {
	title, artist, album string
	track                int
}

// readTrackMetadata runs only after content recognition. Optional tag or
// duration errors do not change that result; filesystem errors still abort a
// scan so an incomplete index cannot replace a good one.
func readTrackMetadata(ctx context.Context, files fs.FS, name string, track *music.Track) error {
	file, err := files.Open(name)
	if err != nil {
		return err
	}
	defer file.Close()
	seeker, ok := file.(io.ReadSeeker)
	if !ok {
		slog.WarnContext(ctx, "cannot read audio metadata", "path", track.Path, "reason", "file is not seekable")
		return nil
	}
	reader := &audioReadSeeker{ctx: ctx, source: seeker}
	tags, tagErr := readSongTags(reader)
	if reader.failure != nil {
		return reader.failure
	}
	if tagErr != nil && !errors.Is(tagErr, tag.ErrNoTagsFound) {
		slog.WarnContext(ctx, "cannot read song tags", "path", track.Path, "reason", tagErr)
	}
	if title := cleanTagText(tags.title); title != "" {
		track.Title = title
	}
	track.Artist = cleanTagText(tags.artist)
	track.Album = cleanTagText(tags.album)
	track.TrackNum = max(0, tags.track)
	if _, err := reader.Seek(0, io.SeekStart); err != nil {
		return err
	}
	duration, durationErr := readDuration(ctx, reader)
	if reader.failure != nil {
		return reader.failure
	}
	if durationErr != nil {
		slog.WarnContext(ctx, "cannot read audio duration", "path", track.Path, "reason", durationErr)
	} else if duration > 0 {
		track.Duration = duration
	}
	return ctx.Err()
}

func cleanTagText(value string) string {
	return strings.TrimFunc(strings.ToValidUTF8(value, "�"), func(r rune) bool {
		return r == 0 || unicode.IsSpace(r)
	})
}

func readSongTags(source io.ReadSeeker) (tags songTags, err error) {
	// A malformed optional tag must not crash the library scan if the external
	// parser encounters an invalid slice or length in its contents.
	defer func() {
		if failure := recover(); failure != nil {
			err = fmt.Errorf("malformed song tags: %v", failure)
		}
	}()
	var prefix [12]byte
	if _, err := io.ReadFull(source, prefix[:]); err != nil {
		return tags, err
	}
	if _, err := source.Seek(0, io.SeekStart); err != nil {
		return tags, err
	}
	switch {
	case string(prefix[:4]) == "RIFF":
		return readWAVTags(source)
	case string(prefix[4:8]) == "ftyp":
		return readM4ATags(source)
	case string(prefix[:4]) == "OggS":
		// The shared parser handles Vorbis/Opus; Ogg FLAC needs its
		// metadata packets unpacked into a native FLAC header first.
		return readOggSongTags(source)
	}
	if string(prefix[:3]) != "ID3" && string(prefix[:4]) != "fLaC" {
		// ID3v1 needs 128 bytes at EOF. Short, genuine ADTS/MP3 streams
		// simply have no tags; avoid the parser's negative seek before BOF.
		size, err := source.Seek(0, io.SeekEnd)
		if err != nil {
			return tags, err
		}
		if size < 128 {
			return tags, tag.ErrNoTagsFound
		}
		if _, err := source.Seek(0, io.SeekStart); err != nil {
			return tags, err
		}
	}
	metadata, err := tag.ReadFrom(&metadataReadSeeker{source: source, remaining: maxMetadataBytes})
	if err != nil {
		return tags, err
	}
	return tagsFromMetadata(metadata), nil
}

func tagsFromMetadata(metadata tag.Metadata) songTags {
	number, _ := metadata.Track()
	if metadata.Format() == tag.VORBIS {
		// Vorbis track numbers are also commonly written as "3/14".
		value, _ := metadata.Raw()["tracknumber"].(string)
		number = parseTrackNumber(value)
	}
	return songTags{title: metadata.Title(), artist: metadata.Artist(), album: metadata.Album(), track: number}
}

// Bound how much optional metadata is read, while permitting seeks past audio
// data (for example to ID3v1 at EOF). Large artwork is not needed by Track.
type metadataReadSeeker struct {
	source    io.ReadSeeker
	remaining int
}

func (r *metadataReadSeeker) Read(buffer []byte) (int, error) {
	if len(buffer) > r.remaining {
		return 0, fmt.Errorf("song metadata exceeds %d bytes", maxMetadataBytes)
	}
	n, err := r.source.Read(buffer)
	r.remaining -= n
	return n, err
}

func (r *metadataReadSeeker) Seek(offset int64, whence int) (int64, error) {
	return r.source.Seek(offset, whence)
}
