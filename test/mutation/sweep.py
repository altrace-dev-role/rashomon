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
    python3 test/mutation/sweep.py --shard 2/3     the second of three disjoint parts of the sweep
    python3 test/mutation/sweep.py --platform-only darwin
                                                    only the mutants that can be judged on darwin alone
    python3 test/mutation/sweep.py --files 'mutants/DEL-*'   these files only
    GOCACHE=<own dir> python3 test/mutation/sweep.py --trim-cache-each
                                                    each mutant's build trimmed after it

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
import stat
import subprocess
import sys

HERE = pathlib.Path(__file__).resolve().parent
MUTANT_GLOB = "mutants/*.py"
SHARED_TESTS = HERE / "shared-tests.txt"
TRIM_MARKER = "sweep-trim-marker"

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

# The v1.1.0 rubric fix list is a named group, constrained like a row by its
# entry id (DEL-23). A file named RUBRIC_<area>-<n>_<slug>.py claims that
# entry: every TestRubric<AREA><n>_ test its mutants name must be the same
# entry's -- a suffix letter pins its parent, as in a row -- and each mutant
# must name at least one. Other test names claim nothing. A RUBRIC_ file
# whose name carries no entry id is refused, not read as claiming nothing.
RUBRIC_PREFIX = "RUBRIC_"
RUBRIC_FILE = re.compile(r"^RUBRIC_([a-z]+)-(\d+)_")
RUBRIC_TEST = re.compile(r"TestRubric([A-Z]+)(\d+)[a-z]?_")

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


# GOOS names go/build recognizes, plus the "unix" meta-tag a //go:build line
# may use in their place. Not every GOOS Go supports is here, only the ones a
# build constraint in this tree is likely to name; a name outside this set
# is treated as a non-GOOS tag (see _goos_excludes) rather than guessed at.
_KNOWN_GOOS = {
    "aix", "android", "darwin", "dragonfly", "freebsd", "hurd", "illumos",
    "ios", "js", "linux", "netbsd", "openbsd", "plan9", "solaris", "wasip1",
    "windows",
}
_UNIX_GOOS = {
    "aix", "android", "darwin", "dragonfly", "freebsd", "hurd", "illumos",
    "ios", "linux", "netbsd", "openbsd", "solaris",
}
_GOOS_FROM_PLATFORM = {"darwin": "darwin", "linux": "linux", "win32": "windows", "cygwin": "windows"}


def host_goos():
    """The GOOS the sweep is judging mutants for: GOOS itself when the
    caller set it, sys.platform otherwise. The sweep's own CI jobs run `go
    test` for the host they run on, never a cross-compiled one, so this is
    always what the mutated binary would actually build for."""
    return os.environ.get("GOOS") or _GOOS_FROM_PLATFORM.get(sys.platform, sys.platform)


def _build_constraint_line(path):
    """The expression on path's //go:build line, or None if it has none. That
    line must come before the package clause, after only blank lines and
    other comments (go help buildconstraint), so reading a handful of lines
    is enough."""
    try:
        with open(path, encoding="utf-8") as fh:
            for _ in range(10):
                line = fh.readline()
                if not line:
                    break
                line = line.strip()
                if line.startswith("//go:build"):
                    return line[len("//go:build"):].strip()
                if line.startswith("package "):
                    break
    except OSError:
        pass
    return None


class _ConstraintError(Exception):
    """A term in a //go:build expression is not a GOOS name or 'unix': this
    function knows GOOS constraints and nothing else, so an expression it
    cannot fully resolve is not guessed at."""


