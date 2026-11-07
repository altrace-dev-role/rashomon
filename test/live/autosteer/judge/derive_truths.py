"""derive_truths.py: the graph-derived consumer and knowledge truths of a classes.tsv row (W-M1.19).

Given a row's unit truth, the graph (.rashomon/graph.jsonl: {"Node":...} and {"Edge":...} records) yields
two truths that no author writes by hand: the unit's non-local consumers, every file outside the unit whose
declared symbols call, may call or reference a symbol the unit's files declare, most referenced symbols
first, then by path; and the knowledge applying to the unit, every requirement, lesson or decision whose
applies_to edge lands on the unit node, one of its files or one of its symbols, requirements first, then
lessons, then decisions, then by id. A cell is '-' when the set is empty. The derivation is independent of
the engine's Go code on purpose: it is read from the records, so a traversal bug in the engine shows as a
miss against it.

Run from this directory: python3 derive_truths.py --graph <graph.jsonl> --tsv classes.tsv > classes.filled.tsv
fills the consumers and knowledge columns of every row whose cell is empty and leaves filled cells alone.
"""
import argparse
import collections
import json
import sys

CONSUMING = ("calls", "may_call", "references")
KIND_RANK = {"req": 0, "les": 1, "dec": 2}


class Graph:
    def __init__(self):
        self.nodes = {}
        self.out = collections.defaultdict(list)
        self.inn = collections.defaultdict(list)
        self.file_unit = {}
        self.sym_file = {}

    def unit_id(self, path):
        uid = "unit:go:" + path
        if uid not in self.nodes:
            raise KeyError(path)
        return uid

    def files(self, unit):
        return sorted(f for f, u in self.file_unit.items() if u == unit)

    def syms(self, files):
        fs = set(files)
        return [s for s, f in self.sym_file.items() if f in fs]


def load_graph(path):
    g = Graph()
    for line in open(path, encoding="utf-8"):
        d = json.loads(line)
        if d.get("Node"):
            g.nodes[d["Node"]["ID"]] = d["Node"]
        elif d.get("Edge"):
            e = d["Edge"]
            g.out[e["Src"]].append((e["Pred"], e["Dst"]))
            g.inn[e["Dst"]].append((e["Pred"], e["Src"]))
    for n in g.nodes.values():
        if n["Kind"] == "unit":
            for pred, f in g.out[n["ID"]]:
                if pred == "contains":
                    g.file_unit[f] = n["Attrs"]["path"]
    for f in g.file_unit:
        for pred, s in g.out[f]:
            if pred == "declares":
                g.sym_file[s] = f
    return g


def consumers_of_unit(g, unit):
    g.unit_id(unit)
    count = collections.Counter()
    for s in g.syms(g.files(unit)):
        for pred, src in g.inn[s]:
            if pred in CONSUMING:
                f = g.sym_file.get(src)
                if f and g.file_unit.get(f) != unit:
                    count[f] += 1
    ordered = sorted(count, key=lambda f: (-count[f], f))
    return [f.split(":", 1)[1] for f in ordered]


def knowledge_of_unit(g, unit):
    uid = g.unit_id(unit)
    files = g.files(unit)
    found = set()
    for target in [uid] + files + g.syms(files):
        for pred, src in g.inn[target]:
            if pred == "applies_to":
                found.add(src)
    return sorted(found, key=knowledge_key)


def cell(items):
    return ";".join(items) if items else "-"


def knowledge_key(k):
    return (KIND_RANK.get(k.split(":")[0], 3), k)


def fill_row(g, row):
    """A unit cell of 'a;b' accepts either unit, so the derived sets are the union over the alternatives
    (consumers in the first alternative's order, then the next's, duplicates dropped; knowledge re-sorted
    by kind then id); a '-' alternative contributes nothing, and a cell of '-' alone derives '-'."""
    units = [u.strip() for u in row["unit"].split(";") if u.strip() != "-"]
    if not units:
        for k in ("consumers", "knowledge"):
            if not row[k]:
                row[k] = "-"
        return row
    if not row["consumers"]:
        seen = []
        for u in units:
            for f in consumers_of_unit(g, u):
                if f not in seen:
                    seen.append(f)
        row["consumers"] = cell(seen)
    if not row["knowledge"]:
        found = set()
        for u in units:
            found.update(knowledge_of_unit(g, u))
        row["knowledge"] = cell(sorted(found, key=knowledge_key))
    return row


COLUMNS = ("id", "class", "unit", "target", "reuse", "consumers", "knowledge", "allow", "prompt")


def main(argv=None):
    ap = argparse.ArgumentParser()
    ap.add_argument("--graph", required=True)
    ap.add_argument("--tsv", required=True)
    a = ap.parse_args(argv)
    g = load_graph(a.graph)
    for line in open(a.tsv, encoding="utf-8"):
        if not line.strip() or line.startswith("#"):
            sys.stdout.write(line)
            continue
        f = line.rstrip("\n").split("\t", len(COLUMNS) - 1)
        if len(f) != len(COLUMNS):
            sys.exit("row with %d fields: %s" % (len(f), f[0]))
        row = dict(zip(COLUMNS, f))
        fill_row(g, row)
        sys.stdout.write("\t".join(row[c] for c in COLUMNS) + "\n")
    return 0


if __name__ == "__main__":
    sys.exit(main())
