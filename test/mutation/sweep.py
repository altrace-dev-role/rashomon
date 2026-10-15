#!/usr/bin/env python3
"""Show that every acceptance item fails when its implementation is broken.

An item that cannot be made to fail by breaking the implementation is not a
test; it is a line that happens to be green. This script applies one deliberate
break per item, runs the tests that item names, and reports whether they went
red. Every file is restored afterwards, whatever happens.

The breaks live under test/mutation/mutants/, one file per change, each calling
m(name, file, old, new, test) and, where a break needs an import edit as well,
assigning to IMPORTS[name] in the same file. The files load in sorted order,
each in a namespace of its own, so no file can lean on a constant another one
defines or overwrite another one's import edit. A change adds a file; it never
appends to a shared list, which is what ended the merge conflicts the one big
list used to produce.

Run from the module root:

    python3 test/mutation/sweep.py                 the sweep
    python3 test/mutation/sweep.py --list          every mutant name, in load order
    python3 test/mutation/sweep.py --check         the static checks, no test runs
    python3 test/mutation/sweep.py --files 'mutants/DEL-*'   these files only

The sweep refuses to start when --check would fail. A break whose anchor occurs
twice mutates whichever site comes first, and a verdict on a break nobody wrote
is not evidence.
"""
import argparse
import glob
import os
import pathlib
import re
import signal
import subprocess
import sys

HERE = pathlib.Path(__file__).resolve().parent
MUTANT_GLOB = "mutants/*.py"
LEGACY_LIST = HERE / "legacy-duplicate-anchors.txt"
SHARED_TESTS = HERE / "shared-tests.txt"

# The requirement prefixes of the 1.2.0 specification. A mutant file named
# <PREFIX>-<n>_<slug>.py claims that row, and every test it names must pin the
# same row -- a suffix letter, TestREC56b_, pins its parent -- unless
# shared-tests.txt declares the pair. Files named any other way, the H-items
# and the groups that predate the rows, claim nothing and are not checked.
ROW_PREFIXES = ("REC", "VER", "FUN", "LRN", "STR", "MSR", "BRD", "TRU", "RES", "LUI", "DEL")
ROW_FILE = re.compile(r"^(" + "|".join(ROW_PREFIXES) + r")-(\d+)_")
ROW_TEST = re.compile(r"Test(" + "|".join(ROW_PREFIXES) + r")(\d+)[a-z]?_")
ROW_NAME = re.compile(r"^(" + "|".join(ROW_PREFIXES) + r")-(\d+)$")
# Finds the start of every Test<PREFIX> token, matched or not. check() anchors
# ROW_TEST at each start it finds here, so a token that only LOOKS like a row
# test -- a regex metacharacter, a missing trailing underscore, a literal
# anchor -- is reported instead of silently matching nothing.
ROW_TOKEN = re.compile(r"Test(" + "|".join(ROW_PREFIXES) + r")")

# A top-level Go test function, for checking that a mutant's judging test
# names something real -- a typo only showed up as NOT DETECTED in the next
# manual sweep, and push/pull_request no longer run one.
GO_TEST_FUNC = re.compile(r"(?m)^func (Test\w+)\(")

M = []  # (name, file, old, new, test regex)
IMPORTS = {}  # name -> (file, import block as it is, import block with the extra line)
ORIGIN = {}  # name -> the mutant file that declared it


def rel(path):
    try:
        return str(pathlib.Path(path).relative_to(HERE))
    except ValueError:
        return str(path)


def _resolve(pattern):
    """A relative --files pattern first against the current directory, where a
    tab-completed path is written from, then against this directory, which is
    where it is documented as relative to.
    """
    if os.path.isabs(pattern):
        return glob.glob(pattern)
    hits = glob.glob(pattern)
    return hits if hits else glob.glob(str(HERE / pattern))


