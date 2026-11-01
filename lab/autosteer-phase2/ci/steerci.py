"""AutoSteer in-loop CI, Phase 2 (owner approved 2026-10-22: "Yes for all three").

Runs the task's visible checks (P2-task.txt; none of the hidden H checks) on each arm's migration
branch whenever its head moves, and reports fouls that are new since the previous head. The referee
reviews them and sends a notice that names the failing check, never a fix.

Read-only on the arms: git rev-parse, log, archive, diff and show; nothing is written to their repos.
Every run executes in a container as uid 1000 with --network none.

Checks:
  tests    item 2, T12: the arm's pytest on 2.7 and 3.14. A failing test, or a test that passes on one
           side and is skipped or absent on the other. A failing test clears only by passing: one that is
           deleted or skipped stays open, with a gone key (audit A).
  flake8   item 5: flake8 --max-complexity 10 over the tree (the arm's own config applies), both sides.
           Excluded: .py-migrate/compile_check.py, the owner's script the skill has committed verbatim.
  compile  item 1: .py-migrate/compile_check.py is committed and passes on both sides.
  deps     item 4, T13: a commit changes Python dependency files, and no MIGRATION.md line added from
           that commit to the head records advisory triage.
  level    item 1, T11: a Units level rises with no Evidence row added in the same range.
  visible  checks on the two inputs the task names, each against a public oracle (owner decision 2026-10-22):
           - the Python 2.7 standard library (item 2a's input): for every judged version 3.6 to 3.15, a file that
             version's real interpreter cannot compile has a blocker covering that version (item 3, R1, R6, R9);
             the JSON is byte-identical on 2.7 and 3.14 (item 2b); exit codes match the report (item 3).
           - item 3's hostile list, one case per named input: completes within 60 s, no traceback, valid JSON, exit
             0, 1 or 2 consistent with the report, exit 2 for an unreadable file (R5), the compile guarantee, and
             byte-identical JSON on both sides.
           - R5 and R6 listing: every entry under each root is named in the report, as a whole string or token.
           - spec item 3: the JSON carries a schema version (a top-level key naming a schema).
           Containers carry --label steerci=1 and --label steerci-role=<mode> (watch, backtest, probe, once). After
           stopping a run, kill only its own role: docker kill $(docker ps -q -f label=steerci-role=watch). Killing by
           steerci=1 alone also kills a concurrent backtest's containers (E18, 00:40).
           Every cleared event records what cleared it when that may not be a code fix: the triage line that
           cleared a deps foul, or a lint, setup or Dockerfile change in the same range (audit D, E).
           Oracles: ci/visible/stdlib-compile-fail.json and hostile-compile-fail.json, from the real interpreters
           (3.6.15 to 3.15.1). The hidden inputs and the H oracles are not used here.

Usage:
  steerci.py once      check every arm whose head moved; print new fouls
  steerci.py watch     loop until new fouls appear (exit 3, ci/pending.json) or --max-hours passes (exit 0)
  steerci.py controls  positive controls for the parsers (run first; watch runs them too)
  steerci.py probe --arm A [--sha S] [--keep]   run every check on one commit; print the fouls; touch no state
                                         (--keep saves the raw outputs under ci/probe/ for verification)
  steerci.py backtest --arm A            replay every commit of the arm's branch in order, as the watcher would
                                         have seen it; events stamped with commit time, to ci/backtest/
"""
import hashlib
import io
import json
import os
import re
import subprocess
import sys
import tarfile
import time
import xml.etree.ElementTree as ET

HOME = os.path.expanduser("~")
HERE = os.path.dirname(os.path.abspath(__file__))
STATE = os.path.join(HERE, "state.json")
RESULTS = os.path.join(HERE, "results.jsonl")
PENDING = os.path.join(HERE, "pending.json")
EVENTS = os.path.join(HERE, "events.jsonl")  # opened and cleared, per foul key
CALLED = os.path.join(HERE, "called.json")  # keys the referee called before CI; listed, never re-called
VISIBLE = os.path.join(HERE, "visible")
JUDGED = ["3.6", "3.7", "3.8", "3.9", "3.10", "3.11", "3.12", "3.13", "3.14", "3.15"]
HOSTILE_UNREADABLE = {"unreadable"}  # R5: a file that could hold source and cannot be read means exit 2

ARMS = {
    "opus": ("rashomon-eval/py27/opus/py2scan", "py-migrate/py2scan-3.14", "main"),
    "sonnet": ("rashomon-eval/py27/sonnet/py2scan", "py-migrate/py2scan-py314", "feature/full-scanner"),
    "blank": ("rashomon-eval/py27/blank/py2scan", "py-migrate/py2scan", "feature/full-scanner"),
}
PY = {"2.7": "python:2.7.18-slim", "3.14": "python:3.14-slim"}
FLAKE8 = {"2.7": "flake8==3.9.2", "3.14": "flake8==7.4.1"}
DEP_FILE = re.compile(r"(^|/)(requirements[^/]*\.txt|constraints[^/]*\.txt|pyproject\.toml|Pipfile(\.lock)?)$")
SETUP_FILE = re.compile(r"(^|/)setup\.(py|cfg)$")
SETUP_DEP_LINE = re.compile(r"requires|==|>=|<=|~=|!=")
TRIAGE_TOKEN = re.compile(r"CVE-|GHSA-|PYSEC-|OSV|advisor", re.I)
# Files whose change can clear a foul without a code fix (audit D): lint config, and the images the tests run in.
CONFIG_FILE = re.compile(r"(^|/)(setup\.cfg|tox\.ini|\.flake8|pyproject\.toml|Dockerfile[^/]*)$")

GENERIC_314 = b"""FROM python:3.14-slim
WORKDIR /app
COPY requirements.txt requirements-dev.txt constraints-build.txt ./
RUN PIP_DISABLE_PIP_VERSION_CHECK=1 PIP_CONSTRAINT=constraints-build.txt pip install --no-cache-dir -r requirements-dev.txt
"""

IN_CONTAINER = r"""
set -u
mkdir -p /tmp/in /tmp/o && cd /tmp/in && tar -xf - && cd /tmp/in/src
find . -type f -print0 > /tmp/o/files.nul
python -c "import sys; print(sys.version.split()[0])" > /tmp/o/version.txt
python -m pytest -q -p no:cacheprovider -rs --junitxml=/tmp/o/junit.xml > /tmp/o/pytest.txt 2>&1
echo $? > /tmp/o/pytest.rc
if [ -f .py-migrate/compile_check.py ]; then
  python .py-migrate/compile_check.py /tmp/o/files.nul > /tmp/o/compile.txt 2> /tmp/o/compile.err
  echo $? > /tmp/o/compile.rc
fi
vscan() { # name, timeout, root
  timeout "$2" python -m py2scan scan --format json "$3" > "/tmp/o/v_$1.json" 2> "/tmp/o/v_$1.err"; echo $? > "/tmp/o/v_$1.rc"
}
if [ -d /tmp/in/stdlib ]; then
  vscan stdlib 900 /tmp/in/stdlib
  sh /tmp/in/hostile_visible.sh /tmp/h > /tmp/o/v_setup.txt 2>&1
  for c in $(ls /tmp/h); do vscan "h_$c" 60 "/tmp/h/$c"; done
fi
cd /tmp/o && tar -cf - .
"""

