#!/usr/bin/env python3
"""j2c.py RUNDIR, or j2c.py pair GRUN RUN... [-- GRUN RUN...]...: T2's J2 (r3 section 8, as addendum 6 defines it,
addendum 7 amends it and addendum 11 section 12 item 6 amends it again). Installed as t2/judge/j2.py, the name
judge.sh calls; j2.py stays addendum 6's record and j2b.py (99eb3fbb487f742e) addendum 7's.

Item 6 (j2c): every path compare is made after realpath on both sides. A written file_path and the transcript's
first cwd are both realpathed before relpath (rel); a named path, slot.RelPath in the prompt hook's frame (in a
one-prompt T2 run the transcript's first cwd), is joined to that cwd and passed through rel (canon). So /tmp/x and
/private/tmp/x are one path, and so are a path through an in-tree link and its target. realpath reads the
filesystem at judge time: a component that does not exist is kept as written, so a re-judge after the tree is gone
still resolves the /tmp alias but no link inside the tree.

RUNDIR mode writes RUNDIR/judge/j2.json and prints it. On arm G it reads the steer log, the records, the unit-brief
files and RUNDIR/transcript.jsonl:
  (i)  the unit brief given once per unit and within three notes per prompt, and reached: delivery "delivered" with
       the briefed edit's PostToolUse hook_additional_context in the transcript holding the line's model_text;
  (ii) the second write conforms: the session's next landed Write, Edit or MultiEdit after the briefed edit, to a
       path other than the briefed edit's, is a consumer or a rule the delivered brief named. The named set is read
       from the unit-brief file whose digest equals the key's and whose rev equals slots["rev"]; any other file reads
       "replaced, not measurable", counted and never scored. Addendum 7: the named set is read from that file's first
       n lines only, n the count of the line's line<K> slots whose kind is a non-empty string (the said lines; an
       unsaid slot encodes with kind ""). n = 0, or n above the file's line count, is a setup failure (2d).
A holdout or arm_off line in any arm, or any unit-brief line or file on arm U or R, is a setup failure, never a score.
Every arm's j2.json carries first_writes (per Go package directory, the first landed .go write and the second write)
for pair mode, which prints R5 on G beside the same measure on each group's other arms, pooled (addendum 6 sec. 5).
Exit 0 when j2.json is written (a setup failure is written as one); 3 when an input or key cannot be read.
"""
import collections
import glob
import hashlib
import json
import os
import posixpath
import re
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
WRITES = ("Write", "Edit", "MultiEdit")
RULES = ("decision", "requirement")
CAP = 3
NEED = ("T2K_STEER_LOG", "T2K_RECORDS", "T2K_UNITBRIEF_DIR", "T2K_UNITBRIEF_PREFIX")


def keys():
    k = {}
    with open(os.path.join(HERE, "..", "keys.txt"), encoding="utf-8") as f:
        for line in f:
            line = line.strip()
            if line and not line.startswith("#") and "=" in line:
                n, v = line.split("=", 1)
                v = v.strip()
                if len(v) >= 2 and v[0] == v[-1] and v[0] in "'\"":
                    v = v[1:-1]
                k[n.strip()] = v
    return k


def ndjson(paths):
    out, bad = [], 0
    for p in paths:
        with open(p, encoding="utf-8", errors="replace") as f:
            for raw in f:
                raw = raw.strip()
                if not raw:
                    continue
                try:
                    d = json.loads(raw)
                except ValueError:
                    bad += 1
                    continue
                if isinstance(d, dict):
                    out.append(d)
                else:
                    bad += 1
    return out, bad


def matches(D, pattern):
    return sorted(p for p in glob.glob(os.path.join(D, pattern)) if os.path.isfile(p))


