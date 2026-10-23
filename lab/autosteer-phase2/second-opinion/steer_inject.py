#!/usr/bin/env python3
"""Lab delivery hook: hand pending AutoSteer notices to the model through Claude Code's own hook channel.

Install per arm (PostToolUse and SessionStart), after the rashomon entries:
    python3 ~/rashomon-eval/steer/second-opinion/steer_inject.py --arm sonnet

The watcher (steerci) appends notices to  ci/pending/<arm>.jsonl :
    {"id": "P2-24", "urgency": "immediate|deferred", "text": "<one sentence of fact with its receipt>"}
This hook, on PostToolUse, emits up to 3 of them (immediate first, oldest first) as
hookSpecificOutput.additionalContext, which Claude Code inserts where the hook fired and the model reads on
its next request (hooks reference, 2.1.280). Delivered lines move to ci/pending/<arm>.delivered.jsonl with the
tool_use_id and time, so "acted" in the ledger becomes a measured read, not a send.
On SessionStart with source == "compact" it re-emits ci/pending/<arm>.task.txt (task digest and rulings) once.

Rules: never blocks (no decision field, always exit 0); any error -> silent exit 0; under 50 ms; at most 3 notes
per call (the boundary budget). Nothing here runs project code (I1). Content is the architect's, not this file's.
"""
import json, os, sys, time

ROOT = os.environ.get("STEER_PENDING_DIR") or os.path.expanduser("~/rashomon-eval/steer/ci/pending")
MAX_NOTES = 3

def main():
    arm = sys.argv[sys.argv.index("--arm") + 1] if "--arm" in sys.argv else None
    if not arm:
        return
    try:
        payload = json.loads(sys.stdin.read() or "{}")
    except Exception:
        payload = {}
    event = payload.get("hook_event_name", "")
    out = []
    if event == "SessionStart":
        if payload.get("source") == "compact":
            p = os.path.join(ROOT, arm + ".task.txt")
            if os.path.exists(p):
                out.append(open(p).read().strip())
    elif event in ("PostToolUse", "PostToolUseFailure", "UserPromptSubmit"):
        p = os.path.join(ROOT, arm + ".jsonl")
        if not os.path.exists(p):
            return
        lines = [l for l in open(p).read().splitlines() if l.strip()]
        notes = []
        for l in lines:
            try:
                notes.append(json.loads(l))
            except Exception:
                continue
        order = {"immediate": 0, "deferred": 1}
        notes.sort(key=lambda n: order.get(n.get("urgency", "deferred"), 1))
        take, rest = notes[:MAX_NOTES], notes[MAX_NOTES:]
        if not take:
            return
        rec = {"at": time.strftime("%H:%M:%S"), "tool_use_id": payload.get("tool_use_id"),
               "session": payload.get("session_id"), "event": event}
        with open(os.path.join(ROOT, arm + ".delivered.jsonl"), "a") as f:
            for n in take:
                f.write(json.dumps(dict(n, **rec)) + "\n")
        tmp = p + ".tmp"
        with open(tmp, "w") as f:
            for n in rest:
                f.write(json.dumps(n) + "\n")
        os.replace(tmp, p)
        for n in take:
            out.append("[%s] %s" % (n.get("id", "?"), n.get("text", "")))
        if rest:
            out.append("%d more notices wait for the next boundary." % len(rest))
    if out:
        sys.stdout.write(json.dumps({"hookSpecificOutput": {"hookEventName": event,
                                                            "additionalContext": "\n".join(out)}}))

if __name__ == "__main__":
    try:
        main()
    except Exception:
        pass
    sys.exit(0)
