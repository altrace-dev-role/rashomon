"""Tests for done.py, the done-flag precision judge (W-M3.6, AC-M3.5). Run from this directory:
cd /tmp && python3 -I -B -m unittest discover -s <this directory> -p test_done.py -v
(-I keeps the cwd off sys.path, so "-m unittest test_done" cannot import it.)"""
import contextlib
import io
import json
import os
import re
import shutil
import sys
import tempfile
import unittest
from unittest import mock

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import done  # noqa: E402

M = "\u203b rashomon: "
T0 = "2026-11-05T10:00:00Z"
# A synthetic scope: folder names and role codes that follow no real scheme. HELD starts with PROJ on purpose, so
# a skip prefix has to win over an eligible one.
PROJ, HELD, BETA, OTHER = "-srv-alpha", "-srv-alpha-held", "-srv-beta", "-srv-other"
ELIGIBLE, SKIP = [PROJ], [HELD]
ROLE_SRC = "(?<![A-Za-z0-9_])ZZ[0-9]+(?![A-Za-z0-9_])"
ROLE = re.compile(ROLE_SRC)


def ts(minute):
    return "2026-11-05T10:%02d:00Z" % minute


class Fx:
    """Synthetic Claude Code transcripts: one list of records per session."""

    def __init__(self, sid):
        self.sid = sid
        self.recs = []

    def _add(self, r, minute):
        r.update(timestamp=ts(minute), sessionId=self.sid)
        self.recs.append(r)
        return self

    def prompt(self, text, m):
        return self._add({"type": "user", "isSidechain": False, "message": {"role": "user", "content": text}}, m)

    def say(self, text, m):
        return self._add({"type": "assistant", "message": {"content": [{"type": "text", "text": text}]}}, m)

    def call(self, name, inp, tid, m, is_error=False, result_text="out"):
        self._add({"type": "assistant", "message": {"content": [{"type": "tool_use", "name": name, "input": inp,
                                                                 "id": tid}]}}, m)
        return self._add({"type": "user", "message": {"content": [{"type": "tool_result", "tool_use_id": tid,
                                                                   "is_error": is_error, "content": result_text}]}}, m)

    def hook(self, text, m, event="Stop"):
        return self._add({"type": "attachment", "attachment": {"type": "hook_system_message", "hookEvent": event,
                                                               "hookName": event, "content": M + text}}, m)

    def write(self, root, project):
        d = os.path.join(root, project)
        os.makedirs(d, exist_ok=True)
        with open(os.path.join(d, self.sid + ".jsonl"), "w", encoding="utf-8") as f:
            for r in self.recs:
                f.write(json.dumps(r) + "\n")


LEAD = done.LEAD + " "


def build(root):
    a = Fx("aaaa1111-0000")
    a.prompt("fix the parser", 0)
    a.call("Edit", {"file_path": "/r/p.go"}, "e1", 1)
    a.call("Bash", {"command": "go test ./... | tail -3"}, "b1", 2)
    a.say("Done. Tests pass.\n" + M + LEAD + "quoted by the model", 3)
    a.hook(LEAD + "go test ran; its exit status was hidden by its line", 4)                  # firing 1: run
    a.prompt("now the docs; why did it say " + LEAD + "?", 5)                                  # a quote
    a.say("Finished the docs.", 6)
    a.hook(LEAD + "no test or build run is recorded for this turn. expected evidence lint is not on the record "
           "(spec).", 7)                                                                      # firing 2: no-run+expected
    a.hook(LEAD + "not a Stop", 8, event="UserPromptSubmit")                                  # not a firing
    a.say("I quote: " + LEAD + "x", 9)                                                        # a quote
    a.call("Read", {"file_path": "/r/q.go"}, "r1", 10, result_text=LEAD + "in a tool result")  # a quote
    b = Fx("bbbb2222-0000")
    b.prompt("add a file", 20)
    b.call("Write", {"file_path": "/r/new.go"}, "w1", 21)
    b.say("All done.", 22)
    b.hook(LEAD + "no test or build run is recorded for this turn", 23)                        # firing 3: no-run
    b.prompt("build it", 24)
    b.call("Bash", {"command": "go build ./..."}, "b2", 25, is_error=True)
    b.say("Done, as ZZ7 asked.", 26)
    b.hook(LEAD + "go build failed", 27)                                                      # firing 4: role code
    late = Fx("cccc3333-0000")
    late.prompt("p", 50)
    late.say("done", 51)
    late._add({"type": "attachment", "attachment": {"type": "hook_system_message", "hookEvent": "Stop",
                                                    "content": M + LEAD + "late"}}, 52)
    late.recs[-1]["timestamp"] = "2026-12-01T00:00:00Z"                                       # outside the window
    w = Fx("dddd4444-0000")
    w.prompt("p", 30)
    w.say("done", 31)
    w.hook(LEAD + "work", 32)                                                                 # a skipped folder
    a.write(root, PROJ)
    b.write(root, PROJ)
    late.write(root, PROJ)
    w.write(root, HELD)
    return a, b


