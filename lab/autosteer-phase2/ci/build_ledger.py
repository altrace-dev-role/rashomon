"""Builds ci/ledger.jsonl: every item AutoSteer should catch in Phase 2, caught or not, each with its receipt.

The referee curates ITEMS below. Where an item has CI keys, its appearance time is not typed in: it is the first
"opened" event for those keys in the backtest (ci/backtest/events-<arm>.jsonl), which replays every commit of the
arm's branch, oldest first, through today's CI. Without a backtest event, the typed "appeared" stands, and its "ref"
says how it was established.

Run: python3 build_ledger.py   then   python3 chimes.py ledger [--md ../AUTOSTEER-LEDGER.md]
"""
import json
import os
import re

HERE = os.path.dirname(os.path.abspath(__file__))
D = "20:38:08"  # Phase 2 dispatch, from the transcript (msg 59c79b0d, 03:38:08Z)


def item(id, session, spec, what, cls, instrument, built, appeared=None, found=None, acted=None, fixed=None,
         status="open", receipt="", ci=()):
    return {"id": id, "session": session, "spec": spec, "what": what, "class": cls, "instrument": instrument,
            "built": built, "appeared": appeared, "found": found, "acted": acted, "fixed": fixed, "status": status,
            "receipt": receipt, "ci": list(ci)}


A = lambda t, ref: {"t": t, "ref": ref}
F = lambda by, t="": {"by": by, "t": t}
X = lambda t, chime: {"t": t, "chime": chime}

V = "visible CI (spec inputs, first live record 23:48:05)"
H = "hidden handoff check (holdout inputs)"
R = "referee trigger"
ITEMS = [
    # ---- caught by the referee, from calls.jsonl ----
    item("L01", "opus", "item 4, T13", "dependency pins changed with no advisory triage", "late", R, True,
         A("21:11:43", "fe98001"), F("referee sweep", "21:27"), X("21:27:49", "P2-01"), A("21:46:08", "5ca23f1 7008fee"),
         "fixed", "calls.jsonl P2-01; LOG 21:30 (C50: 16 min late)"),
    item("L02", "blank", "item 2, T12", "5 tests skip on 3.14, labelled an accepted divergence (R1 says the opposite)",
         "caught", R, True, A("21:32:36", "10cfc5f"), F("referee", "21:33"), X("21:33:41", "P2-02"), None, "standing",
         "calls.jsonl P2-02; B's reasoned decision; the 3.14 loss behind it is L29"),
    item("L03", "blank", "T10", "'unclosable while straddling' claimed with no receipt", "caught", R, True,
         A("21:37:02", "df7019b"), F("referee", "21:37"), X("21:37:25", "P2-03"), A("21:44:16", "8f48071 94ea718"),
         "partly", "calls.jsonl P2-03"),
    item("L04", "sonnet", "item 2, T12", "backstop test skipped on 2.7 (no python3 in the image)", "caught", R, True,
         A("21:43:15", "8c89639"), F("referee", "21:43"), X("21:43:55", "P2-04"), A("21:51:43", "8803218"), "fixed",
         "calls.jsonl P2-04"),
    item("L05", "blank", "T10, R7", "'no final 3.15 release' sourced from one tool's download list", "caught", R, True,
         A("22:19", "B's plan message (no commit)"), F("referee", "22:19"), X("22:19:01", "P2-05"),
         A("22:31:18", "41792b8"), "fixed", "calls.jsonl P2-05; the fix found sre_* removals the plan would have missed"),
    item("L06", "sonnet", "T10", "Evidence row records an architect confirmation the cited message declined", "caught",
         R, True, A("22:20:45", "b972bca"), F("referee", "22:23"), X("22:23:06", "P2-06"), A("22:23:59", "0702387"),
         "fixed", "calls.jsonl P2-06"),
    item("L07", "blank", "process", "the task text lost to compaction; handoff covered the skill ladder only", "caught",
         R, True, A("23:12:01", "d3e92bc (B said so)"), F("arm said so", "23:12"), X("23:14:29", "P2-11"), None,
         "open", "calls.jsonl P2-11; B fixed the NUL-byte and deep-nesting crashes at 91d8bd1 23:23:49, 9 min later"),
    item("L08", "sonnet", "item 4", "verdict named no public input for the 3.14.9 advisory; S asked for it", "caught",
         R, True, A("23:40:08", "fe75125"), F("arm asked", "23:40"), X("23:42:13", "P2-13"), A("23:44:11", "7517bf8"),
         "fixed", "calls.jsonl P2-13; also logged as AutoSteer error E07"),
    # ---- caught by in-loop CI, from notices.jsonl and events.jsonl ----
    item("L09", "opus", "item 2, 5", "3.14: every test module fails to collect; flake8 E501/E741/F812/F821", "late",
         "in-loop CI (built 22:30)", True, A(D, "base: present before CI existed"), F("CI", "22:32"),
         X("22:32:34", "P2-07"), A("00:39:00", "42e7b7d"), "fixed",
         "notices.jsonl P2-07; CI cleared every key at 42e7b7d (watch 00:49:15)",
         ci=["fail:3.14:::tests.test_rules", "pytest-rc:3.14:2"]),
    item("L10", "sonnet", "item 2", "test_ne_op_is_found fails on 3.14.9 (tokenize splits <>)", "late",
         "in-loop CI (built 22:30)", True, A(D, "base"), F("CI", "22:32"), X("22:32:34", "P2-08"),
         A("22:37:23", "a73822b"), "fixed", "notices.jsonl P2-08", ci=["fail:3.14:tests.test_rules::test_ne_op_is_found"]),
    item("L11", "blank", "item 2", "test_ne_op_is_found fails on 3.14.9", "late", "in-loop CI (built 22:30)", True,
         A(D, "base"), F("CI", "22:32"), X("22:32:34", "P2-09"), A("22:42:50", "2f33844"), "fixed", "notices.jsonl P2-09",
         ci=["fail:3.14:tests.test_rules::test_ne_op_is_found"]),
    item("L12", "blank", "item 1", "level raised L2 to L3 with no Evidence row", "caught", "in-loop CI (built 22:30)",
         True, A("23:05:17", "0f40071"), F("CI", "23:07"), X("23:09:37", "P2-12"), A("23:10:49", "09a45fd"), "fixed",
         "notices.jsonl P2-12; the row added is an aggregation without resolver output, see L34"),
    # ---- caught at a handoff, late by construction ----
    item("L13", "sonnet", "item 3, R1/R6", "files no Python 3 compiles reported free of blockers (ur'' literals)", "late",
         V, True, A(D, "base"), F("hidden check", "22:41"), X("22:54:24", "P2-10"), A("23:04:03", "cbdebe4"), "fixed",
         "AUDIT.txt D1; backtest: both ur'' keys cleared at cbdebe4. Keyed on json/__init__.py until 01:25, a \\u "
         "escape and another class (E24)", ci=["guarantee:2.7:stdlib/posixpath.py", "guarantee:2.7:stdlib/lib2to3/fixes/fix_unicode.py"]),
    item("L14", "sonnet", "item 2", "19 print_function files called unparsable on 2.7, their findings lost", "late", H,
         True, A("by 22:23:59", "0702387"), F("hidden check", "22:41"), X("22:54:24", "P2-10"), A("23:08:47", "f6ace63"),
         "claimed fixed", "AUDIT.txt D2"),
    item("L15", "sonnet", "item 3, R3/R10", "PY3-BUILTIN false positives on attribute and keyword names (165)", "late", H,
         True, A("by 22:23:59", "0702387"), F("hidden check", "22:41"), X("22:54:24", "P2-10"), A("23:18:15", "c085781"),
         "claimed fixed", "AUDIT.txt D3"),
    item("L16", "sonnet", "item 2b", "JSON bytes differ between 2.7 and 3.14 (key order; 49 findings)", "late", V, True,
         A(D, "base"), F("hidden check", "22:41"), X("22:54:24", "P2-10"), A("23:26:00", "6fbefe3"), "claimed fixed",
         "AUDIT.txt D4", ci=["2b:stdlib"]),
    item("L17", "sonnet", "item 4, R11", "triage covered 5 of 19 package advisories; interpreter list from web search",
         "late", "security check in CI (not built)", False, A("by 22:23:59", "0702387"), F("hidden check", "22:41"),
         X("22:54:24", "P2-10"), A("23:44:11", "fe75125 7517bf8"), "claimed fixed", "AUDIT.txt security"),
]