def load(patterns):
    """Exec every mutant file the patterns name, in sorted order.

    Returns the problems found: a name declared twice, a file that declares
    nothing, a file that does not run. A relative pattern is tried against the
    current directory first, then against this directory.
    """
    files = set()
    for p in patterns:
        files.update(_resolve(p))
    problems = []
    if not files:
        problems.append("no mutant file matches " + ", ".join(patterns))
    for path in sorted(files):
        before = len(M)

        def m(name, f, old, new, test, _path=path):
            if name in ORIGIN:
                problems.append(f"{rel(_path)}: {name!r} is already declared in {rel(ORIGIN[name])}")
                return
            ORIGIN[name] = _path
            M.append((name, f, old, new, test))

        declared = {}  # this file's import edits, merged below
        ns = {"m": m, "IMPORTS": declared}
        try:
            exec(compile(pathlib.Path(path).read_text(), path, "exec"), ns)
        except Exception as e:  # a mutant file that does not run is a defect, not a crash
            problems.append(f"{rel(path)}: {type(e).__name__}: {e}")
        if len(M) == before:
            problems.append(f"{rel(path)}: declares no mutant")
        # A file that writes `IMPORTS = {...}` rebinds the name in its own
        # namespace instead of updating the dict passed in, so `declared`
        # never sees the edit -- silently, since nothing else notices. Reading
        # the namespace back and comparing identity catches it.
        if ns.get("IMPORTS") is not declared:
            problems.append(f"{rel(path)}: IMPORTS was reassigned instead of updated with IMPORTS[name] = (...)")
            continue
        # An import edit belongs to the file that declares its mutant. Merging
        # any other would let one file overwrite another's edit in silence.
        for name, entry in declared.items():
            if not (isinstance(entry, tuple) and len(entry) == 3 and all(isinstance(x, str) for x in entry)):
                problems.append(f"{rel(path)}: IMPORTS[{name!r}] is {entry!r}, not a 3-tuple of strings")
                continue
            if ORIGIN.get(name) != path:
                where = f"declared in {rel(ORIGIN[name])}" if name in ORIGIN else "not declared"
                problems.append(f"{rel(path)}: IMPORTS[{name!r}]: its mutant is {where}, not in this file")
            else:
                IMPORTS[name] = entry
    return problems


def read_list(path):
    """The entries of a list file: one per line, blank and # lines ignored."""
    if not path.exists():
        return []
    return [ln.strip() for ln in path.read_text().splitlines() if ln.strip() and not ln.lstrip().startswith("#")]


def go_test_names():
    """Every top-level func Test name under the current directory."""
    names = set()
    for gofile in pathlib.Path(".").rglob("*_test.go"):
        try:
            names.update(GO_TEST_FUNC.findall(gofile.read_text()))
        except OSError:
            pass
    return names


