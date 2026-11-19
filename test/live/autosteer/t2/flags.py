#!/usr/bin/env python3
"""flags.py RUNDIR ARM: r3 section 7's flags for one run, each one line in RUNDIR/flags (this script's earlier lines
replaced, every other line kept) and printed. tools.txt and cap_hit are batch.sh's.
  NO-CONSENT             any arm whose consent.txt lacks T2K_CONSENT_SCOPE (U, R, and G without it) holding a file
                         named T2K_OVERLAY_GLOB in its run directory (judge/ and the tracked tree excluded;
                         tree/.rashomon and tree/.pgraph read).
  overlay-full           G: a file named T2K_OVERLAY_FULL.
  duplicate_abstraction  G: T2K_DUP_KEY in a steer log line, a hooks.log line or an attachment record's text.
  plan-pre-line          G: T2K_PLAN_PRE_KEY in a PreToolUse attachment record's text, or in a steer log line that
                         also holds PreToolUse; and, counted "undelivered", a steer log line holding PreToolUse and
                         T2K_PLAN_PRE_PATTERN without T2K_PLAN_PRE_KEY (a plan-check line the model never saw: not
                         treatment, but the row shows the engine fired; the director 10:13 PST item 6). The flag
                         fires when any of the three counts is non-zero.
T2K_STEER_LOG is a glob relative to RUNDIR (the engine names the log per session); every match is read, in name order.
Exit 3 when keys.txt lacks a value or holds TBD for a key read here, so judge.sh writes no row.
"""
import fnmatch, glob, json, os, sys

HERE = os.path.dirname(os.path.abspath(__file__))
NEED = ("T2K_STEER_LOG", "T2K_OVERLAY_GLOB", "T2K_OVERLAY_FULL", "T2K_DUP_KEY", "T2K_PLAN_PRE_KEY", "T2K_PLAN_PRE_PATTERN", "T2K_CONSENT_SCOPE")
KINDS = ("NO-CONSENT", "overlay-full", "duplicate_abstraction", "plan-pre-line")


def keys():
    k = {}
    with open(os.path.join(HERE, "keys.txt"), encoding="utf-8") as f:
        for line in f:
            line = line.strip()
            if line and not line.startswith("#") and "=" in line:
                n, v = line.split("=", 1)
                v = v.strip()
                if len(v) >= 2 and v[0] == v[-1] and v[0] in "'\"":
                    v = v[1:-1]
                k[n.strip()] = v
    return k


def read(p):
    try:
        with open(p, encoding="utf-8", errors="replace") as f:
            return f.read()
    except OSError:
        return ""


def strings(x):
    if isinstance(x, str):
        yield x
    elif isinstance(x, dict):
        for v in x.values():
            yield from strings(v)
    elif isinstance(x, list):
        for v in x:
            yield from strings(v)


def attachments(D):
    out = []
    for i, raw in enumerate(read(os.path.join(D, "transcript.jsonl")).splitlines(), 1):
        try:
            d = json.loads(raw)
        except ValueError:
            continue
        if isinstance(d, dict) and d.get("type") == "attachment":
            a = d.get("attachment") or {}
            out.append((i, str(a.get("hookName")), [l for s in strings(a) for l in s.splitlines()]))
    return out


def run_files(D):
    out = []
    for top in (D, os.path.join(D, "tree", ".rashomon"), os.path.join(D, "tree", ".pgraph")):
        for d, dirs, names in os.walk(top):
            if d == D:
                dirs[:] = [x for x in dirs if x not in ("judge", "tree")]
            out += [os.path.relpath(os.path.join(d, n), D) for n in names]
    return sorted(out)


D, arm = sys.argv[1].rstrip("/"), sys.argv[2]
K = keys()
bad = [n for n in NEED if K.get(n, "TBD") in ("", "TBD")]
if bad:
    print("flags.py: keys.txt has no value for %s; no flag is read" % ", ".join(bad), file=sys.stderr)
    sys.exit(3)
files = run_files(D)
new = []
if K["T2K_CONSENT_SCOPE"] not in read(os.path.join(D, "consent.txt")):
    ov = [f for f in files if fnmatch.fnmatch(os.path.basename(f), K["T2K_OVERLAY_GLOB"])]
    if ov:
        new.append("NO-CONSENT %s: %d overlay file(s) with no session scope in consent.txt: %s"
                   % (arm, len(ov), " ".join(ov[:5])))
if arm == "G":
    full = [f for f in files if os.path.basename(f) == K["T2K_OVERLAY_FULL"]]
    if full:
        new.append("overlay-full: %s" % " ".join(full))
    steer = [l for p in sorted(glob.glob(os.path.join(D, K["T2K_STEER_LOG"]))) for l in read(p).splitlines()]
    hooks = read(os.path.join(D, "hooks.log")).splitlines()
    att = attachments(D)
    dk, pk, pp = K["T2K_DUP_KEY"], K["T2K_PLAN_PRE_KEY"], K["T2K_PLAN_PRE_PATTERN"]
    dup = ["steer log %d" % sum(dk in l for l in steer), "hooks.log %d" % sum(dk in l for l in hooks),
           "attachment records %s" % ([i for i, _, ls in att if any(dk in l for l in ls)][:10] or 0)]
    if any(dk in l for l in steer + hooks) or any(any(dk in l for l in ls) for _, _, ls in att):
        new.append("duplicate_abstraction: %s" % "; ".join(dup))
    pre = [i for i, h, ls in att if h.startswith("PreToolUse") and any(pk in l for l in ls)]
    spre = sum(pk in l and "PreToolUse" in l for l in steer)
    upre = sum(pp in l and "PreToolUse" in l and pk not in l for l in steer)
    if pre or spre or upre:
        new.append("plan-pre-line: attachment records %s; steer log lines %d; undelivered %d" % (pre[:10] or 0, spre, upre))
old = [l for l in read(os.path.join(D, "flags")).splitlines() if l.strip() and not l.startswith(KINDS)]
with open(os.path.join(D, "flags"), "w", encoding="utf-8") as f:
    f.write("".join(l + "\n" for l in old + new))
for l in new or ["flags: none of %s" % ", ".join(KINDS)]:
    print(l)