def transcript(path):
    root, uses, done, ctx = None, [], {}, collections.defaultdict(list)
    recs, _ = ndjson([path])
    for d in recs:
        if root is None and isinstance(d.get("cwd"), str):
            root = d["cwd"]
        t = d.get("type")
        if t == "attachment":
            a = d.get("attachment") or {}
            if a.get("type") == "hook_additional_context" and a.get("hookEvent") == "PostToolUse":
                c = a.get("content")
                ctx[str(a.get("toolUseID"))].extend(c if isinstance(c, list) else [c])
            continue
        m = d.get("message")
        if not isinstance(m, dict) or not isinstance(m.get("content"), list):
            continue
        for b in m["content"]:
            if not isinstance(b, dict):
                continue
            if t == "assistant" and b.get("type") == "tool_use":
                inp = b.get("input")
                fp = inp.get("file_path") if isinstance(inp, dict) else None
                uses.append((str(b.get("id")), str(b.get("name")), fp))
            elif t == "user" and b.get("type") == "tool_result":
                done[str(b.get("tool_use_id"))] = bool(b.get("is_error"))
    return root, uses, done, ctx


def rel(path, root):
    if not isinstance(path, str) or not path:
        return None
    p = posixpath.normpath(path)
    if posixpath.isabs(p) and root:
        p = posixpath.relpath(os.path.realpath(p), os.path.realpath(root))
    return p


def canon(name, root):
    # j2c: a named path in the written side's frame, joined to root and passed through rel (realpath, both sides).
    return rel(posixpath.join(root or "", name), root)


def unit_dir(p):
    return posixpath.dirname(p) or "."


def landed_writes(uses, done, root):
    out = []
    for i, (uid, name, fp) in enumerate(uses):
        p = rel(fp, root)
        if name in WRITES and done.get(uid) is False and p is not None:
            out.append((i, uid, p))
    return out


def second(writes, pos, path):
    for i, _, p in writes:
        if i > pos and p != path:
            return p
    return None


def first_writes(writes):
    fw = {}
    for i, _, p in writes:
        if p.endswith(".go") and not p.startswith("../"):
            d = unit_dir(p)
            if d not in fw:
                fw[d] = {"first": p, "second": second(writes, i, p)}
    return fw


def digest(data):
    # cmd/rashomon/graph_unitbrief.go unitBriefDigest (as/m2-w23h 91e9d268): no lines key, or a last byte that is not
    # the closing brace, names no brief.
    i = data.find(b',"lines":')
    if i < 0 or not data.endswith(b"}"):
        return None
    return hashlib.sha256(data[i + 9:-1]).hexdigest()[:12]


def named(doc):
    n = set()
    for line in doc.get("lines") or []:
        if not isinstance(line, dict):
            continue
        c, u, k = line.get("code"), line.get("unit"), line.get("knowledge")
        if isinstance(c, dict) and c.get("Role") == "consumer" and isinstance(c.get("Path"), str) and c["Path"]:
            n.add(posixpath.normpath(c["Path"]))
        if isinstance(u, dict):
            n.update(posixpath.normpath(p) for p in u.get("Consumers") or [] if isinstance(p, str) and p)
        if isinstance(k, dict) and k.get("Kind") in RULES and isinstance(k.get("Path"), str) and k["Path"]:
            n.add(posixpath.normpath(k["Path"]))
    return n


def split_key(key, prefix):
    body = key[len(prefix):]
    if "@" not in body:
        return body, None
    u, g = body.rsplit("@", 1)
    return u, g


def given(d):
    if d.get("state") == "delivered" and d.get("delivery") in ("delivered", "holdout"):
        return True
    return d.get("suppressed_by") == "arm_off" and d.get("level") == "nudge"


def counted(d):
    return d.get("delivery") == "delivered" or (d.get("suppressed_by") == "arm_off" and d.get("level") == "nudge")


def briefs(D, pattern):
    by_unit, bad = collections.defaultdict(list), []
    for p in matches(D, os.path.join(pattern, "*.json")):
        if os.path.basename(p).startswith("."):
            continue
        with open(p, "rb") as f:
            data = f.read()
        try:
            doc = json.loads(data.decode("utf-8"))
        except ValueError:
            bad.append(os.path.basename(p))
            continue
        if not isinstance(doc, dict):
            bad.append(os.path.basename(p))
            continue
        by_unit[str(doc.get("unit"))].append((os.path.basename(p), data, doc))
    return by_unit, bad


