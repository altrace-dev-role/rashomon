"""Tests for validate_classes.py. Run from this directory: python3 -m unittest test_validate_classes -v"""
import os
import tempfile
import unittest

import validate_classes as vc


def row(id_="X1", cls="bug-symptom", unit="internal/a", target="internal/a/a.go", reuse="-", prompt="The a thing breaks."):
    return [id_, cls, unit, target, reuse, "", "", "*", prompt]


class Validate(unittest.TestCase):
    def setUp(self):
        self.tree = tempfile.mkdtemp()
        os.makedirs(os.path.join(self.tree, "internal", "a"))
        open(os.path.join(self.tree, "internal", "a", "a.go"), "w").close()
        self.units = {"internal/a"}
        self.files = {"internal/a/a.go"}

    def check(self, r):
        return [b[1] for b in vc.check_row(r, self.tree, self.units, self.files)]

    def test_clean_row_has_no_findings(self):
        self.assertEqual(self.check(row()), [])

    def test_unknown_unit_and_missing_target(self):
        self.assertEqual(self.check(row(unit="internal/zz", target="internal/a/zz.go")), ["unit not in graph", "target not in tree"])

    def test_reuse_alternatives_are_checked_one_by_one(self):
        os.makedirs(os.path.join(self.tree, "internal", "b"))
        open(os.path.join(self.tree, "internal", "b", "b.go"), "w").close()
        self.files.add("internal/b/b.go")
        self.assertEqual(self.check(row(reuse="internal/a/a.go;internal/b/b.go")), [])
        self.assertEqual(self.check(row(reuse="internal/a/a.go;internal/b/zz.go")), ["reuse not in tree"])

    def test_prompt_naming_a_path_outside_the_path_classes(self):
        self.assertEqual(self.check(row(prompt="Fix internal/a/a.go now.")), ["prompt names a path"])
        self.assertEqual(self.check(row(cls="named-path", prompt="Fix internal/a/a.go now.")), [])

    def test_greenfield_package_must_be_absent(self):
        self.assertEqual(self.check(row(cls="greenfield", unit="-", target="-", reuse="*", prompt="Add a new package internal/a for x.")), ["greenfield package exists"])
        self.assertEqual(self.check(row(cls="greenfield", unit="-", target="-", reuse="*", prompt="Add a new package internal/new for x.")), [])

    def test_pasted_error_needs_an_error_shape(self):
        self.assertEqual(self.check(row(cls="pasted-error", prompt="something odd")), ["pasted-error without an error shape"])

    def test_per_alternative_sets_follow_the_unit_cell(self):
        r = row(unit="internal/a;-;internal/b")
        self.units.add("internal/b")
        r[5], r[6] = "internal/a=x.go|internal/b=-", "-"
        self.assertEqual(self.check(r), [])
        r[5] = "internal/b=-|internal/a=x.go"
        self.assertEqual(self.check(r), ["consumers sets are not one per unit alternative"])
        r[5], r[6] = "x.go", "internal/a=-"
        self.assertEqual(self.check(r), ["knowledge sets are not one per unit alternative"])

    def test_set_refuses_taken_ids_and_near_duplicate_prompts(self):
        others = [["B1"] + [""] * 7 + ["The report prints a timeline row twice when two tool calls share an id."]]
        rows = [row(id_="B1", prompt="Something new entirely."),
                row(id_="BS01", prompt="The report prints a timeline row twice when two tool calls share one id."),
                row(id_="BS02", prompt="Something new entirely, again.")]
        self.assertEqual(vc.check_set(rows, others), [("B1", "id taken", "B1"), ("BS01", "prompt near-duplicates", "B1")])
        self.assertEqual(vc.check_set([row(id_="X1"), row(id_="X1", prompt="Other words here.")], []), [("X1", "id taken", "X1")])

    def test_set_per_class_counts_every_claimed_class(self):
        rows = [row(id_="X%d%d" % (j, i), cls=c, prompt="p %s %d" % (c, i)) for j, c in enumerate(vc.CLAIMED) for i in range(2)]
        self.assertEqual(vc.check_set(rows, [], per_class=2), [])
        bad = vc.check_set(rows[1:] + [row(id_="Z1", cls="multi-task", prompt="z")], [], per_class=2)
        self.assertEqual(bad, [("-", "class rows not 2", "feature-add 1"), ("-", "class not claimed", "multi-task")])


if __name__ == "__main__":
    unittest.main()
