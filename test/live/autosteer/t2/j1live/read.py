#!/usr/bin/env python3
"""read.py RUNDIR: J1-LIVE's verdicts (pre-registration r3 section 8; the plan line's channel is POST only, the director
2026-11-11 16:40:01 PST). Reads RUNDIR/transcript.jsonl (run.sh's copy) and beside it RUNDIR/j1/steps.tsv,
RUNDIR/hooks.log and RUNDIR/store/. Verdicts come from transcript records (record = 1-based line number), never from
what the model says.
Each ExitPlanMode call is classed by its tool_result: accepted (not an error), rejected (an error: the driver's
"No, keep planning"), unavailable (an error reading "No such tool available": the tool is off, as under -p).
For every call: its tool_use and tool_result records, and the hook_success and hook_additional_context attachment
records whose attachment.toolUseID is the call's id, by hookName (PreToolUse:ExitPlanMode, PostToolUse:ExitPlanMode).
  REJECTED_EXIT_LINE                none (EXPECTED: a stated product fact, a debt row, not a failure) | present
  PLAN_POST_LINE_FOR_ACCEPTED_EXIT  yes when exactly one PostToolUse:ExitPlanMode hook_additional_context record
                                    carries the accepted call's id (EXPECTED yes); NO otherwise, with the count
  PLAN_PRE_LINE                     FLAGGED when any PreToolUse:ExitPlanMode hook_additional_context exists on any call
                                    (16:40:01 item 2), else none
  RECORD_ORDER_ACCEPTED             precedes yes | NO | vacuous: the accepted call's Post line record against the
                                    session's first Edit, Write or MultiEdit tool_use record; vacuous when there is no
                                    such tool_use, never read as shown (16:29:38 item 2)
For the rejected call it reports any line that attached and that line's order the same way.
Beside, never a verdict: attachment counts by (type, hookName); hooks.log's "== " headers; every store line holding
"plan" (the steer log key plan, the record's plan event; formats fixed by W-M2.6); the driver's steps; the transcript
cost estimate (usd(), kept equal to drive.py's) against the 0.50 USD cap.
A reader: exit 0 when the transcript read, 2 when it is missing.
"""
import collections, json, os, sys

D = sys.argv[1].rstrip("/") if len(sys.argv) > 1 else ""
TP = os.path.join(D, "transcript.jsonl")
if not D or not os.path.exists(TP):
    print("read.py: STOP: no transcript.jsonl in %r" % D, file=sys.stderr)
    sys.exit(2)
EDITS = ("Edit", "Write", "MultiEdit")
CAP_USD = 0.50
recs = []
for line in open(TP, errors="replace"):
    try:
        recs.append(json.loads(line))
    except ValueError:
        recs.append({})


def blocks(d):
    m = d.get("message")
    c = m.get("content") if isinstance(m, dict) else None
    return [b for b in c if isinstance(b, dict)] if isinstance(c, list) else []


def usd(recs):
    """Kept equal to drive.py's usd(): Sonnet 5 list prices per MTok, usage deduplicated by message id."""
    per, models = {}, set()
    for d in recs:
        if d.get("type") != "assistant":
            continue
        m = d.get("message") or {}
        u = m.get("usage") or {}
        models.add(m.get("model"))
        mid = m.get("id") or id(d)
        p = per.setdefault(mid, {})
        for k in ("input_tokens", "cache_creation_input_tokens", "cache_read_input_tokens", "output_tokens"):
            p[k] = max(p.get(k, 0), int(u.get(k) or 0))
        cc = u.get("cache_creation") or {}
        p["h1"] = max(p.get("h1", 0), int(cc.get("ephemeral_1h_input_tokens") or 0))
    total = 0.0
    for p in per.values():
        cw = p["cache_creation_input_tokens"]
        h1 = min(p["h1"], cw)
        total += (p["input_tokens"] * 2.0 + (cw - h1) * 2.5 + h1 * 4.0 + p["cache_read_input_tokens"] * 0.2
                  + p["output_tokens"] * 10.0) / 1e6
    return total, len(per), sorted(str(x) for x in models)


def text(v, n):
    s = v if isinstance(v, str) else json.dumps(v)
    s = " ".join(s.split())
    return s if len(s) <= n else s[:n] + "..."


calls = []          # ExitPlanMode ids in order
use_rec, res = {}, {}
first_edit = None   # (record, tool name, id)
hooks = collections.defaultdict(list)   # (id, kind, hookName) -> [(record, attachment)]
counts = collections.Counter()
for i, d in enumerate(recs, 1):
    if d.get("type") == "attachment":
        a = d.get("attachment") or {}
        counts[(a.get("type"), a.get("hookName"))] += 1
        if a.get("type") in ("hook_success", "hook_additional_context"):
            hooks[(a.get("toolUseID"), a.get("type"), a.get("hookName"))].append((i, a))
    for b in blocks(d):
        if b.get("type") == "tool_use":
            if b.get("name") == "ExitPlanMode" and b.get("id") not in use_rec:
                calls.append(b.get("id"))
                use_rec[b.get("id")] = i
            if b.get("name") in EDITS and first_edit is None:
                first_edit = (i, b.get("name"), b.get("id"))
        if b.get("type") == "tool_result" and b.get("tool_use_id") in use_rec and b.get("tool_use_id") not in res:
            body = text(b.get("content"), 200)
            cls = "accepted"
            if b.get("is_error"):
                cls = "unavailable" if "No such tool available" in body else "rejected"
            res[b.get("tool_use_id")] = (i, cls, body)