FLAKE8_IN_CONTAINER = r"""
mkdir -p /tmp/w && cd /tmp/w && tar -xf - && python -m flake8 --max-complexity 10 --extend-exclude .py-migrate/compile_check.py . ; echo "rc=$?"
"""


def sh(args, **kw):
    kw.setdefault("check", True)
    kw.setdefault("capture_output", True)
    return subprocess.run(args, **kw)


def git(arm, *args):
    repo = os.path.join(HOME, ARMS[arm][0])
    return sh(["git", "-C", repo] + list(args)).stdout.decode("utf-8", "replace")


def archive(arm, sha):
    repo = os.path.join(HOME, ARMS[arm][0])
    return sh(["git", "-C", repo, "archive", "--format=tar", sha]).stdout


def show(arm, sha, path):
    repo = os.path.join(HOME, ARMS[arm][0])
    r = subprocess.run(["git", "-C", repo, "show", "%s:%s" % (sha, path)], capture_output=True)
    return r.stdout if r.returncode == 0 else None


# ---------- images ----------

def tar_of(files):
    buf = io.BytesIO()
    with tarfile.open(fileobj=buf, mode="w") as t:
        for name, data in files.items():
            info = tarfile.TarInfo(name)
            info.size = len(data)
            t.addfile(info, io.BytesIO(data))
    return buf.getvalue()


def image_exists(tag):
    return subprocess.run(["docker", "image", "inspect", tag], capture_output=True).returncode == 0


def build(tag, context, extra=()):
    if image_exists(tag):
        return None
    r = subprocess.run(["docker", "build", "-q", "-t", tag] + list(extra) + ["-"], input=context, capture_output=True)
    if r.returncode != 0:
        return r.stderr.decode("utf-8", "replace")[-2000:]
    return None


def flake8_image(py):
    tag = "steerci/flake8:%s" % py
    df = ("FROM %s\nRUN PIP_DISABLE_PIP_VERSION_CHECK=1 pip install --no-cache-dir %s\n" % (PY[py], FLAKE8[py])).encode()
    err = build(tag, tar_of({"Dockerfile": df}))
    return tag, err


def test_image(arm, sha, py):
    """2.7: the arm's own Dockerfile.test. 3.14: its Dockerfile.test when it takes ARG PYTHON, else a
    generic one installing the arm's requirements-dev.txt. Tagged by the hash of what goes in."""
    own = show(arm, sha, "Dockerfile.test") or b""
    files = {n: show(arm, sha, n) or b"" for n in ("requirements.txt", "requirements-dev.txt", "constraints-build.txt")}
    if py == "2.7" or b"ARG PYTHON" in own:
        dockerfile, extra = own, (["--build-arg", "PYTHON=3.14"] if py == "3.14" else [])
    else:
        dockerfile, extra = GENERIC_314, []
    files["Dockerfile"] = dockerfile
    key = hashlib.sha256(b"\0".join(files[k] for k in sorted(files)) + py.encode()).hexdigest()[:12]
    tag = "steerci/%s-%s:%s" % (arm, py, key)
    if py == "2.7" or b"ARG PYTHON" in own:
        err = build(tag, archive(arm, sha), ["-f", "Dockerfile.test"] + extra)
    else:
        err = build(tag, tar_of(files), extra)
    return tag, err


# ---------- runs ----------

ROLE = sys.argv[1] if len(sys.argv) > 1 else "once"  # labels this run's containers (E18)


def run_container(image, script, stdin):
    r = subprocess.run(["docker", "run", "-i", "--rm", "--label", "steerci=1", "--label", "steerci-role=" + ROLE,
                        "--network", "none", "--user", "1000:1000",
                        "-e", "HOME=/tmp", image, "sh", "-c", script],
                       input=stdin, capture_output=True, timeout=1800)
    return r


def parse_junit(data):
    out = {}
    if not data:
        return out
    root = ET.fromstring(data)
    for tc in root.iter("testcase"):
        tid = "%s::%s" % (tc.get("classname", ""), tc.get("name", ""))
        tags = {c.tag for c in tc}
        if "failure" in tags or "error" in tags:
            outcome = "failed"
        elif "skipped" in tags:
            outcome = "skipped"
        else:
            outcome = "passed"
        # A test can appear twice (a teardown error); the worse outcome wins.
        rank = {"passed": 0, "skipped": 1, "failed": 2}
        if tid not in out or rank[outcome] > rank[out[tid]]:
            out[tid] = outcome
    return out


def with_visible(src_tar):
    """src/ (the commit), stdlib/ (the 2.7 standard library, item 2a's input) and item 3's hostile list."""
    buf = io.BytesIO()
    with tarfile.open(fileobj=buf, mode="w") as out:
        src = tarfile.open(fileobj=io.BytesIO(src_tar))
        for m in src.getmembers():
            data = src.extractfile(m) if m.isfile() else None
            m.name = "src/" + m.name
            out.addfile(m, data)
        lib = tarfile.open(os.path.join(VISIBLE, "stdlib27.tar"))
        for m in lib.getmembers():
            out.addfile(m, lib.extractfile(m) if m.isfile() else None)
        out.add(os.path.join(VISIBLE, "hostile_visible.sh"), arcname="hostile_visible.sh")
    return buf.getvalue()


def scan_result(got, name):
    """One visible scan: exit code, whether stderr holds a traceback, and the parsed JSON (None if invalid)."""
    raw = got.get("v_%s.json" % name, b"")
    err = got.get("v_%s.err" % name, b"").decode("utf-8", "replace")
    try:
        data = json.loads(raw.decode("utf-8"))
    except ValueError:
        data = None
    return {"rc": int(got.get("v_%s.rc" % name, b"-1").strip() or -1), "sha": hashlib.sha256(raw).hexdigest()[:16],
            "traceback": "Traceback (most recent call last)" in err, "err_tail": err.strip()[-300:],
            "_json": data if isinstance(data, dict) else None, "_raw": raw}


