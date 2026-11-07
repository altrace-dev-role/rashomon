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


if __name__ == "__main__":
    unittest.main()
