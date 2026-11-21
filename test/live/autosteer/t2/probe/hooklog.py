#!/usr/bin/env python3
"""hooklog.py pre|post: the plan probe's hook (plan-probe.sh loads it through --settings, for the probe session only).

Appends one JSON line per event to $PROBE_DIR/hooks.ndjson: the event, the tool, the permission_mode Claude Code
reports, the tool_input's key names and, for ExitPlanMode, the plan's length in bytes. No plan text, edit body or
prompt word is written (the D160 discipline the product keeps). On ExitPlanMode it prints additionalContext carrying
a nonce ($PROBE_NONCE), "probe-pre-<nonce>" at PreToolUse and "probe-post-<nonce>" at PostToolUse, so the reader can
find in the transcript whether either line reached the model. Every other event prints nothing. Exit 0 always: the
probe observes and never decides a permission.
"""
import json, os, sys, time

phase = sys.argv[1] if len(sys.argv) > 1 else "?"
try:
    d = json.load(sys.stdin)
except ValueError:
    d = {}
ti = d.get("tool_input") or {}
row = {
    "t": round(time.time(), 3),
    "phase": phase,
    "event": d.get("hook_event_name"),
    "tool": d.get("tool_name"),
    "permission_mode": d.get("permission_mode"),
    "input_keys": sorted(ti.keys()) if isinstance(ti, dict) else None,
}
if d.get("tool_name") == "ExitPlanMode" and isinstance(ti, dict) and isinstance(ti.get("plan"), str):
    row["plan_bytes"] = len(ti["plan"].encode("utf-8"))
with open(os.path.join(os.environ["PROBE_DIR"], "hooks.ndjson"), "a") as f:
    f.write(json.dumps(row, sort_keys=True) + "\n")
if d.get("tool_name") == "ExitPlanMode":
    event = "PreToolUse" if phase == "pre" else "PostToolUse"
    mark = "probe-%s-%s" % (phase, os.environ.get("PROBE_NONCE", "none"))
    print(json.dumps({"hookSpecificOutput": {"hookEventName": event, "additionalContext": mark}}))
sys.exit(0)