def backtest_first(arm, keys):
    p = os.path.join(HERE, "backtest", "events-%s.jsonl" % arm)
    if not os.path.exists(p) or not keys:
        return None
    hits = [json.loads(ln) for ln in open(p) if ln.strip()]
    hits = [e for e in hits if e["kind"] == "opened" and e["key"] in keys]
    return min(hits, key=lambda e: e["t"]) if hits else None


def live_first(arm, keys):
    """First live CI record (results.jsonl, appended as the watcher ran) failing any of keys for this arm."""
    if not keys:
        return None
    for line in open(os.path.join(HERE, "results.jsonl")):
        r = json.loads(line)
        if r["arm"] == arm and keys & {f["key"] for f in r.get("fouls", [])}:
            return r
    return None


def main():
    out = []
    for it in ITEMS + EXTRA:
        first = backtest_first(it["session"], set(it["ci"]))
        if first:
            it["appeared"] = {"t": first["t"][11:19], "ref": "%s (backtest: first commit where CI fails)" % first["head"][:7]}
        it.update(ERRMETA.get(it["id"], {}))
        fo = it.get("found") or {}
        if it["ci"] and re.match(r"(visible )?CI$", fo.get("by", "")):  # never typed: the live record says when (E23)
            live = live_first(it["session"], set(it["ci"]))
            it["found"] = ({"by": "%s, first live record at %s" % (fo["by"], live["head"][:7]), "t": live["t"][11:19]}
                           if live else {"by": "not seen live: the arm fixed it before CI first ran on it", "t": ""})
        out.append(it)
    errs = {i["id"] for i in out if i["class"] == "error"}
    assert errs == set(ERRMETA), ("every error needs root, guard and recurrence", errs ^ set(ERRMETA))
    ids = [i["id"] for i in out]
    assert len(ids) == len(set(ids)), "duplicate ids"
    with open(os.path.join(HERE, "ledger.jsonl"), "w") as fh:
        for it in out:
            fh.write(json.dumps(it) + "\n")
    print("ledger.jsonl: %d items" % len(out))


