# Audio test fixtures

The `valid.*` files are short generated silence fixtures without user metadata.
They were created with FFmpeg 7.1.3 from the GNOME Flatpak runtime (the binary
was not on `PATH`):

```text
/var/lib/flatpak/runtime/org.gnome.Platform/x86_64/49/e51263e53d04900556e2f97ac2b27201f632f604d68d67f50323f9a99389fdb0/files/bin/ffmpeg
/var/lib/flatpak/runtime/org.gnome.Platform/x86_64/49/e51263e53d04900556e2f97ac2b27201f632f604d68d67f50323f9a99389fdb0/files/bin/ffprobe
```

The existing MP3 fixture uses the built-in `libmp3lame` encoder:

```sh
ffmpeg -f lavfi -i anullsrc=r=44100:cl=mono -t 0.5 \
  -c:a libmp3lame -b:a 32k -write_xing 0 -map_metadata -1 \
  -id3v2_version 0 -write_id3v1 0 valid.mp3
```

The other fixtures use a 0.4 second source, which keeps the encoded duration at
or below 0.5 seconds (AAC frame padding included):

```sh
ffmpeg -f lavfi -i anullsrc=r=44100:cl=mono -t 0.4 \
  -c:a flac -map_metadata -1 valid.flac
ffmpeg -f lavfi -i anullsrc=r=44100:cl=mono -t 0.4 \
  -c:a pcm_s16le -map_metadata -1 valid.wav
ffmpeg -f lavfi -i anullsrc=r=44100:cl=mono -t 0.4 \
  -c:a libvorbis -q:a 1 -map_metadata -1 valid.ogg
ffmpeg -f lavfi -i anullsrc=r=44100:cl=mono -t 0.4 \
  -c:a aac -b:a 32k -movflags +faststart -map_metadata -1 valid.m4a
ffmpeg -f lavfi -i anullsrc=r=44100:cl=mono -t 0.4 \
  -c:a aac -b:a 32k -f adts -map_metadata -1 valid.aac
```

The content-mismatch and Ogg codec regression fixtures were created with:

```sh
ffmpeg -f lavfi -i color=c=black:s=16x16:r=10 -t 0.1 -an \
  -c:v mpeg4 -map_metadata -1 video-only.mp4
ffmpeg -f lavfi -i anullsrc=r=48000:cl=mono -t 0.1 \
  -c:a libopus -b:a 16k -map_metadata -1 -f ogg opus.ogg
```

`video-only.mp4` contains only a 16x16 MPEG-4 video stream and is copied under
an `.m4a` name by the filesystem test to ensure a video container is rejected.
`opus.ogg` is a short Ogg Opus stream used alongside the Ogg Vorbis fixture.

The resulting files were independently checked with `file` and `ffprobe` as
FLAC, PCM WAV, Ogg Vorbis, AAC in an M4A container, AAC ADTS, an MP4 video-only
container, and Ogg Opus. The MP3 fixture was checked as MPEG-1 Layer III audio.
The library tests use these files to exercise content recognition without
relying on filename extensions.
