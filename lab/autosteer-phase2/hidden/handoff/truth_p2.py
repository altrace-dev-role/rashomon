# py2.7, in python:2.7.18-slim: python truth_p2.py <root> <stdlib_removed.json> <arm.json> <out.json>
# Phase 2 ground truth, scored against one arm's JSON (scanned at <root>, same relative paths):
#   H2b  every 2.7 `except X, e:` clause (a top-level comma before the colon) needs a blocker at its line for
#        every judged version: 3.6 to 3.13 cannot compile it, and 3.14+ compiles it with another meaning (PEP 758).
#   H8s  PY3-STDLIB per version (R6, R8): an import of a 2.7 top-level module that a judged version lacks
#        (stdlib_removed.json, interpreter plus primary source) is a blocker for exactly those versions, or a
#        warning when R2 guards it. Implicit relative imports (a sibling module inside a package) are excluded.
# Positive controls first.
import ast, collections, io, json, os, sys, tokenize
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from truth_guarded_r2 import guarded_lines  # frozen R2 guard, with its own control

JUDGED = ["3.6", "3.7", "3.8", "3.9", "3.10", "3.11", "3.12", "3.13", "3.14", "3.15"]
# C53: stdlib_removed.json lists only modules the Linux 2.7 image has, minus names starting with "_". Removals
# of names it cannot see, verified in the frozen pack (phase2-pack/text):
SUPPLEMENT = {"__builtin__": ("3.0", "py3-whatsnew30.txt:646 Renamed module __builtin__ to builtins"),
              "_winreg": ("3.0", "pep-3108.txt:831 _winreg -> winreg"),
              "dl": ("3.0", "pep-3108.txt:699 dl [done]"),
              "msilib": ("3.13", "pep-0594.txt:106 msilib")}
# Project roots in the scan root: a module a project ships at its root is local, so an import of it is ambiguous.
# C58 (00:12): stdlib/ was left out ("it IS the stdlib"), which scored stdlib/distutils/core.py importing distutils
# as a required R8 blocker. A scanner that treats a module the scanned tree ships as local is defensible, and R8 does
# not settle it, so those lines are scored neither way, the same as trac and moin.
SUBROOTS = ("trac", "moin", "stdlib")
# C59 (00:38): 2.7 modules every 3.x lacks but no primary source calls removed, and too built-in for R8's
# interpreter test (a C module). R8 does not settle them, so their imports are scored neither way.
UNCONFIRMED = ("exceptions",)
# E26 (01:40): the generator's own unconfirmed list (strop, future_builtins: C modules every 3.x lacks, with no primary
# source in the pack) was ignored here, so an arm flagging them was charged an extra. The list is now read from
# stdlib_removed.json, never typed.


def unconfirmed_names(entries):
    """stdlib_removed.json's unconfirmed list: [module, first-absent] pairs from the generator, or C59's dicts."""
    return tuple(e["module"] if isinstance(e, dict) else e[0] for e in entries)


def vt(v):
    return tuple(int(x) for x in v.split("."))


def except_comma_lines(src):
    out = []
    toks = tokenize.generate_tokens(io.BytesIO(src).readline)
    in_clause, depth, comma, line = False, 0, False, 0
    for tok in toks:
        typ, s, start = tok[0], tok[1], tok[2]
        if typ == tokenize.NAME and s == "except" and not in_clause:
            in_clause, depth, comma, line = True, 0, False, start[0]
            continue
        if not in_clause:
            continue
        if typ == tokenize.OP and s in "([{":
            depth += 1
        elif typ == tokenize.OP and s in ")]}":
            depth -= 1
        elif typ == tokenize.OP and s == "," and depth == 0:
            comma = True
        elif typ == tokenize.OP and s == ":" and depth == 0:
            if comma:
                out.append(line)
            in_clause = False
    return out


def imports(tree, dirpath, is_pkg_dir, project_root=None):
    """(line, top-level module) for absolute imports. Excluded as ambiguous: a sibling module (an implicit relative
    import in a package; in a plain directory such as lib-tk, local whenever that directory is on sys.path, C58), and
    names the scanned project itself ships at its root (they resolve to the project's module)."""
    out, ambiguous = [], []
    for node in ast.walk(tree):
        names = []
        if isinstance(node, ast.Import):
            names = [a.name for a in node.names]
        elif isinstance(node, ast.ImportFrom) and not node.level and node.module and node.module != "__future__":
            names = [node.module]
        for n in names:
            top = n.split(".")[0]
            sibling = os.path.exists(os.path.join(dirpath, top + ".py")) or os.path.isdir(os.path.join(dirpath, top))
            local = project_root is not None and (os.path.exists(os.path.join(project_root, top + ".py"))
                                                  or os.path.isdir(os.path.join(project_root, top)))
            (ambiguous if sibling or local or top in UNCONFIRMED else out).append((node.lineno, top))
    return out, ambiguous


