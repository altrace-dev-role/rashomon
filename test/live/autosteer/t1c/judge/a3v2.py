#!/usr/bin/env python3
"""A3', A4', A5': T1c's secondaries, read against what the product emits (pins.txt delta 3; precheck 3f).

Grounding: the callers record is BlastCaller{Package, Dir, Tests} (internal/hook/blastradius.go:47); blastMessage
renders a count, the caller directories (a caller at the root is counted, never listed) and how many hold a test
file (pattern blast-radius/v1, internal/steer/render/render.go:444):
  "rashomon record: the platform graph names other packages with code calling into the package this call edited
   (at least {count}): {dirs}; test files are in {tested} of them."
(an earlier binary renders the count as "(at least N proven, M proven or possible)"). The Stop renders
"platform: go test ./<dir>/... <what>" (internal/recap/platform.go:93). No symbol is rendered anywhere.

  a3v2.py oracle FIXTURE_DIR EDITED_DIR        -> {count, dirs, tested} from .pgraph/pgraph.log: the other packages
     of the module with a PROVEN calls fact whose object is a symbol of the edited package, as the product counts
     (internal/platform/callers.go:15-18: a may-call through an interface, ALGORITHMIC, is not in it; the set is a
     lower bound, hence "at least"); a caller at the root is counted and unlisted; tested = those with a _test.go.
  a3v2.py judge  FIXTURE_DIR EDITED_DIR PROBE  -> verdict from PROBE/PostToolUse.txt and PROBE/Stop.txt
  a3v2.py run    RUNDIR                        -> one verdict per a3.sh replay session (oracle-symbols.txt rows, read
     from RUNDIR/judge/a3-<fixture>-<dir>-<Event>.txt against RK_FIXTURES/<fixture>), the run's aggregate, written to
     RUNDIR/judge/a3v2.json. Aggregate: a session is informative when the oracle names a caller, silent otherwise.
       A3' = both when every informative session is both; neither when no session renders anything; else mixed.
       A4' = unparsed when any session is; else match when every informative session matches and every silent one is
             silent-right; else silent when an informative session gave no note and none mismatched; else mismatch.
       A5' = bounded when every note is a lower bound; exact when any note is a bare count; silent when no note.
     A run whose a3.sh build failed (judge/a3.json build "failed") is unparsed on all three.
Verdicts: a3 = both (callers note at PostToolUse and a platform line at Stop) | one | neither;
          a4 = match (the note's count, dirs set and tested count equal the oracle's) | mismatch | unparsed | silent-right
               (no note and the oracle names no caller) ;
          a5 = bounded (the count is phrased as a lower bound) | silent (no note) | exact (a bare count: fail).
"""
import json, os, re, sys

def rows(log_path):
    out = []
    with open(log_path, encoding="utf-8") as f:
        if f.readline().rstrip("\n") != "pgraph-log v0":
            raise SystemExit("a3v2: unknown log header in " + log_path)
        for line in f:
            p = line.rstrip("\n").split("\t")
            if len(p) == 8 and p[0] == "+":
                out.append(p)
    return out

def pkg_path(sym):
    body = sym.split(":", 1)[1]
    if "/" in body:
        head, tail = body.rsplit("/", 1)
        return head + "/" + tail.split(".")[0]
    return body.split(".")[0]

def oracle(fixture, edited_dir):
    module = [l.split()[1] for l in open(os.path.join(fixture, "go.mod")) if l.startswith("module ")][0]
    edited = module + "/" + edited_dir.strip("/") if edited_dir not in ("", ".") else module
    live = rows(os.path.join(fixture, ".pgraph", "pgraph.log"))
    callers = set()
    for r in live:
        if r[2] == "calls" and r[4] == "PROVEN" and pkg_path(r[3]) == edited:
            sp = pkg_path(r[1])
            if sp != edited and sp.startswith(module):
                callers.add(sp)
    dirs, unlisted, tested = [], 0, 0
    for c in sorted(callers):
        rel = c[len(module):].strip("/")
        d = os.path.join(fixture, rel) if rel else fixture
        if any(f.endswith("_test.go") for f in os.listdir(d)):
            tested += 1
        if rel == "":
            unlisted += 1
        else:
            dirs.append(rel)
    return {"count": len(callers), "dirs": dirs, "unlisted": unlisted, "tested": tested}

NOTE = re.compile(r"the platform graph names other packages with code calling into the package this call edited "
                  r"\((?P<bound>at least )?(?P<count>\d+)(?: proven, (?P<possible>\d+) proven or possible)?\): "
                  r"(?P<dirs>.*?); test files are in (?P<tested>\d+) of them\.")

def parse_note(text):
    m = NOTE.search(text)
    if not m:
        return None
    dirs = re.findall(r"'([^']+)'", m.group("dirs"))
    return {"bound": bool(m.group("bound")), "count": int(m.group("count")),
            "possible": int(m.group("possible")) if m.group("possible") else None,
            "dirs": dirs, "tested": int(m.group("tested"))}