def measurable(files, dig, srev):
    if not files:
        return "no file for the unit", None
    if len(files) > 1:
        return "%d files name the unit" % len(files), None
    name, data, doc = files[0]
    if dig is None:
        return "the key holds no digest", None
    got = digest(data)
    if got != dig:
        return "file %s digest %s, key %s" % (name, got, dig), None
    if not isinstance(srev, str) or doc.get("rev") != srev:
        return "file %s rev %r, slots rev %r" % (name, doc.get("rev"), srev), None
    return "", doc


def judge_g(D, K, root, uses, ctx, writes, out):
    steer, bad_s = ndjson(matches(D, K["T2K_STEER_LOG"]))
    recs, bad_r = ndjson(matches(D, K["T2K_RECORDS"]))
    pre = K["T2K_UNITBRIEF_PREFIX"]
    steer = [d for d in steer if d.get("type") == "steer"]
    ub = [d for d in steer if d.get("pattern") == "task-preflight" and d.get("event") == "PostToolUse"
          and str(d.get("key") or "").startswith(pre)]
    setup = []
    if any(d.get("delivery") == "holdout" for d in steer):
        setup.append("a steer line with delivery holdout (2a)")
    if any(d.get("suppressed_by") == "arm_off" for d in steer):
        setup.append("a steer line suppressed_by arm_off (2b)")
    per_prompt = collections.Counter((str(d.get("session_id")), str(d.get("prompt_id") or "")) for d in steer
                                     if counted(d))
    cap_v = sorted("%s/%s=%d" % (s, p, n) for (s, p), n in per_prompt.items() if n > CAP)
    ex = {r["seq"]: str(r.get("tool_use_id")) for r in recs
          if r.get("type") == "execution" and isinstance(r.get("seq"), int) and not isinstance(r.get("seq"), bool)}
    misses = collections.Counter(str(r.get("reason")) for r in recs if r.get("type") == "unitbrief_miss")
    by_unit, bad_files = briefs(D, K["T2K_UNITBRIEF_DIR"])
    pos = {}
    for i, (uid, _, _) in enumerate(uses):
        pos.setdefault(uid, i)
    rows, units, owed, mism = [], collections.Counter(), 0, []
    for d in ub:
        if not given(d):
            owed += 1
            continue
        unit, dig = split_key(str(d["key"]), pre)
        units[unit] += 1
        row = {"unit": unit, "digest": dig, "after_seq": d.get("after_seq"), "prompt_id": d.get("prompt_id")}
        rows.append(row)
        if d.get("delivery") != "delivered":
            row["verdict"] = "given, not reached (%s)" % (d.get("delivery") or d.get("suppressed_by"))
            continue
        uid = ex.get(d.get("after_seq"))
        if uid is None or uid not in pos:
            row["verdict"] = "unjoinable: %s" % ("no execution record at after_seq" if uid is None
                                                 else "tool_use_id %s not in the transcript" % uid)
            continue
        mt = d.get("model_text") or ""
        if not mt or not any(isinstance(c, str) and mt in c for c in ctx.get(uid, [])):
            row["verdict"] = "claimed, not seen"
            continue
        row["reached"] = True
        bpath = rel(uses[pos[uid]][2], root)
        row["briefed"] = bpath
        local = unit.split("#", 1)[-1]
        row["unit_dir"] = local[len("unit:go:"):] if local.startswith("unit:go:") else None
        if row["unit_dir"] is not None and not (bpath and bpath.endswith(".go") and unit_dir(bpath) == row["unit_dir"]):
            mism.append("%s edit %s" % (unit, bpath))
        slots = d.get("slots") if isinstance(d.get("slots"), dict) else {}
        why, doc = measurable(by_unit.get(unit), dig, slots.get("rev"))
        if why:
            row["verdict"] = "replaced, not measurable: " + why
            continue
        # Addendum 7: UnitSaid (internal/graph/engine/unitbrief.go, as/m2-w23h 71450817) may say the file's first n
        # lines only; the digest covers the whole file. A said slot is a line<K> slot whose kind is a non-empty
        # string; setGraphLines leaves the slots past n zero, and graphLineForm (internal/steer/slots_graph.go)
        # encodes a zero line with kind "" (slot.GraphLine.Kind). The record form keeps the kind (GraphLine.Record).
        said = sum(1 for k, v in slots.items() if re.fullmatch(r"line[1-9][0-9]*", k) and isinstance(v, dict)
                   and isinstance(v.get("kind"), str) and v["kind"])
        lines = doc["lines"] if isinstance(doc, dict) and isinstance(doc.get("lines"), list) else []
        row["said"], row["file_lines"] = said, len(lines)
        if said == 0 or said > len(lines):
            setup.append("unit-brief line %s says %d lines of a file holding %d (2d)" % (unit, said, len(lines)))
            row["verdict"] = "said count out of range"
            continue
        n = {canon(x, root) for x in named({"lines": lines[:said]})}
        row["named"] = sorted(n)
        if not n:
            row["verdict"] = "nothing named"
            continue
        s = second(writes, pos[uid], bpath)
        row["second"] = s
        if s is None:
            row["verdict"] = "no second write"
            continue
        row["conforms"] = s in n
        row["verdict"] = "scored"
    v = collections.Counter(r["verdict"].split(":")[0] for r in rows)
    scored = [r for r in rows if r["verdict"] == "scored"]
    conf = sum(1 for r in scored if r["conforms"])
    once_v = sorted("%s=%d" % (u, n) for u, n in units.items() if n > 1)
    c = {"given": len(rows), "units": len(units), "reached": sum(1 for r in rows if r.get("reached")),
         "once_violations": once_v, "cap_violations": cap_v, "claimed_not_seen": v["claimed, not seen"],
         "unjoinable": v["unjoinable"], "owed_lines": owed, "replaced": v["replaced, not measurable"],
         "nothing_named": v["nothing named"], "no_second_write": v["no second write"], "scored": len(scored),
         "conforming": conf, "unit_rule_mismatch": mism, "misses": dict(sorted(misses.items())),
         "unparsed_lines": bad_s + bad_r, "unreadable_brief_files": bad_files}
    out.update(counts=c, lines=rows)
    if setup:
        out["setup"] = "setup failure: " + "; ".join(setup)
        return
    out["unitbrief_delivered"] = ("reached %d of %d given, %d units; once-violations %d; cap-violations %d; "
                                  "claimed-not-seen %d; unjoinable %d; owed lines %d; misses absent=%d stale=%d "
                                  "refused=%d") % (c["reached"], c["given"], c["units"], len(once_v), len(cap_v),
                                                   c["claimed_not_seen"], c["unjoinable"], owed, misses["absent"],
                                                   misses["stale"], misses["refused"])
    out["second_write_conforms"] = ("%d/%d; no second write %d; replaced %d; nothing named %d; unit-rule mismatch "
                                    "%d") % (conf, len(scored), c["no_second_write"], c["replaced"],
                                             c["nothing_named"], len(mism))


