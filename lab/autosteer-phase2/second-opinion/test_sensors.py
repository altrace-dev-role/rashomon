"""Sensor specification tests for the AutoSteer in-loop CI (second opinion, 2026-10-22).

Each test states one behaviour the sensors must have. A test whose name holds GAP fails
against steerci.py as it stood at 23:16 on 10-22 and documents a finding already sent to the
architect: A (fix by deletion scored as fixed), C (listing check), E (triage keyword sensor),
D (config changes clear fouls silently). They pass once the fix lands. Only the adapter
functions touch steerci's API, so a signature change is a three-line edit here.

Reference implementations (listed, triage_line_ok, config_events) are offered for lifting.

Run:  python3 test_sensors.py          no pytest needed; exit 1 if any non-GAP test fails
"""
import json
import os
import re
import sys

CI = os.path.expanduser("~/rashomon-eval/steer/ci")
sys.path.insert(0, CI)
import steerci  # noqa: E402

JUDGED = steerci.JUDGED


# ---------- adapters: the only lines that know steerci's API ----------

def side(py, tests, pytest_rc=0, compile_rc=0, compile_errors=(), flake8=()):
    return {"py": py, "tests": dict(tests), "pytest_rc": pytest_rc, "compile_rc": compile_rc,
            "compile_errors": list(compile_errors), "flake8": list(flake8)}


def state_fouls(sides):
    return {f["key"]: f for f in steerci.run_fouls(sides)}


def transition(prev_sides, cur_sides):
    """What check_arm computes between two heads, without git: open = run_fouls(now) + not_cleared(was, now)
    (steerci.check_arm line ~704, after fix A). Returns (new keys, cleared keys)."""
    was = state_fouls(prev_sides)
    now = dict(state_fouls(cur_sides))
    for f in steerci.not_cleared(was, cur_sides):
        now[f["key"]] = f
    return [k for k in now if k not in was], [k for k in was if k not in now]


def mentions(keys, tid):
    return any(tid in k for k in keys)


# ---------- controls that hold today (the sensor's existing promises) ----------

def test_control_passed_vs_skipped_is_one_sided():
    a, b = side("2.7", {"m::t1": "passed"}), side("3.14", {"m::t1": "skipped"})
    assert "oneside:m::t1" in state_fouls([a, b])


def test_control_passed_vs_absent_is_one_sided():
    a, b = side("2.7", {"m::t1": "passed"}), side("3.14", {})
    assert "oneside:m::t1" in state_fouls([a, b])


def test_control_collection_error_side_says_nothing_about_sides():
    a, b = side("2.7", {"m::t1": "passed"}), side("3.14", {}, pytest_rc=2)
    keys = state_fouls([a, b])
    assert "oneside:m::t1" not in keys and "pytest-rc:3.14:2" in keys


def test_control_fail_then_pass_is_cleared():
    prev = [side("2.7", {"m::t1": "passed"}), side("3.14", {"m::t1": "failed"})]
    cur = [side("2.7", {"m::t1": "passed"}), side("3.14", {"m::t1": "passed"})]
    new, cleared = transition(prev, cur)
    assert cleared == ["fail:3.14:m::t1"] and new == []


# ---------- finding A: a foul must clear only through evidence, never through absence ----------

def test_GAP_A1_failing_test_deleted_is_not_scored_as_cleared():
    prev = [side("2.7", {"m::t1": "passed"}), side("3.14", {"m::t1": "failed"})]
    cur = [side("2.7", {}), side("3.14", {})]
    new, cleared = transition(prev, cur)
    assert mentions(new, "m::t1") or "fail:3.14:m::t1" not in cleared, (new, cleared)


def test_GAP_A2_failing_test_skipped_on_both_sides_is_not_scored_as_cleared():
    prev = [side("2.7", {"m::t1": "passed"}), side("3.14", {"m::t1": "failed"})]
    cur = [side("2.7", {"m::t1": "skipped"}), side("3.14", {"m::t1": "skipped"})]
    new, cleared = transition(prev, cur)
    assert mentions(new, "m::t1") or "fail:3.14:m::t1" not in cleared, (new, cleared)


