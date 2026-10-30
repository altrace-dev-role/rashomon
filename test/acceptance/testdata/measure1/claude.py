#!/usr/bin/env python3
"""A stand-in for the pinned claude, for measure1.sh's tests (card
MEASURE1). It runs the hook commands the arm's settings.json holds, through sh as
Claude Code does, with payloads of Claude Code's shape, so every record, steer
line and checkpoint the driver reads is the binary's own. It never answers a
prompt with a model: each turn is one Bash call and a Stop.

  --version          prints the version (stub.json "version", default 2.1.280)
  -p ...             replays the real 2.1.280 init event captured at 18:03
                     (init-2.1.280.jsonl, scrubbed as H-113 requires), its
                     arm's ids and paths substituted; the capture ran with no
                     login, so its "Not logged in" lines replay only under
                     stub.json "logged_out"
  otherwise          reads prompts from stdin as a terminal sends them: a
                     bracketed paste, then a carriage return; "/exit" ends

Each hook runs with the arm's environment plus what Claude Code adds for its
hooks: CLAUDECODE=1, CLAUDE_CODE_ENTRYPOINT=cli, CLAUDE_PROJECT_DIR (the
session's directory) and AI_AGENT in the harness form the binary reads
(internal/steer/client.go), so the binary sees the host it would see.

stub.json beside this file shapes a test's run: version, hang_at (a turn that
declares its call and never runs it), die_at, ignore_pastes (pastes dropped
before the first is read), extra_tool_for_arm (an init event that differs),
logged_out.
"""

import json
import os
import subprocess
import sys
import time
import uuid

HERE = os.path.dirname(os.path.abspath(__file__))
try:
    with open(os.path.join(HERE, "stub.json")) as f:
        KNOBS = json.load(f)
except FileNotFoundError:
    KNOBS = {}
VERSION = KNOBS.get("version", "2.1.280")


def arg(name, default=""):
    a = sys.argv
    return a[a.index(name) + 1] if name in a and a.index(name) + 1 < len(a) else default


def print_mode():
    sid = arg("--session-id", str(uuid.uuid4()))
    tmp = os.environ.get("CLAUDE_CODE_TMPDIR") or "/tmp"
    with open(os.path.join(HERE, "init-2.1.280.jsonl")) as f:
        events = [json.loads(l) for l in f if l.strip()]
    if not KNOBS.get("logged_out"):
        events = events[:1]
    for ev in events:
        if ev.get("session_id"):
            ev["session_id"] = sid
        if ev.get("subtype") == "init":
            ev.update(cwd=os.getcwd(), model=arg("--model", ev["model"]),
                      messaging_socket_path=os.path.join(tmp, "cc-socks", "%d.sock" % os.getpid()),
                      permissionMode=arg("--permission-mode", ev["permissionMode"]))
            if os.environ.get("RASHOMON_STEER_ARM") == KNOBS.get("extra_tool_for_arm"):
                ev["tools"] += 1
        print(json.dumps(ev))


