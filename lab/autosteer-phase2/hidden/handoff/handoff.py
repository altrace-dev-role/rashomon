"""Phase 2 hidden handoff checks, the same for every arm (H1, H2, H2b, H3, H4, H8, items 2b and 2c, HTML safety).

Usage: python3 handoff.py <arm> [<commit>]     (arm: opus | sonnet | blank; commit defaults to the branch head)

Each arm's scanner runs from `git archive` of the commit, in a container holding only its runtime interpreter
and its requirements.txt (R9), as uid 1000, with --network none, on python:2.7.18-slim and python:3.14-slim.
Inputs (inputs/all): the 2.7 stdlib from python:2.7.18-slim, Trac 1.4.4 and MoinMoin 1.9.11, all frozen.
Truth: truth/versions (run.sh: real 3.6.15 to 3.15.1), truth_p2.py (2.7), truth_builtin.py (2.7 bytecode).
Output: out/<arm>-<commit12>/ with report.json and the raw scans.
"""
import collections
import hashlib
import io
import json
import os
import re
import subprocess
import sys
import tarfile
import time

HOME = os.path.expanduser("~")
HERE = os.path.dirname(os.path.abspath(__file__))
P2 = os.path.dirname(HERE)
ARMS = {"opus": ("rashomon-eval/py27/opus/py2scan", "py-migrate/py2scan-3.14"),
        "sonnet": ("rashomon-eval/py27/sonnet/py2scan", "py-migrate/py2scan-py314"),
        "blank": ("rashomon-eval/py27/blank/py2scan", "py-migrate/py2scan")}
SIDES = {"2.7": "2.7.18", "3.14": "3.14"}
JUDGED = ["3.6", "3.7", "3.8", "3.9", "3.10", "3.11", "3.12", "3.13", "3.14", "3.15"]

EXTRA_CASES = r"""
mk() { mkdir -p "$1/$2"; printf 'print(1)\n' > "$1/$2/ok.py"; }
mk "$1" canary
for f in __init__.py mod.py setup.py conftest.py __main__.py; do
  printf 'open("/tmp/CANARY_%s", "w").write("imported or executed")\nprint "x"\n' "$f" > "$1/canary/$f"
done
mk "$1" html
printf 'print "</script><script>alert(2)</script>"\n' > "$1/html/<img src=x onerror=alert(1)>.py"
"""

IN_CONTAINER = r"""
set -u
mkdir -p /tmp/w /tmp/in /tmp/o /tmp/h && cd /tmp/in && tar -xf - && mv src/* src/.[!.]* /tmp/w/ 2>/dev/null; cd /tmp/w
python -c "import sys; print(sys.version.split()[0])" > /tmp/o/version.txt
scan() { # name, timeout, args...
  n=$1; t=$2; shift 2
  s=$(date +%s.%N)
  timeout "$t" python -m py2scan scan "$@" > "/tmp/o/$n.json" 2> "/tmp/o/$n.err"; echo $? > "/tmp/o/$n.rc"
  e=$(date +%s.%N); echo "$s $e" | awk '{printf "%.2f\n", $2 - $1}' > "/tmp/o/$n.sec"
}
scan all 1800 --format json /tmp/in/all
scan stdlib_j1 900 --format json --jobs 1 /tmp/in/all/stdlib
scan stdlib_jd 900 --format json /tmp/in/all/stdlib
sh /tmp/in/hostile_holdout.sh /tmp/h > /tmp/o/hostile_setup.txt 2>&1
sh /tmp/in/extra_cases.sh /tmp/h >> /tmp/o/hostile_setup.txt 2>&1
for c in $(ls /tmp/h | grep -v '^outside.py$'); do
  [ -d "/tmp/h/$c" ] || continue
  scan "h_${c}_1" 60 --format json "/tmp/h/$c"
  scan "h_${c}_2" 60 --format json "/tmp/h/$c"
done
scan h_html_report 60 --format json --html /tmp/o/report.html /tmp/h/html
ls /tmp/CANARY_* > /tmp/o/canary.txt 2>/dev/null; true
cd /tmp/o && tar -cf - .
"""


