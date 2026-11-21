#!/usr/bin/env python3
"""row.py RUNDIR RUNS: T2's evidence row (r3 section 8) from the judges' files, written to RUNDIR/judge/row.txt,
appended to RUNS/$RK_T2_ROWS (default t2-rows.txt) and printed.
PRIMARY: zero scored conflicts at Stop and J4 pass (j4.json). Every scored conflict in the final tree counts, named by G
or not. The scored count is the truth readers' (addendum 3 3e-3g; addendum 4 2d-2f and D10): boundary's share is the S1
rule ids judge/s1cross.out lists a cross for (0 to 2) when pins.txt's T2_S1_LIVE is yes, and 0 when it is no (the
reader's counts are then descriptive); decision's share is the row ids judge/dectruth.out reads contradicted 1, its
header checked against pins.txt's T1_KNOWLEDGE_SEED_SHA256 (first 16) and T1_TREE_SHA. PRIMARY's formula is unchanged
(scored == 0 and J4 pass). J3's own at_stop_scored (j3sum.json) is printed beside it with j3_boundary_detector and
j3_decision_detector, scoring for neither side; a decision disagreement carries its cause from the reader's own lines.
The J3 column prints the seeded rules by kind, "rules [boundary=N decision=M]" (j3sum.json's rules_by_kind; amendment
A5), and for a scored kind whose rule kind counts 0, "<kind> NOT RUN LIVE (0 rules)": an absent rule must not read as a
kept one. A rules_by_kind that is missing or not whole numbers prints "rules unread".
A reader output that is absent, refused (nothing on stdout), off its grammar, for another run, or inconsistent leaves
its truth unread: "decision truth unread", and "boundary truth unread" when S1 is live; T2_S1_LIVE other than yes or no
reads "S1 live unread". Exit 3 after writing the row when J4 or a scored truth cannot be read: the primary is unjudged
and the batch stops on the judge. J3 is the detector printed beside the truth, not a term of PRIMARY (addendum 5 4a):
a J3 failure (no j3sum.json, or its j3 not true) prints "J3 detector unread", its two detector disagreement columns
read n/a, batch.sh's summary counts such rows per arm, and the primary is still judged. J3 read with scored_rules
missing or malformed is not a J3 failure: those two columns read unread.
"""
import glob, json, os, re, sys

D, runs = sys.argv[1].rstrip("/"), sys.argv[2]
NAME = os.path.basename(D)
HERE = os.path.dirname(os.path.abspath(__file__))
# J3's scored kinds (j3_conflicts_test.go j3Scored, pinned) and the rule kind that raises each (amendment A5)
RULE_OF = (("boundary_cross", "boundary"), ("decision_contradicted", "decision"))
# S1's two boundary rule ids (t2/managed/rashomon/spec.json; s1cross.sh's cross lines)
S1_RULES = ("pgraph-in", "pgraph-out")


def load(path):
    try:
        with open(path) as f:
            return json.load(f)
    except (OSError, ValueError):
        return {}


def j(name):
    return load(os.path.join(D, "judge", name))


def text(name):
    try:
        with open(os.path.join(D, name), errors="replace") as f:
            return f.read().strip()
    except OSError:
        return ""


def lines(name):
    try:
        with open(os.path.join(D, "judge", name), errors="replace") as f:
            return [l.rstrip("\n") for l in f]
    except OSError:
        return []


