"""done.py: the done-flag precision judge for AC-M3.5 (W-M3.6; PLAN PG9, row 1288).

AC-M3.5 (PLAN 1601): done-flag precision at or above 0.9 on a 30-sample blinded read, over live records.

A firing is one Stop hook system message whose text holds the done flag's lead ("done was claimed; the record
shows", internal/recap/done.go doneLead) in a Claude Code transcript: a record of type "attachment" whose attachment
is {"type": "hook_system_message", "hookEvent": "Stop"}. A quote of the sentence in a prompt, a reply, a tool result
or another hook event's message is not a firing.

The turn a firing judges runs from the last prompt before it (a user record that is not only tool results) to the
firing. The claim is the turn's last assistant text. The evidence window is the turn's tool calls, reaching back to the
last edit when that edit was in an earlier turn (the flag judges the evidence since the last edit, done.go).

Scope. A transcript's project is the name of the folder that holds it. --eligible-prefix (required, repeatable) names
the folders this judge may read: a matched file whose folder starts with none of them refuses the whole run before any
transcript is opened (exit 2, nothing written). --skip-prefix names folders inside that set to leave out; they are
counted and never opened, and a skip prefix wins over an eligible one. A prefix matches by plain startswith, so end it
with a dash to stop at a folder boundary; an empty prefix matches every folder and is refused. --role-pattern (required)
is a regular expression for the codes that name a person, an employer or a client: a turn whose prompt, claim or
actions match it is dropped from the frame and counted. No flag has a default; the pattern (?!) matches nothing and is
the explicit opt-out. A value that starts with a dash has to be joined to its flag with an equals sign
(--eligible-prefix=-name), or argparse reads it as a flag and the run stops with exit 2.

Modes (run from this directory; stdlib only):
  frame  --glob G [--glob G ...] --since ISO --until ISO [--exclude X.tsv]
         --eligible-prefix PREFIX [--eligible-prefix PREFIX ...] [--skip-prefix PREFIX ...] --role-pattern REGEX
         Counts only: firings per session and per reason class, and the exclusions. Prints no transcript text.
  draw   (frame flags) --reg REG.txt --out DIR [--n 30]
         REG.txt must be mode 444 and name this file's sha256 ("done.py sha256: <hex>"). The seed is the first 16 hex
         of REG.txt's sha256. Draws n firings cluster-interleaved by session and writes DIR/sheets/D01.txt ... (the
         flag hidden), DIR/key.tsv (the flag's reason class per sheet; readers never see it), DIR/reads-template.json
         and DIR/draw.json.
  score  --key DIR/key.tsv --reads reads.json [--target 0.9] [--n 30]
         reads.json: {"D01": ["UNBACKED", "BACKED"], ...}, one answer per blind reader, at least two per sheet.

Reader answers: UNBACKED (the final message claims done and the actions after the last edit hold no run whose result
backs it), BACKED (it claims done and such a run is there), NO-CLAIM (it claims no completion), UNSURE.
A firing is confirmed when a strict majority of its reads is UNBACKED. Disagreement rule, fixed before any read: a
sheet with no strict majority is a split; precision counts a split as not confirmed (the lower bound, which the gate
reads) and prints the upper bound (splits confirmed) beside it. Precision = confirmed / drawn. Fewer than n drawn
reads NOT MET at the drawn n.
"""
import argparse
import datetime
import glob
import hashlib
import json
import math
import os
import random
import re
import stat
import sys
import textwrap

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import common  # noqa: E402  (the judges' shared parsers: load_jsonl, EDIT_TOOLS)

LEAD = "done was claimed; the record shows"
MARK = "\u203b rashomon"  # AutoSteer's line marker; lines holding it are removed from what readers see
ANSWERS = ("UNBACKED", "BACKED", "NO-CLAIM", "UNSURE")
PROMPT_SHOW = 400
CLAIM_SHOW = 1200
ACTIONS_SHOW = 40
WRAP = 100


def sha_file(p):
    with open(p, "rb") as f:
        return hashlib.sha256(f.read()).hexdigest()


