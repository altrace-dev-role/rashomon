#!/usr/bin/env python3
"""probe2-read.py RUNDIR: probe 2's two verdicts, from the run directory probe2.sh wrote.

The director's question (2026-11-11 15:50:40 PST item 3): does a hook's hookSpecificOutput.additionalContext reach the
model, at PreToolUse and at PostToolUse, under `claude -p --permission-mode acceptEdits`? Verdicts come from the
transcript's attachment records (records with type "attachment"), never from what the model says:
  PRE_CONTEXT_REACHES_MODEL   an attachment record holds probe-a-<nonce A> (the PreToolUse hook's line)
  POST_CONTEXT_REACHES_MODEL  an attachment record holds probe-b-<nonce B> (the PostToolUse hook's line)
Printed beside, for the reader and never as a verdict: the hook rows in order; every transcript record holding either
mark (record number, record type, attachment type or the content block type it sits in); its place relative to the
edit's tool_use and tool_result; and whether the model's final reply quotes the mark (the prompt asks it to).
A reader: exit 0 when the files read, 2 when one is missing.
"""
import json, os, sys

D = sys.argv[1].rstrip("/")


def need(name):
    p = os.path.join(D, name)
    if not os.path.exists(p):
        print("probe2-read: STOP: no %s" % name, file=sys.stderr)
        sys.exit(2)
    return p


r = json.load(open(need("result.json")))
na = open(need("nonce-a")).read().strip()
nb = open(need("nonce-b")).read().strip()
hp = os.path.join(D, "hooks.ndjson")
rows = [json.loads(l) for l in open(hp) if l.strip()] if os.path.exists(hp) else []
tlines = open(need("transcript.jsonl"), errors="replace").read().splitlines()
calc = open(os.path.join(D, "tree", "calc.go")).read() if os.path.exists(os.path.join(D, "tree", "calc.go")) else ""
marks = {"A (pre)": "probe-a-" + na, "B (post)": "probe-b-" + nb}

print("claude %s; model %s" % (open(need("claude-version")).read().strip(), open(need("model")).read().strip()))
denials = r.get("permission_denials") or []
print("result: subtype %s is_error %s turns %s cost %s; permission_denials %d %s"
      % (r.get("subtype"), r.get("is_error"), r.get("num_turns"), r.get("total_cost_usd"), len(denials),
         sorted({str(x.get("tool_name")) for x in denials if isinstance(x, dict)})))
print("calc.go declares Double: %s" % ("yes" if "func Double" in calc else "NO"))
print("hooks (%d rows):" % len(rows))
for x in rows:
    print("  %s %s %s mode=%s emitted=%s" % (x.get("phase"), x.get("event"), x.get("tool"), x.get("permission_mode"), x.get("emitted")))

edit_use, edit_result = [], []
ids = set()
hits = {k: [] for k in marks}
for i, l in enumerate(tlines, 1):
    try:
        d = json.loads(l)
    except ValueError:
        continue
    m = d.get("message")
    content = m.get("content") if isinstance(m, dict) else None
    for b in (content if isinstance(content, list) else []):
        if not isinstance(b, dict):
            continue
        if b.get("type") == "tool_use" and b.get("name") in ("Edit", "Write", "MultiEdit"):
            ids.add(b.get("id")); edit_use.append(i)
        if b.get("type") == "tool_result" and b.get("tool_use_id") in ids:
            edit_result.append(i)
    for k, mark in marks.items():
        if mark not in l:
            continue
        where = d.get("type")
        if where == "attachment":
            a = d.get("attachment") or {}
            where = "attachment/%s" % a.get("type")
        elif isinstance(content, list):
            where = "%s/%s" % (where, ",".join(sorted({str(b.get("type")) for b in content if isinstance(b, dict) and mark in json.dumps(b)})))
        hits[k].append((i, where))
print("edit tool_use at records %s; their tool_result at %s" % (edit_use, edit_result))
final = str(r.get("result") or "")
for k, mark in marks.items():
    print("mark %s %s: records %s; final reply quotes it %s" % (k, mark, hits[k] or "none", "yes" if mark in final else "no"))
yn = lambda b: "yes" if b else "NO"
print("PRE_CONTEXT_REACHES_MODEL %s" % yn(any(w.startswith("attachment/") for _, w in hits["A (pre)"])))
print("POST_CONTEXT_REACHES_MODEL %s" % yn(any(w.startswith("attachment/") for _, w in hits["B (post)"])))
