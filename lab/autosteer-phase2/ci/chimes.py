"""Every time AutoSteer chimed in on a session: what it said, the direction before and after, and the
owner's optional feedback on whether it was right and what could be better.

Working name only; the owner has not named the command yet.

Sources:
  calls.jsonl     referee chime-ins (trigger calls and handoff verdicts), written by the controller
  notices.jsonl   CI notices; their before/after comes from events.jsonl (opened and cleared per foul key)
  feedback.jsonl  the owner's feedback. Each line has the shape of a future rashomon record:
                  {"schema": 1, "kind": "steer_feedback", "chime": ID, "session": S,
                   "verdict": "right" | "wrong" | "partly", "better": TEXT, "at": ISO-8601, "by": "owner"}
                  verdict is a closed vocabulary; better is optional free text, shared only by choice.

Usage:
  python3 chimes.py                     the table
  python3 chimes.py show ID             one chime-in in full, with its feedback
  python3 chimes.py feedback ID right|wrong|partly ["what could be better"]
  python3 chimes.py ledger [--md PATH]  every item AutoSteer should catch: caught (with when it acted), late,
                                        pending, missed, unmeasured, and its own errors; --md writes the doc
"""
import json
import os
import re
import sys
import time

HERE = os.path.dirname(os.path.abspath(__file__))
VERDICTS = ("right", "wrong", "partly")
CI_IDS = {"opus": "P2-07", "sonnet": "P2-08", "blank": "P2-09"}  # first CI notices; later ones get P2-CI-n


def jl(name):
    p = os.path.join(HERE, name)
    return [json.loads(ln) for ln in open(p) if ln.strip()] if os.path.exists(p) else []


def ci_chimes():
    events = jl("events.jsonl")
    called = json.load(open(os.path.join(HERE, "called.json"))) if os.path.exists(os.path.join(HERE, "called.json")) else {}
    out = []
    for n, nt in enumerate(jl("notices.jsonl")):
        keys = set(nt["keys"]) - set(called.get(nt["arm"], []))  # fouls called before CI belong to those calls
        cleared = [e for e in events if e["kind"] == "cleared" and e["arm"] == nt["arm"] and e["key"] in keys
                   and e["t"] >= nt["t"]]
        done = {e["key"] for e in cleared}
        status = "fixed" if done >= keys else ("partly" if done else "open")
        last = max(cleared, key=lambda e: e["t"]) if cleared else None
        out.append({
            "id": nt.get("id") or (CI_IDS.get(nt["arm"]) if nt.get("kind") == "ci-first" else "P2-CI-%d" % n),
            "t": nt["t"], "session": nt["arm"], "trigger": "CI", "msg": nt["msg"],
            "said": "%d failing check(s): %s" % (len(keys), "; ".join(sorted(k.split(":", 1)[1][:40] for k in keys))[:200]),
            "before": {"commit": nt["head"][:7], "t": nt["t"][11:19], "state": "%d failing" % len(keys)},
            "after": ({"commit": last["head"][:7], "t": last["t"][11:19],
                       "state": "%d of %d cleared" % (len(done), len(keys))} if last else None),
            "status": status})
        if nt.get("after"):  # an event foul (level) never clears by itself: the referee records what followed
            out[-1].update(after=nt["after"], status=nt.get("status", status))
    return out


def all_chimes():
    return sorted(jl("calls.jsonl") + ci_chimes(), key=lambda c: c["t"])


def mins(a, b):
    if not a or not b:
        return ""
    ta, tb = [int(x[0:2]) * 60 + int(x[3:5]) + int(x[6:8]) / 60.0 for x in (a, b)]
    return "%.0f min" % (tb - ta) if tb >= ta else ""


def feedback_by_chime():
    fb = {}
    for f in jl("feedback.jsonl"):
        fb[f["chime"]] = f  # latest wins
    return fb


def cut(s, n):
    s = " ".join(str(s).split())
    return s if len(s) <= n else s[: n - 1] + "~"


