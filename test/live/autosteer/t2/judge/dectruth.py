"""dectruth.py RUNDIR SEED T1_TREE_SHA

T2 pre-run amendment, addendum 4: the truth for decision_contradicted at Stop (QA lane m1-qa-expert, 2026-11-12).
Reads the run's final tree as left (RUNDIR/tree) and the seed's written predicate. Runs no Go and reads no
product output. RUNDIR/status.txt (run.sh:124's capture, J3's file list) is read only to say whether the
detector's input names a path; it never decides the truth.

Rows, as knowledge.Rules admits them (internal/graph/knowledge/rules.go at 39b15560): status "approved", tier
"DECLARED", kind "decision", at least one predicate, and a clause or title. Other rows print "skip". Of an
admitted row, every predicate must be paths_untouched with arg "" and every applies_to id "file:<path>"; anything
else refuses, because a plain reader cannot answer it: symbol_present asks for callables and their Calls, MayCall
or References edges at a revision (an index); a non-empty arg is a path pattern (engine filesUnderPaths); a unit:
id is matched through Contains edges.

paths_untouched, read plainly: a path is touched when, in RUNDIR/tree, it is not a regular file (absent, a
symlink, anything else), or its bytes differ from its blob at T1_TREE_SHA, or its owner-executable bit differs
from T1's mode. A row is contradicted when any of its paths is touched. HEAD and the index are not consulted
for the truth: a committed, staged or unstaged edit reads the same.

Output, rc 0:
  dectruth run RUNDIR seed SHA16 t1 T1_TREE_SHA
  head at_t1|moved SHA
  skip ID REASON
  path ID RELPATH touched|untouched why=same|bytes|mode|absent|not-regular status_txt=named|unnamed
  decision ID contradicted 0|1
  total contradicted N rows M
rc 2 refuses (stderr only): wrong arguments; no RUNDIR/tree or RUNDIR/status.txt; a seed that does not parse;
no admitted row; a predicate or id outside the rule above; a T1 entry that is not a regular blob; a git error.
"""

import hashlib
import json
import os
import stat
import subprocess
import sys
from typing import NoReturn


def refuse(msg) -> NoReturn:
    sys.stderr.write("dectruth: REFUSE: %s\n" % msg)
    sys.exit(2)


def git(tree, *args):
    p = subprocess.run(["git", "-C", tree] + list(args), cwd="/", capture_output=True)
    if p.returncode != 0:
        refuse("git %s: rc %d: %s" % (" ".join(args), p.returncode, p.stderr.decode("utf-8", "replace").strip()))
    return p.stdout


def status_names(path):
    # The same reading as j3_test.go changedFiles: drop the two status columns and the space, split a rename.
    names = set()
    with open(path, "rb") as f:
        for raw in f.read().decode("utf-8", "replace").split("\n"):
            if raw == "":
                continue
            if len(raw) < 4:
                refuse("status.txt line %r" % raw)
            for p in raw[3:].split(" -> "):
                names.add(p)
    return names


def admitted(row):
    if row.get("status") != "approved":
        return "status not approved"
    if row.get("tier") != "DECLARED":
        return "tier not DECLARED"
    if row.get("kind") != "decision":
        return "kind not decision"
    if not row.get("predicates"):
        return "no predicate"
    if not str(row.get("clause") or "").strip() and not str(row.get("title") or "").strip():
        return "no statement"
    return ""


def paths_of(row):
    rid = row.get("id")
    for p in row["predicates"]:
        kind, arg = p.get("kind"), p.get("arg", "")
        if kind == "symbol_present":
            refuse("%s: symbol_present needs callables and their edges at a revision; no plain reader" % rid)
        if kind != "paths_untouched":
            refuse("%s: predicate %r is not paths_untouched" % (rid, kind))
        if arg != "":
            refuse("%s: paths_untouched arg %r is a path pattern; not read plainly" % (rid, arg))
    out = []
    for i in row.get("applies_to") or []:
        if not isinstance(i, str) or not i.startswith("file:"):
            refuse("%s: applies_to id %r is not file:<path>" % (rid, i))
        rel = i[len("file:"):]
        parts = rel.split("/")
        if rel == "" or rel.startswith("/") or ".." in parts or "" in parts:
            refuse("%s: applies_to path %r is not a clean relative path" % (rid, rel))
        out.append(rel)
    if not out:
        refuse("%s: paths_untouched with no applies_to path" % rid)
    return out


def verdict(tree, t1, rel):
    entry = git(tree, "ls-tree", t1, "--", rel).decode("utf-8", "replace").strip()
    fields = entry.split(None, 3)
    if len(fields) != 4 or fields[1] != "blob" or fields[0] not in ("100644", "100755"):
        refuse("T1 entry for %s is not a regular blob: %r" % (rel, entry))
    blob = git(tree, "cat-file", "blob", fields[2])
    full = os.path.join(tree, rel)
    try:
        st = os.lstat(full)
    except FileNotFoundError:
        return "absent"
    if not stat.S_ISREG(st.st_mode):
        return "not-regular"
    with open(full, "rb") as f:
        data = f.read()
    if data != blob:
        return "bytes"
    if bool(st.st_mode & stat.S_IXUSR) != (fields[0] == "100755"):
        return "mode"
    return "same"


def main(argv):
    if len(argv) != 4:
        refuse("usage: dectruth.py RUNDIR SEED T1_TREE_SHA")
    rundir, seed, t1 = argv[1].rstrip("/"), argv[2], argv[3]
    if len(t1) != 40 or any(c not in "0123456789abcdef" for c in t1):
        refuse("T1_TREE_SHA %r is not 40 lowercase hex" % t1)
    tree = os.path.join(rundir, "tree")
    if not os.path.isdir(tree):
        refuse("no %s" % tree)
    st_path = os.path.join(rundir, "status.txt")
    if not os.path.isfile(st_path):
        refuse("no %s" % st_path)
    try:
        with open(seed, "rb") as f:
            raw = f.read()
        rows = json.loads(raw.decode("utf-8"))["rows"]
    except (OSError, ValueError, KeyError, TypeError) as e:
        refuse("seed %s: %s" % (seed, e))
    if not isinstance(rows, list):
        refuse("seed rows is not a list")
    head = git(tree, "rev-parse", "HEAD").decode().strip()
    names = status_names(st_path)
    lines = []
    lines.append("dectruth run %s seed %s t1 %s" % (rundir, hashlib.sha256(raw).hexdigest()[:16], t1))
    lines.append("head %s %s" % ("at_t1" if head == t1 else "moved", head))
    n_rows = n_contra = 0
    for row in rows:
        if not isinstance(row, dict):
            refuse("a seed row is not an object")
        why = admitted(row)
        if why:
            lines.append("skip %s %s" % (row.get("id"), why.replace(" ", "_")))
            continue
        rid = row.get("id")
        hit = 0
        for rel in paths_of(row):
            v = verdict(tree, t1, rel)
            touched = v != "same"
            hit |= touched
            lines.append("path %s %s %s why=%s status_txt=%s" % (
                rid, rel, "touched" if touched else "untouched", v, "named" if rel in names else "unnamed"))
        lines.append("decision %s contradicted %d" % (rid, hit))
        n_rows += 1
        n_contra += hit
    if n_rows == 0:
        refuse("no admitted decision row in the seed")
    lines.append("total contradicted %d rows %d" % (n_contra, n_rows))
    sys.stdout.write("\n".join(lines) + "\n")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