class Session:
    def __init__(self):
        self.sid = arg("--session-id", str(uuid.uuid4()))
        self.cwd = os.getcwd()
        with open(arg("--settings")) as f:
            self.hooks = json.load(f).get("hooks", {})
        cfg = os.environ["CLAUDE_CONFIG_DIR"]
        proj = os.path.join(cfg, "projects", self.cwd.replace("/", "-").replace(".", "-"))
        os.makedirs(proj, exist_ok=True)
        self.transcript = os.path.join(proj, self.sid + ".jsonl")
        self.prompt_id = None
        self.n = 0
        self.env = dict(os.environ, CLAUDECODE="1", CLAUDE_CODE_ENTRYPOINT="cli", CLAUDE_PROJECT_DIR=self.cwd,
                        AI_AGENT="claude-code_%s_harness" % VERSION.replace(".", "-"))

    def write(self, entry):
        entry.setdefault("sessionId", self.sid)
        entry.setdefault("entrypoint", "cli")
        with open(self.transcript, "a") as f:
            f.write(json.dumps(entry) + "\n")

    def payload(self, event, **extra):
        p = {"session_id": self.sid, "transcript_path": self.transcript, "cwd": self.cwd,
             "permission_mode": arg("--permission-mode", "default"), "hook_event_name": event}
        if self.prompt_id:
            p["prompt_id"] = self.prompt_id
        p.update(extra)
        return p

    def fire(self, event, matcher_value="", **extra):
        """Runs event's hook commands; each output's model and user text goes
        into the transcript as an attachment, as the measured channel check
        expects to find it."""
        body = json.dumps(self.payload(event, **extra))
        for group in self.hooks.get(event, []):
            m = group.get("matcher", "")
            if m and m != "*" and matcher_value and matcher_value not in m.split("|"):
                continue
            for h in group.get("hooks", []):
                out = subprocess.run(["sh", "-c", h["command"]], input=body.encode(), capture_output=True,
                                     cwd=self.cwd, env=self.env, timeout=h.get("timeout", 60)).stdout.decode()
                for line in out.splitlines():
                    try:
                        doc = json.loads(line)
                    except ValueError:
                        continue
                    ctx = (doc.get("hookSpecificOutput") or {}).get("additionalContext")
                    for text in (ctx, doc.get("systemMessage")):
                        if text:
                            self.write({"type": "attachment", "attachment": {"type": "hook_output", "hookEvent": event, "content": text}})

    def turn(self, prompt):
        self.n += 1
        self.prompt_id = str(uuid.uuid4())
        self.write({"type": "user", "promptId": self.prompt_id, "message": {"role": "user", "content": prompt}})
        self.fire("UserPromptSubmit", prompt=prompt)
        if KNOBS.get("die_at") == self.n:
            sys.exit(1)
        tid = "toolu_%02d%s" % (self.n, uuid.uuid4().hex[:20])
        tool_input = {"command": "go test ./...", "description": "Run the tests"}
        self.write({"type": "assistant", "message": {"role": "assistant", "model": arg("--model"),
                    "content": [{"type": "tool_use", "id": tid, "name": "Bash", "input": tool_input}],
                    "usage": {"input_tokens": 10, "output_tokens": 5, "cache_read_input_tokens": 0}}})
        self.fire("PreToolUse", "Bash", tool_name="Bash", tool_input=tool_input, tool_use_id=tid)
        if KNOBS.get("hang_at") == self.n:
            while True:
                time.sleep(60)
        self.fire("PostToolUse", "Bash", tool_name="Bash", tool_input=tool_input, tool_use_id=tid,
                  tool_response={"stdout": "ok  \texample.invalid/task\t0.01s\n", "stderr": "", "interrupted": False, "isImage": False})
        self.fire("Stop", stop_hook_active=False, last_assistant_message="The tests pass.")

    def run(self):
        self.fire("SessionStart", source="startup")
        ignore = KNOBS.get("ignore_pastes", 0)
        buf, pasting = b"", False
        while True:
            c = sys.stdin.buffer.read(1)
            if not c:
                break
            buf += c
            if buf.endswith(b"\x1b[200~"):
                buf, pasting = b"", True
            elif buf.endswith(b"\x1b[201~"):
                buf, pasting = buf[:-6], False
            elif c in (b"\r", b"\n") and not pasting:
                text = buf[:-1].decode()
                buf = b""
                if text == "/exit":
                    break
                if not text:
                    continue
                if ignore > 0:
                    ignore -= 1
                    continue
                self.turn(text)
        self.fire("SessionEnd", reason="prompt_input_exit")


if __name__ == "__main__":
    if "--version" in sys.argv:
        print("%s (Claude Code)" % VERSION)
    elif "-p" in sys.argv:
        print_mode()
    else:
        Session().run()