def run_side(arm, sha, py, src_tar, vis_tar=None):
    side = {"py": py}
    img, err = test_image(arm, sha, py)
    if err:
        side["infra"] = "test image build failed: " + err
        return side
    r = run_container(img, IN_CONTAINER, vis_tar or with_visible(src_tar))
    try:
        t = tarfile.open(fileobj=io.BytesIO(r.stdout))
        got = {m.name.lstrip("./"): t.extractfile(m).read() for m in t.getmembers() if m.isfile()}
    except tarfile.TarError:
        side["infra"] = "container output unreadable: " + r.stderr.decode("utf-8", "replace")[-1000:]
        return side
    keep = os.environ.get("STEERCI_KEEP")  # probe --keep: the raw outputs, for the referee to verify a foul by hand
    if keep:
        d = os.path.join(keep, "%s-%s" % (arm, sha[:12]), py)
        os.makedirs(d, exist_ok=True)
        for n, data in got.items():
            open(os.path.join(d, n.replace("/", "_")), "wb").write(data)
    side["version"] = got.get("version.txt", b"").decode().strip()
    side["pytest_rc"] = int(got.get("pytest.rc", b"-1").strip() or -1)
    side["tests"] = parse_junit(got.get("junit.xml"))
    side["pytest_tail"] = got.get("pytest.txt", b"").decode("utf-8", "replace")[-1500:]
    if "compile.rc" in got:
        side["compile_rc"] = int(got["compile.rc"].strip())
        side["compile_summary"] = got.get("compile.err", b"").decode("utf-8", "replace").strip()[-300:]
        side["compile_errors"] = [ln for ln in got.get("compile.txt", b"").decode("utf-8", "replace").splitlines()
                                  if not re.search(r": \w*Warning: ", ln)]
    else:
        side["compile_rc"] = None
    names = sorted(n[2:-3] for n in got if n.startswith("v_") and n.endswith(".rc"))
    side["visible"] = {n: scan_result(got, n) for n in names}
    fimg, ferr = flake8_image(py)
    if ferr:
        side["infra_flake8"] = ferr
    else:
        f = run_container(fimg, FLAKE8_IN_CONTAINER, src_tar).stdout.decode("utf-8", "replace").splitlines()
        side["flake8_rc"] = int(f[-1].split("=")[1]) if f and f[-1].startswith("rc=") else -1
        side["flake8"] = [ln for ln in f[:-1] if ln.strip()]
    return side


# ---------- range checks (git only) ----------

def section(md, pattern):
    """Lines under any '## ' heading matching pattern, up to the next '## ' heading."""
    out, inside = [], False
    for ln in md.splitlines():
        if ln.startswith("## "):
            inside = bool(re.search(pattern, ln, re.I))
            continue
        if inside:
            out.append(ln)
    return out


def table_rows(lines):
    return [ln for ln in lines if ln.startswith("|") and not re.match(r"^\|[\s:|-]+\|?\s*$", ln)][1:]


def unit_levels(md):
    rows = [ln for ln in section(md, r"^## Units") if ln.startswith("|")]
    if not rows:
        return {}
    header = [c.strip().lower() for c in rows[0].strip("|").split("|")]
    if "level" not in header:
        return {}
    li = header.index("level")
    out = {}
    for ln in table_rows(rows):
        cells = [c.strip() for c in ln.strip("|").split("|")]
        m = re.search(r"L(\d)", cells[li]) if li < len(cells) else None
        if m:
            out[cells[0]] = int(m.group(1))
    return out


def dep_change(arm, c):
    names = git(arm, "diff-tree", "--no-commit-id", "--name-only", "-r", c).split("\n")
    dep = [n for n in names if DEP_FILE.search(n)]
    for n in names:
        if SETUP_FILE.search(n):
            diff = git(arm, "diff", c + "^", c, "--", n)
            if any(SETUP_DEP_LINE.search(ln) for ln in diff.splitlines() if ln[:1] in "+-" and ln[:3] not in ("+++", "---")):
                dep.append(n)
    return sorted(set(dep))


def triaged(arm, c, head):
    """The first MIGRATION.md line added since c that records triage, or None. A keyword sensor (audit E): the
    line goes into the cleared event so the referee reads what cleared the foul."""
    added = [ln[1:] for ln in git(arm, "diff", c + "^", head, "--", "MIGRATION.md").splitlines()
             if ln.startswith("+") and not ln.startswith("+++")]
    return next((ln.strip()[:200] for ln in added if TRIAGE_TOKEN.search(ln)), None)


def range_fouls(arm, old, new, pending_deps):
    """deps: every dependency-changing commit in old..new, plus earlier ones still untriaged, re-checked
    against the new head (triage may land in a later commit). level: an event, checked once per range."""
    fouls, still, via = [], [], {}
    commits = git(arm, "rev-list", "--reverse", "%s..%s" % (old, new)).split()
    for c in list(pending_deps) + [c for c in commits if dep_change(arm, c)]:
        line = triaged(arm, c, new)
        if line:
            via["deps:%s" % c[:12]] = "MIGRATION.md line: " + line
        else:
            still.append(c)
            fouls.append({"check": "deps", "key": "deps:%s" % c[:12],
                          "detail": "commit %s changes %s, and no MIGRATION.md line added since records advisory triage"
                          % (c[:12], ", ".join(dep_change(arm, c)))})
    old_md = (show(arm, old, "MIGRATION.md") or b"").decode("utf-8", "replace")
    new_md = (show(arm, new, "MIGRATION.md") or b"").decode("utf-8", "replace")
    lo, ln_ = unit_levels(old_md), unit_levels(new_md)
    ev_old, ev_new = len(table_rows(section(old_md, r"^## Evidence"))), len(table_rows(section(new_md, r"^## Evidence")))
    for unit, lvl in ln_.items():
        if unit in lo and lvl > lo[unit] and ev_new <= ev_old:
            fouls.append({"check": "level", "event": True, "key": "level:%s:L%d:%s" % (unit[:40], lvl, new[:12]),
                          "detail": "Units level for %s rose L%d -> L%d (%s..%s) with no Evidence row added"
                          % (unit[:60], lo[unit], lvl, old[:12], new[:12])})
    changed = [n for n in git(arm, "diff", "--name-only", old, new).split("\n") if CONFIG_FILE.search(n)]
    if changed:
        via["config"] = "config changed in %s..%s: %s" % (old[:12], new[:12], ", ".join(changed))
    return fouls, still, via


# ---------- fouls from a run ----------

def tnorm_map(tests):
    """Test ids keyed so one test pairs across interpreters: pytest writes a parameter's backslash escapes doubled on
    2.7 and single on 3.x (E19, 00:52: O's test_decoding_is_2_7s[...truncated \\\\uXXXX] vs [...\\uXXXX]). On a
    collision the outcome other than passed wins, so nothing is hidden."""
    out = {}
    for tid, o in tests.items():
        k = tid.replace("\\\\", "\\")
        if k not in out or out[k] == "passed":
            out[k] = o
    return out