def judge_other(D, K, arm, out):
    pre = K["T2K_UNITBRIEF_PREFIX"]
    steer, _ = ndjson(matches(D, K["T2K_STEER_LOG"]))
    setup = []
    if any(str(d.get("key") or "").startswith(pre) for d in steer):
        setup.append("a unit-brief steer line on arm %s (2c)" % arm)
    if any(d.get("delivery") == "holdout" for d in steer):
        setup.append("a steer line with delivery holdout (2a)")
    if any(d.get("suppressed_by") == "arm_off" for d in steer):
        setup.append("a steer line suppressed_by arm_off (2b)")
    if matches(D, os.path.join(K["T2K_UNITBRIEF_DIR"], "*.json")):
        setup.append("a unit-brief file on arm %s (2c)" % arm)
    if setup:
        out["setup"] = "setup failure: " + "; ".join(setup)
        return
    out["unitbrief_delivered"] = out["second_write_conforms"] = "n/a (arm %s: no unit brief)" % arm


def run(D):
    J = os.path.join(D, "judge")
    out: dict = {"setup": "ok"}
    try:
        with open(os.path.join(D, "arm")) as f:
            arm = f.read().strip()
        K = keys()
    except OSError as e:
        print("j2.py: %s" % e, file=sys.stderr)
        return 3
    out["arm"] = arm
    miss = [k for k in NEED if not K.get(k) or K[k] == "TBD"]
    tpath = os.path.join(D, "transcript.jsonl")
    if miss or not os.path.isfile(tpath) or (arm == "G" and not matches(D, K["T2K_STEER_LOG"])):
        why = ("keys.txt lacks " + " ".join(miss)) if miss else ("no transcript" if not os.path.isfile(tpath)
                                                                 else "no steer log")
        out["unitbrief_delivered"] = out["second_write_conforms"] = "unread: " + why
        rc = 3
    else:
        root, uses, done, ctx = transcript(tpath)
        writes = landed_writes(uses, done, root)
        out["tree_root"] = root
        out["first_writes"] = first_writes(writes)
        if arm == "G":
            judge_g(D, K, root, uses, ctx, writes, out)
        else:
            judge_other(D, K, arm, out)
        if out["setup"] != "ok":
            out["unitbrief_delivered"] = out["second_write_conforms"] = out["setup"]
        rc = 0
    os.makedirs(J, exist_ok=True)
    with open(os.path.join(J, "j2.json"), "w") as f:
        json.dump(out, f, indent=1, sort_keys=True)
    print(json.dumps(out, sort_keys=True))
    return rc


