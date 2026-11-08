#!/usr/bin/env python3
"""A1a, A1b, A1c for one run directory (section 7.4). Prints one JSON object; writes judge/a1.json.

A1a per consumers.txt item: opened (a Read or a reading Bash of the file), searched (Grep, Glob, LS, Bash naming
the file or its directory), or none, counted before the first edit tool call.
A1b: the assistant's text before its first edit names the reuse symbol or file and at least one consumer file.
A1c (R, G): the first UserPromptSubmit hook's additionalContext names, per line, the unit, the reuse target, a
consumer and a knowledge line (PL11, LAW10, RES-2 or D161); off-task knowledge lines are counted.
"""
import json, os, sys
sys.path.insert(0, os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", ".."))
from judge.common import load_jsonl, tool_calls, first_edit_index, texts_before_first_edit, call_mentions, hook_events, additional_context

D = sys.argv[1].rstrip("/")
HERE = os.path.dirname(os.path.abspath(__file__))
items = [l.split("\t") for l in open(os.path.join(HERE, "..", "consumers.txt")) if l.strip() and not l.startswith("#")]
items = [(k, p, n.split()) for k, p, n in (x[:3] for x in items)]
tr = load_jsonl(os.path.join(D, "transcript.jsonl"))
calls = tool_calls(tr)
fe = first_edit_index(calls)
before = calls if fe is None else calls[:fe]
a1a = {}
for kind, rel, _ in items:
    best = "none"
    for _, name, inp in before:
        r = call_mentions(name, inp, rel)
        if r == "opened":
            best = "opened"; break
        if r == "searched":
            best = "searched"
    a1a[rel] = best
text = "\n".join(texts_before_first_edit(tr))
reuse_named = any(n in text for k, _, names in items if k == "reuse" for n in names)
consumer_named = any(n in text for k, _, names in items if k == "consumer" for n in names)
a1b = bool(reuse_named and consumer_named)
a1c = None
ev = hook_events(os.path.join(D, "hooks.log"))
first_prompt = next((e for e in ev if e["verb"] == "recap" and e["args"].startswith("pending")), None)
if first_prompt is not None:
    lines = []
    for ctx in additional_context(first_prompt["stdout"]):
        lines.extend(l for l in ctx.splitlines() if l.strip())
    knowledge_terms = ("PL11", "unknown", "LAW10", "100 lines", "RES-2", "regexp", "D161", "truth table", "truth_table")
    a1c = {
        "lines": len(lines),
        "unit": any("internal/platform" in l for l in lines),
        "reuse": any(("Callers" in l or "callers.go" in l) for l in lines),
        "consumer": any(any(p in l for _, p, _ in items if _ != "reuse") for l in lines),
        "knowledge": sum(1 for l in lines if any(t in l for t in knowledge_terms)),
        "lines_text": lines,
    }
out = {"run": os.path.basename(D), "arm": open(os.path.join(D, "arm")).read().strip() if os.path.exists(os.path.join(D, "arm")) else "",
       "first_edit_call": fe, "calls_before_first_edit": len(before), "a1a": a1a, "a1b": a1b, "a1c": a1c}
os.makedirs(os.path.join(D, "judge"), exist_ok=True)
json.dump(out, open(os.path.join(D, "judge", "a1.json"), "w"), indent=1)
print(json.dumps(out))
