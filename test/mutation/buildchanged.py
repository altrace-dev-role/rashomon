#!/usr/bin/env python3
"""Every mutant row a change adds or edits builds (DEL-2, MASKED-BUILD2).

usage: python3 test/mutation/buildchanged.py BASE

A mutant that does not compile is judged BUILD BROKEN, which the sweep counts
as undetected. Before MASKED-BUILD1 the judge read that build failure as a
kill, and 36 rows passed every sweep with no test ever run against them: 13
left an import unused, 21 a local, and 2 called a function with too few
arguments. sweep.py --check refuses the import half from the text alone; the
rest needs the compiler. So every row whose target, anchor, replacement or
IMPORTS entry differs from BASE's is applied with its IMPORTS entry, its
target's package is compiled with its tests (go test -run '^$', with the vet
checks go test runs, as the sweep's own go test does), and the file is put
back. A target excluded from this GOOS by its build tags compiles nowhere
here, so its row is not judged.

Run from the module root on a tree with no changes to the files it edits, so
putting a target back loses nothing. Exits 1 naming each row that does not
build, 2 when it cannot judge.
"""
import glob, os, pathlib, re, subprocess, sys

MUTANTS = "test/mutation/mutants"


def load(sources):
    """The rows and IMPORTS entries the mutant files in sources define: name ->
    (target, anchor, replacement) and name -> (target, import block, edited
    block). A file that does not load defines nothing, so its rows read as
    changed and are compiled."""
    rows, imports = {}, {}
    for _, src in sorted(sources.items()):
        found, entries = {}, {}

        def m(name, path, old, new, test, *a, **k):
            found[name] = (path, old, new)

        try:
            exec(src, {"m": m, "IMPORTS": entries, "__builtins__": __builtins__})
        except Exception:
            continue
        rows.update(found)
        imports.update(entries)
    return rows, imports


def at_base(base):
    files = subprocess.run(["git", "ls-tree", "--name-only", base, MUTANTS + "/"],
                           capture_output=True, text=True).stdout.split()
    out = {}
    for f in files:
        r = subprocess.run(["git", "show", f"{base}:{f}"], capture_output=True, text=True)
        if r.returncode == 0 and f.endswith(".py"):
            out[f] = r.stdout
    return out


def main():
    if len(sys.argv) != 2:
        print(__doc__.splitlines()[2], file=sys.stderr)
        return 2
    base = sys.argv[1]
    if subprocess.run(["git", "rev-parse", "--verify", "--quiet", base + "^{commit}"],
                      capture_output=True).returncode != 0:
        print(f"buildchanged: {base} is not a commit here (fetch it first)", file=sys.stderr)
        return 2
    rows, imports = load({f: pathlib.Path(f).read_text() for f in glob.glob(MUTANTS + "/*.py")})
    brows, bimports = load(at_base(base))
    # A row whose target is not Go (sweep.py's own rows) cannot break a build.
    changed = sorted(n for n in rows if (rows[n], imports.get(n)) != (brows.get(n), bimports.get(n))
                     and rows[n][0].endswith(".go"))
    targets = sorted({rows[n][0] for n in changed} | {imports[n][0] for n in changed if n in imports})
    dirty = subprocess.run(["git", "status", "--porcelain", "--", *targets], capture_output=True, text=True).stdout
    if targets and dirty.strip():
        print(f"buildchanged: refusing, these targets have changes a restore would lose:\n{dirty}", file=sys.stderr)
        return 2
    bad = 0
    for name in changed:
        target = rows[name][0]
        edits = ([imports[name]] if name in imports else []) + [rows[name]]
        touched, note = [], None
        for path, old, new in edits:
            p = pathlib.Path(path)
            s = p.read_text() if p.is_file() else ""
            if s.count(old) != 1:
                note = f"its anchor occurs {s.count(old)} times in {path}"
                break
            p.write_text(s.replace(old, new, 1))
            touched.append(path)
        if note is None:
            pkg = "./" + (os.path.dirname(target) or ".")
            r = subprocess.run(["go", "test", "-count=1", "-run", "^$", pkg], capture_output=True, text=True)
            errs = [l.strip() for l in (r.stdout + r.stderr).splitlines() if re.search(r"\.go:\d+:\d+: ", l)]
            if r.returncode != 0:
                note = "does not build: " + (" | ".join(errs[:2]) or (r.stdout + r.stderr).strip()[-300:])
        if touched:
            subprocess.run(["git", "checkout", "-q", "--", *sorted(set(touched))])
        if note is not None:
            bad += 1
            print(f"buildchanged: {name!r} {note}", file=sys.stderr)
        else:
            print(f"  builds <- {name}")
    print(f"buildchanged: {len(changed)} row(s) added or edited since {base[:12]}; {bad} do not build")
    return 1 if bad else 0


if __name__ == "__main__":
    sys.exit(main())