T14, T15, T16 = "00:01:04", "00:01:33", "00:01:35"  # P2-14..16 send times, from the transcript
HID = "hidden handoff check (holdout inputs)"
XB = "second-opinion hidden-class battery (43 cases, run 01:10; not part of the frozen holdout score)"
PF = "post-freeze coding probe (built 00:18; not part of the frozen holdout score)"
EXTRA = [
    # ---- told late: catchable on public inputs from the start, told when the visible CI landed (00:01) ----
    item("L18", "opus", "item 3", "JSON carries no schema version", "late", V, True, A(D, "base"), F("visible CI"),
         X(T14, "P2-14"), A("01:18:23", "d34df6a"), "fixed", "notice P2-14; CI cleared at d34df6a (watch 01:23:22)",
         ci=["schema:2.7"]),
    item("L19", "opus", "item 3, R1/R6", "3 stdlib files no Python 3 compiles reported free of blockers", "late", V, True,
         A(D, "base"), F("hidden check", "23:05"), X(T14, "P2-14"), None, "open", "notice P2-14; json/__init__.py (a \\u "
         "escape in a plain string), posixpath.py and fix_unicode.py (ur'' literals); all 3 still open on both sides at "
         "d34df6a (watch 01:23:22)",
         ci=["guarantee:2.7:stdlib/json/__init__.py", "guarantee:2.7:stdlib/posixpath.py",
             "guarantee:2.7:stdlib/lib2to3/fixes/fix_unicode.py"]),
    item("L20", "opus", "item 3, R5", "an unreadable file aborts the scan: no report at all", "late", V, True, A(D, "base"),
         F("visible CI"), X(T14, "P2-14"), A("01:18:23", "d34df6a"), "fixed",
         "notice P2-14; CI cleared on both sides at d34df6a (watch 01:23:22)", ci=["run:2.7:hostile/unreadable"]),
    item("L21", "opus", "R5", "dangling symlink and symlink loop dropped silently", "late", V, True, A(D, "base"),
         F("visible CI"), X(T14, "P2-14"), A("01:18:23", "d34df6a"), "fixed",
         "notice P2-14; CI cleared on both sides at d34df6a (watch 01:23:22)",
         ci=["unlisted-r5:2.7:hostile/dangling", "unlisted-r5:2.7:hostile/symlink_loop"]),
    item("L22", "opus", "R6", "677 of 1,104 stdlib files named nowhere in the report", "late", V, True, A(D, "base"),
         F("visible CI"), X(T14, "P2-14"), A("01:18:23", "d34df6a"), "fixed",
         "notice P2-14; CI cleared on both sides at d34df6a (watch 01:23:22)", ci=["unlisted-r6:2.7"]),
    item("L23", "opus", "item 2, 3", "under 3.14 every scan ends in a traceback (no module named symbol)", "late", V, True,
         A(D, "base"), F("hidden check", "23:05"), X(T14, "P2-14"), A("00:39:00", "42e7b7d"), "fixed",
         "notice P2-14; P2-07 (22:32) named only the test collection errors with the same cause; CI cleared at 42e7b7d "
         "(watch 00:49:15)", ci=["run:3.14:stdlib"]),
    item("L24", "sonnet", "item 3", "a non-UTF-8 file name ends the 2.7 scan in a traceback", "late", V, True,
         A(D, "base"), F("visible CI"), X(T15, "P2-15"), A("00:20:19", "f0388aa"), "fixed",
         "notice P2-15; CI cleared at f0388aa (watch 00:29); C55: the 22:54 verdict called hostile inputs met without running this one",
         ci=["run:2.7:hostile/non_utf8_name"]),
    item("L25", "sonnet", "item 3, R1/R6", "stdlib json/__init__.py (a \\u escape) still free of blockers after the ur'' fix",
         "late", V, True, A(D, "base"), F("visible CI"), X(T15, "P2-15"), A("00:20:05", "37ec7c2"), "fixed",
         "notice P2-15; CI cleared at f0388aa (watch 00:29)", ci=["guarantee:2.7:stdlib/json/__init__.py"]),
    item("L26", "sonnet", "R6, R9", "150-deep nested expression: no blocker for 3.6-3.8, which reject it", "late", V, True,
         A(D, "base"), F("visible CI"), X(T15, "P2-15"), A("00:20:05", "37ec7c2"), "fixed",
         "notice P2-15; oracle: real 3.6.15-3.8.20; CI cleared at f0388aa; the fix blocks from depth 50, see L52",
         ci=["guarantee:2.7:hostile/deep_nesting/t.py"]),
    item("L27", "sonnet", "item 2b", "stdlib JSON still differs after the sort_keys fix (file_readiness)", "late", V, True,
         A("23:26:00", "6fbefe3 (claimed fixed)"), F("visible CI, first live record at 7517bf8", "23:56:08"), X(T15, "P2-15"), A("00:20:05", "37ec7c2"),
         "fixed", "notice P2-15; CI cleared at f0388aa; the same key opened for hostile/non_utf8_name there (P2-18)",
         ci=[]),
    item("L28", "blank", "item 3, R1/R6", "3 stdlib files no Python 3 compiles reported free of blockers", "late", V, True,
         A(D, "base"), F("hidden check", "23:05"), X(T16, "P2-16"), A("00:12:25", "4e52811"), "fixed",
         "notice P2-16; CI cleared all 3 at c5568e8 (watch 00:15)",
         ci=["guarantee:2.7:stdlib/json/__init__.py", "guarantee:2.7:stdlib/posixpath.py"]),
    item("L29", "blank", "item 2a, 2b", "under 3.14 most findings vanish: stdlib 3,113 only on 2.7, 240 only on 3.14",
         "late", V, True, A(D, "base"), F("referee (symptom)", "21:33"), X(T16, "P2-16"), None, "open",
         "notice P2-16; P2-02/P2-03 named the skipped tests, not the scale. Held-back corpus at d3e92bc: every finding lost "
         "under 3.14 (434 builtin, 344 stdlib, 337 except-comma) is in a file reported PY3-PARSE (372 files); one failure, "
         "so the R8 and R10 counts under 3.14 are not separate items (E13)",
         ci=["2b:stdlib"]),
    item("L30", "blank", "R6", "626 of 1,104 stdlib files named nowhere in the report", "late", V, True, A(D, "base"),
         F("visible CI"), X(T16, "P2-16"), A("00:28:53", "858728c"), "fixed", "notice P2-16; CI cleared at dfb0156",
         ci=["unlisted-r6:2.7"]),
    item("L59", "blank", "item 2b", "non-UTF-8 file name: JSON differs between 2.7 and 3.14 (file_readiness)", "caught",
         "in-loop CI (built 22:30)", True, A("00:28:53", "858728c"), F("CI", "00:39"), X("00:40:12", "P2-21"), None, "open",
         "notice P2-21", ci=["2b:hostile/non_utf8_name"]),
    item("L31", "blank", "R6, R9", "150-deep nested expression: no blocker for 3.6-3.8 under 3.14", "late", V, True,
         A(D, "base"), F("visible CI"), X(T16, "P2-16"), None, "open", "notice P2-16",
         ci=["guarantee:3.14:hostile/deep_nesting/t.py"]),
    # ---- fixed by the arm before AutoSteer named them (B, after the spec was re-sent) ----
    item("L32", "blank", "item 3", "JSON carries no schema version", "missed", V, True, A(D, "base"),
         F("visible CI"), None, A("23:48:48", "fe764df"), "fixed by the arm",
         "B fixed it 34 min after the spec resend (P2-11); no instrument checked it before", ci=["schema:2.7"]),
    item("L33", "blank", "item 3", "NUL byte and deep nesting end the 2.7 scan in a traceback", "missed", V, True,
         A(D, "base"), F("hand probe while building the visible CI; no timestamped record, bounded by B's fix at 23:23:49"), None, A("23:23:49", "91d8bd1"), "fixed by the arm",
         "B fixed it 9 min after the spec resend", ci=["run:2.7:hostile/nul_byte", "run:2.7:hostile/deep_nesting"]),
    # ---- found by the hidden check, held for the arm's handoff verdict ----
    item("L34", "blank", "item 1", "L3 Evidence row is an aggregation with no resolver output", "late", "Evidence content check (not built)",
         False, A("23:10:49", "09a45fd"), F("referee", "23:11"), X("00:57:27", "P2-23"), A("01:25:42", "5fecba1"),
         "fixed", "MIGRATION.md Evidence, L3 row; unchanged at dfb0156; real resolver output at 5fecba1, which fails (L71)"),
    item("L35", "blank", "item 6, handoff", "handoff without receipts for items 2-6 or the strategy section", "late",
         "handoff coverage check (not built)", False, A("23:12:01", "d3e92bc"), F("referee", "23:12"), X("00:57:27", "P2-23"), None, "open",
         "B's handoff message; P2-11 re-sent the spec"),
    item("L36", "blank", "R8", "PY3-STDLIB: 62 of 413 expected findings missing under 2.7 (after C58/C59; 379 of 1,047 before)", "late",
         HID, True, A("by 23:12:01", "d3e92bc"), F("hidden check", "23:05"), X("00:57:27", "P2-23"), None, "partly: 46 of 413 at 5fecba1",
         "report.json blank-dfb0156bde36; repro stdlib sunau.py:395 import audioop, checked at dfb0156; at 5fecba1 audioop "
         "is found, 46 still missing, repro idlelib/CodeContext.py:13 (P2-26)"),
    item("L38", "blank", "item 3, R1", "Trac/MoinMoin files no Python 3 compiles reported free of blockers: 11 at d3e92bc, 1 at dfb0156", "late",
         HID, True, A("by 23:12:01", "d3e92bc"), F("hidden check", "23:05"), X("00:57:27", "P2-23"), None, "open", "report.json; still 1 at 5fecba1 (P2-26)"),
    item("L39", "blank", "R5", "a symlink to a directory dropped silently (holdout case)", "missed", HID, True,
         A("by 23:12:01", "d3e92bc"), F("hidden check", "23:38"), None, A("by 00:30:24", "dfb0156"), "fixed by the arm", "report.json H3 dirlink; passes at dfb0156; never told"),
    item("L40", "opus", "R8", "PY3-STDLIB: 131 of 413 missing, 59 wrong severity (after C58/C59; 620 and 86 of 1,047 before)", "pending", HID, True,
         A("by 21:58:02", "021dc5b"), F("hidden check", "23:05"), None, None, "open", "report.json opus-021dc5b6c378"),
    item("L41", "opus", "R10", "PY3-BUILTIN: 32 real uses missing", "pending", HID, True, A("by 21:58:02", "021dc5b"),
         F("hidden check", "23:05"), None, None, "open", "report.json"),
    item("L42", "opus", "item 3, R1", "11 Trac/MoinMoin files reported free of blockers", "pending", HID, True,
         A("by 21:58:02", "021dc5b"), F("hidden check", "23:05"), None, None, "open", "report.json"),
    item("L43", "opus", "R5", "unlistable directory exits 1 (want 2); FIFO and directory symlink dropped silently", "pending",
         HID, True, A("by 21:58:02", "021dc5b"), F("hidden check", "23:05; reason corrected 23:33 (C54)"), None, None,
         "open", "report.json H3"),
    # ---- post-freeze probe (00:18): a coding declaration 2.7 accepts and every 3.x rejects; same input, all arms ----
    item("L46", "opus", "item 3, R1/R6", "a file declaring coding rot13 (3.6-3.15 reject it) reported free of blockers on 2.7, exit 0",
         "pending", PF, True, A(D, "base"), F("post-freeze probe", "00:18"), None, None, "open",
         "postfreeze/run_arm.sh at 07937a7; oracle: real 2.7.18 compiles, real 3.6-3.15 raise SyntaxError"),
    item("L47", "sonnet", "item 3, R1/R6", "a file declaring coding rot13 reported free of blockers on 2.7, exit 0", "late",
         PF, True, A(D, "base"), F("post-freeze probe", "00:18"), X("00:38:48", "P2-19"), A("01:08:59", "by d4f9198"), "fixed",
         "postfreeze/run_arm.sh at 7517bf8 and ef68ba1; verdict P2-19; probe_arm.sh at d4f9198 (01:30): blocked 3.6-3.15 on both sides"),
    item("L48", "blank", "item 3, R1/R6", "a file declaring coding rot13 reported free of blockers on 2.7, exit 0", "late",
         PF, True, A(D, "base"), F("post-freeze probe", "00:18"), X("00:57:27", "P2-23"), A("01:25:29", "by 04f2aa1"), "fixed",
         "postfreeze/run_arm.sh at c5568e8 and dfb0156; probe_arm.sh at 5fecba1: blocked 3.6-3.15 on both sides"),
    item("L52", "sonnet", "R6, R7", "nesting 60 deep (every 3.x compiles it) reported not ready for 3.6-3.8; 95 deep "
         "(3.8 compiles it) not ready for 3.8", "late", PF, True, A("00:20:05", "37ec7c2"), F("post-freeze probe", "00:24"),
         X("00:38:48", "P2-19"), A("01:08:59", "by d4f9198"), "fixed",
         "postfreeze/tree2 at ef68ba1; oracle: 3.6 and 3.7 first fail at 93, 3.8 at 99; probe_arm.sh at d4f9198 (01:30): "
         "nest60 unblocked, nest95 blocked for 3.6 and 3.7 only"),
    item("L53", "opus", "item 3, R1/R6", "nesting 95 deep (3.6 and 3.7 reject it) reported free of blockers on 2.7",
         "pending", PF, True, A(D, "base"), F("post-freeze probe", "00:24"), None, None, "open", "postfreeze/tree2 at 07937a7"),
    item("L54", "blank", "item 3, R1/R6", "nesting 95 deep (3.6 and 3.7 reject it) reported free of blockers on 2.7 "
         "(the 3.14 side is L31)", "late", PF, True, A(D, "base"), F("post-freeze probe", "00:24"), X("00:57:27", "P2-23"), A("01:25:29", "by 04f2aa1"), "fixed",
         "postfreeze/tree2 at c5568e8; probe_arm.sh at 5fecba1: nest95 blocked for 3.6 and 3.7 only, nest60 unblocked"),
    item("L55", "blank", "item 2, T12", "2 more tests run on 2.7 only (ur-string rules)", "caught", "in-loop CI (built 22:30)",
         True, A("00:12:25", "4e52811"), F("CI", "00:15"), X("00:16:12", "P2-17"), None, "open", "notice P2-17",
         ci=["oneside:tests.test_rules::test_ur_string_is_found"]),
    item("L56", "sonnet", "item 2b", "non-UTF-8 file name: JSON differs between 2.7 and 3.14 (file_readiness)", "late",
         "in-loop CI (built 22:30)", True, A("00:20:19", "f0388aa"), F("CI", "00:28"), X("00:29:36", "P2-18"), A("00:33:08", "1a9359a"), "fixed",
         "notice P2-18; 9 min: the watcher was re-armed by hand after P2-17", ci=["2b:hostile/non_utf8_name"]),
    item("L57", "sonnet", "item 2, T12", "a 2.7-only test (display_path on undecodable bytes)", "late",
         "in-loop CI (built 22:30)", True, A("00:20:19", "f0388aa"), F("CI", "00:28"), X("00:29:36", "P2-18"), A("00:33:08", "1a9359a"), "fixed",
         "notice P2-18", ci=["oneside:tests.test_encoding::test_display_path_on_python2_replaces_undecodable_bytes"]),
    item("L60", "blank", "item 3, R3/R10", "2 real builtin uses missed (default-argument values)", "late", HID, True,
         A("by 23:12:01", "d3e92bc"), F("hidden check", "23:05"), X("00:57:27", "P2-23"), A("01:25:29", "04f2aa1"), "fixed",
         "repro stdlib json/encoder.py:276; hidden check at 5fecba1: 0 missed, 0 flagged wrongly on 2.7"),
    item("L61", "blank", "item 2b, R1", "nul_byte and invalid_coding JSON differences recorded as not defects; 'no code "
         "change possible' without a receipt", "caught", R, True, A("00:30:24", "dfb0156"), F("referee", "00:40"), X("00:57:27", "P2-23"), None,
         "partly: nul_byte and invalid_coding identical at 5fecba1", "MIGRATION.md at dfb0156; R1 allows a difference only "
         "where the guarantee requires it; at 5fecba1 the stdlib and 16 held-back trees still differ, claimed necessary "
         "under R1; aifc.py repro: 68 findings on 2.7, 1 on 3.14, the 2.7 side blocking all 10 versions (P2-26)"),
    item("L70", "blank", "item 2b", "rot13 probe: t.py's finding names a different rule under 2.7 and under 3.14, though "
         "both block 3.6-3.15", "late", PF, True, A("by 01:25:29", "04f2aa1, the first commit measured since the rot13 fix"),
         F("post-freeze probe, probe_arm.sh", "01:36"), X("01:43:46", "P2-26"), None, "open",
         "probe_arm.sh at 5fecba1: PY3-NON-TEXT-CODING on 2.7, PY3-PARSE on 3.14 (same class as L69)"),
    item("L71", "blank", "item 1", "Units table claims L4 while the arm's own resolver output finds no solution for 3.14 "
         "(markupsafe 1.1.1)", "late", "Evidence content check (not built)", False, A("01:25:42", "5fecba1"),
         F("referee, reading the claims table", "01:40"), X("01:43:46", "P2-26"), None, "open",
         "B's claims table and MIGRATION.md at 5fecba1; the pin question went to the owner"),
    # ---- S round 2 (hidden check at 7517bf8) ----
    item("L58", "sonnet", "R8", "9 imports of hotshot, dummy_thread and dummy_threading get no finding (held-back corpus)",
         "late", HID, True, A("by 22:23:59", "0702387"), F("hidden check", "22:41 (as part of 46)"), X("00:38:48", "P2-19"), A("01:08:59", "by d4f9198"), "fixed",
         "repro: a file holding only `import hotshot`, checked at 541bcb1; the round-1 verdict listed R8 under Met (E17); "
         "hidden check at d4f9198: 413 of 413 on both sides"),
    item("L49", "sonnet", "item 3, R3/R10", "3 real builtin uses missed (a method named raw_input calling the builtin; "
         "default-argument values)", "late", HID, True, A("by 22:23:59", "0702387"), F("hidden check", "22:41"), X("00:38:48", "P2-19"), None,
         "partly: 2 of 3 still missed at d4f9198", "repro stdlib code.py:281, re-checked at 541bcb1; found at round 1 and left "
         "out of that verdict (E15); at d4f9198 code.py is fixed, json/encoder.py:276 is not (P2-25)"),
    item("L50", "sonnet", "item 3, R3/R10", "3 non-uses flagged as builtin uses (import aliases: 'as basestring', 'unichr as chr')",
         "late", HID, True, A("by 23:18:15", "c085781"), F("hidden check", "00:12"), X("00:38:48", "P2-19"), None, "partly: 1 of 3 still flagged at d4f9198",
         "repro site-packages/pip/_vendor/urllib3/response.py:24, re-checked at 541bcb1; at d4f9198 "
         "html5lib/_tokenizer.py:3 is still flagged (P2-25)"),
    item("L51", "sonnet", "item 4", "32 of the 37 CPython 2.7.18 advisories named in no committed file; the triage row "
         "points to an Evidence list that does not exist", "late", "security coverage check (by hand 00:10; security/coverage.py 01:31)",
         True, A("23:40:08", "fe75125"), F("referee coverage check", "00:12"), X("00:38:48", "P2-19"), A("01:08:51", "94cf987"),
         "fixed (naming; triage reasoning not measured)", "security/coverage.py: control reproduces 5 of 37 at 7517bf8; "
         "37 of 37 at d4f9198"),
    # ---- second opinion's 43 hidden-class cases, oracles from real 2.7.18 and 3.6-3.15, run 01:10 (score-0112.txt) ----
    item("L62", "opus", "item 3, R1/R6", "3 encoding and prefix shapes no 3.x compiles reported free of blockers (both sides)",
         "pending", XB, True, A(D, "base (assumed: rule absent)"), F("battery", "01:12"), None, None, "open",
         "postfreeze/extra/score-0112.txt at 42e7b7d"),
    item("L63", "opus", "R5", "a socket, a /dev/null link, a link to /, an ELOOP chain dropped silently; an unreadable "
         "directory exits 0", "pending", XB, True, A(D, "base"), F("battery", "01:12"), None, None, "open",
         "same class as L43 (holdout); score-0112.txt"),
    item("L64", "opus", "item 3", "very deep directories end the 2.7 scan in a traceback", "pending", XB, True, A(D, "base"),
         F("battery", "01:12"), None, None, "open", "score-0112.txt"),
    item("L65", "sonnet", "item 3, R1/R6", "a BOM with a latin-1 coding line (2.7 and 3.x reject it) reported ready, exit 0",
         "late", XB, True, A("by 01:08:59", "d4f9198, the only commit measured; lag is a lower bound"), F("battery", "01:12"),
         X("01:32:14", "P2-25"), None, "open", "score-0112.txt at d4f9198; told as a count, no input (P2-25)"),
    item("L66", "sonnet", "item 3", "1,000-deep nesting ends the scan in a traceback (both sides); very deep directories "
         "on 2.7", "late", XB, True, A(D, "base"), F("battery", "01:12"), X("01:32:14", "P2-25"), None, "open",
         "score-0112.txt; told as counts, no input (P2-25)"),
    item("L69", "sonnet", "item 2b", "rot13 probe: t.py's finding names a different rule under 2.7 and under 3.14, though "
         "both block 3.6-3.15", "late", PF, True, A("by 01:08:59", "d4f9198, the only commit measured since the rot13 fix"),
         F("post-freeze probe, probe_arm.sh", "01:30"), X("01:32:14", "P2-25"), None, "open",
         "probe_arm.sh at d4f9198: PY3-BAD-ENCODING-COOKIE on 2.7, PY3-PARSE on 3.14; item 2b holds for any input, R1 "
         "excuses a difference only where the guarantee requires it"),
    item("L67", "blank", "item 3, R1/R6", "2 encoding shapes no 3.x compiles reported free of blockers on 2.7", "late",
         XB, True, A(D, "base"), F("battery", "01:12"), X("01:43:46", "P2-26"), None, "open",
         "score-0112.txt at dfb0156; unchanged at 5fecba1 (probe_arm.sh 01:36); told as a count (P2-26)"),
    item("L68", "blank", "item 3", "a symlink to / and very deep directories end the scan in a traceback (both sides)",
         "late", XB, True, A(D, "base"), F("battery", "01:12"), X("01:43:46", "P2-26"), None, "open",
         "score-0112.txt; unchanged at 5fecba1; told as a count (P2-26)"),
    # ---- process items AutoSteer never called ----
    item("L44", "sonnet", "skill order", "fix work before assess (L1 skipped)", "missed", "skill-order sensor (not built)",
         False, A("21:18:54", "b58b1e9"), F("referee review", "23:05"), None, None, "not called", "LOG 23:05"),
    item("L45", "sonnet", "item 4, T13", "dependency change b58b1e9 with incomplete triage, judged not fired", "missed", R,
         True, A("21:18:54", "b58b1e9"), F("referee review", "23:05"), None, A("23:44:11", "fe75125 7517bf8"),
         "fixed by the arm later", "LOG 23:05"),
    # ---- not measured: no instrument yet ----
    item("U01", "all", "item 2a", "findings after the migration against before, on the 2.7 stdlib", "unmeasured",
         "2a before/after diff (not built)", False, None, None, None, None, "unmeasured", ""),
    item("U02", "all", "item 5", "adding a new rule touches at most two files", "unmeasured", "rule-addition probe (not built)",
         False, None, None, None, None, "unmeasured", ""),
    item("U03", "all", "item 6", "strategy: prior art, value added, decision served, parsing alternatives with numbers",
         "unmeasured", "H7 blind review (proposed, not run)", False, None, None, None, None, "unmeasured", ""),
    item("U04", "opus, blank", "item 4, R11", "advisory scan and triage for the current environments", "unmeasured",
         "security check in CI (not built)", False, None, None, None, None, "unmeasured", "S checked at 22:41"),
    item("U05", "all", "T11", "a sweep of every level claim against its recorded output", "unmeasured",
         "Evidence content check (not built)", False, None, None, None, None, "unmeasured", ""),
    # ---- AutoSteer's own errors ----
    item("E01", "controller", "C50", "the steering sensors were never started (0 calls for 50 min)", "error", "-", True,
         A("20:38:08", "dispatch"), F("referee, counting", "21:30"), None, A("21:35", "sensors started"), "fixed", "LOG 21:35"),
    item("E02", "controller", "C52", "LOG times estimated, 4 headings wrong by up to 17 min", "error", "-", True, None,
         F("referee", "23:05"), None, None, "fixed", "LOG 23:05; checklist 15e"),
    item("E03", "controller", "C53", "stdlib truth blind to __builtin__, _winreg, dl, msilib: 51 false extras", "error",
         "-", True, None, F("referee", "22:48"), None, None, "fixed before sending", "LOG 22:48"),
    item("E04", "controller", "C54", "hidden harness required exit 2 for a FIFO, against R5 bullet 2", "error", "-", True,
         None, F("peer audit", "23:16"), None, None, "fixed", "LOG 23:52"),
    item("E05", "controller", "C55", "verdict called hostile inputs met; the spec's own list was never run", "error", "-",
         True, A("22:54:24", "P2-10"), F("visible CI, first live record at 7517bf8", "23:56:08"), X("00:01:33", "P2-15"),
         None, "confirmed wrong (backtest 01:20): at 0702387 a 2.7 traceback on the non-UTF-8 name and no 3.6-3.8 blocker "
         "for deep nesting, both present since 8c89639 21:43:15; named to S 67 min after the verdict", "backtest/results-sonnet.jsonl"),
    item("E06", "controller", "C56", "deletion guard misfired on still-failing tests", "error", "-", True, None,
         F("referee, first live output", "23:35"), None, None, "fixed before sending", "LOG 23:52"),
    item("E07", "controller", "verdict format", "round-1 verdict gave no public input for the 3.14.9 advisory", "error",
         "-", True, A("22:54:24", "P2-10"), F("arm asked", "23:40"), None, A("23:42:13", "P2-13"), "fixed", "L08"),
    item("E08", "controller", "C57", "a line-number convention counted as an except-comma miss for B", "error", "-", True,
         None, F("referee", "23:41"), None, None, "withdrawn", "LOG 23:52"),
    item("E09", "controller", "audit A", "CI would score a deleted or skipped failing test as fixed", "error", "-", True,
         None, F("peer audit", "23:16"), None, None, "fixed; no wrong 'fixed' was recorded", "events.jsonl"),
    item("E10", "controller", "record", "json/__init__.py logged as the ur'' class; it is a \\u escape in a plain string",
         "error", "-", True, None, F("referee", "23:58"), None, None, "corrected here", "compile() on Python 3: line 125"),
    item("E11", "controller", "audit F", "AUDIT.txt D5 labelled a defect; a judgment call, never sent", "error", "-", True,
         None, F("peer audit", "23:16"), None, None, "relabelled", "AUDIT.txt"),
    item("E12", "controller", "C58", "hidden R8 truth required blockers for the stdlib importing its own packages "
         "(307 of B's 377 misses); a scanner treating shipped modules as local is defensible", "error", "-", True,
         None, F("referee, before B's verdict", "00:10"), None, A("00:12", "truth_p2.py C58 + control; all reports rescored"),
         "fixed before sending", "handoff/r8_breakdown.py; report.pre-C58.json kept"),
    item("E13", "controller", "double count", "B's 3.14 losses drafted as R8 and R10 failures; they are one failure (L29)",
         "error", "-", True, None, F("referee, before B's verdict", "00:11"), None, None, "fixed before sending",
         "every lost finding sits in a PY3-PARSE file (434/434, 344/344, 337/337)"),
    item("E14", "controller", "alias merge", "S's item 4 coverage first matched CVE ids only: 14 package advisories looked "
         "missing; S names them by GHSA id", "error", "-", True, None, F("referee, own check", "00:13"), None, None,
         "fixed before sending", "the C21 class, in AutoSteer's own check"),
    item("E16", "controller", "C59", "truth table cited no primary source for the exceptions module and the wrong line "
         "for sets, new, thread, Tkinter, HTMLParser, urllib2; exceptions was about to be S's R8 repro", "error", "-", True,
         None, F("referee, checking the repro's source", "00:36"), None, A("00:38", "stdlib_removed.json + truth_p2.py C59"),
         "fixed before sending", "versions/stdlib_removed.json.bak-0037; control in truth_p2.py"),
    item("E17", "controller", "verdict wording", "round-1 verdict for S listed 'PY3-STDLIB per version, with no wrong "
         "severities or version lists' under Met while the same check had 46 misses (20 real after C58/C59)", "error", "-",
         True, A("22:54:24", "P2-10"), F("referee, round 2", "00:36"), None, A("00:38:48", "P2-19 names the misses"),
         "fixed late", "L58"),
    item("E18", "controller", "ops", "killed containers by the shared steerci label while restarting the watcher; one "
         "backtest commit (S b58b1e9) lost its result", "error", "-", True, None, F("referee, reading the log", "00:41"), None,
         A("00:43", "steerci-role label for runs started after 00:41:15"), "fixed for the watcher; the S and B backtest rerun "
         "started 00:41:01 and carries no role label (E21)", "steerci.py.bak-0043; ps lstart"),
    item("E19", "controller", "CI pairing", "one parametrized test read as two one-side tests: pytest escapes a "
         "backslash in the id differently on 2.7 and 3.x (O, 42e7b7d)", "error", "-", True, None,
         F("referee, reading the notice before sending", "00:50"), None, A("00:52", "tnorm_map + control; 2 events rolled back"),
         "fixed before sending", "steerci.py.bak-0052; events.jsonl.bak-0053"),
    item("E20", "controller", "ledger", "in-time or late was a hand label; L59 (11 min) and L61 (27 min) sat in the "
         "in-time table its own heading excludes", "error", "-", True, None, F("referee and peer audit, independently", "01:19"),
         None, A("01:24", "chimes.derive(): class from the clock; lint blocks the write; controls"), "fixed",
         "chimes.py.bak-0122; karthik-ca message 01:2x (I2)"),
    item("E21", "controller", "ops", "S and B backtest rerun started 00:41:01 on code without the role label (00:43) and "
         "without the E19 pairing fix (00:50:39)", "error", "-", True, None, F("referee, ps lstart against file mtimes", "01:17"),
         None, A("01:44", "recheck of all 71 replay records: 0 differ"), "closed: harmless (test fouls recomputed with "
         "today's code: 0 of 71 differ); the containers still lacked a role label", "backtest/results-*.jsonl"),
    item("E26", "controller", "C59 again", "the R8 scorer typed UNCONFIRMED = ('exceptions',) while stdlib_removed.json lists "
         "strop and future_builtins as unconfirmed too; B's 2 correct strop warnings at 5fecba1 scored as extras",
         "error", "-", True, None, F("referee, checking extras at the source before the verdict", "01:38"), None,
         A("01:41", "truth_p2.py reads the list from the data; control fails on the old rule; 5 reports rescored"),
         "fixed before sending", "truth_p2.py.bak-01xx; report.pre-E26.json kept; the E20 class (typed, not derived) in the truth"),
    item("E22", "controller", "verification", "my first E21 check compared test fouls against all fouls (unlike sets) "
         "and printed false differences", "error", "-", True, None, F("referee, reading the output", "01:18"), None,
         A("01:18", "re-run comparing like with like"), "fixed before use", "the C21 class, in a check of a check"),
    item("E23", "controller", "times", "found times typed by hand: '23:2x' on 5 items with no record behind it, and '23:59' "
         "where the first live record is 23:48:05 (O) or 23:56:08 (S); L32 and L33 were never seen live at all",
         "error", "-", True, None, F("referee, lint", "01:21"), None, A("01:27", "found derived from results.jsonl; lint gate"),
         "fixed", "the C52 class, in the ledger"),
    item("E24", "controller", "keys", "L13 (ur'' literals) keyed on json/__init__.py, the \\u escape class; lag from a "
         "pre-dispatch commit time (S base 19:23:37) overstated L13 and L25 by 75 min each", "error", "-", True, None,
         F("referee", "01:20"), None, A("01:24", "keys fixed; lag runs from the later of appearance and dispatch"), "fixed",
         "backtest/events-sonnet.jsonl; the E10 class again"),
    item("E25", "controller", "receipts", "receipts with as-of claims went stale in a generated table: L09 'no commit since "
         "21:58' (O committed 5 times after), L13 're-check pending' (cleared at cbdebe4)", "error", "-", True, None,
         F("peer audit (L09), referee scan (L13)", "01:21"), None, A("01:27", "both corrected from events.jsonl"), "fixed",
         "checklist 10: every number carries an as-of time"),
    item("E27", "controller", "record", "LOG.md credits the visible-input CI to an 'owner decision 22:52' (that answer was "
         "about the feedback channel; the checks were approved at 22:13:59) and heads the answers '22:18' for answers given at "
         "22:13:59", "error", "-", True, None, F("referee, sourcing every decision from the transcript", "01:52"), None,
         A("01:56", "AUTOSTEER-RECORD.md cites transcript times; LOG.md gets a correction note"), "fixed", "transcript tool "
         "results for toolu_01Dyz8ih (22:13:59) and toolu_01CbJcYh (22:52:55)"),
    item("E15", "controller", "verdict omission", "round-1 verdict for S omitted the 3 missed builtin uses its own check "
         "found (AUDIT.txt: 'The 3 FN are default-argument and class-scope cases')", "error", "-", True,
         A("22:54:24", "P2-10"), F("referee, round 2", "00:14"), X("00:38:48", "P2-19"), None, "fixed late: P2-19 names them",
         "L49"),
]