def check(complete):
    """The static checks. Returns the problems found.

    Every anchor occurs exactly once in its target, so the break lands where
    its author looked; a legacy-listed anchor may occur more than once, and the
    list may only shrink, so an entry whose anchor has become unique is itself
    a problem. Every import edit's anchor occurs exactly once too. A mutant in
    a row's file names only that row's tests, or a declared shared pair.
    `complete` says the whole set was loaded, which is when a legacy entry that
    names no mutant can be reported.
    """
    problems = []
    legacy = set(read_list(LEGACY_LIST))
    shared = set()
    for ln in read_list(SHARED_TESTS):
        parts = ln.split()
        if len(parts) != 2 or not ROW_NAME.match(parts[0]) or not ROW_TEST.fullmatch(parts[1]):
            problems.append(f"{SHARED_TESTS.name}: expected '<PREFIX>-<n> Test<PREFIX><n>[a-z]?_', got {ln!r}")
            continue
        shared.add((parts[0], parts[1]))
    texts = {}

    def text(f):
        if f not in texts:
            try:
                texts[f] = pathlib.Path(f).read_text()
            except OSError:
                texts[f] = None
        return texts[f]

    go_tests = go_test_names() if complete else None
    names = set()
    for name, f, old, new, test in M:
        names.add(name)
        top = test.split("/", 1)[0]
        try:
            rx = re.compile(top)
        except re.error as e:
            problems.append(f"{name!r}: test regex {test!r} does not compile: {e}")
        else:
            if go_tests is not None and not any(rx.search(n) for n in go_tests):
                problems.append(f"{name!r}: test regex {test!r} matches no func Test under the current directory")
        s = text(f)
        if s is None:
            problems.append(f"{name!r}: target {f} cannot be read")
            continue
        count = s.count(old)
        if name in legacy:
            if count == 1:
                problems.append(f"{name!r}: anchor now occurs exactly once; remove it from {LEGACY_LIST.name}")
            elif count == 0:
                problems.append(f"{name!r}: anchor missing from {f}")
        elif count != 1:
            problems.append(f"{name!r}: anchor occurs {count} times in {f}, not exactly once")
        row = ROW_FILE.match(os.path.basename(ORIGIN[name]))
        if row:
            claimed = f"{row.group(1)}-{row.group(2)}"
            for tok in ROW_TOKEN.finditer(test):
                t = ROW_TEST.match(test, tok.start())
                if t is None:
                    problems.append(f"{name!r} in {rel(ORIGIN[name])}: {test!r} has a Test{tok.group(1)} token "
                                    f"at {tok.start()} that is not Test<PREFIX><n>[a-z]?_")
                    continue
                pinned = f"{t.group(1)}-{t.group(2)}"
                # A suffixed test, TestREC56b_, pins its parent row -- so the
                # shared-pair lookup is keyed on the parent's own name,
                # TestREC56_, not the literal (possibly suffixed) token.
                parent_test = f"Test{t.group(1)}{t.group(2)}_"
                if pinned != claimed and (claimed, parent_test) not in shared:
                    problems.append(f"{name!r} in {rel(ORIGIN[name])}: names {t.group(0)}, which pins {pinned}, "
                                    f"not {claimed}; declare the pair in {SHARED_TESTS.name} if it is shared on purpose")
    if complete:
        for name in sorted(legacy - names):
            problems.append(f"{LEGACY_LIST.name}: {name!r} is not a declared mutant")
    mutation_of = {name: (f, old, new) for name, f, old, new, _test in M}
    for name, (f, io_, in_) in IMPORTS.items():
        s = text(f)
        if s is None:
            problems.append(f"IMPORTS[{name!r}]: target {f} cannot be read")
            continue
        # The sweep applies the import edit AFTER the main mutation, to the
        # same file when the two target the same one, so this must count the
        # anchor in the bytes the sweep actually sees, not the pristine file.
        mf, old, new = mutation_of.get(name, (None, None, None))
        if mf == f:
            s = s.replace(old, new, 1)
        count = s.count(io_)
        if count != 1:
            problems.append(f"IMPORTS[{name!r}]: import anchor occurs {count} times in {f} after the mutation, not exactly once")
    return problems


def report(problems):
    for p in problems:
        print("check:", p, file=sys.stderr)
    return 1 if problems else 0


backups = {}
def backup(f):
    if f not in backups:
        backups[f] = pathlib.Path(f).read_bytes()
def restore():
    for f, b in backups.items():
        pathlib.Path(f).write_bytes(b)

# RESTORE ON SIGNAL, not only on the way out of the try block.
#
# This script rewrites TRACKED SOURCE FILES IN PLACE and puts them back from an
# in-memory copy. `finally` covers a normal exit and an exception; it does not
# run when the process is killed. The realistic case is not exotic: piping the
# sweep into `head` or `grep` that exits early delivers SIGPIPE, and Python's
# default disposition terminates the process. What is left behind is a source
# file that has been broken ON PURPOSE, in a tree somebody is about to commit
# from -- and `git status` showing it is how a peer found one mid-run and had
# to ask whether it was a real edit.
#
# Each handler restores and then re-raises with the default disposition, so the
# exit status still says the process was signalled. Swallowing the signal would
# trade one lie for another.
def _restore_and_reraise(signum, _frame):
    restore()
    signal.signal(signum, signal.SIG_DFL)
    os.kill(os.getpid(), signum)