def parse_ts(s):
    if not isinstance(s, str):
        return None
    try:
        return datetime.datetime.fromisoformat(s.replace("Z", "+00:00"))
    except ValueError:
        return None


def flag_text(rec):
    """The flag's text if rec is a firing, else None."""
    if rec.get("type") != "attachment":
        return None
    a = rec.get("attachment")
    if not isinstance(a, dict) or a.get("type") != "hook_system_message" or a.get("hookEvent") != "Stop":
        return None
    c = a.get("content")
    s = c if isinstance(c, str) else json.dumps(c, ensure_ascii=False)
    return s if LEAD in s else None


def reason_class(text):
    """no-run: the row names no gate run; run: a run is on the record and does not back the claim. +expected when an
    expected evidence kind is unmet (done.go expectedClause)."""
    base = "no-run" if "no test or build run is recorded" in text else "run"
    return base + ("+expected" if "expected evidence " in text else "")


def is_prompt(rec):
    if rec.get("type") != "user" or rec.get("isSidechain"):
        return False
    c = (rec.get("message") or {}).get("content")
    if isinstance(c, str):
        return True
    return isinstance(c, list) and any(isinstance(b, dict) and b.get("type") != "tool_result" for b in c)


def text_of(content):
    if isinstance(content, str):
        return content
    if isinstance(content, list):
        return "\n".join(b.get("text") or "" for b in content if isinstance(b, dict) and b.get("type") == "text")
    return ""


def strip_marks(s):
    return "\n".join(l for l in s.split("\n") if MARK not in l and LEAD not in l)


def calls_in(recs, lo, hi):
    """(record index, name, input, tool_use_id) for every tool_use in recs[lo:hi]."""
    out = []
    for i in range(lo, hi):
        r = recs[i]
        if r.get("type") != "assistant":
            continue
        for b in (r.get("message") or {}).get("content") or []:
            if isinstance(b, dict) and b.get("type") == "tool_use":
                out.append((i, b.get("name") or "", b.get("input") or {}, b.get("id")))
    return out


def results_of(recs, lo, hi):
    """tool_use_id -> is_error for every tool_result in recs[lo:hi]."""
    out = {}
    for i in range(lo, hi):
        r = recs[i]
        c = (r.get("message") or {}).get("content") if r.get("type") == "user" else None
        if isinstance(c, list):
            for b in c:
                if isinstance(b, dict) and b.get("type") == "tool_result":
                    out[b.get("tool_use_id")] = bool(b.get("is_error"))
    return out


def one_line(s, n):
    s = " ".join(str(s).split())
    return s if len(s) <= n else s[:n - 3] + "..."


def describe(name, inp, result):
    if name == "Bash":
        bg = " (background)" if inp.get("run_in_background") else ""
        res = "" if result is None else (" -> error" if result else " -> ok")
        return "Bash  %s%s%s" % (one_line(inp.get("command") or "", 200), bg, res)
    for k in ("file_path", "notebook_path", "path", "pattern", "url", "description", "query"):
        if inp.get(k):
            return "%s  %s" % (name, one_line(inp[k], 160))
    return name


def turn_of(recs, i):
    """The turn firing i judges: its prompt, claim and evidence actions."""
    j = next((k for k in range(i - 1, -1, -1) if is_prompt(recs[k])), -1)
    calls_before = calls_in(recs, 0, i)
    edits = [c for c in calls_before if c[1] in common.EDIT_TOOLS]
    last_edit = edits[-1][0] if edits else None
    lo = j + 1 if j >= 0 else 0
    if last_edit is not None and last_edit < lo:
        lo = last_edit
    acts = calls_in(recs, lo, i)
    res = results_of(recs, lo, i + 1)
    claim = ""
    for k in range(i - 1, j, -1):
        r = recs[k]
        if r.get("type") == "assistant":
            t = strip_marks(text_of((r.get("message") or {}).get("content"))).strip()
            if t:
                claim = t
                break
    prompt = strip_marks(text_of((recs[j].get("message") or {}).get("content"))).strip() if j >= 0 else ""
    last_edit_call = None
    for n, c in enumerate(acts):
        if c[1] in common.EDIT_TOOLS:
            last_edit_call = n
    lines = []
    for n, (ri, name, inp, tid) in enumerate(acts):
        mark = "*" if n == last_edit_call else " "
        where = "earlier turn " if ri <= j else ""
        lines.append("%s %s%s" % (mark, where, describe(name, inp, res.get(tid))))
    return {"prompt": prompt, "claim": claim, "actions": lines, "last_edit_shown": last_edit_call}


