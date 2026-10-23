"""The full AutoSteer Phase 2 record: every item, every AutoSteer error, every message, every instrument and every
owner decision, generated from the data files, never typed.

Inputs (ci/): ledger.jsonl (build_ledger.py), calls.jsonl, notices.jsonl, decisions.jsonl, instruments.json.
Integrity checks run first; any failure is printed in the record as an open defect, not hidden.

Run: python3 build_ledger.py && python3 record.py [out.md]
Default out: ~/.k-hidden-accept/phase2/AUTOSTEER-RECORD.md (hidden-class content: it names held-back inputs).
"""
import collections
import json
import os
import re
import sys
import time

import chimes
from chimes import CI_RUN_MIN, DISPATCH, LATE_MIN, PUBLIC, SPEC_NAMED, minutes, start_of

HERE = os.path.dirname(os.path.abspath(__file__))
OUT = os.path.expanduser("~/.k-hidden-accept/phase2/AUTOSTEER-RECORD.md")
ARM = {"opus": "O (py27-opus)", "sonnet": "S (py27-sonnet)", "blank": "B (py27-sonnet-ctx, blank)",
       "controller": "AutoSteer", "all": "all three arms"}
ROOT = {"A": "claim with no running instrument", "B": "ground truth wrong", "C": "identity and keys",
        "D": "verdict prose not derived from evidence", "E": "record typed, not derived", "F": "ops and guards"}
CLASS = {"caught": "caught in time", "late": "caught late", "pending": "found, not yet told", "missed": "missed",
         "unmeasured": "not measured (no instrument)", "error": "AutoSteer error"}


def jl(name):
    return [json.loads(l) for l in open(os.path.join(HERE, name)) if l.strip()]


def hhmm(t):
    m = re.search(r"(\d\d:\d\d(?::\d\d)?)", t or "")
    return m.group(1) if m else ""


