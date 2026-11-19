#!/usr/bin/env python3
"""hookctx.py pre|post: probe 2's hook (probe2.sh loads it through --settings, for the probe session only).

Appends one JSON line per event to $PROBE_DIR/hooks.ndjson: the event, the tool, the permission_mode Claude Code
reports and the tool_input's key names (no edit body, path or prompt word: the D160 discipline). On Edit, Write and
MultiEdit it prints additionalContext: "probe-a-<nonce A>" at PreToolUse and "probe-b-<nonce B>" at PostToolUse
($PROBE_NONCE_A, $PROBE_NONCE_B; two nonces, so a sighting names its channel). Every other tool prints nothing.
Exit 0 always: the probe observes and never decides a permission.
"""
import json, os, sys, time

phase = sys.argv[1] if len(sys.argv) > 1 else "?"
try:
    d = json.load(sys.stdin)
except ValueError:
    d = {}
ti = d.get("tool_input") or {}
tool = d.get("tool_name")
emit = tool in ("Edit", "Write", "MultiEdit")
row = {
    "t": round(time.time(), 3),
    "phase": phase,
    "event": d.get("hook_event_name"),
    "tool": tool,
    "permission_mode": d.get("permission_mode"),
    "input_keys": sorted(ti.keys()) if isinstance(ti, dict) else None,
    "emitted": emit,
}
with open(os.path.join(os.environ["PROBE_DIR"], "hooks.ndjson"), "a") as f:
    f.write(json.dumps(row, sort_keys=True) + "\n")
if emit:
    if phase == "pre":
        event, mark = "PreToolUse", "probe-a-" + os.environ.get("PROBE_NONCE_A", "none")
    else:
        event, mark = "PostToolUse", "probe-b-" + os.environ.get("PROBE_NONCE_B", "none")
    print(json.dumps({"hookSpecificOutput": {"hookEventName": event, "additionalContext": mark}}))
sys.exit(0)