def table():
    cs, fb = all_chimes(), feedback_by_chime()
    per = {}
    for c in cs:
        p = per.setdefault(c["session"], {"n": 0, "fixed": 0, "open": 0, "standing": 0, "partly": 0})
        p["n"] += 1
        p[c["status"]] = p.get(c["status"], 0) + 1
    print("AutoSteer chime-ins, Phase 2: %d in total" % len(cs))
    for s in sorted(per):
        p = per[s]
        print("  %-7s %2d   fixed %d, partly %d, standing %d, open %d" % (s, p["n"], p["fixed"], p["partly"], p["standing"], p["open"]))
    print()
    hdr = "%-6s %-7s %-5s %-8s %-46s %-50s %-7s %-8s %s" % ("id", "session", "sent", "trigger", "what AutoSteer said",
                                                             "before -> after", "to fix", "status", "feedback")
    print(hdr)
    print("-" * len(hdr))
    for c in cs:
        b, a = c["before"], c.get("after")
        ba = "%s -> %s" % (b["state"], a["state"] if a else "(nothing yet)")
        f = fb.get(c["id"])
        print("%-6s %-7s %-5s %-8s %-46s %-50s %-7s %-8s %s" % (
            c["id"], c["session"], c["t"][11:16], cut(c["trigger"], 8), cut(c["said"], 46), cut(ba, 50),
            mins(c["t"][11:19], a and a.get("t")), c["status"], f["verdict"] if f else "-"))
    print("\nFeedback (optional): python3 %s feedback ID right|wrong|partly \"what could be better\"" % os.path.abspath(__file__))


def show(cid):
    for c in all_chimes():
        if c["id"] == cid:
            print(json.dumps(dict(c, feedback=feedback_by_chime().get(cid)), indent=1))
            return 0
    print("no chime-in %s" % cid)
    return 1


def feedback(cid, verdict, better=""):
    ids = {c["id"]: c for c in all_chimes()}
    if cid not in ids:
        print("no chime-in %s; ids: %s" % (cid, " ".join(sorted(ids))))
        return 1
    if verdict not in VERDICTS:
        print("verdict must be one of %s" % ", ".join(VERDICTS))
        return 1
    rec = {"schema": 1, "kind": "steer_feedback", "chime": cid, "session": ids[cid]["session"], "verdict": verdict,
           "better": better, "at": time.strftime("%Y-%m-%dT%H:%M:%S%z"), "by": "owner"}
    with open(os.path.join(HERE, "feedback.jsonl"), "a") as fh:
        fh.write(json.dumps(rec) + "\n")
    print("recorded: %s %s%s" % (cid, verdict, (" - " + better) if better else ""))
    return 0


# ---------- the ledger: every item AutoSteer should catch, caught or not ----------
# ledger.jsonl, one record per item (curated by the referee, each with its receipt):
#   {"id", "session", "spec", "what", "appeared": {"t", "ref"}, "found": {"t", "by"},
#    "acted": {"t", "chime"} | null, "fixed": {"t", "ref"} | null, "status", "class", "instrument", "built", "receipt"}
# class: caught (acted within LATE_MIN of appearing), late (acted later), pending (found, not yet told to the arm),
#        missed (no instrument saw it until tonight), unmeasured (no instrument yet), error (AutoSteer itself wrong)
LATE_MIN = 10
CLASSES = [("caught", "Caught and acted on in time (within %d min of appearing)" % LATE_MIN),
           ("late", "Caught, but late"),
           ("pending", "Found by AutoSteer, not yet told to the arm"),
           ("missed", "Missed: AutoSteer never told the arm (no instrument saw it, or it was held) before the arm fixed it or the review found it"),
           ("unmeasured", "Not measured yet: no instrument exists"),
           ("error", "AutoSteer's own errors (wrong, overstated or unchecked claims)")]