def load(path):
    try:
        with open(path) as f:
            return json.load(f)
    except (OSError, ValueError):
        return None


def pair(args):
    groups, cur = [], []
    for a in args:
        if a == "--":
            groups.append(cur)
            cur = []
        else:
            cur.append(a.rstrip("/"))
    groups.append(cur)
    pool = collections.defaultdict(collections.Counter)
    notes = collections.Counter()
    for g in groups:
        got = [(p, load(os.path.join(p, "judge", "j2.json"))) for p in g]
        if any(not isinstance(j, dict) for _, j in got):
            print("j2.py pair: unreadable j2.json in %s" % " ".join(p for p, j in got if not isinstance(j, dict)),
                  file=sys.stderr)
            return 3
        js = [(p, j) for p, j in got if isinstance(j, dict)]
        gs = [j for _, j in js if j.get("arm") == "G"]
        if len(gs) != 1:
            print("j2.py pair: group %s holds %d G runs, want 1" % (" ".join(g), len(gs)), file=sys.stderr)
            return 3
        G = gs[0]
        if G.get("setup") != "ok":
            notes["groups with G setup failure"] += 1
            continue
        scored = [r for r in G.get("lines") or [] if r.get("verdict") == "scored"]
        pool["G"]["scored"] += len(scored)
        pool["G"]["conforming"] += sum(1 for r in scored if r.get("conforms"))
        for _, j in js:
            arm = j.get("arm")
            if arm == "G":
                continue
            if j.get("setup") != "ok":
                notes["%s runs with setup failure" % arm] += 1
                continue
            fw = j.get("first_writes") or {}
            for r in scored:
                w = fw.get(r.get("unit_dir")) if r.get("unit_dir") is not None else None
                if r.get("unit_dir") is None:
                    pool[arm]["not a go unit"] += 1
                elif not w:
                    pool[arm]["no write in unit"] += 1
                elif w.get("second") is None:
                    pool[arm]["no second write"] += 1
                else:
                    pool[arm]["scored"] += 1
                    pool[arm]["conforming"] += w["second"] in (r.get("named") or [])
    res = {"groups": len(groups), "arms": {a: dict(c) for a, c in sorted(pool.items())}, "notes": dict(notes)}
    if not pool["G"]["scored"]:
        res["line"] = "no baseline: no scored G line"
    else:
        parts = []
        for a in sorted(k for k in pool if k != "G"):
            c = pool[a]
            parts.append("%s %d/%d (no write in unit %d, no second write %d)" % (
                a, c["conforming"], c["scored"], c["no write in unit"], c["no second write"]))
        res["line"] = "R5 second write conforms: G %d/%d beside %s" % (
            pool["G"]["conforming"], pool["G"]["scored"], "; ".join(parts) or "no other arm")
    print(json.dumps(res, sort_keys=True))
    return 0


if __name__ == "__main__":
    if len(sys.argv) >= 3 and sys.argv[1] == "pair":
        sys.exit(pair(sys.argv[2:]))
    if len(sys.argv) != 2:
        print(__doc__, file=sys.stderr)
        sys.exit(2)
    sys.exit(run(sys.argv[1].rstrip("/")))