def _eval_goos_constraint(expr, goos):
    """Evaluate a //go:build boolean expression (&&, ||, !, parens, bare
    identifiers) against goos. Raises _ConstraintError if any identifier is
    not a GOOS name or the 'unix' meta-tag -- a custom tag such as perf or
    rashomonfault -- since this function exists to catch a GOOS mismatch, not
    to model every build tag a mutant's target might carry."""
    tokens = re.findall(r"&&|\|\||[!()]|[A-Za-z0-9_]+", expr)
    pos = 0

    def peek():
        return tokens[pos] if pos < len(tokens) else None

    def take():
        nonlocal pos
        t = tokens[pos]
        pos += 1
        return t

    def parse_or():
        v = parse_and()
        while peek() == "||":
            take()
            v = v or parse_and()
        return v

    def parse_and():
        v = parse_unary()
        while peek() == "&&":
            take()
            v = v and parse_unary()
        return v

    def parse_unary():
        if peek() == "!":
            take()
            return not parse_unary()
        if peek() == "(":
            take()
            v = parse_or()
            if peek() != ")":
                raise _ConstraintError(f"unbalanced parens in {expr!r}")
            take()
            return v
        if peek() is None:
            raise _ConstraintError(f"expected a term in {expr!r}")
        t = take()
        if t == "unix":
            return goos in _UNIX_GOOS
        if t in _KNOWN_GOOS:
            return t == goos
        raise _ConstraintError(f"{t!r} in {expr!r} is not a GOOS name or 'unix'")

    v = parse_or()
    if pos != len(tokens):
        raise _ConstraintError(f"trailing tokens in {expr!r}")
    return v


def _goos_excludes(path, goos):
    """True if path declares a //go:build line that is false for goos, so a
    text mutation to it can never be observed: the file is not part of the
    build goos compiles, and no test run under goos can see a change to it.
    A file with no go:build line, or one whose expression this function
    cannot fully evaluate (a mix of GOOS/unix terms and something else), is
    never reported excluded -- that would turn an unrelated custom tag into a
    false NOT JUDGED and hide a mutant the sweep could otherwise detect."""
    expr = _build_constraint_line(path)
    if expr is None:
        return False
    try:
        return not _eval_goos_constraint(expr, goos)
    except _ConstraintError:
        return False


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

        def m(name, f, old, new, test, _path=path, platform=None):
            if name in ORIGIN:
                problems.append(f"{rel(_path)}: {name!r} is already declared in {rel(ORIGIN[name])}")
                return
            if platform is not None and platform not in _KNOWN_GOOS:
                problems.append(f"{rel(_path)}: {name!r} declares platform={platform!r}, not a known GOOS")
                return
            ORIGIN[name] = _path
            M.append((name, f, old, new, test, platform))

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


def run_go_test(argv):
    """Run go test and return its result with stdout and stderr as text.

    The output is decoded with undecodable bytes replaced: a test that prints
    the raw bytes it fed the code under test (invalid UTF-8 among them) must
    not stop the sweep mid-run, which would leave every later mutant
    unjudged (the finally clause still restores the tree).
    """
    r = subprocess.run(argv, capture_output=True)
    r.stdout = r.stdout.decode("utf-8", errors="replace")
    r.stderr = r.stderr.decode("utf-8", errors="replace")
    return r


def build_broken(r):
    """Report a go test run that could not build or set up a package.

    go test marks such a package on its own FAIL line with [build failed] or
    [setup failed]; a test's output never carries those markers. Searching
    the whole output for "build failed" read a failing test that printed the
    sentence it checks as a broken build, and a detected mutant as undetected.
    """
    return any(ln.startswith("FAIL") and ("[build failed]" in ln or "[setup failed]" in ln)
               for ln in (r.stdout + r.stderr).splitlines())


def test_failed(r):
    """Report positive evidence that a test ran and failed.

    go test writes "--- FAIL: Name" for a failing test and "FAIL<TAB>pkg<TAB>time"
    for a package whose tests ran and failed. A non-zero exit without either (a
    go.mod the tool could not parse, a pattern that matched no package) is not a
    red test: nothing ran, and the sweep must not count it as detection.
    """
    for ln in (r.stdout + r.stderr).splitlines():
        if ln.startswith("--- FAIL") or (ln.startswith("FAIL\t") and ln.count("\t") >= 2):
            return True
    return False


def own_cache():
    """Return the build cache this run owns and None, or None and why not.

    --trim-cache-each deletes from whatever cache go resolves (`go env
    GOCACHE`), so that has to be a cache this run owns: never the one go
    resolves without GOCACHE in the environment (a symlink resolved), which
    every other build on the machine shares. GOCACHE unset resolves to that
    one, so it is refused by the same comparison.
    """
    root = subprocess.run(["go", "env", "GOCACHE"], capture_output=True, text=True).stdout.strip()
    env = {k: v for k, v in os.environ.items() if k != "GOCACHE"}
    shared = subprocess.run(["go", "env", "GOCACHE"], capture_output=True, text=True, env=env).stdout.strip()
    if not root or root == "off":
        return None, f"--trim-cache-each needs a build cache; go env GOCACHE reads {root!r}"
    if shared and os.path.realpath(root) == os.path.realpath(shared):
        return None, f"--trim-cache-each would trim the shared build cache {shared}; set GOCACHE to a cache this run owns"
    return root, None