# SIGKILL CANNOT BE CAUGHT, and neither can a hard OOM kill. The backup is in
# memory, so it dies with the process and no handler can help. That residue is
# real and this script cannot prevent it -- what it can do is refuse to start
# on top of it, below, rather than mutating an already-mutated file and
# restoring it to the wrong bytes.
def _dirty_targets():
    """Files this run would mutate that git already reports as modified."""
    targets = sorted({f for _, f, _, _, _ in M} | {f for f, _, _ in IMPORTS.values()})
    r = subprocess.run(["git", "status", "--porcelain", "--"] + targets,
                       capture_output=True, text=True)
    if r.returncode != 0:
        return []  # not a git checkout, or git unavailable: not this script's problem
    return [ln[3:] for ln in r.stdout.splitlines() if ln.strip()]


def run_sweep(strict=False):
    for _sig in (signal.SIGINT, signal.SIGTERM, signal.SIGHUP, signal.SIGPIPE):
        signal.signal(_sig, _restore_and_reraise)

    _dirty = _dirty_targets()
    if _dirty:
        # A WARNING, not a refusal. The first version of this refused to start, and
        # it was wrong: it conflated two states that need opposite treatment.
        #
        # A developer's own uncommitted work in a mutated file is SAFE. The backup
        # is taken at startup, so their version is what gets restored -- mutate,
        # test, put it back exactly as it was. Refusing there blocks the normal way
        # of working on this repository, which is how this was found: it fired on
        # the very next commit's work-in-progress.
        #
        # Residue from a killed run is the dangerous state, and the GREEN BASELINE
        # CHECK below already catches it: a file left holding a deliberate break
        # makes the suite red, and the sweep stops there and says so. That check is
        # the guard; this is a note, because a developer who does not know why a
        # tracked file is modified should be told which ones and how to undo it.
        print("NOTE: files this sweep mutates are already modified:\n")
        for f in _dirty:
            print("   ", f)
        print("\nIf that is your own work in progress, this is fine -- the sweep backs up what\n"
              "is there now and restores exactly that. If it is residue from a run killed with\n"
              "SIGKILL (which no signal handler can catch), undo it first:\n"
              "    git checkout -- " + " ".join(_dirty) + "\n"
              "Residue would also have failed the green-baseline check below, which is the\n"
              "real guard.\n")

    # GREEN BASELINE, before the first mutation.
    #
    # The sweep decides "the test went red" from a non-zero exit. With a
    # pre-existing failure anywhere the filter happens to match, every mutation
    # reports as detected and the sweep certifies a suite it never exercised --
    # the tool whose job is guarding premises, not guarding its own. Found by a
    # review that noticed the tree was red while the sweep would have passed.
    print("baseline: the suite must be green before anything is mutated")
    _baseline = subprocess.run(["go", "test", "./...", "-count=1"], capture_output=True, text=True)
    if _baseline.returncode != 0:
        print("BASELINE NOT GREEN. Every mutation below would report as detected, because a\n"
              "mutation is judged by a non-zero exit and the suite already gives one. Fix the\n"
              "failure first; the sweep proves nothing until then.\n")
        print(_baseline.stdout[-4000:])
        print(_baseline.stderr[-2000:])
        return 1
    print("baseline: green\n")

    undetected = []
    skipped = []
    try:
        for name, f, old, new, test in M:
            backup(f)
            s = pathlib.Path(f).read_text()
            if old not in s:
                print(f"  ANCHOR MISSING <- {name}"); undetected.append(name); continue
            s = s.replace(old, new, 1)
            pathlib.Path(f).write_text(s)
            if name in IMPORTS:
                fi, io_, in_ = IMPORTS[name]; backup(fi)
                t = pathlib.Path(fi).read_text()
                if io_ not in t:
                    # Not an assertion: one missing import anchor must not end
                    # the sweep for every mutant after it. Recorded the same
                    # way a missing anchor on the main mutation is.
                    print(f"  IMPORT ANCHOR MISSING <- {name}"); undetected.append(name); restore(); continue
                pathlib.Path(fi).write_text(t.replace(io_, in_, 1))
            r = subprocess.run(["go", "test", "./...", "-run", test, "-count=1", "-v"], capture_output=True, text=True)
            judged = r.stdout + r.stderr
            # "--- SKIP" ONLY, never "no tests to run". The suite is invoked as
            # `go test ./... -run <regex>`, so every package without a matching
            # test prints "no tests to run" -- which made any mutation whose tests
            # passed look unjudged rather than UNDETECTED. It masked a real one:
            # a count mutation that no test asserted against was reported as "not
            # judged here" instead of as the gap it was. A check that cannot tell
            # "nothing ran" from "nothing matched in this package" is worse than no
            # check, because it converts findings into reassurance.
            if r.returncode == 0 and "--- SKIP" in judged:
                # The judging test did not RUN, so this mutation was not judged.
                # Reporting it as "the test cannot be made to fail" would name a
                # spec defect that may not exist: the socket half of H-17 needs
                # strace, which ubuntu-latest installs in CI and macOS does not
                # have, so the same mutation is judged there and unjudgeable here.
                # A verdict that cannot tell "we checked and it passed" from "we
                # could not check" is the error this whole suite exists to prevent.
                print(f"  NOT JUDGED     <- {name}   (the judging test skipped here)")
                skipped.append(name)
            elif r.returncode == 0:
                print(f"  NOT DETECTED   <- {name}   (spec defect: the test cannot be made to fail)"); undetected.append(name)
            elif "build failed" in r.stdout + r.stderr or "cannot" in r.stderr and "FAIL" not in r.stdout:
                print(f"  BUILD BROKEN   <- {name}\n{r.stdout}{r.stderr}"); undetected.append(name)
            else:
                print(f"  went red       <- {name}")
            restore()
    finally:
        restore()

    print()
    print("undetected:", undetected if undetected else "none")
    if skipped:
        print("not judged here:", skipped)
        print("  These were not checked because the judging test skipped in this")
        print("  environment. They are not passes. Run the sweep where those tests")
        print("  run -- CI installs strace on ubuntu-latest for exactly this reason.")
        if strict:
            print("  --strict is set: treating them as failures.")
    # Without --strict, a NOT JUDGED mutant exits 0: that is correct for a
    # laptop that cannot run the socket half of H-17, where skipping is
    # expected. In CI, where the tooling that lets them run is installed on
    # purpose, the same skip means that tooling broke, and --strict is what
    # turns a green sweep back into the red one that silence would hide.
    return 1 if undetected or (strict and skipped) else 0


def main(argv):
    ap = argparse.ArgumentParser(description="the mutation sweep; run from the module root")
    ap.add_argument("--list", action="store_true", help="print every mutant name in load order and exit")
    ap.add_argument("--check", action="store_true", help="run the static checks and exit")
    ap.add_argument("--files", action="append", default=[], metavar="GLOB",
                    help="load only the mutant files matching GLOB, tried against the current "
                         "directory first and then against test/mutation, unless absolute; repeatable")
    ap.add_argument("--strict", action="store_true",
                    help="exit non-zero if any mutant was not judged here, not only if undetected")
    args = ap.parse_args(argv)
    problems = load(args.files or [MUTANT_GLOB])
    if args.list:
        for name, *_ in M:
            print(name)
        return report(problems)
    problems += check(complete=not args.files)
    if args.check:
        if not problems:
            print(f"check: {len(M)} mutants in {len(set(ORIGIN.values()))} files; every anchor occurs exactly once")
        return report(problems)
    if problems:
        print("refusing to sweep: the static checks fail", file=sys.stderr)
        return report(problems)
    return run_sweep(strict=args.strict)


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