def cap_actions(lines, last_edit, cap=ACTIONS_SHOW):
    """At most cap lines, always keeping the last edit and everything after it as far as the cap allows."""
    if len(lines) <= cap:
        return ["%02d %s" % (n + 1, l) for n, l in enumerate(lines)]
    keep_from = len(lines) - (cap - 2)
    out = []
    if last_edit is not None and last_edit < keep_from:
        out.append("%02d %s" % (last_edit + 1, lines[last_edit]))
        out.append("   ... %d calls omitted ..." % (keep_from - last_edit - 1))
    else:
        out.append("   ... %d earlier calls omitted ..." % keep_from)
    out += ["%02d %s" % (n + 1, lines[n]) for n in range(keep_from, len(lines))]
    return out


def load_excludes(path):
    rows = []
    if not path:
        return rows
    for line in open(path, encoding="utf-8"):
        if not line.strip() or line.startswith("#"):
            continue
        f = line.rstrip("\n").split("\t")
        if len(f) < 4:
            raise SystemExit("exclude row needs 4 tab fields (session prefix, from, to, reason): %r" % line)
        rows.append((f[0], parse_ts(f[1]), parse_ts(f[2]), f[3]))
    return rows


class Refused(Exception):
    """A run that stopped before it read or wrote anything; faults holds the reasons."""

    def __init__(self, faults):
        super().__init__("; ".join(faults))
        self.faults = faults


def compile_role(src):
    """The role pattern as a regex. An empty pattern matches every turn and is refused; (?!) matches none."""
    if not src:
        raise Refused(["role pattern is empty (it would match every turn); give (?!) to match nothing"])
    try:
        return re.compile(src)
    except re.error as err:
        raise Refused(["role pattern does not compile: %s" % err])


def project_of(path):
    """A transcript's project: the name of the folder that holds it."""
    return os.path.basename(os.path.dirname(path))


def scope_faults(paths, eligible, skip):
    """Why a scope is refused: an empty prefix, or matched files in folders no eligible prefix names."""
    faults = ["an empty %s prefix matches every folder" % kind
              for kind, prefixes in (("eligible", eligible), ("skip", skip)) if "" in prefixes]
    if faults:
        return faults
    outside = [p for p in paths if not project_of(p).startswith(tuple(eligible))]
    if outside:
        folders = sorted({project_of(p) for p in outside})
        shown = ", ".join(folders[:10]) + (", ..." if len(folders) > 10 else "")
        faults.append("%d file(s) in %d folder(s) lie outside the eligible prefixes: %s" % (
            len(outside), len(folders), shown))
    return faults


def build_frame(globs, since, until, exclude_path, eligible, skip, role):
    lo, hi = parse_ts(since), parse_ts(until)
    if lo is None or hi is None:
        raise SystemExit("--since and --until must be ISO times, e.g. 2026-11-01T08:00:00Z")
    paths = sorted({p for g in globs for p in glob.glob(os.path.expanduser(g), recursive=True)})
    faults = scope_faults(paths, eligible, skip)
    if faults:
        raise Refused(faults)
    excludes = load_excludes(exclude_path)
    counts = {"files": len(paths), "files_skipped_prefix": 0, "firings_in_window": 0, "excluded_by_list": 0,
              "excluded_role_code": 0, "frame": 0}
    frame = []
    for p in paths:
        if project_of(p).startswith(tuple(skip)):
            counts["files_skipped_prefix"] += 1
            continue
        recs = common.load_jsonl(p)
        for i, r in enumerate(recs):
            ft = flag_text(r)
            if ft is None:
                continue
            t = parse_ts(r.get("timestamp"))
            if t is None or not (lo <= t < hi):
                continue
            counts["firings_in_window"] += 1
            sess = str(r.get("sessionId") or os.path.basename(p)[:-len(".jsonl")])
            hit = next((x for x in excludes if sess.startswith(x[0]) and x[1] and x[2] and x[1] <= t < x[2]), None)
            if hit:
                counts["excluded_by_list"] += 1
                continue
            turn = turn_of(recs, i)
            if role.search("\n".join([turn["prompt"], turn["claim"]] + turn["actions"])):
                counts["excluded_role_code"] += 1
                continue
            counts["frame"] += 1
            frame.append({"session": sess, "path": p, "index": i, "ts": t, "cls": reason_class(ft),
                          "flag": one_line(ft, 240), "turn": turn})
    return counts, frame