def expected(root, removed):
    exc, std, amb, unparsable = {}, collections.Counter(), set(), []
    for d, dirs, files in os.walk(root):
        dirs.sort()
        is_pkg = "__init__.py" in files
        for f in sorted(files):
            if not f.endswith(".py"):
                continue
            p = os.path.join(d, f)
            rel = os.path.relpath(p, root)
            src = open(p, "rb").read()
            try:
                tree = compile(src, rel, "exec", ast.PyCF_ONLY_AST, 1)
                exc[rel] = except_comma_lines(src)
            except (SyntaxError, TypeError, ValueError, tokenize.TokenError, IndentationError):
                unparsable.append(rel)
                continue
            guarded = guarded_lines(tree)
            first = rel.split(os.sep)[0]
            proot = os.path.join(root, first) if first in SUBROOTS else None
            got, ambig = imports(tree, d, is_pkg, proot)
            amb.update((rel, line) for line, _ in ambig)
            for line, top in got:
                if top in removed:
                    since = removed[top]["removed_in"]
                    vers = tuple(v for v in JUDGED if since == "3.0" or vt(v) >= vt(since))
                    std[(rel, line, "warning" if line in guarded else "blocker", vers)] += 1
    return exc, std, amb, unparsable


def score(root, removed, arm):
    removed = dict(removed)
    for k, (since, src) in SUPPLEMENT.items():
        removed.setdefault(k, {"removed_in": since, "source": src})
    exc, std, amb, unparsable = expected(root, removed)
    blockers = collections.defaultdict(set)  # (file, line) -> versions with any blocker there
    arm_std = collections.Counter()
    for f in arm["findings"]:
        vers = tuple(f.get("versions") or JUDGED)
        if f["severity"] == "blocker":
            blockers[(f["file"], f["line"])].update(vers)
        if f["rule"] == "PY3-STDLIB":
            arm_std[(f["file"], f["line"], f["severity"], tuple(sorted(vers, key=vt)))] += 1
    # H2b
    need = [(rel, ln) for rel, lines in exc.items() for ln in lines]
    missing = [(rel, ln, sorted(set(JUDGED) - blockers.get((rel, ln), set()), key=vt)) for rel, ln in need]
    missing = [m for m in missing if m[2]]
    # H8s: compare per (file, line) the multiset of (severity, versions)
    want_by, got_by = collections.defaultdict(collections.Counter), collections.defaultdict(collections.Counter)
    for (rel, ln, sev, vers), n in std.items():
        want_by[(rel, ln)][(sev, vers)] += n
    for (rel, ln, sev, vers), n in arm_std.items():
        if (rel, ln) in amb and (rel, ln) not in want_by:
            continue  # an ambiguous import line is scored neither way
        got_by[(rel, ln)][(sev, vers)] += n
    exact = fn = fp = sev_wrong = ver_wrong = 0
    ex = collections.defaultdict(list)
    for key in set(want_by) | set(got_by):
        w, g = want_by.get(key, collections.Counter()), got_by.get(key, collections.Counter())
        common = w & g
        exact += sum(common.values())
        w, g = w - common, g - common
        wl, gl = sorted(w.elements()), sorted(g.elements())
        while wl and gl:
            a, b = wl.pop(), gl.pop()
            if a[0] != b[0]:
                sev_wrong += 1
                ex["severity"].append("%s:%d want %s got %s" % (key[0], key[1], a[0], b[0]))
            else:
                ver_wrong += 1
                ex["versions"].append("%s:%d want %s.. got %s.." % (key[0], key[1], a[1][0], b[1][0] if b[1] else "-"))
        fn += len(wl)
        fp += len(gl)
        ex["missed"] += ["%s:%d" % key] * len(wl)
        ex["extra"] += ["%s:%d" % key] * len(gl)
    return {"except_comma_clauses": len(need), "except_comma_missing_blocker": len(missing),
            "except_comma_examples": missing[:5],
            "stdlib_expected": sum(std.values()), "stdlib_exact": exact, "stdlib_missed": fn, "stdlib_extra": fp,
            "stdlib_severity_wrong": sev_wrong, "stdlib_versions_wrong": ver_wrong,
            "stdlib_examples": {k: v[:6] for k, v in ex.items()},
            "ambiguous_lines_excluded": len(amb), "unparsable_on_27": len(unparsable)}