def minutes(a, b):
    """Minutes from a to b, both HH:MM[:SS] on the same evening; '' when either is missing."""
    ma, mb = [re.search(r"(\d\d):(\d\d)(?::(\d\d))?", x or "") for x in (a, b)]
    if not ma or not mb:
        return ""

    def f(m):  # the run spans midnight: an hour before noon is the next day
        h = int(m.group(1))
        return (h + 24 if h < 12 else h) * 60 + int(m.group(2)) + int(m.group(3) or 0) / 60.0
    d = f(mb) - f(ma)
    return "%.0f" % d if d >= 0 else ""


DISPATCH = "20:38:08"  # Phase 2 dispatch (transcript msg 59c79b0d); AutoSteer's clock cannot start before it


def start_of(r):
    """When AutoSteer's clock starts for an item: its appearance, or dispatch if it was there before dispatch."""
    ap = (r.get("appeared") or {}).get("t") or ""
    return ap if minutes(DISPATCH, ap) != "" else (DISPATCH if re.search(r"\d\d:\d\d", ap) else "")


def bad_until(r, now=None):
    """Minutes the item stayed known-bad and untold: to the act if told, else to the arm's own fix, else to now."""
    if r.get("lag"):
        return r["lag"]
    end = (r.get("fixed") or {}).get("t") or now or time.strftime("%H:%M:%S")
    return minutes(start_of(r), end)


def derive(rows):
    """Lag and class come from the clock, never from a hand label (E20): lag runs from the later of the
    appearance and dispatch to the act; an acted item is 'caught' if that lag is within LATE_MIN, else 'late'."""
    for r in rows:
        r["lag"] = minutes(start_of(r), (r.get("acted") or {}).get("t"))
        r["to_fix"] = minutes((r.get("acted") or {}).get("t"), (r.get("fixed") or {}).get("t"))
        if r["class"] in ("caught", "late"):
            cls = "caught" if r["lag"] != "" and float(r["lag"]) <= LATE_MIN else "late"
            if cls != r["class"]:
                r["relabelled"], r["class"] = r["class"], cls
    return rows


def lint(rows):
    """Gates before the ledger is written: every time present is a recorded HH:MM, every acted item has a lag,
    ids are unique. A failure blocks the write (C52: no estimated times)."""
    bad = []
    for r in rows:
        for f in ("appeared", "found", "acted", "fixed"):
            t = (r.get(f) or {}).get("t")
            if t and not re.search(r"\d\d:\d\d", t):
                bad.append("%s: %s time %r is not a recorded time" % (r["id"], f, t))
        if r.get("acted") and r["lag"] == "":
            bad.append("%s: acted but no lag (appeared %r)" % (r["id"], (r.get("appeared") or {}).get("t")))
    ids = [r["id"] for r in rows]
    bad += ["%s: duplicate id" % i for i in sorted({i for i in ids if ids.count(i) > 1})]
    return bad


def ledger_controls():
    """Positive controls for derive() and lint(): each must fire on the case it exists for."""
    rows = derive([
        {"id": "c1", "class": "late", "appeared": {"t": "19:23:37"}, "acted": {"t": "20:40:08"}},   # pre-dispatch: lag 2
        {"id": "c2", "class": "caught", "appeared": {"t": "00:30:24"}, "acted": {"t": "00:57:27"}},  # L61's shape: 27
        {"id": "c3", "class": "late", "appeared": {"t": "23:59:00"}, "acted": {"t": "00:05:00"}},    # across midnight: 6
        {"id": "c4", "class": "pending", "appeared": {"t": "20:38:08"}, "found": {"t": "23:2x"}},    # estimated time
    ])
    got = {r["id"]: (r["class"], r["lag"]) for r in rows}
    fixed_by_arm = {"class": "missed", "appeared": {"t": "20:54:57"}, "fixed": {"t": "23:48:48"}, "lag": ""}
    assert bad_until(fixed_by_arm, now="01:50:00") == "174", bad_until(fixed_by_arm, now="01:50:00")  # ends at the fix
    assert got == {"c1": ("caught", "2"), "c2": ("late", "27"), "c3": ("caught", "6"), "c4": ("pending", "")}, got
    assert [b for b in lint(rows)] == ["c4: found time '23:2x' is not a recorded time"], lint(rows)
    print("ledger controls ok")