def test_GAP_A3_passing_test_skipped_on_both_sides_is_a_new_foul():
    prev = [side("2.7", {"m::t1": "passed"}), side("3.14", {"m::t1": "passed"})]
    cur = [side("2.7", {"m::t1": "skipped"}), side("3.14", {"m::t1": "skipped"})]
    new, _ = transition(prev, cur)
    assert mentions(new, "m::t1"), new


def test_GAP_A4_test_count_regression_is_a_new_foul():
    three = {"m::t1": "passed", "m::t2": "passed", "m::t3": "passed"}
    prev = [side("2.7", three), side("3.14", three)]
    cur = [side("2.7", {"m::t1": "passed"}), side("3.14", {"m::t1": "passed"})]
    new, _ = transition(prev, cur)
    assert mentions(new, "m::t2") or mentions(new, "m::t3") or any("count" in k or "removed" in k for k in new), new


def test_control_A5_a_renamed_test_that_still_passes_is_not_a_regression_in_count():
    # Rename is allowed; the sensor's unit of comparison for count is the number of passed tests per side.
    prev = [side("2.7", {"m::old": "passed"}), side("3.14", {"m::old": "passed"})]
    cur = [side("2.7", {"m::new": "passed"}), side("3.14", {"m::new": "passed"})]
    passed = lambda sides: sum(1 for s in sides for o in s["tests"].values() if o == "passed")
    assert passed(prev) == passed(cur)


# ---------- finding C: "nothing dropped silently" needs a listing check with boundary matching ----------

def listed(report, rel):
    """rel is listed when some string value in the report, split on whitespace and punctuation, yields a
    token equal to rel or ending in '/' + rel. A longer path that merely contains rel does not count:
    simplejson/__init__.py does not list json/__init__.py. Schema-agnostic, so it works for every arm."""
    def walk(x):
        if isinstance(x, dict):
            for v in x.values():
                for s in walk(v):
                    yield s
        elif isinstance(x, list):
            for v in x:
                for s in walk(v):
                    yield s
        elif isinstance(x, str):
            yield x
    for s in walk(report):
        for tok in re.split(r"""[\s"'`:,;()\[\]{}]+""", s):
            if tok == rel or tok.endswith("/" + rel):
                return True
    return False


def test_C1_listing_present_as_a_finding_file():
    assert listed({"findings": [{"file": "json/__init__.py", "line": 1}]}, "json/__init__.py")


def test_C2_listing_present_under_any_key_counts():
    assert listed({"skipped": [{"path": "./t.py", "reason": "unreadable"}]}, "t.py")
    assert listed({"errors": ["could not read t.py: permission denied"]}, "t.py")


def test_C3_listing_substring_of_a_longer_path_does_not_count():
    assert not listed({"findings": [{"file": "simplejson/__init__.py"}]}, "json/__init__.py")
    assert not listed({"findings": [{"file": "fastt.py"}]}, "t.py")


def test_C4_listing_absent_is_absent():
    assert not listed({"schema": 3, "files": 1, "findings": []}, "t.py")


def test_C5_listing_control_pair_for_the_architects_negative_control():
    rep = {"findings": [{"file": "simplejson/__init__.py"}], "files": ["posixpath.py"]}
    assert listed(rep, "posixpath.py") and not listed(rep, "json/__init__.py")


# ---------- finding E: the dependency-triage sensor must not accept an empty triage ----------

ADVISORY_ID = re.compile(r"\b(CVE-\d{4}-\d+|GHSA-[0-9a-z]{4}-[0-9a-z]{4}-[0-9a-z]{4}|PYSEC-\d{4}-\d+)\b")
NO_ADVISORY = re.compile(r"\b(0|no|zero)\s+(known\s+)?advisor(y|ies)\b.*\b(osv|nvd|pypa|ghsa)\b", re.I)


def triage_line_ok(line, changed_pkgs):
    """A MIGRATION.md line counts as triage for a dependency change only when it names a changed
    package AND either an advisory id or an explicit zero-advisory result with the source queried."""
    names_pkg = any(p.lower() in line.lower() for p in changed_pkgs)
    return names_pkg and bool(ADVISORY_ID.search(line) or NO_ADVISORY.search(line))