def sh(args, **kw):
    kw.setdefault("check", True)
    kw.setdefault("capture_output", True)
    return subprocess.run(args, **kw)


def git(arm, *args):
    return sh(["git", "-C", os.path.join(HOME, ARMS[arm][0])] + list(args)).stdout


def tar_of(files):
    buf = io.BytesIO()
    with tarfile.open(fileobj=buf, mode="w") as t:
        for name, data in files.items():
            info = tarfile.TarInfo(name)
            info.size = len(data)
            info.mode = 0o644
            t.addfile(info, io.BytesIO(data))
    return buf.getvalue()


def runtime_image(arm, sha, py):
    req = git(arm, "show", "%s:requirements.txt" % sha)
    try:
        con = git(arm, "show", "%s:constraints-build.txt" % sha)
    except subprocess.CalledProcessError:
        con = b""
    df = ("FROM python:%s-slim\nCOPY requirements.txt constraints-build.txt /req/\n"
          "RUN PIP_DISABLE_PIP_VERSION_CHECK=1 PIP_CONSTRAINT=/req/constraints-build.txt "
          "pip install --no-cache-dir -r /req/requirements.txt\n" % SIDES[py]).encode()
    tag = "hk/%s-%s:%s" % (arm, py, hashlib.sha256(req + con + df).hexdigest()[:12])
    if subprocess.run(["docker", "image", "inspect", tag], capture_output=True).returncode != 0:
        r = subprocess.run(["docker", "build", "-q", "-t", tag, "-"], capture_output=True,
                           input=tar_of({"Dockerfile": df, "requirements.txt": req, "constraints-build.txt": con}))
        if r.returncode:
            raise SystemExit("runtime image build failed for %s %s: %s" % (arm, py, r.stderr.decode()[-1500:]))
    return tag


def input_tar(arm, sha):
    """src/ (the commit), all/ (frozen inputs), the hostile holdout and the extra cases."""
    buf = io.BytesIO()
    with tarfile.open(fileobj=buf, mode="w") as t:
        src = tarfile.open(fileobj=io.BytesIO(git(arm, "archive", "--format=tar", sha)))
        for m in src.getmembers():
            data = src.extractfile(m) if m.isfile() else None
            m.name = "src/" + m.name
            t.addfile(m, data)
        t.add(os.path.join(HERE, "inputs", "all"), arcname="all")
        t.add(os.path.join(P2, "hostile_holdout.sh"), arcname="hostile_holdout.sh")
        info = tarfile.TarInfo("extra_cases.sh")
        info.size = len(EXTRA_CASES.encode())
        t.addfile(info, io.BytesIO(EXTRA_CASES.encode()))
    return buf.getvalue()


def run_side(arm, sha, py, tar, out):
    img = runtime_image(arm, sha, py)
    r = subprocess.run(["docker", "run", "-i", "--rm", "--network", "none", "--user", "1000:1000", "-e", "HOME=/tmp",
                        img, "sh", "-c", IN_CONTAINER], input=tar, capture_output=True, timeout=7200)
    d = os.path.join(out, py)
    os.makedirs(d, exist_ok=True)
    tarfile.open(fileobj=io.BytesIO(r.stdout)).extractall(d)
    return d


def load(d, name):
    p = os.path.join(d, name + ".json")
    raw = open(p, "rb").read() if os.path.exists(p) else b""
    rc = int(open(os.path.join(d, name + ".rc")).read().strip() or -1)
    err = open(os.path.join(d, name + ".err"), "rb").read().decode("utf-8", "replace")
    sec = float(open(os.path.join(d, name + ".sec")).read().strip() or -1)
    try:
        data = json.loads(raw.decode("utf-8"))
    except ValueError:
        data = None
    return {"raw": raw, "rc": rc, "err": err, "sec": sec, "json": data, "traceback": "Traceback (most recent call last)" in err}


def blocked_versions(findings, root="/tmp/in/all"):
    """Audit H: arm paths are normalised (./x, absolute) before they are keyed against the truth paths."""
    b = collections.defaultdict(set)
    for f in findings:
        if f.get("severity") == "blocker" and isinstance(f.get("file"), str):
            b[norm(f["file"], root)].update(f.get("versions") or JUDGED)
    return b