def ledger_rows():
    return derive(jl("ledger.jsonl"))


CI_RUN_MIN = 8  # one CI round on one commit: gaps between results.jsonl records at 01:01, median 4.1, p75 8.1 min
PUBLIC = ("in-loop CI", "visible CI", "post-freeze")  # automatic, on public inputs: may run on every commit
SPEC_NAMED = ("in-loop CI", "visible CI")  # inputs the spec named at dispatch; post-freeze probes were invented later


def counterfactual(rows):
    """For late, pending and missed items whose instrument is built: when the arm would have been told had that
    instrument run on every commit from dispatch. Hidden-input checks stay at handoff by policy, so they are
    counted apart, not moved."""
    pub = [r for r in rows if r["class"] in ("late", "pending", "missed") and r.get("built")
           and r["instrument"].startswith(PUBLIC) and re.search(r"\d\d:\d\d", (r.get("appeared") or {}).get("t") or "")]
    lost, now_in_time, spec, later = 0.0, [], [], []
    for r in pub:
        actual = r["lag"] or bad_until(r)
        if actual and float(actual) > CI_RUN_MIN:
            lost += float(actual) - CI_RUN_MIN
            now_in_time.append(r["id"])
            (spec if r["instrument"].startswith(SPEC_NAMED) else later).append(r["id"])
    hid = [r["id"] for r in rows if r["class"] in ("late", "pending", "missed") and r.get("built")
           and not r["instrument"].startswith(PUBLIC) and "hidden" in r["instrument"]]
    caught = len([r for r in rows if r["class"] == "caught"])
    obs = len([r for r in rows if r["class"] in ("caught", "late", "pending", "missed")]) - len(hid)
    pct = lambda k: "%d of %d (%d%%)" % (k, obs, round(100.0 * k / obs)) if obs else "-"
    return ["", "## Counterfactual: every built public-input instrument running on every commit from dispatch", "",
            "- One CI round takes 4 min at the median and 8 at the 75th percentile; with %d, each item below would have "
            "reached its arm within %d min of the commit that introduced it." % (CI_RUN_MIN, CI_RUN_MIN),
            "- Items that would have moved to 'in time': %d" % len(now_in_time),
            "  - on inputs the spec named at dispatch (the 2.7 stdlib, item 3's hostile list, the arm's own tests): "
            "%d (%s)" % (len(spec), ", ".join(spec)),
            "  - on probes invented after dispatch (hindsight; an upper bound, not a miss of the spec's inputs): "
            "%d (%s)" % (len(later), ", ".join(later)),
            "- Item-minutes of known-bad state this removes: %d (sum over those items of actual lag "
            "minus %d; an item never told counts to the arm's own fix, or to now if unfixed; as of %s, so it grows while "
            "items stay untold)" % (round(lost), CI_RUN_MIN, time.strftime("%H:%M")),
            "- Found by the hidden-input check, held for the handoff by policy (not moved): %d (%s)" % (len(hid), ", ".join(hid)),
            "- In time, of the items observable without the hidden inputs: now %s; spec-named instruments from "
            "dispatch %s; every built public instrument from dispatch %s" % (
                pct(caught), pct(caught + len(spec)), pct(caught + len(now_in_time)))]