def trim_cache(root, marker):
    """Remove every regular file under root newer than marker; return the bytes before and after.

    The marker is touched after the baseline, so what goes is what the
    mutant runs since then built, and the baseline's objects stay for the
    next mutant's incremental build. go's own README, trim.txt and log.txt
    stay, and the marker is never newer than itself. go touches an entry it
    uses only when the entry is over an hour old (mtimeInterval in cmd/go's
    cache), so past the hour some baseline entries go too: a removed entry is
    a cache miss, which go rebuilds, so that costs time and never a verdict.
    """
    keep = {os.path.join(root, n) for n in ("README", "trim.txt", "log.txt")}
    since = os.stat(os.path.join(root, marker)).st_mtime_ns
    before = removed = 0
    for d, _, files in os.walk(root):
        for f in files:
            path = os.path.join(d, f)
            st = os.lstat(path)
            if not stat.S_ISREG(st.st_mode):
                continue
            before += st.st_size
            if path not in keep and st.st_mtime_ns > since:
                os.unlink(path)
                removed += st.st_size
    return before, before - removed


def read_list(path):
    """The entries of a list file: one per line, blank and # lines ignored."""
    if not path.exists():
        return []
    return [ln.strip() for ln in path.read_text().splitlines() if ln.strip() and not ln.lstrip().startswith("#")]


def outside_cwd(path):
    """True if path is absolute, or resolves outside the current directory.

    A mutant's target is otherwise unconstrained, and _dirty_targets() runs
    `git status -- <targets>`: git exits non-zero for a path outside the
    repository, which silently drops the already-modified note for every
    target in that run, not just the offending one.
    """
    if os.path.isabs(path):
        return True
    try:
        pathlib.Path(path).resolve().relative_to(pathlib.Path(".").resolve())
    except ValueError:
        return True
    return False


def go_test_names():
    """Every top-level func Test name under the current directory."""
    names = set()
    for gofile in pathlib.Path(".").rglob("*_test.go"):
        try:
            names.update(GO_TEST_FUNC.findall(gofile.read_text()))
        except OSError:
            pass
    return names


def rubric_problems(name, origin, test):
    """The DEL-23 problems of one mutant: none unless its file is a RUBRIC_ one."""
    base = os.path.basename(origin)
    if not base.startswith(RUBRIC_PREFIX):
        return []
    entry = RUBRIC_FILE.match(base)
    if not entry:
        return [f"{rel(origin)}: a rubric file is named RUBRIC_<area>-<n>_<slug>.py, after the entry it claims"]
    claimed = entry.group(1).upper() + entry.group(2)
    problems = []
    pinned = set()
    for t in RUBRIC_TEST.finditer(test):
        pinned.add(t.group(1) + t.group(2))
        if t.group(1) + t.group(2) != claimed:
            problems.append(f"{name!r} in {rel(origin)}: names {t.group(0)}, another rubric entry's test, "
                            f"not TestRubric{claimed}_")
    if claimed not in pinned:
        problems.append(f"{name!r} in {rel(origin)}: names no TestRubric{claimed}_ test of its own entry")
    return problems