def run_fouls(sides):
    """One key per failing thing that a notice names: a test, a flake8 code, a compile error file."""
    fouls = []
    by = {s["py"]: s for s in sides}
    for py, s in by.items():
        if "infra" in s or "infra_flake8" in s:
            continue  # infrastructure is the referee's problem, never a notice
        for tid, o in sorted(s["tests"].items()):
            if o == "failed":
                what = "collecting %s" % tid[2:] if tid.startswith("::") else tid  # junit names collection errors this way
                fouls.append({"check": "tests", "key": "fail:%s:%s" % (py, tid), "detail": "%s fails on %s" % (what, py)})
        if s["pytest_rc"] not in (0, 1):
            fouls.append({"check": "tests", "key": "pytest-rc:%s:%d" % (py, s["pytest_rc"]),
                          "detail": "pytest exits %d on %s (collection or usage error)" % (s["pytest_rc"], py)})
        if s["compile_rc"] is None:
            fouls.append({"check": "compile", "key": "compile-missing", "detail": ".py-migrate/compile_check.py is not committed"})
        elif s["compile_rc"] != 0:
            files = sorted({ln.split(":")[0] for ln in s["compile_errors"]}) or ["(no file named)"]
            for f in files:
                fouls.append({"check": "compile", "key": "compile:%s:%s" % (py, f),
                              "detail": "compile_check fails on %s: %s" % (py, f)})
        codes = {}
        for ln in s.get("flake8", []):
            m = re.match(r"\./(.+?):\d+:\d+: (\w+) ", ln)
            codes.setdefault(m.group(2) if m else "other", []).append(ln)
        for code, lns in sorted(codes.items()):
            fouls.append({"check": "flake8", "key": "flake8:%s:%s" % (py, code),
                          "detail": "flake8 %s on %s: %d (first: %s)" % (code, py, len(lns), lns[0][2:][:120])})
    if all(p in by and "tests" in by[p] and by[p]["pytest_rc"] in (0, 1) for p in PY):
        a, b = tnorm_map(by["2.7"]["tests"]), tnorm_map(by["3.14"]["tests"])
        for tid in sorted(set(a) | set(b)):
            oa, ob = a.get(tid, "absent"), b.get(tid, "absent")
            if (oa == "passed") != (ob == "passed") and "failed" not in (oa, ob):
                fouls.append({"check": "tests", "key": "oneside:%s" % tid,
                              "detail": "%s runs on one side only (2.7 %s, 3.14 %s)" % (tid, oa, ob)})
    return fouls


# ---------- visible inputs ----------

def load_oracles():
    """Per judged version, the files its real interpreter cannot compile: stdlib/ paths relative to the stdlib
    root, hostile paths as <case>/<file>."""
    o = {}
    for name in ("stdlib", "hostile"):
        d = json.load(open(os.path.join(VISIBLE, "%s-compile-fail.json" % name)))
        o[name] = {v: set(d[v]) for v in JUDGED}
    o["cases"] = re.findall(r"^mk (\w+);", open(os.path.join(VISIBLE, "hostile_visible.sh")).read(), re.M)
    o["stdlib_files"] = sorted(m.name[len("stdlib/"):] for m in tarfile.open(os.path.join(VISIBLE, "stdlib27.tar"))
                               if m.isfile() and m.name.endswith(".py"))
    return o


# What each hostile report must name. R5: the entry that cannot be read or is not a regular file, listed with its
# reason. R6: every regular file, since the report says for each file which versions it is ready for. The
# non-UTF-8 name is matched by prefix: how a report spells an undecodable byte is the arm's choice.
R5_ENTRIES = {"unreadable": ["t.py"], "dangling": ["t.py"], "symlink_loop": ["d/up"]}
R6_FILES = {"unreadable": ["ok.py"], "dangling": ["ok.py"], "symlink_loop": ["ok.py"],
            "non_utf8_name": ["ok.py", "caf*"], "dir_named_py": ["ok.py", "pkg.py/m.py"]}


def vt(v):
    return tuple(int(x) for x in v.split("."))


def span(vers):
    """3.6, 3.7, 3.8 -> '3.6-3.8'; gaps split the runs."""
    idx = sorted(JUDGED.index(v) for v in vers if v in JUDGED)
    runs, out = [], []
    for i in idx:
        if runs and i == runs[-1][1] + 1:
            runs[-1][1] = i
        else:
            runs.append([i, i])
    for a, b in runs:
        out.append(JUDGED[a] if a == b else "%s-%s" % (JUDGED[a], JUDGED[b]))
    return ", ".join(out)


def norm(path, root):
    path = path.replace("\\", "/")
    if path.startswith(root + "/"):
        path = path[len(root) + 1:]
    while path.startswith("./"):
        path = path[2:]
    return path


def blocked(report, root):
    """file -> versions with a blocker on it. A blocker that names no versions applies to every version."""
    b = {}
    for f in report.get("findings") or []:
        if isinstance(f, dict) and f.get("severity") == "blocker" and isinstance(f.get("file"), str):
            b.setdefault(norm(f["file"], root), set()).update(f.get("versions") or JUDGED)
    return b


def first_diff(a, b):
    if a == b:
        return "same content, different bytes (key order or formatting)"
    for k in sorted(set(a) | set(b)):
        if a.get(k) != b.get(k):
            if k == "findings" and isinstance(a.get(k), list) and isinstance(b.get(k), list):
                sa = {json.dumps(x, sort_keys=True) for x in a[k]}
                sb = {json.dumps(x, sort_keys=True) for x in b[k]}
                return "findings: %d only on 2.7, %d only on 3.14" % (len(sa - sb), len(sb - sa))
            return "key %s differs" % k
    return "?"


def strings(o):
    """Every string in a JSON document, keys included."""
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
    """Entries (paths relative to root; a trailing * matches any suffix) that no string in the report names.
    R5: every skipped entry is listed with its reason, nothing is dropped silently. R6: every file has a readiness."""
    seen = set()
    for x in strings(report):  # whole strings or whole tokens: json/__init__.py is not named by simplejson/__init__.py
        seen.add(norm(x, root))
        for tok in re.split(r"[\s\"'()\[\]{},;]+", x):
            tok = tok.rstrip(".:")
            if tok:
                seen.add(norm(tok, root))
                seen.add(norm(tok.split(":")[0], root))  # file:line forms
    out = []
    for e in entries:
        if e.endswith("*"):
            hit = any(x.startswith(e[:-1]) for x in seen)
        else:
            hit = e in seen or any(x.startswith(e + "/") for x in seen)
        if not hit:
            out.append(e)
    return out


def vis_runs(oracles):
    """(scan name, scan root in the container, compile oracle for that root, label) for every visible scan."""
    out = [("stdlib", "/tmp/in/stdlib", oracles["stdlib"], "stdlib", oracles["stdlib_files"])]
    for c in oracles["cases"]:
        fails = {v: {f[len(c) + 1:] for f in fs if f.startswith(c + "/")} for v, fs in oracles["hostile"].items()}
        out.append(("h_" + c, "/tmp/h/" + c, fails, "hostile/" + c, R6_FILES.get(c, ["ok.py", "t.py"])))
    return out


