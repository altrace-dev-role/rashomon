#!/usr/bin/env python3
"""A stand-in for tmux, for measure1.sh's tests (card MEASURE1): the subset
of tmux the driver calls, on a private server named by -S. new-session starts
a relay that holds the socket, runs the client with a pipe for its terminal
input, and forwards what paste-buffer and send-keys append to SOCK.in. What
it cannot reproduce is stated in the card: a real server's global
environment, its default shell and paste timing.
"""

import os
import signal
import socket
import subprocess
import sys
import time


def opt(args, name):
    return args[args.index(name) + 1] if name in args else None


def append(sock, data):
    with open(sock + ".in", "ab") as f:
        f.write(data)


def alive(sock):
    try:
        with open(sock + ".pid") as f:
            pid = int(f.read().split()[0])
        os.kill(pid, 0)
        return pid
    except (OSError, ValueError, IndexError):
        return 0


def relay(sock, cwd, cmd):
    s = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
    s.bind(sock)
    open(sock + ".in", "wb").close()
    with open(sock + ".pane", "wb") as pane:
        child = subprocess.Popen(cmd, cwd=cwd, stdin=subprocess.PIPE, stdout=pane, stderr=pane)
    stdin = child.stdin
    assert stdin is not None
    with open(sock + ".pid", "w") as f:
        f.write("%d %d\n" % (os.getpid(), child.pid))
    sent = 0
    while child.poll() is None:
        with open(sock + ".in", "rb") as f:
            f.seek(sent)
            data = f.read()
        if data:
            sent += len(data)
            try:
                stdin.write(data)
                stdin.flush()
            except BrokenPipeError:
                break
        time.sleep(0.05)
    s.close()
    for suffix in ("", ".pid"):
        try:
            os.remove(sock + suffix)
        except FileNotFoundError:
            pass


def main(argv):
    if argv and argv[0] == "__relay":
        relay(argv[1], argv[2], argv[3:])
        return 0
    sock = opt(argv, "-S")
    if sock is None:
        print("tmux stub: -S SOCKET is required", file=sys.stderr)
        return 2
    i = argv.index("-S") + 2
    if argv[i:i + 1] == ["-f"]:
        i += 2
    verb, rest = argv[i], argv[i + 1:]
    if verb == "new-session":
        j, cwd = 0, os.getcwd()
        while j < len(rest) and rest[j].startswith("-"):
            if rest[j] in ("-s", "-x", "-y", "-c"):
                if rest[j] == "-c":
                    cwd = rest[j + 1]
                j += 2
            else:
                j += 1
        subprocess.Popen([sys.executable, os.path.abspath(__file__), "__relay", sock, cwd] + rest[j:],
                         start_new_session=True, stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        for _ in range(100):
            if alive(sock):
                return 0
            time.sleep(0.05)
        return 1
    if verb == "load-buffer":
        with open(rest[-1], "rb") as src, open(sock + ".buf-" + opt(rest, "-b"), "wb") as dst:
            dst.write(src.read())
        return 0
    if verb == "paste-buffer":
        path = sock + ".buf-" + opt(rest, "-b")
        with open(path, "rb") as f:
            data = f.read()
        append(sock, b"\x1b[200~" + data + b"\x1b[201~" if "-p" in rest else data)
        if "-d" in rest:
            os.remove(path)
        return 0
    if verb == "send-keys":
        keys = [k for k in rest if k not in ("-t", opt(rest, "-t"), "-l")]
        for k in keys:
            append(sock, k.encode() if "-l" in rest else {"Enter": b"\r"}.get(k, k.encode()))
        return 0
    if verb == "capture-pane":
        with open(sock + ".pane", "rb") as f:
            sys.stdout.write(f.read()[-4096:].decode("utf-8", "replace"))
        return 0
    if verb == "has-session":
        return 0 if alive(sock) else 1
    if verb == "kill-server":
        try:
            with open(sock + ".pid") as f:
                pids = [int(p) for p in f.read().split()]
        except FileNotFoundError:
            return 1
        for pid in reversed(pids):
            try:
                os.kill(pid, signal.SIGTERM)
            except ProcessLookupError:
                pass
        for suffix in ("", ".pid"):
            try:
                os.remove(sock + suffix)
            except FileNotFoundError:
                pass
        return 0
    print("tmux stub: %s is not stubbed" % verb, file=sys.stderr)
    return 2


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