def check(complete, shared_tests=SHARED_TESTS):
    """The static checks. Returns the problems found.

    Every anchor occurs exactly once in its target, so the break lands where
    its author looked. Every import edit's anchor occurs exactly once too. A
    mutant in a row's file names only that row's tests, or a declared shared
    pair, and a mutant in a rubric entry's file names its own entry's test and
    no other entry's (rubric_problems). `complete` says the whole set was
    loaded, which is when a test regex can be checked against every real func
    Test. `shared_tests` is the file declaring shared pairs; a test points it
    at a fixture instead of writing to the real one.
    """
    problems = []
    shared = set()
    for ln in read_list(shared_tests):
        parts = ln.split()
        if len(parts) != 2 or not ROW_NAME.match(parts[0]) or not ROW_TEST.fullmatch(parts[1]):
            problems.append(f"{shared_tests.name}: expected '<PREFIX>-<n> Test<PREFIX><n>[a-z]?_', got {ln!r}")
            continue
        named = ROW_TEST.fullmatch(parts[1])
        if parts[1] != f"Test{named.group(1)}{named.group(2)}_":
            problems.append(f"{shared_tests.name}: {ln!r} names a suffixed test, which pins its parent row; "
                            f"name the parent, Test{named.group(1)}{named.group(2)}_, or the pair never matches")
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
    for name, f, old, new, test, _platform in M:
        top = test.split("/", 1)[0]
        rx = None
        try:
            rx = re.compile(top)
        except re.error as e:
            problems.append(f"{name!r}: test regex {test!r} does not compile: {e}")
        else:
            if go_tests is not None and not any(rx.search(n) for n in go_tests):
                problems.append(f"{name!r}: test regex {test!r} matches no func Test under the current directory")
        if outside_cwd(f):
            problems.append(f"{name!r}: target {f} is outside the current directory")
            continue
        s = text(f)
        if s is None:
            problems.append(f"{name!r}: target {f} cannot be read")
            continue
        count = s.count(old)
        if count != 1:
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
                                    f"not {claimed}; declare the pair in {shared_tests.name} if it is shared on purpose")
            # The scan above finds a row token by its literal spelling, so a
            # regex that reaches the same test through a metacharacter wedged
            # between its letters -- TestRE[C]49_, Test(REC)49_ -- names no
            # literal "TestREC" and passes with no problem reported, even
            # though it pins REC-49 exactly as the spelled-out form would.
            # Checked only when the real test corpus is loaded (`complete`):
            # a regex can match a real test no literal scan of its own text
            # can name, and matching it against the corpus is the only way to
            # know which row it actually pins.
            if go_tests is not None and rx is not None:
                for real_name in go_tests:
                    rm = ROW_TEST.match(real_name)
                    if rm is None or not rx.search(real_name):
                        continue
                    real_row = f"{rm.group(1)}-{rm.group(2)}"
                    real_parent = f"Test{rm.group(1)}{rm.group(2)}_"
                    if real_row != claimed and (claimed, real_parent) not in shared:
                        problems.append(f"{name!r} in {rel(ORIGIN[name])}: {test!r} matches real test {real_name!r}, "
                                        f"which pins {real_row}, not {claimed}; declare the pair in "
                                        f"{shared_tests.name} if it is shared on purpose")
        problems += rubric_problems(name, ORIGIN[name], test)
    mutation_of = {name: (f, old, new) for name, f, old, new, _test, _platform in M}
    for name, (f, io_, in_) in IMPORTS.items():
        if outside_cwd(f):
            problems.append(f"IMPORTS[{name!r}]: target {f} is outside the current directory")
            continue
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
    targets = sorted({f for _, f, _, _, _, _ in M} | {f for f, _, _ in IMPORTS.values()})
    r = subprocess.run(["git", "status", "--porcelain", "--"] + targets,
                       capture_output=True, text=True)
    if r.returncode != 0:
        return []  # not a git checkout, or git unavailable: not this script's problem
    return [ln[3:] for ln in r.stdout.splitlines() if ln.strip()]


