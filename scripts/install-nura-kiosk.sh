#!/bin/sh
set -eu

if [ "$#" -ne 1 ]; then
	printf 'Usage: %s /path/to/player\n' "$0" >&2
	exit 2
fi

player_binary=$1
if [ ! -f "$player_binary" ] || [ ! -x "$player_binary" ]; then
	printf 'Player binary is missing or not executable: %s\n' "$player_binary" >&2
	exit 1
fi

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
bin_dir="$HOME/.local/bin"
config_home=${XDG_CONFIG_HOME:-"$HOME/.config"}
autostart_dir="$config_home/autostart"

mkdir -p "$bin_dir" "$autostart_dir" "$HOME/Music"
cp "$player_binary" "$bin_dir/openipod"
cp "$script_dir/nura-kiosk-launch.sh" "$bin_dir/openipod-kiosk"
chmod 755 "$bin_dir/openipod" "$bin_dir/openipod-kiosk"

cat > "$autostart_dir/openipod-kiosk.desktop" <<EOF
[Desktop Entry]
Type=Application
Name=OpeniPod kiosk
Exec="$bin_dir/openipod-kiosk"
Terminal=false
X-GNOME-Autostart-enabled=true
EOF

printf 'OpeniPod kiosk autostart enabled for the next graphical login.\n'
printf 'Disable it with: %s/remove-nura-kiosk.sh\n' "$script_dir"