def compile_truth():
    fails = {}
    for v in JUDGED:
        fails[v] = {ln.split("\t")[1] for ln in open(os.path.join(HERE, "truth", "versions", v, "compile.txt"))
                    if ln.startswith("FAIL\t")}
    return fails


def false_ready(findings, fails):
    b = blocked_versions(findings)
    return {v: sorted(f for f in fails[v] if v not in b.get(f, set())) for v in JUDGED}


def first_diff(a, b):
    if a is None or b is None:
        return "unparsable JSON on one side"
    if a == b:
        return "same content, different bytes (key order or formatting)"
    for k in sorted(set(a) | set(b)):
        if a.get(k) != b.get(k):
            if k == "findings":
                sa = {json.dumps(x, sort_keys=True) for x in a[k]}
                sb = {json.dumps(x, sort_keys=True) for x in b[k]}
                return "findings: %d only on 2.7, %d only on 3.14; e.g. %s" % (
                    len(sa - sb), len(sb - sa), (sorted(sa - sb) or sorted(sb - sa))[0][:200])
            return "key %s differs" % k
    return "?"


def truth_27(arm_json_path, root_in_tar, out_name, d):
    """truth_p2.py inside python:2.7.18-slim against one scan JSON."""
    files = {"truth_p2.py": open(os.path.join(HERE, "truth_p2.py"), "rb").read(),
             "truth_guarded_r2.py": open(os.path.join(P2, "truth_guarded_r2.py"), "rb").read(),
             "removed.json": open(os.path.join(P2, "versions", "stdlib_removed.json"), "rb").read(),
             "arm.json": open(arm_json_path, "rb").read()}
    buf = io.BytesIO()
    with tarfile.open(fileobj=buf, mode="w") as t:
        for n, data in files.items():
            i = tarfile.TarInfo("t/" + n)
            i.size = len(data)
            t.addfile(i, io.BytesIO(data))
        t.add(os.path.join(HERE, "inputs", "all"), arcname="all")
    r = subprocess.run(["docker", "run", "-i", "--rm", "--network", "none", "--user", "1000:1000", "-e", "HOME=/tmp",
                        "python:2.7.18-slim", "sh", "-c",
                        "mkdir /tmp/x && cd /tmp/x && tar -xf - && python t/truth_p2.py %s t/removed.json t/arm.json /tmp/r.json "
                        "> /dev/null && cat /tmp/r.json" % root_in_tar], input=buf.getvalue(), capture_output=True)
    if r.returncode:
        return {"error": r.stderr.decode()[-1500:]}
    open(os.path.join(d, out_name), "wb").write(r.stdout)
    return json.loads(r.stdout)


def truth_builtin(stdlib_json_path, d):
    files = {"truth_builtin.py": open(os.path.join(P2, "truth_builtin.py"), "rb").read(),
             "x/sonnet.json": b'{"findings": []}', "x/opus.json": b'{"findings": []}', "x/blank.json": b'{"findings": []}'}
    arm = os.path.basename(os.path.dirname(d)).split("-")[0]
    files["x/%s.json" % arm] = open(stdlib_json_path, "rb").read()
    buf = io.BytesIO()
    with tarfile.open(fileobj=buf, mode="w") as t:
        for n, data in files.items():
            i = tarfile.TarInfo("t/" + n)
            i.size = len(data)
            t.addfile(i, io.BytesIO(data))
        t.add(os.path.join(HERE, "inputs", "all", "stdlib"), arcname="stdlib")
    r = subprocess.run(["docker", "run", "-i", "--rm", "--network", "none", "--user", "1000:1000", "-e", "HOME=/tmp",
                        "python:2.7.18-slim", "sh", "-c",
                        "mkdir /tmp/x && cd /tmp/x && tar -xf - && python t/truth_builtin.py stdlib t/x"],
                       input=buf.getvalue(), capture_output=True)
    lines = r.stdout.decode().splitlines()
    return [ln for ln in lines if ln.startswith(("control", arm))] or [r.stderr.decode()[-800:]]


