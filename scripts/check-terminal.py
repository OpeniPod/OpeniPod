"""Linux PTY smoke check: graceful SIGTERM, restored terminal, separate logs."""

import json
import os
import pathlib
import pty
import select
import signal
import subprocess
import sys
import tempfile
import termios
import time

player = str(pathlib.Path(sys.argv[1] if len(sys.argv) > 1 else "./player").resolve())
master, slave = pty.openpty()
original = termios.tcgetattr(slave)
process = None
try:
    with tempfile.TemporaryDirectory(prefix="openipod-terminal-") as directory:
        path = pathlib.Path(directory) / "player.log"
        process = subprocess.Popen(
            [player, "--log-level=debug", "--log-file", str(path)],
            stdin=slave,
            stdout=slave,
            stderr=subprocess.PIPE,
        )
        deadline = time.monotonic() + 5
        while termios.tcgetattr(slave)[3] & termios.ICANON:
            assert process.poll() is None, "player exited before enabling raw mode"
            assert time.monotonic() < deadline, "raw mode was not enabled"
            time.sleep(0.01)
        process.send_signal(signal.SIGTERM)
        _, errors = process.communicate(timeout=5)
        assert process.returncode == 0, (process.returncode, errors)
        assert not errors, errors
        assert termios.tcgetattr(slave) == original, "terminal settings not restored"
        records = [json.loads(line) for line in path.read_text().splitlines()]
        stopped = [record for record in records if record["msg"] == "application stopped"]
        assert len(stopped) == 1 and stopped[0]["reason"] == "cancelled", records
        assert not any(record["level"] == "ERROR" for record in records), records
        screen = b""
        while select.select([master], [], [], 0)[0]:
            screen += os.read(master, 4096)
        assert b"Music" in screen, screen
        assert b"application started" not in screen and b'"component"' not in screen
        print("PASS: SIGTERM shutdown, terminal restoration, UI/log separation")
finally:
    if process is not None and process.poll() is None:
        process.kill()
        process.wait()
    os.close(master)
    os.close(slave)