WINDOW = ["--since", "2026-11-05T00:00:00Z", "--until", "2026-11-06T00:00:00Z"]


class Base(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.mkdtemp(prefix="donepy-")
        self.root = os.path.join(self.tmp, "projects")
        self.a, self.b = build(self.root)
        self.glob = os.path.join(self.root, "*", "*.jsonl")

    def tearDown(self):
        shutil.rmtree(self.tmp)

    def run_main(self, argv):
        buf = io.StringIO()
        with contextlib.redirect_stdout(buf):
            rc = done.main(argv)
        return rc, buf.getvalue()

    def scope(self, eligible=ELIGIBLE, skip=SKIP, role=ROLE_SRC):
        """The flags that name the scope, as the command line takes them. Each value is joined to its flag with an
        equals sign: a folder name starts with a dash, and argparse takes a bare value that starts with a dash for a
        flag."""
        argv = ["--role-pattern=" + role]
        argv += ["--eligible-prefix=" + prefix for prefix in eligible]
        argv += ["--skip-prefix=" + prefix for prefix in skip]
        return argv

    def frame(self, exclude=None, eligible=ELIGIBLE, skip=SKIP, role=ROLE):
        return done.build_frame([self.glob], WINDOW[1], WINDOW[3], exclude, eligible, skip, role)

    def add_session(self, project, sid):
        """One more transcript in project, with one firing in the window (minute 42)."""
        extra = Fx(sid)
        extra.prompt("p", 40)
        extra.say("done", 41)
        extra.hook(LEAD + "extra", 42)
        extra.write(self.root, project)


class Frame(Base):
    def test_only_structural_stop_firings_count(self):
        counts, frame = self.frame()
        self.assertEqual(counts["files_skipped_prefix"], 1)
        self.assertEqual(counts["firings_in_window"], 4)
        self.assertEqual(counts["excluded_role_code"], 1)
        self.assertEqual(counts["frame"], 3)
        self.assertEqual(sorted(f["cls"] for f in frame), ["no-run", "no-run+expected", "run"])

    def test_quotes_and_other_events_are_not_firings(self):
        for r in self.a.recs:
            if r.get("type") != "attachment":
                self.assertIsNone(done.flag_text(r))
        ups = [r for r in self.a.recs if r.get("attachment", {}).get("hookEvent") == "UserPromptSubmit"]
        self.assertEqual(len(ups), 1)
        self.assertIsNone(done.flag_text(ups[0]))

    def test_exclude_list_drops_a_firing_by_session_and_time(self):
        p = os.path.join(self.tmp, "ex.tsv")
        with open(p, "w") as f:
            f.write("# PART 4 rows\naaaa\t2026-11-05T10:03:30Z\t2026-11-05T10:04:30Z\tPART 4 row 1\n")
        counts, frame = self.frame(exclude=p)
        self.assertEqual(counts["excluded_by_list"], 1)
        self.assertEqual(counts["frame"], 2)
        self.assertNotIn("run", [f["cls"] for f in frame])

    def test_frame_mode_prints_counts_and_no_text(self):
        rc, out = self.run_main(["frame", "--glob", self.glob] + WINDOW + self.scope())
        self.assertEqual(rc, 0)
        self.assertNotIn("Tests pass", out)
        self.assertEqual(json.loads(out)["counts"]["frame"], 3)


class Turn(Base):
    def firing(self, cls):
        _, frame = self.frame()
        return next(f for f in frame if f["cls"] == cls)

    def test_claim_is_last_text_without_marker_lines(self):
        t = self.firing("run")["turn"]
        self.assertEqual(t["claim"], "Done. Tests pass.")
        self.assertEqual(t["prompt"], "fix the parser")

    def test_actions_mark_last_edit_and_results(self):
        acts = self.firing("run")["turn"]["actions"]
        self.assertEqual(acts[0], "* Edit  /r/p.go")
        self.assertEqual(acts[1], "  Bash  go test ./... | tail -3 -> ok")

    def test_window_reaches_back_to_an_earlier_turns_edit(self):
        acts = self.firing("no-run+expected")["turn"]["actions"]
        self.assertTrue(acts[0].startswith("* earlier turn Edit"), acts)
        self.assertTrue(all("earlier turn" in a for a in acts), acts)

    def test_reason_class(self):
        self.assertEqual(done.reason_class(LEAD + "no test or build run is recorded for this turn"), "no-run")
        self.assertEqual(done.reason_class(LEAD + "x. expected evidence vet is not on the record (r)."),
                         "run+expected")

    def test_cap_keeps_the_last_edit(self):
        lines = ["* Edit a"] + ["  Read %d" % i for i in range(60)]
        out = done.cap_actions(lines, 0, cap=10)
        self.assertEqual(len(out), 10)
        self.assertEqual(out[0], "01 * Edit a")
        self.assertIn("omitted", out[1])


class Draw(Base):
    def reg(self, body, seal=True):
        p = os.path.join(self.tmp, "REG-%d.txt" % len(os.listdir(self.tmp)))
        with open(p, "w") as f:
            f.write(body)
        if seal:
            os.chmod(p, 0o444)
        return p

    def draw(self, reg, out):
        return self.run_main(["draw", "--glob", self.glob] + WINDOW + ["--reg", reg, "--out", out] + self.scope())

    def test_refuses_unsealed_unnamed_or_existing_output(self):
        me = done.sha_file(done.__file__)
        rc, out = self.draw(self.reg("done.py sha256: " + me, seal=False), os.path.join(self.tmp, "o1"))
        self.assertEqual(rc, 2)
        self.assertIn("not sealed", out)
        rc, out = self.draw(self.reg("done.py sha256: 0000"), os.path.join(self.tmp, "o2"))
        self.assertEqual(rc, 2)
        self.assertIn("does not name", out)
        os.mkdir(os.path.join(self.tmp, "o3"))
        rc, out = self.draw(self.reg("done.py sha256: " + me), os.path.join(self.tmp, "o3"))
        self.assertEqual(rc, 2)
        self.assertFalse(os.path.exists(os.path.join(self.tmp, "o1")))

    def test_draw_is_deterministic_blind_and_keyed(self):
        reg = self.reg("done.py sha256: " + done.sha_file(done.__file__))
        o1, o2 = os.path.join(self.tmp, "d1"), os.path.join(self.tmp, "d2")
        self.assertEqual(self.draw(reg, o1)[0], 0)
        self.assertEqual(self.draw(reg, o2)[0], 0)
        k1 = open(os.path.join(o1, "key.tsv")).read()
        self.assertEqual(k1, open(os.path.join(o2, "key.tsv")).read())
        rows = done.read_key(os.path.join(o1, "key.tsv"))
        self.assertEqual(len(rows), 3)
        self.assertEqual(sorted(r["class"] for r in rows), ["no-run", "no-run+expected", "run"])
        for n in range(1, 4):
            s = open(os.path.join(o1, "sheets", "D%02d.txt" % n), encoding="utf-8").read()
            self.assertNotIn(done.LEAD, s)
            self.assertNotIn("\u203b", s)
            self.assertIn("ANSWER one of", s)
        d = json.load(open(os.path.join(o1, "draw.json")))
        self.assertEqual((d["n_wanted"], d["n_drawn"]), (30, 3))
        self.assertEqual(d["key_sha256"], done.sha_file(os.path.join(o1, "key.tsv")))

    def test_interleave_takes_one_per_cluster_per_round(self):
        items = [{"path": "p", "index": i, "c": c} for i, c in enumerate("aaabbc")]
        o1 = done.interleave([dict(x) for x in items], 7, key=lambda x: x["c"])
        o2 = done.interleave([dict(x) for x in items], 7, key=lambda x: x["c"])
        self.assertEqual([x["index"] for x in o1], [x["index"] for x in o2])
        self.assertEqual(len({x["c"] for x in o1[:3]}), 3)
        self.assertEqual([x["round"] for x in o1], [1, 1, 1, 2, 2, 3])


class Scope(Base):
    """The judge reads only the folders it is told it may and drops only the role codes it is given. Each test names
    the change that would break it."""

    def refused(self, argv):
        rc, out = self.run_main(argv)
        self.assertEqual(rc, 2, out)
        self.assertIn("REFUSED (nothing written):", out)
        self.assertNotIn('"counts"', out)
        return out

    def test_frame_refuses_a_file_outside_the_eligible_folders(self):
        # Breaks if a file in a folder no prefix names is read or counted instead of stopping the run.
        self.add_session(OTHER, "eeee5555-0000")
        out = self.refused(["frame", "--glob", self.glob] + WINDOW + self.scope())
        self.assertIn(OTHER, out)

    def test_draw_refuses_before_it_writes(self):
        # Breaks if the scope check runs after the output folder is made.
        self.add_session(OTHER, "eeee5555-0000")
        reg = os.path.join(self.tmp, "REG-scope.txt")
        with open(reg, "w") as handle:
            handle.write("done.py sha256: " + done.sha_file(done.__file__))
        os.chmod(reg, 0o444)
        out_dir = os.path.join(self.tmp, "o-scope")
        self.refused(["draw", "--glob", self.glob] + WINDOW + ["--reg", reg, "--out", out_dir] + self.scope())
        self.assertFalse(os.path.exists(out_dir))

    def test_refusal_comes_before_any_transcript_is_read(self):
        # Breaks if the check moves into the read loop, after some transcript text is already loaded.
        self.add_session(OTHER, "eeee5555-0000")
        with mock.patch.object(done.common, "load_jsonl", side_effect=AssertionError("a transcript was read")):
            self.refused(["frame", "--glob", self.glob] + WINDOW + self.scope())

    def test_every_eligible_prefix_counts(self):
        # Breaks if only the first prefix is used.
        self.add_session(BETA, "eeee5555-0000")
        counts, _ = self.frame(eligible=[PROJ, BETA])
        self.assertEqual((counts["files"], counts["frame"]), (5, 4))

    def test_nothing_is_skipped_unless_named(self):
        # Breaks if a folder is skipped by a built-in rule and not by the skip prefix given.
        counts, _ = self.frame(skip=[])
        self.assertEqual((counts["files_skipped_prefix"], counts["frame"]), (0, 4))

    def test_the_role_pattern_is_the_one_given(self):
        # Breaks if a built-in pattern still decides which turns are dropped.
        counts, frame = self.frame(role=re.compile("(?!)"))
        self.assertEqual((counts["excluded_role_code"], counts["frame"]), (0, 4))
        self.assertTrue(any("go build failed" in item["flag"] for item in frame))
        counts, frame = self.frame(role=re.compile("fix the parser"))
        self.assertEqual(counts["excluded_role_code"], 1)
        self.assertTrue(any("go build failed" in item["flag"] for item in frame))
        _, frame = self.frame()
        self.assertFalse(any("go build failed" in item["flag"] for item in frame))

    def test_an_empty_prefix_is_refused(self):
        # Breaks if "" is accepted: every folder starts with it, so an empty eligible prefix admits all of them and an
        # empty skip prefix drops all of them.
        for flags in (self.scope(eligible=[""]), self.scope(skip=[""])):
            out = self.refused(["frame", "--glob", self.glob] + WINDOW + flags)
            self.assertIn("empty", out)

    def test_an_empty_or_broken_role_pattern_is_refused(self):
        # Breaks if an empty pattern is compiled (it matches every turn) or a bad one ends in a traceback.
        for src in ("", "("):
            out = self.refused(["frame", "--glob", self.glob] + WINDOW + self.scope(role=src))
            self.assertIn("role pattern", out)

    def test_the_scope_flags_have_no_default(self):
        # Breaks if a flag gets a built-in default: a forgotten flag would then run on a scheme nobody named.
        for flag in ("--eligible-prefix", "--role-pattern"):
            argv = [item for item in self.scope() if not item.startswith(flag + "=")]
            err = io.StringIO()
            with contextlib.redirect_stderr(err), self.assertRaises(SystemExit) as caught:
                done.main(["frame", "--glob", self.glob] + WINDOW + argv)
            self.assertEqual(caught.exception.code, 2)
            # The usage line lists every flag, so the flag name alone would pass; the required message names it.
            self.assertIn("required: " + flag, err.getvalue())

    def test_a_file_in_a_deeper_folder_is_refused(self):
        # Breaks if a file counts as eligible because some folder above its own folder has an eligible name: only the
        # folder that holds the file is compared with the prefixes.
        self.add_session(os.path.join(PROJ, "nested"), "eeee5555-0000")
        deep_glob = os.path.join(self.root, "**", "*.jsonl")
        out = self.refused(["frame", "--glob", deep_glob] + WINDOW + self.scope())
        self.assertIn("nested", out)

    def test_a_skip_prefix_is_a_plain_prefix(self):
        # Breaks if a skip prefix has to equal a whole folder name: a prefix names where a folder name starts. With
        # nothing skipped the frame is 4, and one of those firings sits in the held folder.
        counts, _ = self.frame(skip=[HELD[:-2]])
        self.assertEqual((counts["files_skipped_prefix"], counts["frame"]), (1, 3))

    def test_the_refusal_names_ten_folders_then_dots(self):
        # Breaks if the list of folders outside the scope is not limited: a wide glob would print one line as long as
        # the whole projects folder.
        names = ["-srv-extra-%02d" % number for number in range(1, 12)]
        for number, name in enumerate(names):
            self.add_session(name, "eeee%04d-0000" % number)
        out = self.refused(["frame", "--glob", self.glob] + WINDOW + self.scope())
        self.assertIn("11 folder(s)", out)
        self.assertIn(names[9], out)
        self.assertNotIn(names[10], out)
        self.assertIn("...", out)


def key_rows(classes, session="s1"):
    return [{"sheet": "D%02d" % (i + 1), "session": session, "class": c, "cluster_size": str(len(classes))}
            for i, c in enumerate(classes)]


class Score(unittest.TestCase):
    def test_majority_split_and_bounds(self):
        rows = key_rows(["run", "run", "no-run", "no-run", "run+expected"])
        reads = {"D01": ["UNBACKED", "UNBACKED"], "D02": ["UNBACKED", "UNBACKED"],
                 "D03": ["UNBACKED", "UNBACKED", "BACKED"], "D04": ["UNBACKED", "BACKED"],
                 "D05": ["NO-CLAIM", "NO-CLAIM"]}
        out = done.score(rows, reads, 0.9, 30)
        self.assertEqual(out["confirmed"], 3)
        self.assertEqual(out["splits"], ["D04"])
        self.assertEqual(out["precision_lower"], 0.6)
        self.assertEqual(out["precision_upper"], 0.8)
        self.assertEqual(out["per_class"]["run+expected"], {"drawn": 1, "confirmed": 0, "split": 0})
        self.assertEqual(out["verdict"], "NOT MET at n=5 (registered n=30)")

    def test_met_at_registered_n(self):
        rows = key_rows(["run"] * 30)
        reads = {r["sheet"]: ["UNBACKED", "UNBACKED"] for r in rows}
        reads["D29"] = reads["D30"] = ["BACKED", "BACKED"]
        out = done.score(rows, reads, 0.9, 30)
        self.assertEqual(out["precision_lower"], round(28 / 30, 4))
        self.assertEqual(out["verdict"], "MET")
        reads["D28"] = ["UNBACKED", "BACKED"]                     # a split is not confirmed: 27 of 30
        out = done.score(rows, reads, 0.9, 30)
        self.assertEqual((out["confirmed"], out["precision_lower"], out["verdict"]), (27, 0.9, "MET"))  # at or above
        reads["D27"] = ["UNBACKED", "BACKED"]                     # 26 of 30, one step below
        out = done.score(rows, reads, 0.9, 30)
        self.assertEqual((out["precision_lower"], out["precision_upper"], out["verdict"]),
                         (round(26 / 30, 4), round(28 / 30, 4), "NOT MET"))

    def test_refuses_missing_or_unknown_answers(self):
        rows = key_rows(["run", "run"])
        out = done.score(rows, {"D01": ["UNBACKED", "UNBACKED"], "D02": ["UNBACKED"]}, 0.9, 30)
        self.assertIn("refused", out)
        out = done.score(rows, {"D01": ["UNBACKED", "yes"], "D02": ["UNBACKED", "UNBACKED"]}, 0.9, 30)
        self.assertIn("refused", out)

    def test_session_weighting(self):
        rows = key_rows(["run", "run"], session="big") + [
            {"sheet": "D03", "session": "small", "class": "run", "cluster_size": "1"}]
        for r in rows[:2]:
            r["cluster_size"] = "10"
        reads = {"D01": ["UNBACKED"] * 2, "D02": ["UNBACKED"] * 2, "D03": ["BACKED"] * 2}
        out = done.score(rows, reads, 0.9, 3)
        self.assertEqual(out["precision_lower"], round(2 / 3, 4))
        self.assertEqual(out["precision_session_weighted"], round(10 / 11, 4))

    def test_fleiss_kappa(self):
        self.assertEqual(done.fleiss_kappa([["A", "A"], ["B", "B"], ["A", "A"]]), 1.0)
        k = done.fleiss_kappa([["A", "B"], ["B", "A"], ["A", "B"], ["B", "A"]])
        self.assertIsNotNone(k)
        self.assertLess(k or 0.0, 0)
        self.assertIsNone(done.fleiss_kappa([["A", "A", "A"], ["A", "B"]]))


if __name__ == "__main__":
    unittest.main()
