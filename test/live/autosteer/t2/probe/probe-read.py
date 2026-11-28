#!/usr/bin/env python3
"""probe-read.py RUNDIR: the plan probe's four verdicts, from the run directory plan-probe.sh wrote.

Reads result.json (the session's end), hooks.ndjson (hooklog.py's rows, in order), transcript.jsonl (the session as
Claude Code recorded it) and tree/calc.go. Prints the hook sequence (event, tool, permission_mode), the ExitPlanMode
tool results (is_error and the first 120 characters: harness text, the probe's own toy plan), and:
  PRE_FIRES                  a PreToolUse row for ExitPlanMode
  LEAVES_PLAN_AND_EDITS      a PostToolUse row for Edit or Write after the first ExitPlanMode PreToolUse, and calc.go
                             declares Double
  PRE_CONTEXT_REACHES_MODEL  the transcript holds probe-pre-<nonce> (attachment-typed lines counted beside)
  POST_CONTEXT_REACHES_MODEL the same for probe-post-<nonce>
A reader: exit 0 when the files read, 2 when one is missing; the verdicts are for the director.
"""
import json, os, sys

D = sys.argv[1].rstrip("/")


def need(name):
    p = os.path.join(D, name)
    if not os.path.exists(p):
        print("probe-read: STOP: no %s" % name, file=sys.stderr)
        sys.exit(2)
    return p


r = json.load(open(need("result.json")))
nonce = open(need("nonce")).read().strip()
rows = [json.loads(l) for l in open(need("hooks.ndjson")) if l.strip()] if os.path.exists(os.path.join(D, "hooks.ndjson")) else []
tlines = open(need("transcript.jsonl"), errors="replace").read().splitlines()
calc = open(os.path.join(D, "tree", "calc.go")).read() if os.path.exists(os.path.join(D, "tree", "calc.go")) else ""

denials = r.get("permission_denials") or []
print("claude %s; model %s" % (open(os.path.join(D, "claude-version")).read().strip(), open(os.path.join(D, "model")).read().strip()))
print("result: subtype %s is_error %s turns %s cost %s; permission_denials %d %s"
      % (r.get("subtype"), r.get("is_error"), r.get("num_turns"), r.get("total_cost_usd"), len(denials),
         sorted({str(x.get("tool_name")) for x in denials if isinstance(x, dict)})))
print("hooks (%d rows):" % len(rows))
for x in rows:
    extra = (" plan_bytes %d" % x["plan_bytes"]) if "plan_bytes" in x else ""
    print("  %s %s %s mode=%s%s" % (x.get("phase"), x.get("event"), x.get("tool"), x.get("permission_mode"), extra))

# ExitPlanMode tool_use ids, then their tool_result blocks
ids = set()
results = []
for l in tlines:
    try:
        d = json.loads(l)
    except ValueError:
        continue
    content = ((d.get("message") or {}).get("content")) if isinstance(d.get("message"), dict) else None
    if not isinstance(content, list):
        continue
    for b in content:
        if not isinstance(b, dict):
            continue
        if b.get("type") == "tool_use" and b.get("name") == "ExitPlanMode":
            ids.add(b.get("id"))
        if b.get("type") == "tool_result" and b.get("tool_use_id") in ids:
            c = b.get("content")
            text = c if isinstance(c, str) else " ".join(x.get("text", "") for x in (c or []) if isinstance(x, dict))
            results.append((b.get("is_error"), " ".join(text.split())[:120]))
print("ExitPlanMode tool_use %d; tool_result %d" % (len(ids), len(results)))
for e, t in results:
    print("  is_error %s: %s" % (e, t))

first_exit = next((i for i, x in enumerate(rows) if x.get("phase") == "pre" and x.get("tool") == "ExitPlanMode"), None)
edits_after = [x for i, x in enumerate(rows) if first_exit is not None and i > first_exit
               and x.get("phase") == "post" and x.get("tool") in ("Edit", "Write")]
pre_mark, post_mark = "probe-pre-" + nonce, "probe-post-" + nonce


def seen(mark):
    anyl = sum(1 for l in tlines if mark in l)
    att = sum(1 for l in tlines if mark in l and "hook_additional_context" in l)
    return anyl, att


pa, pt = seen(pre_mark)
qa, qt = seen(post_mark)
yn = lambda b: "yes" if b else "NO"
print("PRE_FIRES %s" % yn(first_exit is not None))
print("LEAVES_PLAN_AND_EDITS %s (edit posts after the first ExitPlanMode %d; calc.go declares Double %s)"
      % (yn(bool(edits_after) and "func Double" in calc), len(edits_after), yn("func Double" in calc)))
print("PRE_CONTEXT_REACHES_MODEL %s (transcript lines %d, attachment-typed %d)" % (yn(pa > 0), pa, pt))
print("POST_CONTEXT_REACHES_MODEL %s (transcript lines %d, attachment-typed %d)" % (yn(qa > 0), qa, qt))