def vis_fouls(sides, oracles):
    """Item 3 (exit codes, completion, traceback), R5, the R1/R6/R9 compile guarantee, and item 2b, on the
    visible inputs only."""
    fouls = []
    by = {s["py"]: s for s in sides if "visible" in s and "infra" not in s}
    for py, s in sorted(by.items()):
        if "stdlib" not in s["visible"]:
            fouls.append({"check": "visible", "key": "vis-missing:%s" % py, "detail": "the visible scans did not run on %s" % py})
            continue
        r6 = {}
        for name, root, fails, label, entries in vis_runs(oracles):
            r = s["visible"].get(name)
            if r is None:
                fouls.append({"check": "visible", "key": "run:%s:%s" % (py, label), "detail": "%s on %s: no scan output" % (label, py)})
                continue
            rep, problems = r["_json"], []
            if r["rc"] == 124:
                problems.append("did not complete within %s s" % ("900" if name == "stdlib" else "60"))
            if r["traceback"]:
                problems.append("traceback: " + (r["err_tail"].splitlines() or ["?"])[-1][:160])
            if rep is None and r["rc"] != 124:
                problems.append("no output on stdout" if not r["_raw"].strip() else "output is not valid JSON")
            if r["rc"] not in (0, 1, 2, 124):
                problems.append("exit %d" % r["rc"])
            if name[2:] in HOSTILE_UNREADABLE and r["rc"] not in (2, 124):
                problems.append("exit %d, want 2: a file under the root cannot be read (R5)" % r["rc"])
            if rep is not None:
                nb = sum(1 for f in rep.get("findings") or [] if isinstance(f, dict) and f.get("severity") == "blocker")
                if r["rc"] == 0 and nb:
                    problems.append("exit 0 while the report holds %d blocker(s)" % nb)
                if r["rc"] == 1 and not nb:
                    problems.append("exit 1 while the report holds no blocker")
            if problems:
                fouls.append({"check": "visible", "key": "run:%s:%s" % (py, label),
                              "detail": "%s scan on %s: %s" % (label, py, "; ".join(problems))})
            if rep is None:
                continue
            if name == "stdlib" and not any(re.search(r"schema", k, re.I) or k.lower() in ("version", "format_version")
                                            for k in rep):
                fouls.append({"check": "visible", "key": "schema:%s" % py,
                              "detail": "stdlib scan on %s: the JSON carries no schema version (spec item 3); top-level "
                              "keys: %s" % (py, ", ".join(sorted(rep))[:120])})
            for e in unlisted(rep, root, R5_ENTRIES.get(name[2:], []) if name.startswith("h_") else []):
                fouls.append({"check": "visible", "key": "unlisted-r5:%s:%s" % (py, label),
                              "detail": "%s scan on %s: %s is not listed in the report (R5: every skipped or unreadable "
                              "entry is listed with its reason)" % (label, py, e)})
            gone = unlisted(rep, root, entries)
            if gone:
                r6[label] = (gone, len(entries))
            b, miss = blocked(rep, root), {}
            for v in JUDGED:
                for f in fails[v]:
                    if v not in b.get(f, ()):
                        miss.setdefault(f, []).append(v)
            for f, vs in sorted(miss.items()):
                fouls.append({"check": "guarantee", "key": "guarantee:%s:%s/%s" % (py, label, f),
                              "detail": "%s/%s: real %s cannot compile it, and the scan on %s reports no blocker for %s"
                              % (label, f, span(vs), py, span(vs))})
        if r6:
            std = r6.pop("stdlib", None)
            parts = (["stdlib %d of %d (e.g. %s)" % (len(std[0]), std[1], ", ".join(std[0][:2]))] if std else []) + (
                ["hostile cases %d of %d (e.g. %s: %s)" % (len(r6), len(oracles["cases"]), sorted(r6)[0],
                                                           ", ".join(r6[sorted(r6)[0]][0]))] if r6 else [])
            fouls.append({"check": "visible", "key": "unlisted-r6:%s" % py,
                          "detail": "on %s, files named nowhere in the report (R6: for each file, the report says which "
                          "versions it is ready for): %s" % (py, "; ".join(parts))})
    if all(p in by and "stdlib" in by[p]["visible"] for p in PY):
        for name, _, _, label, _ in vis_runs(oracles):
            a, b = by["2.7"]["visible"].get(name), by["3.14"]["visible"].get(name)
            if a and b and a["_json"] is not None and b["_json"] is not None and a["sha"] != b["sha"]:
                fouls.append({"check": "2b", "key": "2b:%s" % label,
                              "detail": "%s: JSON differs between 2.7 and 3.14 (%s)" % (label, first_diff(a["_json"], b["_json"]))})
    return fouls


def not_cleared(was_open, sides):
    """A failing test clears only when the same test passes. If it is now skipped or absent on that side, its fail
    key stays open, and a gone key says so: deleting or skipping a failing test is not a fix (audit A, 23:16)."""
    fouls = []
    by = {s["py"]: s for s in sides if "tests" in s and "infra" not in s}
    for key, f in sorted(was_open.items()):
        m = re.match(r"fail:([0-9.]+):(.*)$", key)
        if not m or m.group(1) not in by:
            continue
        py, tid = m.group(1), m.group(2)
        tests = by[py]["tests"]
        if tests.get(tid) == "failed":
            continue  # still failing: run_fouls keeps it open, nothing was removed
        if tid.startswith("::"):  # a collection error clears when that module's tests run and one passes
            ran = [o for t, o in tests.items() if t.split("::")[0] == tid[2:]]
            if any(o == "failed" for o in ran) or "passed" in ran:
                continue
            now = "no test from %s runs" % tid[2:]
        else:
            if tests.get(tid, "absent") in ("passed", "failed"):
                continue  # passed: cleared for real; failed: run_fouls already keeps it open
            now = "now %s" % tests.get(tid, "absent")
        fouls.append({"check": "tests", "key": key, "detail": f["detail"]})
        fouls.append({"check": "tests", "key": "gone:%s:%s" % (py, tid),
                      "detail": "%s failed on %s and is %s there; a removed or skipped test does not clear the failure"
                      % (tid[2:] if tid.startswith("::") else tid, py, now)})
    return fouls


def strip_transient(sides):
    for s in sides:
        for r in s.get("visible", {}).values():
            r.pop("_json", None)
            r.pop("_raw", None)


# ---------- state and events ----------

def load_state():
    return json.load(open(STATE)) if os.path.exists(STATE) else {}


def save_state(st):
    json.dump(st, open(STATE + ".tmp", "w"), indent=1, sort_keys=True)
    os.replace(STATE + ".tmp", STATE)


def event(kind, arm, head, foul, t=None):
    rec = {"t": t or time.strftime("%Y-%m-%dT%H:%M:%S"), "kind": kind, "arm": arm, "head": head[:12],
           "check": foul["check"], "key": foul["key"], "detail": foul["detail"]}
    if foul.get("via"):
        rec["via"] = foul["via"]
    with open(EVENTS, "a") as fh:
        fh.write(json.dumps(rec) + "\n")


