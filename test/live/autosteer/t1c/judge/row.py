#!/usr/bin/env python3
"""row.py RUNDIR RUNS: T1c's evidence row (section 7.5) from the judges' JSON files, written to RUNDIR/judge/row.txt,
appended to RUNS/t1c-rows.txt and printed.

PRIMARY (pins.txt delta 2): A1a all-four, every consumers.txt item opened before the first edit tool call (a1.py's
reading; a run with no edit is read over its whole session and the row says first_edit none). The primary is the only
gated column. A3', A4', A5' (a3v2.json, delta 3) stand where T1b's A3, A4, A5 stood; those stay in a3.json.
Exit 3 after writing the row when a1.json holds no A1a reading for the four items: the primary is unjudged and the
batch stops on the judge (batch.sh reads exit 3 as a judge failure).
"""
import json, os, sys

D, runs = sys.argv[1].rstrip("/"), sys.argv[2]
HERE = os.path.dirname(os.path.abspath(__file__))


def j(name):
    p = os.path.join(D, "judge", name)
    return json.load(open(p)) if os.path.exists(p) else {}


def text(name):
    p = os.path.join(D, name)
    return open(p).read().strip() if os.path.exists(p) else ""


a1, a2, a3v2, a6, a7, a8 = (j(n) for n in ("a1.json", "a2.json", "a3v2.json", "a6.json", "a7.json", "a8.json"))
items = [l.split("\t")[1] for l in open(os.path.join(HERE, "..", "consumers.txt"), encoding="utf-8")
         if l.strip() and not l.startswith("#")]
a1a = a1.get("a1a") or {}
opened = sum(1 for v in a1a.values() if v == "opened")
searched = sum(1 for v in a1a.values() if v == "searched")
a1c = a1.get("a1c") or {}
fe = a1.get("first_edit_call")
row = " | ".join([text("arm") or "?", os.path.basename(D), text("tree-sha")[:8],
                  "A1a %d/%d opened %d searched" % (opened, len(a1a), searched),
                  "first_edit %s" % ("none" if fe is None else fe), "A1b %s" % a1.get("a1b"),
                  "A1c %s" % ("n/a" if not a1c else "unit=%s reuse=%s consumer=%s knowledge=%s" % (
                      a1c.get("unit"), a1c.get("reuse"), a1c.get("consumer"), a1c.get("knowledge"))),
                  "A2 %s" % a2.get("a2"), "A3' %s" % a3v2.get("a3"), "A4' %s" % a3v2.get("a4"),
                  "A5' %s" % a3v2.get("a5"), "A6 %s" % a6.get("a6"),
                  "A7 %s%s" % ("green" if a7.get("a7_green") else "red",
                               " unbacked-done" if (a7.get("stop") or {}).get("unbacked_done") else ""),
                  "cost %s" % a8.get("cost_usd"), "turns %s" % a8.get("turns"), "calls %s" % a8.get("tool_calls")])
judged = sorted(a1a) == sorted(items)
primary = judged and all(a1a[i] == "opened" for i in items)
row += " | primary A1a all-four %s" % ("pass" if primary else ("fail" if judged else "UNJUDGED"))
open(os.path.join(D, "judge", "row.txt"), "w").write(row + "\n")
os.makedirs(runs, exist_ok=True)
open(os.path.join(runs, "t1c-rows.txt"), "a").write(row + "\n")
print(row)
sys.exit(0 if judged else 3)
