"""validate_classes.py: the pre-registration checks on classes.tsv against the indexed tree it is judged on (W-M1.19).

Every unit alternative but '-' is a unit node of the tree's graph; every target and reuse path but '-' and '*' is a
file in the tree and a file node of the graph; a prompt outside the named-path, non-go, pasted-error and greenfield
classes names no path; a greenfield prompt names a package the tree does not hold; a pasted-error prompt carries an
error shape. Run from this directory: python3 validate_classes.py --tree <indexed root> <classes.tsv>; exit 1 on any
finding, each printed as BAD <id> <check> <value>.
"""
import argparse
import json
import os
import re
import sys

PATH_FREE = {"named-path", "non-go", "pasted-error", "greenfield"}
PATH_RE = re.compile(r"[\w/]+\.go\b|internal/|cmd/")
PKG_RE = re.compile(r"internal/\w+")
ERROR_RE = re.compile(r"\.go|FAIL|panic")


def graph_sets(tree):
    units, files = set(), set()
    with open(os.path.join(tree, ".rashomon", "graph.jsonl"), encoding="utf-8") as f:
        next(f)
        for line in f:
            n = json.loads(line).get("Node")
            if not n:
                continue
            if n["Kind"] == "unit":
                units.add(n["ID"][len("unit:go:"):])
            elif n["Kind"] == "file":
                files.add(n["ID"][len("file:"):])
    return units, files


def rows_of(path):
    out = []
    for line in open(path, encoding="utf-8"):
        if not line.strip() or line.startswith("#"):
            continue
        f = line.rstrip("\n").split("\t", 8)
        if len(f) != 9:
            sys.exit("row with %d fields: %s" % (len(f), f[0]))
        out.append(f)
    return out


def check_row(row, tree, units, files):
    """The findings on one row as (id, check, value) tuples."""
    id_, cls, unit, target, reuse, _, _, _, prompt = row
    bad = []
    for u in unit.split(";"):
        if u != "-" and u not in units:
            bad.append((id_, "unit not in graph", u))
    for col, v in (("target", target), ("reuse", reuse)):
        if v in ("-", "*"):
            continue
        if not os.path.isfile(os.path.join(tree, v)):
            bad.append((id_, col + " not in tree", v))
        elif v not in files:
            bad.append((id_, col + " not a graph file", v))
    if cls not in PATH_FREE and PATH_RE.search(prompt):
        bad.append((id_, "prompt names a path", prompt[:60]))
    if cls == "greenfield":
        m = PKG_RE.search(prompt)
        if not m:
            bad.append((id_, "greenfield names no new package", prompt[:60]))
        elif os.path.isdir(os.path.join(tree, m.group(0))):
            bad.append((id_, "greenfield package exists", m.group(0)))
    if cls == "pasted-error" and not ERROR_RE.search(prompt):
        bad.append((id_, "pasted-error without an error shape", prompt[:60]))
    return bad


def main(argv=None):
    ap = argparse.ArgumentParser()
    ap.add_argument("--tree", required=True)
    ap.add_argument("tsv")
    a = ap.parse_args(argv)
    units, files = graph_sets(a.tree)
    rows = rows_of(a.tsv)
    bad = [b for r in rows for b in check_row(r, a.tree, units, files)]
    for b in bad:
        print("BAD", *b)
    print("rows %d bad %d units %d files %d" % (len(rows), len(bad), len(units), len(files)))
    return 1 if bad else 0


if __name__ == "__main__":
    sys.exit(main())