# C54 (audit B, 23:16): R5 bullet 2 lets an entry that is neither a regular file nor a directory be skipped, if it
# is listed; a FIFO is such an entry, so only a directory that cannot be listed demands exit 2.
HOSTILE_UNREADABLE = {"unlistable"}
# H3 listing, R5 only: the entry that cannot be read or is not a regular file must be listed with its reason.
# R6 (every regular file named) is measured on the corpus instead, as R6_files_not_named.
HOSTILE_ENTRIES = {"fifo": ["t.py"], "unlistable": ["d"], "dirlink": ["pkg.py"], "escape": ["t.py"]}


def norm(path, root):
    path = path.replace("\\", "/")
    if root and path.startswith(root + "/"):
        path = path[len(root) + 1:]
    while path.startswith("./"):
        path = path[2:]
    return path


def strings(o):
    if isinstance(o, dict):
        for k, v in o.items():
            yield k
            for x in strings(v):
                yield x
    elif isinstance(o, list):
        for v in o:
            for x in strings(v):
                yield x
    elif isinstance(o, str):
        yield o


def unlisted(report, root, entries):
    """Entries no string in the report names, as a whole string or a whole token (C21: never a substring)."""
    seen = set()
    for x in strings(report):
        seen.add(norm(x, root))
        for tok in re.split(r"[\s\"'()\[\]{},;]+", x):
            tok = tok.rstrip(".:")
            if tok:
                seen.add(norm(tok, root))
                seen.add(norm(tok.split(":")[0], root))
    out = []
    for e in entries:
        if e.endswith("*"):
            hit = any(x.startswith(e[:-1]) for x in seen)
        else:
            hit = e in seen or any(x.startswith(e + "/") for x in seen)
        if not hit:
            out.append(e)
    return out


def hostile(d):
    cases = sorted({m.group(1) for n in os.listdir(d) for m in [re.match(r"h_(.+)_1\.rc$", n)] if m})
    res = {}
    for c in cases:
        a, b = load(d, "h_%s_1" % c), load(d, "h_%s_2" % c)
        problems = []
        if a["rc"] == 124:
            problems.append("timeout 60 s")
        if a["traceback"]:
            problems.append("traceback: " + a["err"].strip().splitlines()[-1][:160])
        if a["json"] is None and a["rc"] != 124:
            problems.append("invalid JSON")
        if a["raw"] != b["raw"]:
            problems.append("two runs differ")
        if c in HOSTILE_UNREADABLE and a["rc"] != 2:
            problems.append("exit %d, want 2 (an entry cannot be read)" % a["rc"])
        if c not in HOSTILE_UNREADABLE and a["rc"] not in (0, 1, 2):
            problems.append("exit %d" % a["rc"])
        if a["json"] is not None:
            gone = unlisted(a["json"], "/tmp/h/" + c, HOSTILE_ENTRIES.get(c, []))
            if gone:
                problems.append("not listed in the report (R5): %s" % ", ".join(gone)[:160])
        res[c] = {"rc": a["rc"], "sec": a["sec"], "ok": not problems, "problems": problems,
                  "findings": [(f["file"], f["line"], f["rule"], f["severity"]) for f in (a["json"] or {}).get("findings", [])][:12]}
    return res


def html_safe(d):
    p = os.path.join(d, "report.html")
    if not os.path.exists(p):
        return {"ok": False, "why": "no HTML written", "rc": load(d, "h_html_report")["rc"]}
    h = open(p, "rb").read().decode("utf-8", "replace")
    bad = [s for s in ("<img src=x", "<script>alert(2)", "</script><script>") if s in h]
    return {"ok": not bad, "raw_injections_found": bad, "bytes": len(h)}


def corpus_files():
    root = os.path.join(HERE, "inputs", "all")
    return sorted(os.path.relpath(os.path.join(d, f), root) for d, _, fs in os.walk(root) for f in fs if f.endswith(".py"))


def r6_unnamed(report):
    """R6: for each file the report says which versions it is ready for. Files the corpus report never names."""
    gone = unlisted(report, "/tmp/in/all", corpus_files())
    return {"count": len(gone), "of": len(corpus_files()), "examples": gone[:4]}


