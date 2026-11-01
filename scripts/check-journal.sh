#!/usr/bin/env bash
# Run in an X11 session on Linux with audio, journald, systemd-cat, journalctl,
# and Python 3. The player opens a graphical window.
# This script only sends/reads test messages; it does not reconfigure journald.
set -euo pipefail

player=$(realpath "${1:-./player}")
work=$(mktemp -d)
tag="openipod-check-$$"
collector=""
app_pid=""
cleanup() {
    if [[ -n "$app_pid" ]]; then
        kill -TERM "$app_pid" 2>/dev/null || true
        wait "$app_pid" 2>/dev/null || true
    fi
    if [[ -n "$collector" ]]; then
        kill "$collector" 2>/dev/null || true
        wait "$collector" 2>/dev/null || true
    fi
    # Only this script's mktemp directory is removed.
    rm -r -- "$work"
}
trap cleanup EXIT

mkdir "$work/music"
mkfifo "$work/stderr"
systemd-cat --identifier="$tag" --level-prefix=yes < "$work/stderr" &
collector=$!
"$player" --log-target=journal --log-level=debug \
    --music-dir="$work/music" --settings-file="$work/settings.json" \
    > "$work/stdout" 2> "$work/stderr" &
app_pid=$!
ready=false
for attempt in {1..40}; do
    if ! kill -0 "$app_pid" 2>/dev/null; then
        echo "Player exited before loading the library" >&2
        exit 1
    fi
    journalctl --identifier="$tag" --output=cat --no-pager > "$work/startup"
    if grep -Fq '"msg":"library loaded"' "$work/startup"; then
        ready=true
        break
    fi
    sleep 0.25
done
if [[ "$ready" != true ]]; then
    echo "Timed out waiting for player startup in the journal" >&2
    exit 1
fi
kill -TERM "$app_pid"
wait "$app_pid"
app_pid=""
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
for message in ("application started", "library loaded",
                "application stopped", "settings loaded"):
    assert messages.count(message) == 1, (message, messages)
priorities = {"DEBUG": "7", "INFO": "6", "WARN": "4", "ERROR": "3"}
for entry, record in zip(entries, records):
    assert entry["PRIORITY"] == priorities[record["level"]], entry
    assert "component" in record and "operation" in record, record
assert (work / "stdout").read_text() == ""
assert records[-1]["msg"] == "application stopped"
assert records[-1]["reason"] == "cancelled"
print("PASS: graphical application -> journald -> journalctl; levels and SIGTERM")
PY

# Cause a settings load failure and check that it is recorded exactly once
# with the native ERROR priority.
printf 'invalid JSON\n' > "$work/settings.json"
mkfifo "$work/error-stderr"
systemd-cat --identifier="$tag-error" --level-prefix=yes < "$work/error-stderr" &
collector=$!
status=0
"$player" --log-target=journal --music-dir="$work/music" \
    --settings-file="$work/settings.json" 2> "$work/error-stderr" || status=$?
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
assert record["component"] == "storage" and record["operation"] == "load_settings", record
assert record["error"], record
print("PASS: fatal error stored once and found with journalctl --priority=err")
PY
