# Run OpeniPod on Nura

Develop on a Linux laptop, check the app in Nura with QEMU, then test the
same source on a physical phone. The test targets are Xiaomi Redmi 4X
(`santoni`) and Google Pixel 4a (`sunfish`). On a physical phone, start
with Nura (formerly postmarketOS) booted into a graphical session.

## Nura in QEMU

To create a VM on an x86-64 Linux laptop, install
[pmbootstrap](https://docs.nura.eco/pmbootstrap/main/installation.html)
and QEMU with graphical support. In `pmbootstrap init`, select vendor
`qemu`, device `amd64`, and the `plasma-mobile` interface. Then run:

```sh
pmbootstrap install
pmbootstrap qemu --image-size=8G --memory=4096 --display=gtk --audio=pa --host-qemu --tablet
```

The launch example uses the host QEMU and a PulseAudio-compatible audio
server. For an existing VM, run the second command without `--image-size=8G`.
See the [QEMU options](https://docs.nura.eco/pmbootstrap/main/usage.html#pmbootstrap-qemu)
for other display and audio configurations. Keep the VM image between runs
and update the application inside it as you develop.

## Build and launch in Nura

Use these steps in the VM or on a phone. Install the build dependencies as an
administrator. Use `doas` in place of `sudo` if that is how your image is
configured.

```sh
sudo apk add go build-base pkgconf alsa-lib-dev
```

Install `xwayland` with `sudo apk add xwayland` if your Wayland session
supports XWayland but the package is missing.

Copy the source checkout into Nura. Put a test MP3 in your writable
`~/Music` directory. From a terminal in the graphical session, run
the following in the checkout as your regular user:

```sh
go version
CGO_ENABLED=1 go build -o player ./cmd/player
./player --music-dir "$HOME/Music" --scale 1
```

Use Go 1.23 or later. Building in Nura uses its own C toolchain and ALSA
libraries. An x86-64 VM build targets the VM; build from the same
source in ARM64 Nura for the phones.

The window needs X11 or XWayland. If it fails to open, check that the
graphical session provides X11 access and sets `DISPLAY`. The player
opens an ALSA playback device on startup. If it reports an audio error,
check that Nura has a working audio route and that ALSA can open it.
On a PulseAudio or PipeWire setup, this may require the session's ALSA
compatibility configuration.

Focus the player window and use the [keyboard controls](controls.md).
Use a keyboard connected to Nura for the phone test. The current app
handles keyboard input. Touch navigation and the phone's volume buttons
still need integration.

After the VM check, test launch, local MP3 playback, audio routing and
behavior after screen lock on each real phone. Record which device,
Nura version and output were tested. See the [test guide](testing.md)
for the checks to run.