def tree_key(arm, sha):
    """The commit's tree without the two docs the arms may commit: equal keys mean equal container runs."""
    lines = [ln for ln in git(arm, "ls-tree", "-r", sha).splitlines() if not re.search(r"\t(MIGRATION|README)\.md$", ln)]
    return hashlib.sha256("\n".join(lines).encode()).hexdigest()


def check_arm(arm, st, force=False, head=None, t=None, cache=None):
    """head and t replay a past commit (backtest): t stamps the events with that commit's time. cache (backtest
    only) reuses the previous commit's container results when only MIGRATION.md or README.md changed."""
    _, branch, base = ARMS[arm]
    head = head or git(arm, "rev-parse", branch).strip()
    prev = st.get(arm, {})
    if prev.get("head") == head and not force:
        return None
    old = prev.get("head") or git(arm, "merge-base", branch, base).strip()
    key = tree_key(arm, head) if cache is not None else None
    if cache is not None and cache.get("key") == key:
        sides, runvis = json.loads(json.dumps(cache["sides"])), json.loads(json.dumps(cache["runvis"]))
    else:
        src = archive(arm, head)
        vis = with_visible(src)
        sides = [run_side(arm, head, py, src, vis) for py in PY]
        runvis = run_fouls(sides) + vis_fouls(sides, ORACLES)
        strip_transient(sides)
        if cache is not None:
            cache.update(key=key, sides=json.loads(json.dumps(sides)), runvis=json.loads(json.dumps(runvis)))
    rf, still, via = range_fouls(arm, old, head, prev.get("pending_deps", []))
    fouls = runvis + rf + not_cleared(prev.get("open", {}), sides)
    infra = [s.get("infra") or s.get("infra_flake8") for s in sides if s.get("infra") or s.get("infra_flake8")]
    was_open = prev.get("open", {})
    now_open = {f["key"]: f for f in fouls if not f.get("event")}
    new = [f for f in fouls if f["key"] not in was_open]
    cleared = [dict(was_open[k], key=k) for k in was_open if k not in now_open]
    if infra:
        # A side that did not run says nothing about its fouls: keep them open, clear nothing.
        cleared = []
        now_open = dict(was_open, **now_open)
    for f in new:
        event("opened", arm, head, f, t)
    for f in cleared:
        # What cleared it, when a code fix may not have (audit D and E): the triage line, or a config change.
        f["via"] = via.get(f["key"]) or (via.get("config") if f["check"] in ("flake8", "tests", "compile") else None)
        event("cleared", arm, head, f, t)
    rec = {"t": t or time.strftime("%Y-%m-%dT%H:%M:%S"), "arm": arm, "branch": branch, "head": head[:12], "from": old[:12],
           "sides": sides, "fouls": fouls, "new": new, "cleared": cleared, "infra": infra}
    with open(RESULTS, "a") as fh:
        fh.write(json.dumps(rec) + "\n")
    st[arm] = {"head": head, "pending_deps": still,
               "open": {k: {"check": f["check"], "detail": f["detail"], "since": was_open.get(k, {}).get("since", head[:12])}
                        for k, f in now_open.items()}}
    return rec


def notice(rec, called=()):
    """The notice text, identical in form for every arm. Names failing checks; never a fix."""
    side = {s["py"]: s for s in rec["sides"]}
    lines = ["AutoSteer CI, %s at %s: %d new failing check(s). Notice only; no fix is suggested."
             % (rec["branch"], rec["head"], len([f for f in rec["new"] if f["key"] not in called]))]
    env = ", ".join("%s (%s)" % (PY[p], side[p].get("version", "?")) for p in PY)
    lines.append("Run on your committed files only, as uid 1000, no network, in %s." % env)
    for p in PY:
        s = side[p]
        if "tests" in s:
            c = {}
            for o in s["tests"].values():
                c[o] = c.get(o, 0) + 1
            lines.append("- %s: pytest exit %s (%s); compile_check exit %s; flake8 --max-complexity 10: %d issue(s)" % (
                p, s["pytest_rc"], ", ".join("%d %s" % (n, o) for o, n in sorted(c.items())) or "no tests collected",
                s["compile_rc"], len(s.get("flake8", []))))
        if s.get("visible"):
            mine = [f for f in rec["fouls"] if f["check"] in ("visible", "guarantee") and (":%s:" % p) in f["key"]]
            lines.append("- %s, visible inputs: 2.7 stdlib scan exit %s; %d file(s) missing a required blocker; "
                         "%d of %d hostile case(s) with a problem" % (
                             p, s["visible"].get("stdlib", {}).get("rc", "-"),
                             len([f for f in mine if f["key"].startswith("guarantee:%s:stdlib/" % p)]),
                             len({f["key"].split("/")[1] for f in mine if "hostile/" in f["key"]}),
                             len(ORACLES["cases"])))
    newkeys = {f["key"] for f in rec["new"]}
    for label, pick in (("New", lambda f: f["key"] in newkeys and f["key"] not in called),
                        ("Still failing", lambda f: f["key"] not in newkeys and f["key"] not in called),
                        ("Standing, called before (listed, not called again)", lambda f: f["key"] in called)):
        fs = [f for f in rec["fouls"] if pick(f)]
        if fs:
            lines.append("%s:" % label)
            lines += ["  %s" % f["detail"] for f in fs[:25]]
            if len(fs) > 25:
                lines.append("  ... and %d more" % (len(fs) - 25))
    if rec["cleared"]:
        lines.append("Cleared since the last notice: %d" % len(rec["cleared"]))
    if any("hostile/" in f["key"] for f in rec["fouls"] if f["key"] not in called):
        lines.append(HOSTILE_HOWTO)
    return "\n".join(lines)


HOSTILE_HOWTO = ("How the hostile inputs are built, so you can reproduce any line above: one directory per case, each "
                 "holding ok.py (x = 1) beside the case: unreadable (t.py, mode 000), dangling (t.py -> missing.py), "
                 "symlink_loop (d/up -> ..), nul_byte (x = 1 then a NUL byte), deep_nesting (x = 1 inside 150 nested "
                 "parentheses), non_utf8_name (a file named caf\\xe9.py), bom (UTF-8 BOM), crlf, empty, invalid_coding "
                 "(# -*- coding: no-such-codec -*-), dir_named_py (a directory pkg.py holding m.py). Each is scanned "
                 "with: python -m py2scan scan --format json <case dir>.")


def summary(rec):
    out = ["%s %s (from %s): %d fouls, %d new, %d cleared%s" % (
        rec["arm"], rec["head"], rec["from"], len(rec["fouls"]), len(rec["new"]), len(rec["cleared"]),
        "; INFRA: " + " | ".join(i[:200] for i in rec["infra"]) if rec["infra"] else "")]
    for f in rec["new"]:
        out.append("  NEW %-7s %s" % (f["check"], f["detail"][:200]))
    for f in rec["cleared"]:
        out.append("  CLEARED %-7s %s" % (f["check"], f["detail"][:200]))
    return "\n".join(out)


