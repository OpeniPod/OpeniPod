#!/bin/sh
set -eu

config_home=${XDG_CONFIG_HOME:-"$HOME/.config"}
autostart_entry="$config_home/autostart/openipod-kiosk.desktop"

if [ -f "$autostart_entry" ]; then
	rm "$autostart_entry"
	printf 'OpeniPod kiosk autostart disabled. The installed player was kept.\n'
else
	printf 'OpeniPod kiosk autostart is already disabled.\n'
fi