def plus(t, mins):
    """A time (HH:MM[:SS], possibly inside a longer string) plus mins, as HH:MM, wrapping at midnight."""
    t = hhmm(t)
    h, m = int(t[:2]), int(t[3:5])
    x = (h * 60 + m + int(round(mins))) % (24 * 60)
    return "%02d:%02d" % (x // 60, x % 60)


def now():
    return time.strftime("%H:%M:%S")


def messages():
    msgs = {}
    for r in jl("calls.jsonl"):
        msgs[r["id"]] = {"id": r["id"], "t": hhmm(r["t"]), "to": r.get("session", ""), "kind": r.get("trigger", ""),
                         "msg": r.get("msg", ""), "text": r.get("said", "")}
    for r in jl("notices.jsonl"):
        msgs[r["id"]] = {"id": r["id"], "t": hhmm(r["t"]), "to": r.get("arm", ""), "kind": "CI notice",
                         "msg": r.get("msg", ""), "text": "%d failing check(s): %s" % (
                             len(r.get("keys", [])), "; ".join(r.get("keys", [])))}
    return msgs


def instrument_of(row, inv):
    hits = [i for i in inv if row["instrument"].startswith(i["match"])]
    return hits[0] if len(hits) == 1 else None


def earliest(row):
    """What could have told the arm sooner, and by when."""
    inst, cls = row["instrument"], row["class"]
    if cls == "caught":
        return "told within %s min of appearing: in time." % row["lag"]
    if cls == "unmeasured":
        return "needs the %s." % inst
    actual = chimes.bad_until(row)
    st = hhmm(start_of(row))
    if inst.startswith(PUBLIC) and row.get("built") and st:
        tag = "an input the spec named at dispatch" if inst.startswith(SPEC_NAMED) else \
            "a probe invented after dispatch (hindsight)"
        gain = (float(actual) - CI_RUN_MIN) if actual else None
        than = ("than it did" if row["lag"] else "than the arm fixed it itself" if row.get("fixed")
                else "than now; it has not been told yet")
        return ("run on every commit from dispatch, this instrument (%s) would have told the arm by %s, %s min "
                "earlier %s." % (tag, plus(st, CI_RUN_MIN), "%.0f" % gain if gain is not None else "?", than))
    if inst.startswith("hidden") or inst.startswith("second-opinion"):
        return ("a hidden-class input: held to the handoff by policy. Under the canary policy proposed in O01, a "
                "public instance of the same class could have run on every commit from %s." % (st or "appearance"))
    if inst.startswith("referee"):
        return ("a judgement call: the earliest is when the referee reads the commit or message. Actual lag %s min."
                % (actual or "?"))
    if "not built" in inst or "not run" in inst:
        return "needs the %s; no built instrument sees it." % inst
    return "%s; actual lag %s min." % (inst, actual or "?")


def checks(rows, msgs, inv, dec):
    out = []
    bad = chimes.lint(rows)
    out.append(("ledger lint (recorded times, lags, unique ids)", not bad, "; ".join(bad)))
    miss, off = [], []
    for r in rows:
        ac = r.get("acted") or {}
        cid = ac.get("chime")
        if not cid:
            continue
        if cid not in msgs:
            miss.append("%s cites %s" % (r["id"], cid))
        elif hhmm(ac.get("t")) and hhmm(ac["t"])[:5] != msgs[cid]["t"][:5]:
            off.append("%s acted %s, %s sent %s" % (r["id"], ac["t"], cid, msgs[cid]["t"]))
    out.append(("every act cites a message that exists", not miss, "; ".join(miss)))
    out.append(("every act's time equals its message's send time (to the minute)", not off, "; ".join(off)))
    nums = sorted(int(k[3:]) for k in msgs)
    gaps = sorted(set(range(1, nums[-1] + 1)) - set(nums))
    out.append(("message ids P2-01..P2-%02d have no gaps" % nums[-1], not gaps, ", ".join("P2-%02d" % g for g in gaps)))
    unmatched = sorted({r["instrument"] for r in rows if r["class"] != "error" and not instrument_of(r, inv)})
    out.append(("every item's instrument is in the inventory (exactly one match)", not unmatched, "; ".join(unmatched)))
    nodec = [d["id"] for d in dec if not d.get("source")]
    out.append(("every owner decision cites its source", not nodec, ", ".join(nodec)))
    noroot = [r["id"] for r in rows if r["class"] == "error" and r.get("root") not in ROOT]
    out.append(("every AutoSteer error has a root cause class", not noroot, ", ".join(noroot)))
    return out


def item_block(r, msgs):
    ap, fo, ac, fx = (r.get(k) or {} for k in ("appeared", "found", "acted", "fixed"))
    lines = ["#### %s, %s: %s" % (r["id"], ARM.get(r["session"], r["session"]), r["what"]), ""]
    lines.append("- Spec: %s. Class: %s%s. Status: %s." % (
        r["spec"], CLASS[r["class"]], " (hand label '%s' overridden by the clock)" % r["relabelled"]
        if r.get("relabelled") else "", r["status"]))
    st = start_of(r)
    lines.append("- Appeared: %s%s." % (" ".join(x for x in (ap.get("t", ""), ap.get("ref", "")) if x) or "-",
                                        "; AutoSteer's clock starts at dispatch %s" % DISPATCH
                                        if st == DISPATCH and hhmm(ap.get("t")) != DISPATCH else ""))
    lines.append("- Found: %s." % (" ".join(x for x in (fo.get("by", ""), fo.get("t", "")) if x) or "-"))
    if ac:
        m = msgs.get(ac.get("chime"), {})
        lines.append("- Acted: %s in %s (msg %s), %s min after the clock started." % (
            ac.get("t", ""), ac.get("chime", ""), m.get("msg", "?"), r["lag"] or "?"))
    else:
        lines.append("- Acted: %s." % ("not yet (held to the handoff)" if r["class"] == "pending" else
                                        "never: the arm fixed it first" if fx else
                                        "never called" if r["class"] == "missed" else "not yet"))
    if fx:
        lines.append("- Fixed: %s%s." % (" ".join(x for x in (fx.get("t", ""), fx.get("ref", "")) if x),
                                         ", %s min after the act" % r["to_fix"] if r["to_fix"] else ""))
    lines.append("- Could have been caught earlier: %s" % earliest(r))
    if r.get("receipt"):
        lines.append("- Receipt: %s." % r["receipt"].rstrip("."))
    return lines + [""]


def error_block(r):
    fo, fx = r.get("found") or {}, r.get("fixed") or {}
    ap = r.get("appeared") or {}
    lines = ["#### %s: %s" % (r["id"], r["what"]), ""]
    lines.append("- Root cause: %s (%s). Label: %s." % (r.get("root", "?"), ROOT.get(r.get("root"), "?"), r["spec"]))
    lines.append("- Found by %s%s." % (fo.get("by", "-"), " at %s" % fo["t"] if fo.get("t") else ""))
    if r.get("reached_arm"):
        lines.append("- Reached an arm: yes, in %s." % (" ".join(x for x in (ap.get("t", ""), ap.get("ref", "")) if x)
                                                       or "a message"))
    else:
        lines.append("- Reached an arm: no.")
    lines.append("- Status: %s%s." % (r["status"], "; fix: %s" % " ".join(
        x for x in (fx.get("t", ""), fx.get("ref", "")) if x) if fx else ""))
    lines.append("- Guard now: %s (%s)." % (r.get("guard", "-"), r.get("guard_kind", "-")))
    if r.get("repeats"):
        lines.append("- Repeats: %s." % r["repeats"])
    if r.get("receipt"):
        lines.append("- Receipt: %s." % r["receipt"].rstrip("."))
    return lines + [""]


def finder(r):
    by, st = (r.get("found") or {}).get("by", ""), r.get("status", "")
    if "peer" in by:
        return "peer audit"
    if "arm" in by:
        return "an arm"
    return "me, before sending" if "before sending" in st or "before use" in st else "me, after the fact"


def main(out_path):
    chimes.ledger_controls()
    rows = chimes.ledger_rows()
    msgs = messages()
    dec = jl("decisions.jsonl")
    invdoc = json.load(open(os.path.join(HERE, "instruments.json")))
    inv = invdoc["instruments"]
    res = checks(rows, msgs, inv, dec)
    items = [r for r in rows if r["class"] != "error"]
    errs = [r for r in rows if r["class"] == "error"]
    n = collections.Counter(r["class"] for r in rows)
    should = sum(n[k] for k in ("caught", "late", "pending", "missed"))
    o = []
    o += ["# AutoSteer Phase 2: the full record (generated by ci/record.py; do not edit by hand)", "",
          "As of %s on 2026-10-23, in flight. Dispatch %s on 2026-10-22." % (now(), DISPATCH), "",
          "Sources: ci/ledger.jsonl (from build_ledger.py), calls.jsonl, notices.jsonl, decisions.jsonl and "
          "instruments.json. Times come from commits, the transcript, `date` or instrument records. Lags, classes "
          "and found times are computed, never typed.", "",
          "Definitions:",
          "- Appeared: the first commit where the failure is observable (the commit replay), or the typed reference.",
          "- Clock: AutoSteer's clock for an item starts at the later of its appearance and dispatch.",
          "- Lag: minutes from the clock start to the act. In time: lag within %d min. Late: anything longer." % LATE_MIN,
          "- Pending: found by AutoSteer, not yet told to the arm. Missed: never told before the arm fixed it, or never called.",
          "- Earlier catch: if the item's instrument had run on every commit from dispatch, one CI round (%d min, "
          "the measured p75) after the clock start." % CI_RUN_MIN, ""]
    o += ["## Integrity checks on this record", "", "| check | result | detail |", "|---|---|---|"]
    o += ["| %s | %s | %s |" % (c, "pass" if ok else "FAIL", d.replace("|", "/") or "-") for c, ok, d in res]
    o += ["", "## Totals", "",
          "| | count |", "|---|---|",
          "| Items AutoSteer should catch | %d |" % should,
          "| Acted on in time (within %d min) | %d |" % (LATE_MIN, n["caught"]),
          "| Acted on late | %d |" % n["late"],
          "| Found, not yet told | %d |" % n["pending"],
          "| Missed | %d |" % n["missed"],
          "| Not measured (no instrument) | %d |" % n["unmeasured"],
          "| AutoSteer's own errors | %d |" % n["error"], ""]
    per = collections.Counter((r["session"], r["class"]) for r in items)
    o += ["| arm | in time | late | not yet told | missed |", "|---|---|---|---|---|"]
    for a in ("opus", "sonnet", "blank"):
        o.append("| %s | %d | %d | %d | %d |" % (ARM[a], per[(a, "caught")], per[(a, "late")], per[(a, "pending")],
                                               per[(a, "missed")]))
    o += chimes.counterfactual(rows)[1:]
    o += ["", "## 1. Every item, by arm", ""]
    order = ("caught", "late", "pending", "missed")
    for a in ("opus", "sonnet", "blank", "all"):
        sel = [r for r in items if r["session"].startswith(a) or (a == "all" and r["class"] == "unmeasured")]
        sel = [r for r in sel if r["class"] in order] if a != "all" else sel
        if not sel:
            continue
        o += ["### %s: %d items" % (ARM.get(a, a) if a != "all" else "Not measured, all arms", len(sel)), ""]
        for c in (order if a != "all" else ("unmeasured",)):
            for r in [x for x in sel if x["class"] == c]:
                o += item_block(r, msgs)
    o += ["## 2. Every AutoSteer error (%d)" % len(errs), ""]
    by_root = collections.Counter(r.get("root") for r in errs)
    o += ["| root cause | errors | ids |", "|---|---|---|"]
    for k in sorted(ROOT):
        o.append("| %s. %s | %d | %s |" % (k, ROOT[k], by_root[k], ", ".join(r["id"] for r in errs if r.get("root") == k)))
    by_kind = collections.Counter(r.get("guard_kind") for r in errs)
    o += ["", "| guard now | errors |", "|---|---|"] + ["| %s | %d |" % (k, v) for k, v in by_kind.most_common()]
    by_finder = collections.Counter(finder(r) for r in errs)
    o += ["", "| found by | errors |", "|---|---|"] + ["| %s | %d |" % (k, v) for k, v in by_finder.most_common()]
    reached = [r["id"] for r in errs if r.get("reached_arm")]
    o += ["", "- Reached an arm: %d (%s)." % (len(reached), ", ".join(reached)),
          "- Repeats of an earlier error or lesson: %d (%s)." % (
              len([r for r in errs if r.get("repeats")]),
              ", ".join("%s repeats %s" % (r["id"], r["repeats"]) for r in errs if r.get("repeats"))), ""]
    for r in sorted(errs, key=lambda x: int(x["id"][1:])):
        o += error_block(r)
    o += ["## 3. Every message AutoSteer sent (%d)" % len(msgs), ""]
    acted_by = collections.defaultdict(list)
    for r in rows:
        cid = (r.get("acted") or {}).get("chime")
        if cid:
            acted_by[cid].append(r["id"])
    for k in sorted(msgs, key=lambda x: int(x[3:])):
        m = msgs[k]
        o += ["#### %s, %s, to %s: %s" % (k, m["t"], ARM.get(m["to"], m["to"]), m["kind"]), "",
              "- Message id: %s." % (m["msg"] or "-"),
              "- Said: %s" % (m["text"] or "-"),
              "- Items it acted on first: %s." % (", ".join(acted_by[k]) or "none (a spec answer or a re-statement)"), ""]
    o += ["## 4. Every instrument", ""]
    for i in inv:
        got = [r for r in items if instrument_of(r, inv) is i]
        cc = collections.Counter(r["class"] for r in got)
        o += ["#### %s (%s)" % (i["name"], i["kind"]), "",
              "- File: %s. Built: %s." % (i["file"], i["built"]),
              "- Measures: %s." % i["measures"],
              "- Oracle: %s. Control: %s." % (i["oracle"], i["control"]),
              "- Items: %d (%s): %s." % (len(got), ", ".join("%s %d" % (CLASS[c], v) for c, v in cc.most_common()) or "-",
                                         ", ".join(r["id"] for r in got) or "-"),
              "- Blind spots: %s." % "; ".join(i["blind"]), ""]
    for k, s in invdoc["supporting"].items():
        if k == "planned":
            o += ["- Planned, not built: %s." % "; ".join(s), ""]
        else:
            o += ["#### %s" % s["name"], "", "- File: %s." % s["file"], "- Measures: %s." % s["measures"],
                  "- Blind spots: %s." % "; ".join(s["blind"]), ""]
    o += ["## 5. Every owner decision, direction and open question", ""]
    for kind, title in (("decision", "Decided"), ("direction", "Directions"), ("request", "Requests"),
                        ("open", "Open, awaiting the owner")):
        sel = [d for d in dec if d["kind"] == kind]
        o += ["### %s: %d" % (title, len(sel)), ""]
        for d in sel:
            o += ["- %s, %s. %s%s Effect: %s. Source: %s." % (
                d["id"], d["t"], d["q"] + " " if d["q"] and not d["q"].startswith("(") else "",
                "Answer: \"%s\"." % d["a"] if d["a"] else "", d["effect"], d["source"])]
        o.append("")
    text = "\n".join(o) + "\n"
    open(out_path, "w").write(text)
    failed = [c for c, ok, _ in res if not ok]
    print("wrote %s: %d items, %d errors, %d messages, %d instruments, %d decisions; checks failed: %d%s" % (
        out_path, len(items), len(errs), len(msgs), len(inv), len(dec), len(failed),
        (" (" + "; ".join(failed) + ")") if failed else ""))
    return 1 if failed else 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1] if len(sys.argv) > 1 else OUT))