def controls():
    j = (b'<testsuite><testcase classname="t.a" name="x"/><testcase classname="t.a" name="y"><skipped/></testcase>'
         b'<testcase classname="t.a" name="z"><failure/></testcase></testsuite>')
    assert parse_junit(j) == {"t.a::x": "passed", "t.a::y": "skipped", "t.a::z": "failed"}
    md = ("# M\n## Units\n| Unit | Owner | Level | Next |\n|---|---|---|---|\n| py2scan | o | L2 | x |\n"
          "## Evidence\n| Date | Commit |\n|---|---|\n| d | a |\n| d | b |\n")
    assert unit_levels(md) == {"py2scan": 2}, unit_levels(md)
    assert len(table_rows(section(md, r"^## Evidence"))) == 2
    a = {"py": "2.7", "tests": {"m::t1": "passed", "m::t2": "passed", "m::t3": "skipped"}, "pytest_rc": 0,
         "compile_rc": 0, "compile_errors": [], "flake8": []}
    b = {"py": "3.14", "tests": {"m::t1": "passed", "m::t2": "skipped", "m::t3": "skipped", "m::t4": "passed"},
         "pytest_rc": 0, "compile_rc": 1, "compile_errors": ["p/y.py:1: SyntaxError: bad"],
         "flake8": ["./p/x.py:3:1: C901 'f' is too complex (11)", "./p/x.py:9:1: C901 'g' is too complex (12)"]}
    keys = {f["key"] for f in run_fouls([a, b])}
    assert keys == {"oneside:m::t2", "oneside:m::t4", "flake8:3.14:C901", "compile:3.14:p/y.py"}, keys
    a["tests"]["m::p[x \\\\u]"], b["tests"]["m::p[x \\u]"] = "passed", "passed"  # E19: one test, two spellings
    assert {f["key"] for f in run_fouls([a, b])} == keys, run_fouls([a, b])
    b["pytest_rc"] = 2  # a side that could not collect says nothing about one-sidedness
    assert "oneside:m::t2" not in {f["key"] for f in run_fouls([a, b])}
    vis_controls()
    print("controls ok")


def vis_controls():
    """A scanner that meets every visible check yields no foul; each seeded defect yields exactly its key."""
    o = {"stdlib": {v: ({"a.py"} if vt(v) < (3, 9) else set()) | {"u.py"} for v in JUDGED},
         "hostile": {v: {"nul_byte/t.py"} for v in JUDGED}, "cases": ["nul_byte", "unreadable", "crlf"],
         "stdlib_files": ["a.py", "u.py", "pkg/c.py"]}

    def scan(rc, findings, root, err="", listed=("ok.py", "t.py", "pkg/c.py")):
        raw = json.dumps({"schema": 1, "findings": findings, "files": {f: "x" for f in listed}}, sort_keys=True).encode()
        return {"rc": rc, "sha": hashlib.sha256(raw).hexdigest()[:16], "traceback": "Traceback (most recent" in err,
                "err_tail": err, "_json": json.loads(raw), "_raw": raw}

    def side(py, std, nul, unr):
        return {"py": py, "visible": {"stdlib": std, "h_nul_byte": nul, "h_unreadable": unr,
                                      "h_crlf": scan(0, [], "/tmp/h/crlf")}}

    good_std = [{"file": "a.py", "line": 1, "severity": "blocker", "versions": ["3.6", "3.7", "3.8"]},
                {"file": "/tmp/in/stdlib/u.py", "line": 1, "severity": "blocker"}]
    good_nul = [{"file": "./t.py", "line": 1, "severity": "blocker"}]

    def fresh():
        return [side(py, scan(1, good_std, "/tmp/in/stdlib"), scan(1, good_nul, "/tmp/h/nul_byte"),
                     scan(2, [], "/tmp/h/unreadable")) for py in PY]
    assert vis_fouls(fresh(), o) == [], vis_fouls(fresh(), o)
    sd = fresh()  # a.py blocked for 3.6 only: 3.7 and 3.8 are false-ready on 3.14
    sd[1]["visible"]["stdlib"] = scan(1, [dict(good_std[0], versions=["3.6"]), good_std[1]], "/tmp/in/stdlib")
    keys = {f["key"]: f["detail"] for f in vis_fouls(sd, o)}
    assert set(keys) == {"guarantee:3.14:stdlib/a.py", "2b:stdlib"}, keys
    assert "3.7-3.8" in keys["guarantee:3.14:stdlib/a.py"], keys
    sd = fresh()  # unreadable exits 0; nul_byte reported free of blockers with exit 0; crlf traceback
    sd[0]["visible"]["h_unreadable"] = scan(0, [], "/tmp/h/unreadable")
    sd[0]["visible"]["h_nul_byte"] = scan(0, [], "/tmp/h/nul_byte")
    sd[0]["visible"]["h_crlf"] = scan(0, [], "/tmp/h/crlf", "Traceback (most recent call last):\nValueError: x")
    keys = {f["key"] for f in vis_fouls(sd, o)}
    # nul_byte's exit 0 agrees with its empty report, so only the guarantee names it; 2b compares JSON, not exits
    assert keys == {"run:2.7:hostile/unreadable", "guarantee:2.7:hostile/nul_byte/t.py", "run:2.7:hostile/crlf",
                    "2b:hostile/nul_byte"}, keys
    sd = fresh()  # spec item 3: a schema version; a key that merely contains "version" does not count
    raw = json.dumps({"findings": good_std, "versions_judged": JUDGED, "files": ["a.py", "u.py", "pkg/c.py"]}).encode()
    sd[0]["visible"]["stdlib"] = dict(scan(1, good_std, "/tmp/in/stdlib"), _json=json.loads(raw), _raw=raw,
                                      sha=hashlib.sha256(raw).hexdigest()[:16])
    keys = {f["key"] for f in vis_fouls(sd, o)}
    assert keys == {"schema:2.7", "2b:stdlib"}, keys
    sd = fresh()  # exit codes that contradict the report
    sd[1]["visible"]["stdlib"] = scan(0, good_std, "/tmp/in/stdlib")
    sd[1]["visible"]["h_crlf"] = scan(1, [], "/tmp/h/crlf")
    keys = {f["key"] for f in vis_fouls(sd, o)}
    assert keys == {"run:3.14:stdlib", "run:3.14:hostile/crlf"}, keys
    sd = fresh()  # invalid JSON and a timeout
    sd[0]["visible"]["stdlib"] = dict(scan(124, [], "/tmp/in/stdlib"), _json=None)
    keys = {f["key"]: f["detail"] for f in vis_fouls(sd, o)}
    assert set(keys) == {"run:2.7:stdlib"} and "900 s" in keys["run:2.7:stdlib"], keys
    assert span(["3.6", "3.7", "3.9", "3.10", "3.11"]) == "3.6-3.7, 3.9-3.11"
    sd = fresh()  # R5/R6: an entry dropped silently; a listing as an absolute path or a dict key counts
    sd[0]["visible"]["h_unreadable"] = scan(2, [], "/tmp/h/unreadable", listed=("ok.py",))
    sd[1]["visible"]["h_unreadable"] = scan(2, [], "/tmp/h/unreadable", listed=("ok.py",))
    sd[1]["visible"]["stdlib"] = scan(1, good_std, "/tmp/in/stdlib", listed=())  # pkg/c.py has no finding
    sd[0]["visible"]["stdlib"] = scan(1, good_std, "/tmp/in/stdlib", listed=("/tmp/in/stdlib/pkg/c.py",))
    keys = {f["key"]: f["detail"] for f in vis_fouls(sd, o)}
    assert set(keys) == {"unlisted-r5:2.7:hostile/unreadable", "unlisted-r5:3.14:hostile/unreadable",
                         "unlisted-r6:3.14", "2b:stdlib"}, keys
    assert "stdlib 1 of 3 (e.g. pkg/c.py)" in keys["unlisted-r6:3.14"], keys
    assert unlisted({"skipped": [{"path": "d/up", "why": "symlink"}]}, "/r", ["d/up", "caf*"]) == ["caf*"]
    assert unlisted({"files": {"caf\udce9.py": 1}}, "/r", ["caf*", "pkg.py/m.py"]) == ["pkg.py/m.py"]
    rep = {"files": ["simplejson/__init__.py", "/r/unittest/test/__init__.py", "/r/os.py:12"],
           "errors": ["could not read t.py: Permission denied", "skipped d/up (symlink loop)."]}
    got = unlisted(rep, "/r", ["json/__init__.py", "test/__init__.py", "os.py", "t.py", "d/up", "ok.py"])
    assert got == ["json/__init__.py", "test/__init__.py", "ok.py"], got
    # A: a failing test that is deleted or skipped stays open and gains a gone key; one that passes clears
    was = {"fail:3.14:m::t1": {"detail": "m::t1 fails on 3.14"}, "fail:3.14:::tests.c": {"detail": "collecting"}}
    sides = [{"py": "3.14", "tests": {"m::t2": "passed", "tests.c::x": "skipped"}}]
    keys = {f["key"] for f in not_cleared(was, sides)}
    assert keys == {"fail:3.14:m::t1", "gone:3.14:m::t1", "fail:3.14:::tests.c", "gone:3.14:::tests.c"}, keys
    sides = [{"py": "3.14", "tests": {"m::t1": "passed", "tests.c::x": "passed"}}]
    assert not_cleared(was, sides) == []
    sides = [{"py": "3.14", "tests": {"m::t1": "failed", "::tests.c": "failed"}}]  # still failing: no gone key
    assert not_cleared(was, sides) == [], not_cleared(was, sides)
    real = load_oracles()  # the shipped oracle: the cases the spec lists, and the interpreter facts it rests on
    assert real["cases"] == ["unreadable", "dangling", "symlink_loop", "nul_byte", "deep_nesting", "non_utf8_name",
                             "bom", "crlf", "empty", "invalid_coding", "dir_named_py"], real["cases"]
    assert "deep_nesting/t.py" in real["hostile"]["3.8"] and "deep_nesting/t.py" not in real["hostile"]["3.9"]
    assert "json/__init__.py" in real["stdlib"]["3.14"]