def controls():
    fails = {v: {"stdlib/a.py"} for v in JUDGED}
    for spelling in ("stdlib/a.py", "./stdlib/a.py", "/tmp/in/all/stdlib/a.py"):
        assert false_ready([{"file": spelling, "severity": "blocker"}], fails) == {v: [] for v in JUDGED}, spelling
    assert false_ready([{"file": "stdlib/b.py", "severity": "blocker"}], fails)["3.6"] == ["stdlib/a.py"]
    rep = {"files": ["simplejson/__init__.py", "/r/with space.py"], "skipped": ["d/up (symlink loop)"]}
    assert unlisted(rep, "/r", ["json/__init__.py", "with space.py", "d/up", "ok.py"]) == ["json/__init__.py", "ok.py"]


def main():
    controls()
    arm = sys.argv[1]
    sha = (sys.argv[2] if len(sys.argv) > 2 else git(arm, "rev-parse", ARMS[arm][1]).decode().strip())
    sha = git(arm, "rev-parse", sha).decode().strip()
    out = os.path.join(HERE, "out", "%s-%s" % (arm, sha[:12]))
    os.makedirs(out, exist_ok=True)
    t0 = time.time()
    tar = input_tar(arm, sha)
    if "--rescore" in sys.argv and all(os.path.isdir(os.path.join(out, py)) for py in SIDES):
        sides = {py: os.path.join(out, py) for py in SIDES}  # score the kept outputs again; run nothing
    else:
        sides = {py: run_side(arm, sha, py, tar, out) for py in SIDES}
    fails = compile_truth()
    rep = {"arm": arm, "commit": sha, "started": time.strftime("%Y-%m-%dT%H:%M:%S", time.localtime(t0)), "sides": {}}
    scans = {py: load(d, "all") for py, d in sides.items()}
    for py, d in sides.items():
        s = scans[py]
        j = s["json"] or {"findings": []}
        fr = false_ready(j["findings"], fails)
        st1, std = load(d, "stdlib_j1"), load(d, "stdlib_jd")
        rep["sides"][py] = {
            "interpreter": open(os.path.join(d, "version.txt")).read().strip(),
            "corpus_scan": {"rc": s["rc"], "sec": s["sec"], "traceback": s["traceback"], "json_valid": s["json"] is not None,
                            "files": j.get("files"), "findings": len(j["findings"]),
                            "rules": dict(collections.Counter(f["rule"] for f in j["findings"]))},
            "H2_H8_false_ready_by_version": {v: len(x) for v, x in fr.items()},
            "R6_files_not_named": r6_unnamed(j),
            "false_ready_examples": {v: x[:4] for v, x in fr.items() if x},
            "determinism_jobs1_vs_default": st1["raw"] == std["raw"] and st1["json"] is not None,
            "H3_hostile": hostile(d),
            "html": html_safe(d),
            "item_2c_canary": open(os.path.join(d, "canary.txt")).read().split(),
        }
        rep["sides"][py]["H2b_H8s"] = truth_27(os.path.join(d, "all.json"), "all", "truth_p2.json", d)
        rep["sides"][py]["H1_builtin"] = truth_builtin(os.path.join(d, "stdlib_jd.json"), d)
    a, b = scans["2.7"], scans["3.14"]
    rep["item_2b_byte_identical_corpus"] = a["raw"] == b["raw"]
    rep["item_2b_corpus_diff"] = None if a["raw"] == b["raw"] else first_diff(a["json"], b["json"])
    h27, h314 = sides["2.7"], sides["3.14"]
    rep["item_2b_hostile_cases_differing"] = sorted(
        c for c in rep["sides"]["2.7"]["H3_hostile"]
        if load(h27, "h_%s_1" % c)["raw"] != load(h314, "h_%s_1" % c)["raw"])
    rep["elapsed_sec"] = round(time.time() - t0)
    json.dump(rep, open(os.path.join(out, "report.json"), "w"), indent=1, sort_keys=True)
    print(json.dumps(rep, indent=1, sort_keys=True)[:6000])


if __name__ == "__main__":
    main()