# Root cause, guard and recurrence of each AutoSteer error (record.py renders them; build_ledger asserts coverage).
# root: A claim with no running instrument, B ground truth wrong, C identity and keys, D verdict prose not derived
# from evidence, E record typed not derived, F ops and guards. guard_kind: "code+control" (a check that fails on the
# old behaviour), "code", "checklist" (prose only), "none".
def em(root, guard, kind, repeats="", reached=False):
    return {"root": root, "guard": guard, "guard_kind": kind, "repeats": repeats, "reached_arm": reached}


ERRMETA = {
    "E01": em("A", "checklist 15d: a sensor not running is a failed gate", "checklist", "C50"),
    "E02": em("E", "checklist 15e; the ledger lint gate refuses estimated times (LOG.md itself is not gated)", "checklist", "C52"),
    "E03": em("B", "truth_p2.py SUPPLEMENT for names the source cannot see, with a control; checklist 0d", "code+control", "C53"),
    "E04": em("B", "checklist 0e: every harness expectation cites the ruling clause it enforces", "checklist", "C54"),
    "E05": em("A", "visible CI runs the spec's own inputs on every commit; checklist 0f: a 'met' names its inputs", "code", "C55", True),
    "E06": em("F", "deletion guard fixed with a control for the still-failing case", "code+control", "C56"),
    "E07": em("D", "verdict format: every failure names a public input; generated verdicts not built", "checklist", "", True),
    "E08": em("B", "checklist 0g; the instrument still emits the known false count (1 except-comma on B), discounted by hand",
              "checklist", "C57"),
    "E09": em("F", "a fail key clears only by passing; the peer's tests A1 and A2 pass", "code+control", ""),
    "E10": em("C", "none in code: class labels are typed", "none", ""),
    "E11": em("D", "relabelled; no guard", "none", ""),
    "E12": em("B", "truth_p2.py C58 rule with a control that failed on the old rule; reports rescored", "code+control", "C58"),
    "E13": em("C", "none in code: one failure per cause is checked by hand", "none", ""),
    "E14": em("C", "security/coverage.py merges aliases, with a control that reproduces L51", "code+control", "C21"),
    "E15": em("D", "none until verdicts are generated from the report", "none", "", True),
    "E16": em("B", "C59 control; citations checked against the pack by hand", "code+control", "C59"),
    "E17": em("D", "checklist 0f; generated verdicts not built", "checklist", "E05", True),
    "E18": em("F", "steerci-role label per run; checklist 15f rewritten at 01:53 (it still prescribed the E18 command)", "code", ""),
    "E19": em("C", "tnorm_map with a control", "code+control", ""),
    "E20": em("E", "chimes.derive and lint, with ledger_controls", "code+control", "E02"),
    "E21": em("A", "none yet: a run record does not carry its instrument's hash", "none", ""),
    "E22": em("C", "none: a check of a check is still hand-written", "none", "C21"),
    "E23": em("E", "found times derived from results.jsonl; lint gate", "code+control", "E02"),
    "E24": em("C", "start_of() floors lag at dispatch (control c1); the key choice is still typed", "code", "E10"),
    "E25": em("E", "none in code: receipts are typed; checklist 10 asks for an as-of time", "checklist", ""),
    "E26": em("B", "truth_p2.py reads the unconfirmed list from the data; the control fails on the old rule", "code+control", "E16"),
    "E27": em("E", "the record takes owner decisions and their times from the transcript's answers, not from LOG.md",
              "code", "E02"),
}


if __name__ == "__main__":
    main()
