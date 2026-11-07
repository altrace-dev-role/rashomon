"""Tests for truth_read.py, the blind second-read agreement judge (W-M1.19). Run from this directory:
python3 -m unittest test_truth_read -v"""
import unittest

import truth_read as tr


class ParseTruth(unittest.TestCase):
    def test_alternatives_split_on_semicolon(self):
        self.assertEqual(tr.parse_truth("internal/hookio;internal/harness"), {"internal/hookio", "internal/harness"})

    def test_dash_is_an_answer(self):
        self.assertEqual(tr.parse_truth("-;test/perf"), {"-", "test/perf"})

    def test_normalise_drops_module_prefix_and_slash(self):
        self.assertEqual(tr.norm("github.com/altrace-dev-role/rashomon/internal/verify/"), "internal/verify")
        self.assertEqual(tr.norm("  - "), "-")
        self.assertEqual(tr.norm(""), "-")


class Majority(unittest.TestCase):
    def test_two_of_three_is_the_majority(self):
        self.assertEqual(tr.majority(["a", "b", "a"]), "a")

    def test_all_different_is_split(self):
        self.assertIsNone(tr.majority(["a", "b", "c"]))

    def test_unanimous(self):
        self.assertEqual(tr.majority(["a", "a", "a"]), "a")


class Judge(unittest.TestCase):
    def row(self, truth, reads, cls="feature-add", rid="X1"):
        return {"id": rid, "cls": cls, "truth": truth, "reads": reads}

    def test_confirmed_when_majority_matches_an_alternative(self):
        j = tr.judge([self.row("internal/hookio;internal/harness", ["internal/harness", "internal/harness", "internal/hook"])])[0]
        self.assertEqual(j["verdict"], "confirmed")
        self.assertEqual(j["majority"], "internal/harness")
        self.assertEqual(j["reader_agree"], 2)

    def test_contested_when_majority_names_another_unit(self):
        j = tr.judge([self.row("internal/verify", ["internal/recap", "internal/recap", "internal/verify"])])[0]
        self.assertEqual(j["verdict"], "contested")
        self.assertEqual(j["reader_agree"], 1)

    def test_split_when_no_majority(self):
        j = tr.judge([self.row("internal/verify", ["a", "b", "internal/verify"])])[0]
        self.assertEqual(j["verdict"], "split")

    def test_dash_truth_confirmed_by_dash_reads(self):
        j = tr.judge([self.row("-", ["-", "-", "cmd/rashomon"])])[0]
        self.assertEqual(j["verdict"], "confirmed")

    def test_missing_read_counts_as_no_answer(self):
        j = tr.judge([self.row("internal/verify", ["internal/verify", None, "internal/verify"])])[0]
        self.assertEqual(j["verdict"], "confirmed")
        self.assertEqual(j["reads"], ["internal/verify", "-", "internal/verify"])


class PerClass(unittest.TestCase):
    def test_counts_per_class(self):
        rows = tr.judge([
            {"id": "A", "cls": "c1", "truth": "u", "reads": ["u", "u", "u"]},
            {"id": "B", "cls": "c1", "truth": "u", "reads": ["v", "v", "u"]},
            {"id": "C", "cls": "c2", "truth": "u", "reads": ["u", "v", "w"]},
        ])
        t = tr.per_class(rows)
        self.assertEqual(t["c1"], {"n": 2, "confirmed": 1, "contested": 1, "split": 0, "unanimous": 1, "reader_agree": 4, "reads": 6})
        self.assertEqual(t["c2"], {"n": 1, "confirmed": 0, "contested": 0, "split": 1, "unanimous": 0, "reader_agree": 1, "reads": 3})


class Kappa(unittest.TestCase):
    def test_full_agreement_is_one(self):
        self.assertAlmostEqual(tr.fleiss_kappa([["a", "a", "a"], ["b", "b", "b"]]), 1.0)

    def test_no_agreement_is_below_zero(self):
        self.assertLess(tr.fleiss_kappa([["a", "b", "c"], ["d", "e", "f"]]), 0.0)


if __name__ == "__main__":
    unittest.main()
