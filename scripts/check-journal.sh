#!/usr/bin/env bash
# Run on Linux with a running journald, systemd-cat, journalctl, and Python 3.
# This script only sends/reads test messages; it does not reconfigure journald.
set -euo pipefail

player=$(realpath "${1:-./player}")
work=$(mktemp -d)
tag="openipod-check-$$"
collector=""
cleanup() {
    if [[ -n "$collector" ]]; then
        kill "$collector" 2>/dev/null || true
        wait "$collector" 2>/dev/null || true
    fi
    # Only this script's mktemp directory is removed.
    rm -r -- "$work"
}
trap cleanup EXIT

mkfifo "$work/stderr"
systemd-cat --identifier="$tag" --level-prefix=yes < "$work/stderr" &
collector=$!
printf '\n\n' | "$player" --log-target=journal --log-level=debug \
    > "$work/ui" 2> "$work/stderr"
wait "$collector"
collector=""
journalctl --sync
journalctl --identifier="$tag" --output=json --no-pager > "$work/journal"

python3 - "$work" <<'PY'
import json
import pathlib
import sys

work = pathlib.Path(sys.argv[1])
entries = [json.loads(line) for line in (work / "journal").read_text().splitlines()]
records = [json.loads(entry["MESSAGE"]) for entry in entries]
messages = [record["msg"] for record in records]
for message in ("application started", "library loaded", "play requested",
                "playback started", "application stopped", "settings loaded"):
    assert messages.count(message) == 1, (message, messages)
priorities = {"DEBUG": "7", "INFO": "6", "WARN": "4", "ERROR": "3"}
for entry, record in zip(entries, records):
    assert entry["PRIORITY"] == priorities[record["level"]], entry
    assert "component" in record and "operation" in record, record
screen = (work / "ui").read_text()
assert "Now Playing" in screen, screen
assert "application started" not in screen and '"component"' not in screen
print("PASS: application -> journald -> journalctl; levels and UI separation")
PY

# Cause a renderer failure and check that it is recorded exactly once, with the
# native ERROR priority. /dev/full is Linux's deterministic failing writer.
mkfifo "$work/error-stderr"
systemd-cat --identifier="$tag-error" --level-prefix=yes < "$work/error-stderr" &
collector=$!
status=0
"$player" --log-target=journal < /dev/null > /dev/full 2> "$work/error-stderr" || status=$?
wait "$collector"
collector=""
[[ "$status" == 1 ]]
journalctl --sync
journalctl --identifier="$tag-error" --priority=err --output=json --no-pager > "$work/errors"
python3 - "$work/errors" <<'PY'
import json
import pathlib
import sys

entries = [json.loads(line) for line in pathlib.Path(sys.argv[1]).read_text().splitlines()]
assert len(entries) == 1, entries
entry = entries[0]
record = json.loads(entry["MESSAGE"])
assert entry["PRIORITY"] == "3", entry
assert record["component"] == "ui" and record["operation"] == "render", record
assert record["error"], record
print("PASS: fatal error stored once and found with journalctl --priority=err")
PY
