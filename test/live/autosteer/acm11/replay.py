#!/usr/bin/env python3
"""AC-M1.1 development set: the repository's own commits as prompts.

Each non-merge commit on a branch becomes one row: the subject with its
package prefix ("verify:", "test/mutation:") and trailing parenthetical
card codes stripped is the prompt; the packages of the non-test Go files
the commit touched, restricted to the units the index holds, are the truth.
A subject that names a path or a Go file is marked, so the Go test can
report the unit rate with and without that help. Zero model cost; the
engine is judged by TestACM11_Replay (internal/graph/engine/acm11_test.go).

usage: replay.py REPO INDEX_JSONL OUT_TSV [BRANCH]
  REPO          a checkout whose history holds the commits (any worktree)
  INDEX_JSONL   the .rashomon/graph.jsonl of the tree the engine reads
  OUT_TSV       sha, truth units (;), names_path (0/1), prompt
"""
import json
import os
import re
import subprocess
import sys


def units_of(index_path):
    units = set()
    with open(index_path) as fh:
        next(fh)
        for line in fh:
            rec = json.loads(line)
            node = rec.get("Node")
            if node and node.get("Kind") == "unit":
                units.add(node["ID"].split(":", 2)[2])
    return units


def clean(subject):
    s = re.sub(r"\s*\([^)]*\)\s*$", "", subject)
    s = re.sub(r"^[a-z0-9/_., -]+:\s+", "", s)
    return " ".join(s.split())


def names_path(subject):
    return 1 if re.search(r"[A-Za-z0-9_]+\.go\b|[A-Za-z0-9_]+/[A-Za-z0-9_]+", subject) else 0


def main():
    if len(sys.argv) < 4:
        sys.exit(__doc__)
    repo, index_path, out = sys.argv[1:4]
    branch = sys.argv[4] if len(sys.argv) > 4 else "origin/main"
    units = units_of(index_path)
    log = subprocess.run(["git", "-C", repo, "log", "--no-merges", "--format=%H|%s", branch],
                         check=True, capture_output=True, text=True).stdout
    rows = []
    for line in log.splitlines():
        sha, subject = line.split("|", 1)
        files = subprocess.run(["git", "-C", repo, "show", "--format=", "--name-only", sha],
                               check=True, capture_output=True, text=True).stdout.split()
        truth = set()
        for f in files:
            if f.endswith(".go") and not f.endswith("_test.go"):
                d = os.path.dirname(f) or "."
                if d in units:
                    truth.add(d)
        prompt = clean(subject)
        if not truth or len(prompt.split()) < 4:
            continue
        rows.append((sha[:12], ";".join(sorted(truth)), names_path(subject), prompt))
    with open(out, "w") as fh:
        fh.write("# sha\tunits\tnames_path\tprompt\n")
        for r in rows:
            fh.write("%s\t%s\t%d\t%s\n" % r)
    print("rows %d of %d commits; names_path %d" % (len(rows), len(log.splitlines()), sum(r[2] for r in rows)))


if __name__ == "__main__":
    main()
