package library

import (
	"bytes"
	"context"
	"os"
	"testing"
	"testing/fstest"

	"github.com/tcolgate/mp3"
)

func TestCheckAudioMP3WithID3Tags(t *testing.T) {
	frames := firstMP3Frames(t, 3)
	if len(frames) >= 1440 {
		t.Fatalf("test MP3 payload = %d bytes, want less than 1440", len(frames))
	}

	fakeFrame := make([]byte, 24)
	copy(fakeFrame, []byte{0xff, 0xfb, 0xe8, 0x00})
	private := append([]byte("openipod\x00"), fakeFrame...)
	// This short PRIV frame has the same size encoding in ID3v2.3 and v2.4.
	body := append([]byte{'P', 'R', 'I', 'V', 0, 0, 0, byte(len(private)), 0, 0}, private...)
	validTag := func(version byte) []byte {
		return append(id3Tag(version, body), frames...)
	}
	oversizedTag := id3Header(4, 1000)
	oversizedTag = append(oversizedTag, frames...)
	partialTag := append(id3Header(4, len(frames)/2), frames...)
	badVersionTag := append(id3Header(0xff, 0), frames...)
	badSizeTag := id3Header(4, 0)
	badSizeTag[6] = 0x80
	badSizeTag = append(badSizeTag, frames...)

	tests := []struct {
		name    string
		data    []byte
		wantErr bool
	}{
		{name: "tagless MP3", data: frames},
		{name: "v2.3 tag with false frame header", data: validTag(3)},
		{name: "v2.4 tag with false frame header", data: validTag(4)},
		{name: "declared tag size exceeds EOF", data: oversizedTag},
		{name: "declared tag size hides first frames", data: partialTag},
		{name: "malformed version", data: badVersionTag},
		{name: "malformed synchsafe size", data: badSizeTag},
		{name: "tag only", data: id3Tag(4, fakeFrame), wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := fstest.MapFS{"audio.mp3": &fstest.MapFile{Data: tt.data}}
			err := checkAudio(context.Background(), files, "audio.mp3")
			if (err != nil) != tt.wantErr {
				t.Fatalf("checkAudio() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func firstMP3Frames(t *testing.T, count int) []byte {
	t.Helper()

	data, err := os.ReadFile("testdata/valid.mp3")
	if err != nil {
		t.Fatal(err)
	}
	source := bytes.NewReader(data)
	decoder := mp3.NewDecoder(source)
	var frame mp3.Frame
	for i := 0; i < count; i++ {
		var skipped int
		if err := decoder.Decode(&frame, &skipped); err != nil {
			t.Fatalf("decode frame %d: %v", i+1, err)
		}
	}
	return data[:len(data)-source.Len()]
}

func id3Tag(version byte, body []byte) []byte {
	return append(id3Header(version, len(body)), body...)
}

func id3Header(version byte, size int) []byte {
	return []byte{
		'I', 'D', '3', version, 0, 0,
		byte(size>>21) & 0x7f,
		byte(size>>14) & 0x7f,
		byte(size>>7) & 0x7f,
		byte(size) & 0x7f,
	}
}
