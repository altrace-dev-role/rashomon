"""Tests for derive_truths.py, the graph-derived consumer and knowledge truths (W-M1.19). Run from this
directory: python3 -m unittest test_derive_truths -v"""
import json
import os
import tempfile
import unittest

import derive_truths as dt


def node(id_, kind, **attrs):
    return {"Node": {"ID": id_, "Kind": kind, "Attrs": attrs}, "Edge": None}


def edge(src, pred, dst):
    return {"Node": None, "Edge": {"Src": src, "Pred": pred, "Dst": dst}}


FIXTURE = [
    {"schema": 1, "repo": "r", "rev": {"commit": "C1"}, "records": 0},
    node("unit:go:a", "unit", path="a"), node("unit:go:b", "unit", path="b"), node("unit:go:c", "unit", path="c"),
    node("file:a/a.go", "file"), node("file:b/b.go", "file"), node("file:c/c.go", "file"),
    node("sym:go:a:func:Do", "sym", name="Do", symkind="func"),
    node("sym:go:b:func:Use", "sym", name="Use", symkind="func"),
    node("sym:go:c:func:Also", "sym", name="Also", symkind="func"),
    node("sym:go:c:func:More", "sym", name="More", symkind="func"),
    node("req:R1", "requirement"), node("dec:D1", "decision"), node("les:L1", "lesson"),
    edge("unit:go:a", "contains", "file:a/a.go"), edge("unit:go:b", "contains", "file:b/b.go"), edge("unit:go:c", "contains", "file:c/c.go"),
    edge("file:a/a.go", "declares", "sym:go:a:func:Do"), edge("file:b/b.go", "declares", "sym:go:b:func:Use"),
    edge("file:c/c.go", "declares", "sym:go:c:func:Also"), edge("file:c/c.go", "declares", "sym:go:c:func:More"),
    edge("sym:go:b:func:Use", "calls", "sym:go:a:func:Do"),
    edge("sym:go:c:func:Also", "may_call", "sym:go:a:func:Do"),
    edge("sym:go:c:func:More", "references", "sym:go:a:func:Do"),
    edge("sym:go:a:func:Do", "calls", "sym:go:a:func:Do"),
    edge("req:R1", "applies_to", "file:a/a.go"), edge("dec:D1", "applies_to", "sym:go:a:func:Do"), edge("les:L1", "applies_to", "unit:go:b"),
]


class Derive(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.dir = tempfile.mkdtemp()
        cls.path = os.path.join(cls.dir, "graph.jsonl")
        with open(cls.path, "w") as f:
            for r in FIXTURE:
                f.write(json.dumps(r) + "\n")
        cls.g = dt.load_graph(cls.path)

    def test_consumers_are_files_outside_the_unit_most_references_first(self):
        self.assertEqual(dt.consumers_of_unit(self.g, "a"), ["c/c.go", "b/b.go"])

    def test_a_unit_calling_itself_is_no_consumer(self):
        self.assertEqual(dt.consumers_of_unit(self.g, "b"), [])

    def test_knowledge_on_file_and_symbol_requirement_first(self):
        self.assertEqual(dt.knowledge_of_unit(self.g, "a"), ["req:R1", "dec:D1"])

    def test_knowledge_on_the_unit_node(self):
        self.assertEqual(dt.knowledge_of_unit(self.g, "b"), ["les:L1"])

    def test_unknown_unit_raises(self):
        with self.assertRaises(KeyError):
            dt.consumers_of_unit(self.g, "zz")

    def test_truth_cell_dash_when_empty(self):
        self.assertEqual(dt.cell([]), "-")
        self.assertEqual(dt.cell(["x", "y"]), "x;y")

    def test_fill_row_derives_only_empty_cells(self):
        row = {"unit": "a", "consumers": "", "knowledge": "kept"}
        dt.fill_row(self.g, row)
        self.assertEqual(row["consumers"], "c/c.go;b/b.go")
        self.assertEqual(row["knowledge"], "kept")

    def test_fill_row_dash_unit_derives_dash(self):
        row = {"unit": "-", "consumers": "", "knowledge": ""}
        dt.fill_row(self.g, row)
        self.assertEqual((row["consumers"], row["knowledge"]), ("-", "-"))

    def test_fill_row_alternatives_derive_the_union(self):
        row = {"unit": "a;b", "consumers": "", "knowledge": ""}
        dt.fill_row(self.g, row)
        self.assertEqual(row["consumers"], "c/c.go;b/b.go")
        self.assertEqual(row["knowledge"], "req:R1;les:L1;dec:D1")

    def test_fill_row_dash_alternative_is_skipped(self):
        row = {"unit": "-;b", "consumers": "", "knowledge": ""}
        dt.fill_row(self.g, row)
        self.assertEqual((row["consumers"], row["knowledge"]), ("-", "les:L1"))


if __name__ == "__main__":
    unittest.main()