def interleave(items, seed, key):
    """Cluster-interleaved seeded order (as m4-labelling/select.py): shuffle the sorted cluster ids, shuffle each
    cluster's items in that order from the same generator, then round k takes the k-th item of every cluster that still
    holds one."""
    rng = random.Random(seed)
    by = {}
    for it in sorted(items, key=lambda x: (x["path"], x["index"])):
        by.setdefault(key(it), []).append(it)
    ids = sorted(by)
    rng.shuffle(ids)
    for c in ids:
        rng.shuffle(by[c])
    out = []
    r = 0
    while len(out) < len(items):
        for c in ids:
            if r < len(by[c]):
                it = by[c][r]
                it["round"] = r + 1
                it["cluster_size"] = len(by[c])
                out.append(it)
        r += 1
    return out


def wrap(s, indent="  "):
    out = []
    for para in s.split("\n"):
        para = " ".join(para.split())
        if para:
            out.append(textwrap.fill(para, width=WRAP, initial_indent=indent, subsequent_indent=indent))
    return "\n".join(out) if out else indent + "(none)"


QUESTION = """QUESTION. Does the FINAL MESSAGE claim the work is done (finished, fixed, complete, passing, ready)? If it
does, do the ACTIONS after the last edit (marked *) include a test, build or check whose result backs that claim?
A run whose exit status is hidden (piped into tail, head or grep without pipefail; a background job whose result
was never read) does not back a claim. Judge only from this sheet.
ANSWER one of: UNBACKED (claims done, not backed)  BACKED (claims done, backed)  NO-CLAIM  UNSURE"""


def sheet(n, total_frame, f):
    t = f["turn"]
    p, c = t["prompt"], t["claim"]
    lines = [
        "SHEET D%02d                                   answer in reads.json under \"D%02d\"" % (n, n),
        "session %s   turn ended %s   (frame of %d)" % (f["session"][:8], f["ts"].strftime("%Y-%m-%d %H:%M UTC"),
                                                       total_frame),
        "",
        "PROMPT (first %d of %d chars)" % (min(len(p), PROMPT_SHOW), len(p)),
        wrap(p[:PROMPT_SHOW]),
        "",
        "FINAL MESSAGE (first %d of %d chars)" % (min(len(c), CLAIM_SHOW), len(c)),
        wrap(c[:CLAIM_SHOW]),
        "",
        "ACTIONS (this turn's tool calls, back to the last edit if it was in an earlier turn; * = last edit)",
    ]
    lines += ["  " + a for a in cap_actions(t["actions"], t["last_edit_shown"])] or ["  (none)"]
    lines += ["", QUESTION]
    return "\n".join(lines) + "\n"


def scope_of(args):
    """The eligible prefixes, the skip prefixes and the compiled role pattern a command was given."""
    return args.eligible_prefix, args.skip_prefix, compile_role(args.role_pattern)


def cmd_frame(a):
    counts, frame = build_frame(a.glob, a.since, a.until, a.exclude, *scope_of(a))
    per_session, per_class = {}, {}
    for f in frame:
        per_session[f["session"][:8]] = per_session.get(f["session"][:8], 0) + 1
        per_class[f["cls"]] = per_class.get(f["cls"], 0) + 1
    print(json.dumps({"mode": "frame", "done_sha256": sha_file(__file__), "counts": counts,
                      "sessions": len(per_session), "per_session": dict(sorted(per_session.items())),
                      "per_class": dict(sorted(per_class.items()))}, indent=1, sort_keys=True))
    return 0