def control():
    assert except_comma_lines(b"try:\n    pass\nexcept ValueError, e:\n    pass\n"
                              b"try:\n    pass\nexcept (A, B), e:\n    pass\n"
                              b"try:\n    pass\nexcept (A, B):\n    pass\n"
                              b"try:\n    pass\nexcept A as e:\n    pass\n") == [3, 7]
    import tempfile, shutil
    t = tempfile.mkdtemp()
    try:
        os.mkdir(os.path.join(t, "pkg"))
        open(os.path.join(t, "pkg", "__init__.py"), "w").write("")
        open(os.path.join(t, "pkg", "parser.py"), "w").write("")
        open(os.path.join(t, "pkg", "m.py"), "w").write("import parser\nimport imp\n")
        open(os.path.join(t, "b.py"), "w").write("import __builtin__\n")
        open(os.path.join(t, "a.py"), "w").write(
            "import md5\ntry:\n    import imp\nexcept ImportError:\n    pass\nfrom asyncore import loop\nimport os\n"
            "try:\n    pass\nexcept IOError, e:\n    pass\n")
        removed = {"md5": {"removed_in": "3.0"}, "imp": {"removed_in": "3.12"}, "asyncore": {"removed_in": "3.12"},
                   "parser": {"removed_in": "3.10"}}
        late = ("3.12", "3.13", "3.14", "3.15")
        perfect = {"findings": [
            {"file": "a.py", "line": 1, "rule": "PY3-STDLIB", "severity": "blocker", "versions": JUDGED},
            {"file": "a.py", "line": 3, "rule": "PY3-STDLIB", "severity": "warning", "versions": list(late)},
            {"file": "a.py", "line": 6, "rule": "PY3-STDLIB", "severity": "blocker", "versions": list(late)},
            {"file": "a.py", "line": 10, "rule": "PY3-EXCEPT-COMMA", "severity": "blocker"},
            {"file": "pkg/m.py", "line": 2, "rule": "PY3-STDLIB", "severity": "blocker", "versions": list(late)},
            {"file": "b.py", "line": 1, "rule": "PY3-STDLIB", "severity": "blocker", "versions": JUDGED}]}
        r = score(t, removed, perfect)
        assert (r["stdlib_expected"], r["stdlib_exact"], r["stdlib_missed"], r["stdlib_extra"]) == (5, 5, 0, 0), r
        assert r["ambiguous_lines_excluded"] == 1 and r["except_comma_missing_blocker"] == 0, r
        flawed = {"findings": [  # imp flagged for every version, asyncore missed, line 10 not blocked
            {"file": "a.py", "line": 1, "rule": "PY3-STDLIB", "severity": "blocker", "versions": JUDGED},
            {"file": "a.py", "line": 3, "rule": "PY3-STDLIB", "severity": "blocker", "versions": list(late)},
            {"file": "pkg/m.py", "line": 2, "rule": "PY3-STDLIB", "severity": "blocker", "versions": JUDGED}]}
        r = score(t, removed, flawed)
        assert (r["stdlib_exact"], r["stdlib_missed"], r["stdlib_severity_wrong"], r["stdlib_versions_wrong"]) == (1, 2, 1, 1), r
        assert r["except_comma_missing_blocker"] == 1, r
    finally:
        shutil.rmtree(t)
    t = tempfile.mkdtemp()  # C58: a module the project ships, at its root or as a plain-directory sibling
    try:
        for d in ("stdlib/distutils", "stdlib/lib-tk", "moin"):
            os.makedirs(os.path.join(t, d))
        open(os.path.join(t, "stdlib", "distutils", "__init__.py"), "w").write("")
        open(os.path.join(t, "stdlib", "distutils", "core.py"), "w").write("from distutils.cmd import Command\n")
        open(os.path.join(t, "stdlib", "lib-tk", "tkCommonDialog.py"), "w").write("")
        open(os.path.join(t, "stdlib", "lib-tk", "tkColorChooser.py"), "w").write("from tkCommonDialog import Dialog\n")
        open(os.path.join(t, "stdlib", "sunau.py"), "w").write("import audioop\n")
        open(os.path.join(t, "moin", "setup.py"), "w").write("import distutils\n")
        open(os.path.join(t, "moin", "w.py"), "w").write("from exceptions import ReferenceError\n")
        open(os.path.join(t, "moin", "v.py"), "w").write("import strop\n")
        removed = {"distutils": {"removed_in": "3.12"}, "tkCommonDialog": {"removed_in": "3.0"},
                   "audioop": {"removed_in": "3.13"}}
        arm = {"findings": [{"file": "moin/w.py", "line": 1, "rule": "PY3-STDLIB", "severity": "blocker"},
                            {"file": "moin/v.py", "line": 1, "rule": "PY3-STDLIB", "severity": "warning"}]}
        r = score(t, removed, arm)  # the old rule, exceptions only: strop is charged as an extra (E26)
        assert (r["stdlib_extra"], r["ambiguous_lines_excluded"]) == (1, 3), r
        global UNCONFIRMED
        old, UNCONFIRMED = UNCONFIRMED, UNCONFIRMED + unconfirmed_names([["strop", "3.6"], {"module": "exceptions"}])
        try:
            r = score(t, removed, arm)
        finally:
            UNCONFIRMED = old
        assert (r["stdlib_expected"], r["stdlib_missed"], r["ambiguous_lines_excluded"]) == (2, 2, 4), r
        assert r["stdlib_extra"] == 0, r  # flagging an unconfirmed module is not an extra either
        assert sorted(r["stdlib_examples"]["missed"]) == ["moin/setup.py:1", "stdlib/sunau.py:1"], r
    finally:
        shutil.rmtree(t)


control()
if __name__ == "__main__" and len(sys.argv) == 5:
    root, removed, arm, out = sys.argv[1:5]
    data = json.load(open(removed))
    UNCONFIRMED = UNCONFIRMED + unconfirmed_names(data.get("unconfirmed", []))  # E26: from the data
    res = score(root, data["removed"], json.load(open(arm)))
    json.dump(res, open(out, "w"), indent=1, sort_keys=True)
    print(json.dumps(res, sort_keys=True)[:1500])
print("control ok")