print("transcript %s: %d records; session %s" % (TP, len(recs), next((d.get("sessionId") for d in recs if d.get("sessionId")), "?")))
for f in ("knobs", "model", "tree-sha"):
    p = os.path.join(D, f)
    if os.path.exists(p):
        print("%s: %s" % (f, " | ".join(open(p).read().split("\n")).strip(" |")))
argv = os.path.join(D, "j1", "argv.json")
if os.path.exists(argv):
    a = json.load(open(argv))
    print("j1 launch: mode %s; received %s; dropped %s" % (a.get("launched_mode"), a.get("received"), a.get("dropped")))
print("first Edit, Write or MultiEdit tool_use: %s" % ("record %d (%s %s)" % first_edit if first_edit else "none"))
print("ExitPlanMode calls: %d" % len(calls))
for n, cid in enumerate(calls, 1):
    r = res.get(cid)
    print("  call %d %s: tool_use record %d; tool_result %s" % (n, cid, use_rec[cid], "record %d %s: %s" % r if r else "none"))
    for kind in ("hook_success", "hook_additional_context"):
        for ph in ("PreToolUse", "PostToolUse"):
            for rec, a in hooks.get((cid, kind, ph + ":ExitPlanMode"), []):
                if kind == "hook_success":
                    print("    record %d %s %s exitCode %s stdout %s" % (rec, kind, ph, a.get("exitCode"), text(a.get("stdout", ""), 200)))
                else:
                    print("    record %d %s %s content %s" % (rec, kind, ph, text(a.get("content"), 300)))

by_class = {}
for cid in calls:
    if cid in res:
        by_class.setdefault(res[cid][1], []).append(cid)
rejected = (by_class.get("rejected") or [None])[0]
accepted = next((c for c in by_class.get("accepted", []) if rejected is None or use_rec[c] > use_rec[rejected]), None)


def lines(cid, phase):
    return hooks.get((cid, "hook_additional_context", phase + ":ExitPlanMode"), [])


def order(rec):
    if first_edit is None:
        return "vacuous (no Edit, Write or MultiEdit tool_use in the session; record %d)" % rec
    if rec < first_edit[0]:
        return "precedes yes (line record %d < first %s tool_use record %d)" % (rec, first_edit[1], first_edit[0])
    return "NO (line record %d >= first %s tool_use record %d)" % (rec, first_edit[1], first_edit[0])


print("VERDICTS")
if rejected is None:
    print("REJECTED_EXIT_LINE not read: no rejected ExitPlanMode call (classes %s)" % {k: len(v) for k, v in by_class.items()})
else:
    got = lines(rejected, "PreToolUse") + lines(rejected, "PostToolUse")
    if not got:
        print("REJECTED_EXIT_LINE none (expected: the plan channel is Post only and a rejected exit fires no Post; a stated product fact, a debt row)")
    else:
        for rec, a in got:
            print("REJECTED_EXIT_LINE present: record %d %s; order %s" % (rec, a.get("hookName"), order(rec)))
if accepted is None:
    print("PLAN_POST_LINE_FOR_ACCEPTED_EXIT not read: no accepted ExitPlanMode call after the rejection")
    print("RECORD_ORDER_ACCEPTED not read")
else:
    post = lines(accepted, "PostToolUse")
    print("PLAN_POST_LINE_FOR_ACCEPTED_EXIT %s (%d Post line records for %s)" % ("yes" if len(post) == 1 else "NO", len(post), accepted))
    if post:
        print("RECORD_ORDER_ACCEPTED %s" % order(post[0][0]))
    else:
        print("RECORD_ORDER_ACCEPTED not read: no Post line")
pre_any = [(rec, cid) for cid in calls for rec, _ in lines(cid, "PreToolUse")]
print("PLAN_PRE_LINE %s" % ("FLAGGED: records %s" % pre_any if pre_any else "none"))

print("BESIDE")
print("attachment counts by (type, hookName):")
for (t, h), n in sorted(counts.items(), key=lambda x: (str(x[0][0]), str(x[0][1]))):
    print("  %s %s %d" % (t, h or "-", n))
hl = os.path.join(D, "hooks.log")
if os.path.exists(hl):
    heads = [l.rstrip("\n") for l in open(hl, errors="replace") if l.startswith("== ")]
    print("hooks.log: %d events" % len(heads))
    for l in heads:
        print("  " + text(l, 160))
else:
    print("hooks.log: absent")
st = os.path.join(D, "store")
shown = 0
for root, _, names in os.walk(st):
    for n in sorted(names):
        p = os.path.join(root, n)
        try:
            body = open(p, errors="strict").read()
        except (UnicodeDecodeError, OSError):
            continue
        for k, l in enumerate(body.splitlines(), 1):
            if "plan" in l and shown < 60:
                print("  store %s:%d %s" % (os.path.relpath(p, D), k, text(l, 300)))
                shown += 1
print("store lines holding \"plan\": %d shown%s" % (shown, " (60 max)" if shown >= 60 else ""))
sp = os.path.join(D, "j1", "steps.tsv")
if os.path.exists(sp):
    print("driver steps:")
    for l in open(sp).read().splitlines()[1:]:
        print("  " + text(l, 220))
cost, nmsg, models = usd(recs)
print("cost: transcript estimate %.4f USD over %d assistant messages, models %s; cap %.2f: %s"
      % (cost, nmsg, models, CAP_USD, "under" if cost <= CAP_USD else "OVER"))