def run_sweep(strict=False, trim_root=None):
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
    _baseline = run_go_test(["go", "test", "./...", "-count=1"])
    if _baseline.returncode != 0:
        print("BASELINE NOT GREEN. Every mutation below would report as detected, because a\n"
              "mutation is judged by a non-zero exit and the suite already gives one. Fix the\n"
              "failure first; the sweep proves nothing until then.\n")
        # Every failing test and package first, then the tail: the tail alone
        # cut a package that failed early whenever a later failing package
        # printed more than 4000 bytes (SWEEP-BASELINE1, batch/1's shard 4).
        # A test killed by -timeout prints no "--- FAIL"; its name is only in
        # the tab-indented "running tests:" block under "panic: test timed out".
        _named, _running = [], False
        for ln in _baseline.stdout.splitlines():
            if _running and ln.startswith("\t"):
                _named.append(ln)
                continue
            _running = ln.startswith("panic: test timed out")
            if ln.startswith("panic:") or ln.lstrip().startswith("--- FAIL") or ln.startswith("FAIL\t"):
                _named.append(ln)
        if _named:
            print("failing on the unmutated tree:")
            for ln in _named:
                print("   ", ln)
            print("\nthe last 4000 bytes of go test's output:")
        print(_baseline.stdout[-4000:])
        print(_baseline.stderr[-2000:])
        return 1
    print("baseline: green\n")
    if trim_root:
        pathlib.Path(trim_root, TRIM_MARKER).touch()

    undetected = []
    skipped = []  # the judging test skipped here; --strict fails on these
    platform = []  # the target is not built for this GOOS, or the mutant
                    # declares a GOOS of its own that this run is not; never
                    # a failure, strict or not
    goos = host_goos()
    try:
        for name, f, old, new, test, mutant_goos in M:
            if _goos_excludes(f, goos):
                # Mutating f would change nothing a build for this GOOS
                # compiles, so neither backup nor the judging test runs: a
                # darwin-only target on a Linux runner, for instance.
                print(f"  NOT JUDGED (platform) <- {name}   ({rel(f)} is not built for GOOS={goos})")
                platform.append(name)
                continue
            if mutant_goos is not None and mutant_goos != goos:
                # The target builds everywhere, but the mutant's judging
                # test only means something on the GOOS it declares -- the
                # test skips elsewhere by design, not because this run's
                # tooling is missing something. Without this, the mutant
                # would land in `skipped` below, which --strict treats as a
                # broken environment; a test that is SUPPOSED to skip here is
                # not that, any more than a build-excluded target is.
                print(f"  NOT JUDGED (platform) <- {name}   (judged on GOOS={mutant_goos}, this run is GOOS={goos})")
                platform.append(name)
                continue
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
            r = run_go_test(["go", "test", "./...", "-run", test, "-count=1", "-v"])
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
            elif test_failed(r):
                # A red test on record is positive evidence the mutant was
                # caught, even when the same `go test ./...` run also failed
                # to build an unrelated package: that package's failure is
                # not this mutant's to answer for.
                print(f"  went red       <- {name}")
            elif build_broken(r):
                print(f"  BUILD BROKEN   <- {name}\n{r.stdout}{r.stderr}"); undetected.append(name)
            else:
                print(f"  NOT JUDGED     <- {name}   (go test exited {r.returncode} with no test failure on record)\n{r.stdout}{r.stderr}"); undetected.append(name)
            restore()
            if trim_root:
                # The disk peak is the baseline's cache plus one mutant's build.
                try:
                    before, after = trim_cache(trim_root, TRIM_MARKER)
                except OSError as e:
                    print(f"CACHE NOT TRIMMED: {e}; the next mutant would build on top of this one, so the sweep stops.")
                    return 1
                print(f"    cache {before / 2**20:.1f} MiB, trimmed to {after / 2**20:.1f} MiB")
    finally:
        restore()

    print()
    print("undetected:", undetected if undetected else "none")
    if skipped or platform:
        print("not judged here:", skipped + platform)
        print("  These were not checked: either the judging test skipped in this")
        print("  environment, or the mutant's target is not built under this GOOS at")
        print("  all. They are not passes. Run the sweep where that coverage runs --")
        print("  CI installs strace on ubuntu-latest for H-17's sockets, and runs a")
        print("  darwin-only target's own job on macos-latest.")
        if strict and skipped:
            print("  --strict is set: treating the skipped ones as failures.")
    # Without --strict, a NOT JUDGED mutant exits 0: that is correct for a
    # laptop that cannot run the socket half of H-17, where skipping is
    # expected. In CI, where the tooling that lets them run is installed on
    # purpose, the same skip means that tooling broke, and --strict is what
    # turns a green sweep back into the red one that silence would hide.
    # A target this GOOS does not build is never counted, strict or not: no
    # tooling installed here can make it build, so it is a fact about the
    # platform, not a broken environment (DEL-18).
    return 1 if undetected or (strict and skipped) else 0