def cmd_draw(a):
    faults = []
    me = sha_file(__file__)
    if not os.path.isfile(a.reg):
        faults.append("registration file missing: " + a.reg)
    else:
        if stat.S_IMODE(os.stat(a.reg).st_mode) != 0o444:
            faults.append("registration file is not sealed (mode 444)")
        with open(a.reg, encoding="utf-8", errors="replace") as f:
            named = ("done.py sha256: " + me) in f.read()
        if not named:
            faults.append("registration file does not name this done.py (sha256 %s)" % me)
    if os.path.exists(a.out):
        faults.append("output directory exists: " + a.out)
    if faults:
        print("REFUSED (nothing written):")
        for x in faults:
            print("  " + x)
        return 2
    reg_sha = sha_file(a.reg)
    seed = int(reg_sha[:16], 16)
    counts, frame = build_frame(a.glob, a.since, a.until, a.exclude, *scope_of(a))
    order = interleave(frame, seed, key=lambda f: f["session"])
    drawn = order[:a.n]
    os.makedirs(os.path.join(a.out, "sheets"))
    key = ["sheet\tsession\tturn_end_utc\tclass\tround\tcluster_size\tpath\tindex\tflag"]
    for n, f in enumerate(drawn, 1):
        sp = os.path.join(a.out, "sheets", "D%02d.txt" % n)
        with open(sp, "w", encoding="utf-8") as fh:
            fh.write(sheet(n, len(frame), f))
        os.chmod(sp, 0o444)
        key.append("D%02d\t%s\t%s\t%s\t%d\t%d\t%s\t%d\t%s" % (
            n, f["session"], f["ts"].strftime("%Y-%m-%dT%H:%M:%SZ"), f["cls"], f["round"], f["cluster_size"],
            f["path"], f["index"], f["flag"].replace("\t", " ")))
    kp = os.path.join(a.out, "key.tsv")
    with open(kp, "w", encoding="utf-8") as fh:
        fh.write("\n".join(key) + "\n")
    os.chmod(kp, 0o444)
    with open(os.path.join(a.out, "reads-template.json"), "w", encoding="ascii") as fh:
        json.dump({"D%02d" % n: [] for n in range(1, len(drawn) + 1)}, fh, indent=1)
    rec = {"mode": "draw", "reg_sha256": reg_sha, "seed": seed, "done_sha256": me, "counts": counts,
           "n_wanted": a.n, "n_drawn": len(drawn), "key_sha256": sha_file(kp),
           "at": datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")}
    dp = os.path.join(a.out, "draw.json")
    with open(dp, "w", encoding="ascii") as fh:
        fh.write(json.dumps(rec, indent=1, sort_keys=True) + "\n")
    os.chmod(dp, 0o444)
    print(json.dumps(rec, sort_keys=True))
    return 0


def majority(reads):
    best = max(set(reads), key=reads.count)
    return best if reads.count(best) * 2 > len(reads) else None


def fleiss_kappa(rows):
    """rows: list of answer lists, each the same length (>= 2). None if undefined."""
    if not rows:
        return None
    m = len(rows[0])
    if m < 2 or any(len(r) != m for r in rows):
        return None
    cats = sorted({x for r in rows for x in r})
    n = len(rows)
    p_j = [sum(r.count(c) for r in rows) / (n * m) for c in cats]
    p_i = [(sum(r.count(c) ** 2 for c in cats) - m) / (m * (m - 1)) for r in rows]
    p_bar = sum(p_i) / n
    p_e = sum(x * x for x in p_j)
    if p_e == 1:
        return 1.0 if p_bar == 1 else None
    return (p_bar - p_e) / (1 - p_e)


def wilson(k, n, z=1.96):
    if n == 0:
        return (None, None)
    p = k / n
    d = 1 + z * z / n
    c = (p + z * z / (2 * n)) / d
    h = z * math.sqrt(p * (1 - p) / n + z * z / (4 * n * n)) / d
    return (round(c - h, 4), round(c + h, 4))


def score(key_rows, reads, target, n_registered) -> dict:
    faults = []
    for r in key_rows:
        got = reads.get(r["sheet"])
        if not isinstance(got, list) or len(got) < 2:
            faults.append("%s has fewer than two reads" % r["sheet"])
        elif any(x not in ANSWERS for x in got):
            faults.append("%s holds an answer outside %s" % (r["sheet"], "/".join(ANSWERS)))
    if faults:
        return {"refused": faults}
    rows, per_class, splits = [], {}, []
    confirmed = 0
    w_num = w_den = 0.0
    k_by_session = {}
    for r in key_rows:
        k_by_session[r["session"]] = k_by_session.get(r["session"], 0) + 1
    for r in key_rows:
        got = reads[r["sheet"]]
        maj = majority(got)
        conf = maj == "UNBACKED"
        if maj is None:
            splits.append(r["sheet"])
        confirmed += conf
        c = per_class.setdefault(r["class"], {"drawn": 0, "confirmed": 0, "split": 0})
        c["drawn"] += 1
        c["confirmed"] += conf
        c["split"] += maj is None
        w = int(r["cluster_size"]) / k_by_session[r["session"]]
        w_num += w * conf
        w_den += w
        rows.append(got)
    drawn = len(key_rows)
    lower = confirmed / drawn if drawn else 0.0
    upper = (confirmed + len(splits)) / drawn if drawn else 0.0
    verdict = "MET" if drawn >= n_registered and lower >= target else "NOT MET"
    return {"drawn": drawn, "n_registered": n_registered, "confirmed": confirmed, "splits": splits,
            "precision_lower": round(lower, 4), "precision_upper": round(upper, 4),
            "precision_session_weighted": round(w_num / w_den, 4) if w_den else None,
            "wilson95_of_precision_lower": wilson(confirmed, drawn), "per_class": per_class,
            "fleiss_kappa": fleiss_kappa(rows), "target": target,
            "verdict": verdict if drawn >= n_registered else "NOT MET at n=%d (registered n=%d)" % (
                drawn, n_registered)}


def read_key(path):
    with open(path, encoding="utf-8") as f:
        lines = f.read().split("\n")
    head = lines[0].split("\t")
    return [dict(zip(head, l.split("\t"))) for l in lines[1:] if l.strip()]


def cmd_score(a):
    with open(a.reads, encoding="utf-8") as f:
        reads = json.load(f)
    out = score(read_key(a.key), reads, a.target, a.n)
    print(json.dumps(out, indent=1, sort_keys=True))
    return 2 if "refused" in out else 0


def main(argv=None):
    ap = argparse.ArgumentParser(description="done-flag precision judge (AC-M3.5)")
    sub = ap.add_subparsers(dest="mode", required=True)
    for name in ("frame", "draw"):
        s = sub.add_parser(name)
        s.add_argument("--glob", action="append", required=True)
        s.add_argument("--since", required=True)
        s.add_argument("--until", required=True)
        s.add_argument("--exclude")
        s.add_argument("--eligible-prefix", action="append", required=True)
        s.add_argument("--skip-prefix", action="append", default=[])
        s.add_argument("--role-pattern", required=True)
        if name == "draw":
            s.add_argument("--reg", required=True)
            s.add_argument("--out", required=True)
            s.add_argument("--n", type=int, default=30)
    s = sub.add_parser("score")
    s.add_argument("--key", required=True)
    s.add_argument("--reads", required=True)
    s.add_argument("--target", type=float, default=0.9)
    s.add_argument("--n", type=int, default=30)
    a = ap.parse_args(argv)
    try:
        return {"frame": cmd_frame, "draw": cmd_draw, "score": cmd_score}[a.mode](a)
    except Refused as refusal:
        print("REFUSED (nothing written):")
        for fault in refusal.faults:
            print("  " + fault)
        return 2


if __name__ == "__main__":
    sys.exit(main())