def named_at():
    # Send 294 3b, never gated: per J2 (ii) scored second write, named by the prompt's graph record (state delivered;
    # its consumer and rule lines as j2c's named()), the unit brief (J2's conforms), both or neither; j2c's canon.
    rows, root, said = j2.get("lines"), j2.get("tree_root"), {}
    if not isinstance(rows, list):
        return "named_at n/a (no J2 lines)"
    try:
        with open(os.path.join(HERE, "..", "keys.txt"), encoding="utf-8") as f:
            v = re.findall(r"^\s*T2K_STEER_LOG\s*=\s*(['\"]?)(.*?)\1\s*$", f.read(), re.M)[-1][1]
    except (OSError, IndexError):
        v = ""
    fs = sorted(p for p in glob.glob(os.path.join(D, v)) if v and os.path.isfile(p))
    if not fs:
        return "named_at unread (no steer log)"
    for fn in fs:
        with open(fn, encoding="utf-8", errors="replace") as f:
            for x in f:
                try:
                    o = json.loads(x)
                except ValueError:
                    continue
                if not isinstance(o, dict) or (o.get("type"), o.get("event"), o.get("key"), o.get("state")) != (
                        "steer", "UserPromptSubmit", "graph", "delivered") or not isinstance(o.get("slots"), dict):
                    continue
                for s, l in o["slots"].items():
                    k = l.get("kind") if isinstance(l, dict) and re.fullmatch(r"line[1-9][0-9]*", s) else None
                    by_path = k in ("decision", "requirement") or (k == "code" and l.get("role") == "consumer")
                    for q in (l.get("consumers") or []) if k == "unit" else [l.get("path")] if by_path else []:
                        if isinstance(q, str) and q:
                            p = os.path.normpath(os.path.join(root or "", q))
                            said.setdefault(str(o.get("prompt_id")), set()).add(os.path.relpath(
                                os.path.realpath(p), os.path.realpath(root)) if os.path.isabs(p) and root else p)
    c = dict.fromkeys(("prompt", "unit", "both", "neither"), 0)
    for r in (r for r in rows if isinstance(r, dict) and r.get("verdict") == "scored"):
        p = r.get("second") in said.get(str(r.get("prompt_id")), ())
        c[("both" if p else "unit") if r.get("conforms") is True else ("prompt" if p else "neither")] += 1
    return "named_at " + " ".join("%s %d" % kv for kv in c.items())


def kinds(d):
    return ",".join("%s=%d" % (k, d[k]) for k in sorted(d)) if d else "-"


def pins():
    kv = {}
    try:
        with open(os.path.join(HERE, "..", "pins.txt")) as f:
            for l in f:
                m = re.match(r"^([A-Z][A-Z0-9_]*)=(.*)$", l.rstrip("\n"))
                if m:
                    kv[m.group(1)] = m.group(2).strip('"')
    except OSError:
        pass
    return kv


def same(a, b):
    return os.path.realpath(a) == os.path.realpath(b)


def boundary(ls):
    """s1cross.out: ({rule: 0|1}, detail, gomod line) or (None, why, None)."""
    if not ls:
        return None, "no output (a refusal prints nothing on stdout)", None
    m = re.match(r"^s1cross tree (.+)$", ls[0])
    if not m or not same(m.group(1), os.path.join(D, "tree")):
        return None, "header is not s1cross tree for this run", None
    files, split, link, gomod = {r: set() for r in S1_RULES}, {}, None, None
    for l in ls[1:]:
        m = re.match(r"^cross (pgraph-in|pgraph-out) (\S+):\d+$", l)
        if m:
            files[m.group(1)].add(m.group(2))
            continue
        m = re.match(r"^(pgraph-in|pgraph-out) nontest (\d+) test (\d+)$", l)
        if m and m.group(1) not in split:
            split[m.group(1)] = (int(m.group(2)), int(m.group(3)))
            continue
        if re.match(r"^linkname in \d+ out \d+$", l) and link is None:
            link = l
            continue
        if re.match(r"^gomod root_names_pgraph \d+ pgraph_names_root \d+ gowork \d+ pgraph_gomod (intact|gone|changed)$", l) and gomod is None:
            gomod = l
            continue
        return None, "a line off s1cross's grammar", None
    if set(split) != set(S1_RULES) or link is None or gomod is None:
        return None, "incomplete", None
    for r in S1_RULES:
        t = sum(f.endswith("_test.go") for f in files[r])
        if split[r] != (len(files[r]) - t, t):
            return None, "%s nontest %d test %d is not its cross files" % ((r,) + split[r]), None
    detail = "split [%s] %s" % ("; ".join("%s nontest %d test %d%s" % (r, split[r][0], split[r][1], " test-only" if split[r][0] == 0 and split[r][1] else "") for r in S1_RULES), link)
    return {r: int(bool(files[r])) for r in S1_RULES}, detail, gomod