def judge(fixture, edited_dir, probe):
    o = oracle(fixture, edited_dir)
    post = open(os.path.join(probe, "PostToolUse.txt"), errors="replace").read() if os.path.exists(os.path.join(probe, "PostToolUse.txt")) else ""
    stop = open(os.path.join(probe, "Stop.txt"), errors="replace").read() if os.path.exists(os.path.join(probe, "Stop.txt")) else ""
    note = parse_note(post)
    platform = re.search(r"platform: go test \./[^ ]+ ", stop) is not None
    rendered = (1 if note else 0) + (1 if platform else 0)
    a3 = {2: "both", 1: "one", 0: "neither"}[rendered]
    if note is None:
        a4 = "silent-right" if o["count"] == 0 and "calling into the package" not in post else ("unparsed" if "calling into the package" in post else "silent")
        a5 = "silent"
    else:
        a4 = "match" if sorted(note["dirs"]) == sorted(o["dirs"]) and note["tested"] == o["tested"] and note["count"] == o["count"] else "mismatch"
        a5 = "bounded" if note["bound"] else "exact"
    return {"oracle": o, "note": note, "platform_line": platform, "a3": a3, "a4": a4, "a5": a5}

def judge_files(fixture, edited_dir, post_path, stop_path):
    """The verdict from two hook stdout files (a3.sh's layout a3-<fixture>-<dir>-<Event>.txt)."""
    import tempfile, shutil
    d = tempfile.mkdtemp(prefix="a3v2-")
    try:
        for src, name in ((post_path, "PostToolUse.txt"), (stop_path, "Stop.txt")):
            if os.path.exists(src):
                shutil.copy(src, os.path.join(d, name))
        return judge(fixture, edited_dir, d)
    finally:
        shutil.rmtree(d)

def sessions(t1_dir):
    """(fixture, tag, edited_dir) per oracle-symbols.txt row, in a3.sh's replay order; tag = the file's directory."""
    out = []
    for line in open(os.path.join(t1_dir, "oracle-symbols.txt"), encoding="utf-8"):
        cols = line.rstrip("\n").split("\t")
        if line.startswith("#") or len(cols) < 4:
            continue
        d = os.path.dirname(cols[2])
        out.append((cols[0], d.split("/")[-1], d))
    return out

def aggregate(per):
    vals = list(per.values())
    informative = [v for v in vals if v["oracle"]["count"] > 0]
    silent = [v for v in vals if v["oracle"]["count"] == 0]
    if informative and all(v["a3"] == "both" for v in informative):
        a3 = "both"
    elif all(v["a3"] == "neither" for v in vals):
        a3 = "neither"
    else:
        a3 = "mixed"
    if any(v["a4"] == "unparsed" for v in vals):
        a4 = "unparsed"
    elif all(v["a4"] == "match" for v in informative) and all(v["a4"] == "silent-right" for v in silent):
        a4 = "match"
    elif any(v["a4"] == "silent" for v in informative) and not any(v["a4"] == "mismatch" for v in vals):
        a4 = "silent"
    else:
        a4 = "mismatch"
    notes = [v["note"] for v in vals if v["note"]]
    a5 = "silent" if not notes else ("bounded" if all(n["bound"] for n in notes) else "exact")
    return a3, a4, a5

def run(rundir, fixtures, t1_dir):
    J = os.path.join(rundir, "judge")
    a3_path = os.path.join(J, "a3.json")
    built = not (os.path.exists(a3_path) and json.load(open(a3_path)).get("build") == "failed")
    per = {}
    for fx, tag, d in sessions(t1_dir):
        sid = "%s-%s" % (fx, tag)
        per[sid] = judge_files(os.path.join(fixtures, fx), d, os.path.join(J, "a3-%s-PostToolUse.txt" % sid),
                               os.path.join(J, "a3-%s-Stop.txt" % sid))
    a3, a4, a5 = aggregate(per) if built else ("unparsed", "unparsed", "unparsed")
    out = {"run": os.path.basename(rundir.rstrip("/")), "built": built, "sessions": per, "a3": a3, "a4": a4, "a5": a5}
    json.dump(out, open(os.path.join(J, "a3v2.json"), "w"), indent=1)
    return out

if __name__ == "__main__":
    cmd = sys.argv[1]
    if cmd == "oracle":
        print(json.dumps(oracle(sys.argv[2], sys.argv[3])))
    elif cmd == "judge":
        print(json.dumps(judge(sys.argv[2], sys.argv[3], sys.argv[4]), indent=1))
    elif cmd == "run":
        o = run(sys.argv[2], os.environ["RK_FIXTURES"], os.path.join(os.path.dirname(os.path.abspath(__file__)), ".."))
        print(json.dumps({k: o[k] for k in ("run", "built", "a3", "a4", "a5")}))
        for sid, v in o["sessions"].items():
            print(" ", sid, v["a3"], v["a4"], v["a5"], (v["oracle"]["count"], v["oracle"]["dirs"], v["oracle"]["tested"]))
    elif cmd == "judge-files":
        print(json.dumps(judge_files(sys.argv[2], sys.argv[3], sys.argv[4], sys.argv[5])))
    else:
        raise SystemExit(__doc__)
