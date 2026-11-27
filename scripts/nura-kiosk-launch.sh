#!/bin/sh
set -eu

player="$HOME/.local/bin/openipod"
music_dir="$HOME/Music"
window_title="OpeniPod"
timeout=60

if [ ! -x "$player" ]; then
	printf 'OpeniPod executable is missing or not executable: %s\n' "$player" >&2
	exit 1
fi

if [ -z "${DISPLAY:-}" ]; then
	printf 'DISPLAY is unset; OpeniPod needs an X11 or XWayland session.\n' >&2
	exit 1
fi

if ! command -v wmctrl >/dev/null 2>&1; then
	printf 'wmctrl is required to request fullscreen mode.\n' >&2
	exit 1
fi

"$player" --music-dir "$music_dir" --scale 1 &
player_pid=$!

elapsed=0
while [ "$elapsed" -lt "$timeout" ]; do
	if ! kill -0 "$player_pid" 2>/dev/null; then
		wait "$player_pid"
		exit $?
	fi

	if wmctrl -l 2>/dev/null | grep -Fq "$window_title"; then
		if wmctrl -r "$window_title" -b add,fullscreen; then
			wait "$player_pid"
			exit $?
		fi
	fi

	sleep 1
	elapsed=$((elapsed + 1))
done

printf 'Timed out waiting for the OpeniPod window to become fullscreen.\n' >&2
kill "$player_pid" 2>/dev/null || true
wait "$player_pid" 2>/dev/null || true
exit 1