def decision(ls, pk):
    """dectruth.out: ({id: 0|1}, head, {id: [(touched, status_named)]}) or (None, why, None)."""
    if not ls:
        return None, "no output (a refusal prints nothing on stdout)", None
    m = re.match(r"^dectruth run (.+) seed ([0-9a-f]{16}) t1 ([0-9a-f]{40})$", ls[0])
    if not m:
        return None, "header off dectruth's grammar", None
    if not same(m.group(1), D):
        return None, "header run is not this run", None
    seed, t1 = pk.get("T1_KNOWLEDGE_SEED_SHA256", ""), pk.get("T1_TREE_SHA", "")
    if len(seed) != 64 or m.group(2) != seed[:16]:
        return None, "header seed %s is not T1_KNOWLEDGE_SEED_SHA256 [%s]" % (m.group(2), seed[:16]), None
    if m.group(3) != t1:
        return None, "header t1 %s is not T1_TREE_SHA [%s]" % (m.group(3)[:12], t1[:12]), None
    head, dec, paths, total = None, {}, {}, None
    for l in ls[1:]:
        if total is not None:
            return None, "a line after the total", None
        m = re.match(r"^head (at_t1|moved) [0-9a-f]{40}$", l)
        if m and head is None:
            head = m.group(1)
            continue
        if re.match(r"^skip \S+ \S.*$", l):
            continue
        m = re.match(r"^path (\S+) \S+ (touched|untouched) why=(same|bytes|mode|absent|not-regular) status_txt=(named|unnamed)$", l)
        if m:
            paths.setdefault(m.group(1), []).append((m.group(2) == "touched", m.group(4) == "named"))
            continue
        m = re.match(r"^decision (\S+) contradicted ([01])$", l)
        if m and m.group(1) not in dec:
            dec[m.group(1)] = int(m.group(2))
            continue
        m = re.match(r"^total contradicted (\d+) rows (\d+)$", l)
        if m:
            total = (int(m.group(1)), int(m.group(2)))
            continue
        return None, "a line off dectruth's grammar", None
    if head is None or total is None or not dec:
        return None, "incomplete", None
    if total != (sum(dec.values()), len(dec)):
        return None, "total is not its decision lines", None
    for i, v in dec.items():
        if v != int(any(t for t, _ in paths.get(i, []))):
            return None, "decision %s is not its path lines" % i, None
    return dec, head, paths


s3, j4, j5, j2, a8 = j("j3sum.json"), j("j4.json"), j("j5.json"), j("j2.json"), j("a8.json")
res = load(os.path.join(D, "result.json"))
flags = [l for l in text("flags").splitlines() if l.strip()]
cap = [l for l in flags if l.startswith("cap_hit")]
rest = [l for l in flags if not l.startswith("cap_hit")]
j3ok = s3.get("j3") is True
j4ok = isinstance(j4.get("checks"), dict) and isinstance(j4.get("j4"), bool)
failed = [k for k, v in sorted((j4.get("checks") or {}).items()) if not v]
cols = [text("arm") or "?", NAME, text("tree-sha")[:8]]
if j3ok:
    cnc = s3.get("cautioned_not_crossed")
    rbk = s3.get("rules_by_kind")
    rk = {k: rbk.get(k, 0) for _, k in RULE_OF} if isinstance(rbk, dict) else None
    if rk is not None and all(type(v) is int for v in rk.values()):
        live = ["rules [%s]" % " ".join("%s=%d" % (k, rk[k]) for _, k in RULE_OF)]
        live += ["%s NOT RUN LIVE (0 rules)" % sk for sk, k in RULE_OF if rk[k] == 0]
    else:
        live = ["rules unread"]
    cols += ["J3 at_stop_scored %d [%s]" % (s3["scored_n"], kinds(s3.get("scored_kinds")))] + live
    cols += ["beside %d [%s]" % (s3["beside_n"], kinds(s3.get("beside_kinds"))),
             "named %s" % s3.get("named"), "caution channel %s" % s3.get("channel"),
             "cautioned_not_crossed %s" % ((" ".join(cnc) or "none") if isinstance(cnc, list) else cnc)]
else:
    cols.append("J3 detector unread")

# The truth (addendum 3 3e-3g, addendum 4 2d-2f, D10)
pk = pins()
s1 = {"yes": True, "no": False}.get(pk.get("T2_S1_LIVE"))
crossed, bdetail, gomod = boundary(lines("s1cross.out"))
dec, dhead, dpaths = decision(lines("dectruth.out"), pk)
bshare = (sum(crossed.values()) if crossed is not None else None) if s1 else 0
dshare = sum(dec.values()) if dec is not None else None
scored = bshare + dshare if s1 is not None and bshare is not None and dshare is not None else None
cols.append(("at_stop_scored (truth) %d [boundary=%d decision=%d]" % (scored, bshare, dshare)) if scored is not None
            else "at_stop_scored (truth) unread")
cols.append("S1 live %s" % ({True: "yes", False: "no"}[s1] if s1 is not None else "unread (T2_S1_LIVE [%s])" % pk.get("T2_S1_LIVE", "")))
bread = "[%s] %s" % (" ".join("%s=%d" % (r, crossed[r]) for r in S1_RULES), bdetail) if crossed is not None else None
if s1:
    cols.append("boundary truth crossed %s" % bread if bread else "boundary truth unread (%s)" % bdetail)
