"""validate_classes.py: the pre-registration checks on classes.tsv against the indexed tree it is judged on (W-M1.19).

Every unit alternative but '-' is a unit node of the tree's graph; every target and reuse path but '-' and '*' is a
file in the tree and a file node of the graph; a prompt outside the named-path, non-go, pasted-error and greenfield
classes names no path; a greenfield prompt names a package the tree does not hold; a pasted-error prompt carries an
error shape. A consumers or knowledge cell of per-alternative sets (derive_truths.py --per-alternative) holds one
'unit=set' part per unit alternative, in the unit cell's order. With --against a.tsv:b.tsv no id is taken and no
prompt repeats or nearly repeats (word-set overlap of 0.8 or more) a prompt there or in the set itself; with
--per-class N every class is one of the sixteen claimed and holds N rows (classes2.tsv, W-M1.27). Run from this
directory: python3 validate_classes.py --tree <indexed root> <classes.tsv>; exit 1 on any finding, each printed as
BAD <id> <check> <value>.
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
WORD_RE = re.compile(r"[a-z0-9]+")
NEAR = 0.8
CLAIMED = ("feature-add", "bug-symptom", "refactor", "delete", "tests-only", "performance", "non-go", "question",
           "named-name", "named-path", "vague", "greenfield", "cross-cutting", "knowledge-id", "negation", "pasted-error")


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


def rows_any(path):
    """A truth file's rows as [id, ..., prompt] with the prompt last, whatever its column count (prompts.tsv and
    edge.tsv hold fewer columns than classes.tsv); only the id and the prompt are read."""
    out = []
    for line in open(path, encoding="utf-8"):
        if not line.strip() or line.startswith("#"):
            continue
        f = line.rstrip("\n").split("\t")
        out.append([f[0]] + [""] * 7 + [f[-1]])
    return out


def check_row(row, tree, units, files):
    """The findings on one row as (id, check, value) tuples."""
    id_, cls, unit, target, reuse, consumers, knowledge, _, prompt = row
    bad = []
    for u in unit.split(";"):
        if u != "-" and u not in units:
            bad.append((id_, "unit not in graph", u))
    alts = [u for u in unit.split(";") if u != "-"]
    for col, c in (("consumers", consumers), ("knowledge", knowledge)):
        if "=" in c and [p.split("=", 1)[0] for p in c.split("|")] != alts:
            bad.append((id_, col + " sets are not one per unit alternative", c[:60]))
    for col, cell in (("target", target), ("reuse", reuse)):
        for v in cell.split(";"):
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


def words(prompt):
    return set(WORD_RE.findall(prompt.lower()))


def check_set(rows, others, per_class=0):
    """The findings across rows: an id taken in the set or by another file, a prompt that repeats one before
    it or one of another file (the same words, or a word-set overlap of NEAR or more), and with per_class a
    class outside CLAIMED or one not holding per_class rows."""
    bad, ids, seen = [], {r[0] for r in others}, [(r[0], words(r[8])) for r in others]
    for r in rows:
        if r[0] in ids:
            bad.append((r[0], "id taken", r[0]))
        ids.add(r[0])
        w = words(r[8])
        for oid, ow in seen:
            if w and len(w & ow) / len(w | ow) >= NEAR:
                bad.append((r[0], "prompt near-duplicates", oid))
                break
        seen.append((r[0], w))
    if per_class:
        count = {}
        for r in rows:
            count[r[1]] = count.get(r[1], 0) + 1
        for cls in sorted(set(count) | set(CLAIMED)):
            if cls not in CLAIMED:
                bad.append(("-", "class not claimed", cls))
            elif count.get(cls, 0) != per_class:
                bad.append(("-", "class rows not %d" % per_class, "%s %d" % (cls, count.get(cls, 0))))
    return bad


def main(argv=None):
    ap = argparse.ArgumentParser()
    ap.add_argument("--tree", required=True)
    ap.add_argument("--against", default="", help="':'-separated truth tsv files whose ids and prompts are taken")
    ap.add_argument("--per-class", type=int, default=0)
    ap.add_argument("tsv")
    a = ap.parse_args(argv)
    units, files = graph_sets(a.tree)
    rows = rows_of(a.tsv)
    others = [r for p in a.against.split(":") if p for r in rows_any(p)]
    bad = [b for r in rows for b in check_row(r, a.tree, units, files)]
    bad += check_set(rows, others, a.per_class)
    for b in bad:
        print("BAD", *b)
    print("rows %d bad %d units %d files %d" % (len(rows), len(bad), len(units), len(files)))
    return 1 if bad else 0


if __name__ == "__main__":
    sys.exit(main())