def parse_shard(spec):
    """Read K/N, 1 <= K <= N, into (K - 1, N); raise ValueError when it is not."""
    k, sep, n = spec.partition("/")
    if not (sep and k.isdigit() and n.isdigit() and 1 <= int(k) <= int(n)):
        raise ValueError(f"--shard wants K/N with 1 <= K <= N, got {spec!r}")
    return int(k) - 1, int(n)


def keep_shard(index, count):
    """Keep every count-th mutant, from index, of M in load order.

    Round-robin rather than contiguous: files cluster by area, and an area
    whose tests take longer would otherwise make one part the slow one. The
    rule depends on load order alone, which is sorted by file and then by
    declaration, so the parts are disjoint and together are the whole sweep
    wherever it runs. Called after the static checks, which read all of M.
    """
    M[:] = M[index::count]


def keep_platform(goos):
    """Keep the mutants only goos can judge: those declaring platform=goos, and
    those whose target file is built for goos but not for linux, the other
    runner (sync_darwin.go). The ubuntu shards report both as NOT JUDGED
    (platform), without failing; a runner of goos judges them here. Called
    after the static checks, which read all of M."""
    M[:] = [x for x in M if x[5] == goos
            or (x[5] is None and not _goos_excludes(x[1], goos) and _goos_excludes(x[1], "linux"))]


def main(argv):
    ap = argparse.ArgumentParser(description="the mutation sweep; run from the module root")
    ap.add_argument("--list", action="store_true", help="print every mutant name in load order and exit")
    ap.add_argument("--check", action="store_true", help="run the static checks and exit")
    ap.add_argument("--files", action="append", default=[], metavar="GLOB",
                    help="load only the mutant files matching GLOB, tried against the current "
                         "directory first and then against test/mutation, unless absolute; repeatable")
    ap.add_argument("--shard", metavar="K/N",
                    help="judge only the K-th of N disjoint parts of the mutants (round-robin in load "
                         "order); the static checks still read them all, and with --list the part is "
                         "what prints")
    ap.add_argument("--platform-only", metavar="GOOS",
                    help="judge only the mutants that GOOS alone can judge (declared platform=GOOS, or a "
                         "target built for GOOS and not for linux); applied before --shard, after the "
                         "static checks, and with --list it is what prints")
    ap.add_argument("--strict", action="store_true",
                    help="exit non-zero if any mutant's judging test skipped here, not only if undetected; "
                         "a target not built for this GOOS is reported but never fails")
    ap.add_argument("--shared-tests", metavar="PATH",
                    help="read declared shared-test pairs from PATH instead of "
                         "test/mutation/shared-tests.txt (a test points this at a fixture)")
    ap.add_argument("--trim-cache-each", action="store_true",
                    help="after every mutant, remove what its build added to the build cache since the baseline, "
                         "so the cache holds the baseline and one mutant's build at most; GOCACHE must name a "
                         "cache this run owns")
    args = ap.parse_args(argv)
    try:
        shard = parse_shard(args.shard) if args.shard else None
    except ValueError as e:
        ap.error(str(e))
    if args.platform_only and args.platform_only not in _KNOWN_GOOS:
        ap.error(f"--platform-only wants a GOOS name, got {args.platform_only!r}")
    problems = load(args.files or [MUTANT_GLOB])
    if args.list:
        if args.platform_only:
            keep_platform(args.platform_only)
        if shard:
            keep_shard(*shard)
        for name, *_ in M:
            print(name)
        return report(problems)
    shared_tests = pathlib.Path(args.shared_tests) if args.shared_tests else SHARED_TESTS
    problems += check(complete=not args.files, shared_tests=shared_tests)
    if args.check:
        if not problems:
            print(f"check: {len(M)} mutants in {len(set(ORIGIN.values()))} files; every anchor occurs exactly once")
        return report(problems)
    if problems:
        print("refusing to sweep: the static checks fail", file=sys.stderr)
        return report(problems)
    if args.platform_only:
        keep_platform(args.platform_only)
    if shard:
        keep_shard(*shard)
    trim_root = None
    if args.trim_cache_each:
        trim_root, why = own_cache()
        if why:
            ap.error(why)
    return run_sweep(strict=args.strict, trim_root=trim_root)


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