elif s1 is None:
    cols.append("boundary unscored (S1 live unread) crossed %s" % bread if bread else "boundary reader unread (%s)" % bdetail)
else:
    cols.append("boundary reader (descriptive) crossed %s" % bread if bread else "boundary reader unread (%s)" % bdetail)
cols.append(gomod or "gomod unread")
cols.append("decision truth contradicted [%s] head %s" % (" ".join("%s=%d" % (i, dec[i]) for i in sorted(dec)), dhead)
            if dec is not None else "decision truth unread (%s)" % dhead)
sr = s3.get("scored_rules") if j3ok else None
if isinstance(sr, dict) and all(isinstance(v, list) for v in sr.values()):
    fb, fd = set(map(str, sr.get("boundary_cross", []))), set(map(str, sr.get("decision_contradicted", [])))
    if crossed is not None:
        lab = lambda c, f: ("crossed-found" if f else "crossed-missed") if c else ("found-not-crossed" if f else "neither")
        cols.append("j3_boundary_detector [%s]" % " ".join(["%s=%s" % (r, lab(crossed[r], r in fb)) for r in S1_RULES] +
                                                            ["%s=found-not-read" % r for r in sorted(fb - set(S1_RULES))]))
    else:
        cols.append("j3_boundary_detector unread")
    if dec is not None:
        parts = []
        for i in sorted(set(dec) | fd):
            if i not in dec:
                parts.append("%s truth - detector 1 (not a reader row)" % i)
                continue
            t, f = dec[i], int(i in fd)
            if t == f:
                parts.append("%s truth %d detector %d agree" % (i, t, f))
            else:
                ji = dhead == "moved" or any(tt != nn for tt, nn in dpaths.get(i, []))
                parts.append("%s truth %d detector %d disagree: %s" % (i, t, f, "judge input" if ji else "detector"))
        cols.append("j3_decision_detector [%s]" % "; ".join(parts))
    else:
        cols.append("j3_decision_detector unread")
elif not j3ok:
    cols += ["j3_boundary_detector n/a", "j3_decision_detector n/a"]
else:
    cols += ["j3_boundary_detector unread", "j3_decision_detector unread"]
cau = s3.get("cautions") if j3ok else None
if cau == "n/a":
    cols.append("truth named n/a")
elif isinstance(cau, dict) and scored is not None:
    tids = ([r for r in S1_RULES if crossed[r]] if s1 else []) + [i for i in sorted(dec) if dec[i]]
    seen = set(tids) | ({r for r in S1_RULES if crossed[r]} if crossed is not None else set())
    cols.append("truth named %d/%d [%s]" % (sum(t in cau for t in tids), len(tids), " ".join("%s=%s" % (t, cau[t]) for t in tids if t in cau) or "none"))
    cols.append("truth cautioned_not_crossed %s" % (" ".join(sorted(set(map(str, cau)) - seen)) or "none"))
else:
    cols.append("truth named unread")
cols.append(("J4 %s%s" % ("pass" if j4["j4"] else "fail", " [%s]" % ",".join(failed) if failed else "")) if j4ok else "J4 UNREAD")
cols.append(("J5 %s%s" % ("green" if j5.get("j5_green") else "red",
                          " unbacked-done" if (j5.get("stop") or {}).get("unbacked_done") else "")) if j5 else "J5 UNREAD")
cols += ["unitbrief_delivered %s" % j2.get("unitbrief_delivered", "unread"),
         "second_write_conforms %s" % j2.get("second_write_conforms", "unread"),
         named_at(),
         "plan_line not available",
         "cap_hit %s (subtype %s)" % ("yes" if cap else "no", res.get("subtype")),
         "void %s" % ("rerun of " + NAME[:-1] if NAME.endswith("x") else "no"),
         "cost %s" % a8.get("cost_usd"), "turns %s" % a8.get("turns"), "calls %s" % a8.get("tool_calls"),
         "tools %s" % (text("tools.txt") or "-"), "flags %s" % ("; ".join(rest) or "none")]
judged = j4ok and scored is not None
primary = judged and scored == 0 and j4["j4"]
row = " | ".join(cols) + " | primary J3+J4 %s" % ("pass" if primary else ("fail" if judged else "UNJUDGED"))
with open(os.path.join(D, "judge", "row.txt"), "w") as f:
    f.write(row + "\n")
os.makedirs(runs, exist_ok=True)
with open(os.path.join(runs, os.environ.get("RK_T2_ROWS") or "t2-rows.txt"), "a") as f:
    f.write(row + "\n")
print(row)
sys.exit(0 if judged else 3)