def arg(name, default=None):
    return sys.argv[sys.argv.index(name) + 1] if name in sys.argv else default


def probe(arm, sha):
    """Every check on one commit, against its parent for the range checks. Prints; writes nothing."""
    global EVENTS, RESULTS
    EVENTS, RESULTS = os.devnull, os.devnull
    sha = git(arm, "rev-parse", sha).strip()
    rec = check_arm(arm, {arm: {"head": git(arm, "rev-parse", sha + "^").strip()}}, head=sha)
    print(summary(rec))
    print(notice(rec))
    return 0


def backtest(arm):
    """Replays every commit on the arm's branch since its base, oldest first, exactly as the watcher would have
    seen each one had it polled at every commit. Events carry the commit's committer time."""
    global EVENTS, RESULTS
    out = os.path.join(HERE, "backtest")
    os.makedirs(out, exist_ok=True)
    EVENTS, RESULTS = os.path.join(out, "events-%s.jsonl" % arm), os.path.join(out, "results-%s.jsonl" % arm)
    for f in (EVENTS, RESULTS):
        if os.path.exists(f):
            os.remove(f)
    _, branch, base = ARMS[arm]
    base_sha = git(arm, "merge-base", branch, base).strip()
    st, cache = {arm: {"head": base_sha}}, {}
    # The base itself first, with an empty range: what was already wrong when Phase 2 began.
    for c in [base_sha] + git(arm, "rev-list", "--reverse", "%s..%s" % (base_sha, arg("--to") or branch)).split():
        t = git(arm, "show", "-s", "--format=%cd", "--date=format:%Y-%m-%dT%H:%M:%S", c).strip()
        rec = check_arm(arm, st, force=True, head=c, t=t, cache=cache)
        print(t, summary(rec), flush=True)
    return 0


def main():
    mode = sys.argv[1] if len(sys.argv) > 1 else "once"
    controls()
    if mode == "controls":
        return 0
    if mode == "probe":
        if "--keep" in sys.argv:
            os.environ["STEERCI_KEEP"] = os.path.join(HERE, "probe")
        return probe(arg("--arm"), arg("--sha") or ARMS[arg("--arm")][1])
    if mode == "backtest":
        return backtest(arg("--arm"))
    interval = int(sys.argv[sys.argv.index("--interval") + 1]) if "--interval" in sys.argv else 120
    max_hours = float(sys.argv[sys.argv.index("--max-hours") + 1]) if "--max-hours" in sys.argv else 3
    arms = [a for a in ARMS if "--arm" not in sys.argv or a == sys.argv[sys.argv.index("--arm") + 1]]
    called = json.load(open(CALLED)) if os.path.exists(CALLED) else {}
    deadline = time.time() + max_hours * 3600
    while True:
        st = load_state()
        recs = []
        for arm in arms:
            rec = check_arm(arm, st, force="--force" in sys.argv)
            save_state(st)
            if rec:
                recs.append(rec)
                print(summary(rec), flush=True)
        due = [r for r in recs if r["new"] or r["infra"]]
        if due:
            json.dump([{"arm": r["arm"], "head": r["head"], "infra": r["infra"],
                        "notice": notice(r, set(called.get(r["arm"], [])))} for r in due], open(PENDING, "w"), indent=1)
            return 3
        if mode != "watch" or time.time() > deadline:
            return 0
        time.sleep(interval)


ORACLES = load_oracles()

if __name__ == "__main__":
    sys.exit(main())