def ledger(md_path=None):
    ledger_controls()
    rows = ledger_rows()
    bad = lint(rows)
    if bad:
        print("ledger not written; %d check(s) failed:\n  %s" % (len(bad), "\n  ".join(bad)))
        return 1
    out = []
    n = {k: len([r for r in rows if r["class"] == k]) for k, _ in CLASSES}
    should = sum(n[k] for k in ("caught", "late", "pending", "missed"))
    acted = n["caught"] + n["late"]
    out.append("# AutoSteer ledger, Phase 2 (generated by ci/chimes.py ledger from ci/ledger.jsonl; do not edit by hand)")
    out.append("")
    out.append("Dispatch %s. Every time is a commit time, a send time from the transcript, `date`, or the first "
               "timestamped instrument record. Lag runs from the later of appearance and dispatch to the act; "
               "the in-time or late class is computed from it, never typed." % DISPATCH)
    relab = [r["id"] for r in rows if r.get("relabelled")]
    if relab:
        out.append("")
        out.append("- Hand labels the clock overrode: %d (%s)" % (len(relab), ", ".join(relab)))
    out.append("")
    out.append("- Items AutoSteer should have caught so far: %d" % should)
    out.append("- Acted on: %d (%d in time, %d late), that is %d%% of them" % (acted, n["caught"], n["late"], round(100.0 * acted / should) if should else 0))
    out.append("- Found, not yet told to the arm: %d" % n["pending"])
    out.append("- Missed until tonight: %d" % n["missed"])
    out.append("- Not measured yet: %d" % n["unmeasured"])
    out.append("- AutoSteer's own errors: %d" % n["error"])
    gaps = {}
    for r in rows:
        if r["class"] in ("late", "pending", "missed", "unmeasured"):
            g = gaps.setdefault(r["instrument"], {"built": r.get("built"), "items": []})
            g["items"].append(r["id"])
    for key, title in CLASSES:
        sel = [r for r in rows if r["class"] == key]
        if not sel:
            continue
        out += ["", "## %s: %d" % (title, len(sel)), ""]
        out.append("| id | session | spec | what | appeared | found by | acted | lag min | status | fixed | min to fix | receipt |")
        out.append("|---|---|---|---|---|---|---|---|---|---|---|---|")
        for r in sel:
            ap, fo, ac, fx = r.get("appeared") or {}, r.get("found") or {}, r.get("acted") or {}, r.get("fixed") or {}
            out.append("| %s | %s | %s | %s | %s | %s | %s | %s | %s | %s | %s | %s |" % (
                r["id"], r["session"], r["spec"], r["what"].replace("|", "/"),
                " ".join(x for x in (ap.get("t", ""), ap.get("ref", "")) if x),
                " ".join(x for x in (fo.get("by", ""), fo.get("t", "")) if x),
                " ".join(x for x in (ac.get("t", ""), ac.get("chime", "")) if x) or "-",
                r["lag"] or "-", r["status"], " ".join(x for x in (fx.get("t", ""), fx.get("ref", "")) if x) or "-",
                r["to_fix"] or "-", r.get("receipt", "").replace("|", "/")))
    out += counterfactual(rows)
    out += ["", "## What would have caught the rest: %d instruments" % len(gaps), "",
            "| instrument | built | items it would have caught or caught earlier | count |", "|---|---|---|---|"]
    for inst, g in sorted(gaps.items(), key=lambda kv: -len(kv[1]["items"])):
        out.append("| %s | %s | %s | %d |" % (inst, "yes" if g["built"] else "no", ", ".join(g["items"]), len(g["items"])))
    text = "\n".join(out) + "\n"
    if md_path:
        open(md_path, "w").write(text)
        print("wrote %s (%d items)" % (md_path, len(rows)))
    else:
        print(text)
    return 0


def main(argv):
    if len(argv) < 2 or argv[1] == "table":
        table()
        return 0
    if argv[1] == "ledger":
        return ledger(argv[3] if len(argv) == 4 and argv[2] == "--md" else None)
    if argv[1] == "show" and len(argv) == 3:
        return show(argv[2])
    if argv[1] == "feedback" and len(argv) in (4, 5):
        return feedback(argv[2], argv[3], argv[4] if len(argv) == 5 else "")
    print(__doc__)
    return 2


if __name__ == "__main__":
    sys.exit(main(sys.argv))
