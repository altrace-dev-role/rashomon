#!/usr/bin/env python3
"""A8 cost and latency for one run directory: cost USD, turns, duration, tool calls from result.json; per-event hook
milliseconds (R, G) from hook-ms.log with p95 per verb. Writes judge/a8.json."""
import json, os, sys
sys.path.insert(0, os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", ".."))
from judge.common import load_jsonl, tool_calls, result_json

D = sys.argv[1].rstrip("/")
r = result_json(os.path.join(D, "result.json"))
calls = tool_calls(load_jsonl(os.path.join(D, "transcript.jsonl")))
ms = {}
p = os.path.join(D, "hook-ms.log")
if os.path.exists(p):
    for line in open(p):
        parts = line.split()
        if len(parts) >= 3 and parts[2].isdigit():
            key = parts[0] + ("-" + parts[1] if parts[1] and not parts[1].startswith("--") else "")
            ms.setdefault(key, []).append(int(parts[2]))
def p95(xs):
    xs = sorted(xs); return xs[min(len(xs) - 1, int(round(0.95 * (len(xs) - 1))))] if xs else None
out = {"run": os.path.basename(D), "cost_usd": r.get("total_cost_usd"), "turns": r.get("num_turns"), "duration_ms": r.get("duration_ms"),
       "usage": r.get("usage"), "tool_calls": len(calls), "exit": open(os.path.join(D, "exit")).read().strip() if os.path.exists(os.path.join(D, "exit")) else None,
       "hook_ms": {k: {"n": len(v), "p95": p95(v), "max": max(v)} for k, v in ms.items()}}
os.makedirs(os.path.join(D, "judge"), exist_ok=True)
json.dump(out, open(os.path.join(D, "judge", "a8.json"), "w"), indent=1)
print(json.dumps(out))