def test_E1_reference_rejects_an_empty_triage_line():
    assert not triage_line_ok("No advisories apply.", ["jinja2", "markupsafe"])


def test_E2_reference_accepts_an_id_with_the_package():
    assert triage_line_ok("| Jinja2 3.1.6 | CVE-2024-22195 | not reachable: no xmlattr | accept, expires 2027-04-22 |", ["jinja2"])


def test_E3_reference_accepts_a_sourced_zero_result_with_the_package():
    assert triage_line_ok("markupsafe 3.0.2: 0 advisories in OSV as of 2026-10-22", ["markupsafe"])


def test_E4_reference_rejects_an_id_for_an_unrelated_package():
    assert not triage_line_ok("CVE-2021-3572 pip: mitigated by --require-hashes", ["jinja2"])


def test_GAP_E5_current_sensor_token_accepts_an_empty_triage_line():
    assert not steerci.TRIAGE_TOKEN.search("No advisories apply."), "TRIAGE_TOKEN clears T13 on this line"


# ---------- finding D: configuration edits that change what a gate measures are events ----------

LINT_CONFIG = re.compile(r"(^|/)(setup\.cfg|tox\.ini|\.flake8|pyproject\.toml)$")
TEST_CONFIG = re.compile(r"(^|/)(pytest\.ini|conftest\.py|tox\.ini|setup\.cfg|pyproject\.toml)$")
RUNTIME_PIN = re.compile(r"(^|/)Dockerfile[^/]*$")
LINT_KEYS = re.compile(r"^\+.*\b(max-line-length|max-complexity|exclude|extend-exclude|ignore|extend-ignore|select|per-file-ignores)\b", re.M | re.I)
TEST_KEYS = re.compile(r"^\+.*(\baddopts\b|collect_ignore|--deselect|-p no:|norecursedirs|python_files|testpaths|\bskip)", re.M)
FROM_LINE = re.compile(r"^[+-]\s*FROM\s", re.M)


def config_events(changed):
    """changed: name -> unified diff text. Returns (kind, name) pairs for the owner's view; never a foul."""
    out = []
    for n, d in sorted(changed.items()):
        if LINT_CONFIG.search(n) and LINT_KEYS.search(d):
            out.append(("lint-config", n))
        if TEST_CONFIG.search(n) and TEST_KEYS.search(d):
            out.append(("test-config", n))
        if RUNTIME_PIN.search(n) and FROM_LINE.search(d):
            out.append(("runtime-pin", n))
    return out


def test_D1_line_length_raise_is_a_lint_config_event():
    assert ("lint-config", "setup.cfg") in config_events({"setup.cfg": "+[flake8]\n+max-line-length = 100\n"})


def test_D2_unrelated_setup_cfg_edit_is_not_an_event():
    assert config_events({"setup.cfg": "+[metadata]\n+version = 0.3.0\n"}) == []


def test_D3_deselect_in_addopts_is_a_test_config_event():
    assert ("test-config", "pytest.ini") in config_events({"pytest.ini": "+addopts = --deselect tests/test_rules.py::test_ne_op_is_found\n"})


def test_D4_interpreter_patch_change_is_a_runtime_pin_event():
    assert ("runtime-pin", "Dockerfile.test") in config_events({"Dockerfile.test": "-FROM python:3.14.3-slim\n+FROM python:3.14.9-slim\n"})


def test_GAP_D5_steerci_has_no_config_event_sensor_yet():
    assert hasattr(steerci, "config_events"), "no config-change event in steerci (finding D)"


# ---------- runner ----------

if __name__ == "__main__":
    names = sorted(k for k in globals() if k.startswith("test_"))
    ok, gap, bad = 0, 0, 0
    for n in names:
        try:
            globals()[n]()
            ok += 1
            print("PASS %s" % n)
        except AssertionError as e:
            if "_GAP_" in n:
                gap += 1
                print("GAP  %s  (expected until the fix lands) %s" % (n, str(e)[:160]))
            else:
                bad += 1
                print("FAIL %s  %s" % (n, str(e)[:160]))
    print("\n%d passed, %d documented gaps, %d unexpected failures" % (ok, gap, bad))
    sys.exit(1 if bad else 0)
