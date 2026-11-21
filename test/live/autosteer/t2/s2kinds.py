"""s2kinds.py: T2's T2K_UNENFORCED_CMD (addendum 2). A parsed read of spec.UnenforcedRuleKinds from
internal/spec/kinds.go at the commit G's binary was built from.

  s2kinds.py BIN WANT_SHA            run mode, cwd in a rashomon worktree; exit 0 on a clean read
  s2kinds.py --rev REV               test mode: no binary check; exit 3 on a clean read
  s2kinds.py --file KINDS [GO...]    test mode: parser only, KINDS plus GO files as the package; exit 3 on a clean read

Exit 2 on every refusal. No test mode exits 0, so none can pass smoke.sh:34.
Run mode:
 (a) go version -m BIN, cwd /, GOTOOLCHAIN=local, GOFLAGS empty: one vcs.revision equal to WANT_SHA (40 hex) and
     one vcs.modified=false. The binary then proves REV compiled, so the parser may skip newlines.
 (b) REV is a commit of cwd's repository; kinds.go is read as a raw blob (git cat-file blob).
 (c) kinds.go names UnenforcedRuleKinds once in code (comments and strings do not count), as a top-level var or
     grouped var spec: optional type []RuleKind, value []RuleKind{...} of bare identifiers, a trailing comma and
     comments allowed, nothing after the closing brace but a newline, ';' or ')'.
 (d) each element resolves to one top-level string constant of the package's non-test files at REV
     (Name [Type] = "v" or Name = Type("v")); the value is printed beside the name, so a renamed constant
     (KindBoundary) still shows its value to the S-2 grep.
 (e) no other non-test Go file at REV names the identifier in code: a writer elsewhere would change the runtime
     value this read reports.
Line 1 is the provenance and holds no word the S-2 grep reads; line 2 is the names and the values.
"""
import os
import re
import subprocess
import sys

IDENT = "UnenforcedRuleKinds"
KINDS = "internal/spec/kinds.go"
PKG = "internal/spec/"
HEX40 = re.compile(r"^[0-9a-f]{40}$")
NAME = re.compile(r"^[A-Za-z_][A-Za-z0-9_]*$")
VALUE = re.compile(r"^[A-Za-z0-9_.-]+$")


class Refuse(Exception):
    pass


def tokens(src, where):
    """Go tokens as (kind, text, line); kind is id, num, str, raw, rune, op or nl. Comments are dropped; a block
    comment that spans lines leaves one nl."""
    out, i, n, line = [], 0, len(src), 1
    while i < n:
        c = src[i]
        if c == "\n":
            out.append(("nl", "\n", line))
            line, i = line + 1, i + 1
        elif c in " \t\r\f":
            i += 1
        elif src.startswith("//", i):
            j = src.find("\n", i)
            i = n if j < 0 else j
        elif src.startswith("/*", i):
            j = src.find("*/", i + 2)
            if j < 0:
                raise Refuse("%s:%d: unterminated block comment" % (where, line))
            k = src.count("\n", i, j)
            if k:
                out.append(("nl", "\n", line))
                line += k
            i = j + 2
        elif c in "\"'":
            j = i + 1
            while j < n and src[j] != c:
                if src[j] == "\n":
                    raise Refuse("%s:%d: newline in a quoted literal" % (where, line))
                j += 2 if src[j] == "\\" else 1
            if j >= n:
                raise Refuse("%s:%d: unterminated quoted literal" % (where, line))
            out.append(("str" if c == "\"" else "rune", src[i:j + 1], line))
            i = j + 1
        elif c == "`":
            j = src.find("`", i + 1)
            if j < 0:
                raise Refuse("%s:%d: unterminated raw string" % (where, line))
            out.append(("raw", src[i:j + 1], line))
            line += src.count("\n", i, j)
            i = j + 1
        elif c.isalpha() or c == "_":
            j = i + 1
            while j < n and (src[j].isalnum() or src[j] == "_"):
                j += 1
            out.append(("id", src[i:j], line))
            i = j
        elif c.isdigit():
            j = i + 1
            while j < n and (src[j].isalnum() or src[j] in "._"):
                j += 1
            out.append(("num", src[i:j], line))
            i = j
        else:
            out.append(("op", c, line))
            i += 1
    return out


def isop(t, s):
    return t[0] == "op" and t[1] == s


def nonl(ft, k, step):
    """The index of the first non-nl token from k in direction step, or -1/len when none."""
    while 0 <= k < len(ft) and ft[k][0] == "nl":
        k += step
    return k


def enclosing(ft, k):
    """The index of the unmatched opener around token k, or -1 at top level."""
    d = 0
    for j in range(k - 1, -1, -1):
        if ft[j][0] != "op":
            continue
        if ft[j][1] in ")]}":
            d += 1
        elif ft[j][1] in "([{":
            if d == 0:
                return j
            d -= 1
    return -1


def refs(ft):
    return [k for k, t in enumerate(ft) if t[0] == "id" and t[1] == IDENT]


def expect(ft, k, kind, text, where, what):
    k = nonl(ft, k, 1)
    if k >= len(ft) or ft[k][0] != kind or ft[k][1] != text:
        raise Refuse("%s: %s: want %r after %s" % (where, IDENT, text, what))
    return k + 1


def declaration(src, where):
    """The element names of the one declaration of IDENT in src."""
    ft = tokens(src, where)
    ks = refs(ft)
    if len(ks) != 1:
        raise Refuse("%s: %s is named %d times in code; want one declaration, nothing else" % (where, IDENT, len(ks)))
    k = ks[0]
    p = nonl(ft, k - 1, -1)
    e = enclosing(ft, k)
    if e < 0:
        if p < 0 or ft[p] != ("id", "var", ft[p][2]):
            raise Refuse("%s:%d: %s is not a top-level var declaration" % (where, ft[k][2], IDENT))
    else:
        v = nonl(ft, e - 1, -1)
        grouped = isop(ft[e], "(") and v >= 0 and ft[v][:2] == ("id", "var") and enclosing(ft, v) < 0
        starts = k - 1 >= 0 and (ft[k - 1][0] == "nl" or isop(ft[k - 1], ";") or k - 1 == e)
        if not (grouped and starts):
            raise Refuse("%s:%d: %s is not a top-level var spec" % (where, ft[k][2], IDENT))
    j = nonl(ft, k + 1, 1)
    if j < len(ft) and isop(ft[j], "["):
        j = expect(ft, j + 1, "op", "]", where, "[")
        j = expect(ft, j, "id", "RuleKind", where, "[]")
    j = expect(ft, j, "op", "=", where, "the name")
    j = expect(ft, j, "op", "[", where, "=")
    j = expect(ft, j, "op", "]", where, "[")
    j = expect(ft, j, "id", "RuleKind", where, "[]")
    j = expect(ft, j, "op", "{", where, "[]RuleKind")
    names = []
    while True:
        j = nonl(ft, j, 1)
        if j >= len(ft):
            raise Refuse("%s: %s: the literal is not closed" % (where, IDENT))
        if isop(ft[j], "}"):
            break
        t = ft[j]
        if t[0] != "id" or not NAME.match(t[1]):
            raise Refuse("%s:%d: %s: element %r is not a bare identifier" % (where, t[2], IDENT, t[1]))
        names.append(t[1])
        j = nonl(ft, j + 1, 1)
        if j < len(ft) and isop(ft[j], ","):
            j += 1
        elif not (j < len(ft) and isop(ft[j], "}")):
            got = ft[j][1] if j < len(ft) else "end of file"
            raise Refuse("%s:%d: %s: element %r is followed by %r" % (where, t[2], IDENT, t[1], got))
    a = j + 1
    if a < len(ft) and not (ft[a][0] == "nl" or isop(ft[a], ";") or isop(ft[a], ")")):
        raise Refuse("%s:%d: %s: %r follows the literal" % (where, ft[a][2], IDENT, ft[a][1]))
    return names


def literal(t):
    if t[0] == "raw":
        return t[1][1:-1]
    if t[0] == "str" and "\\" not in t[1]:
        return t[1][1:-1]
    return None


def consts(src, where):
    """Top-level const specs as {name: [(value or None, where:line)]}."""
    ft = tokens(src, where)
    out = {}
    k = 0
    while k < len(ft):
        t = ft[k]
        if t[:2] != ("id", "const") or enclosing(ft, k) >= 0:
            k += 1
            continue
        j = nonl(ft, k + 1, 1)
        specs = []
        if j < len(ft) and isop(ft[j], "("):
            cur, d, j = [], 0, j + 1
            while j < len(ft):
                u = ft[j]
                if u[0] == "op" and u[1] in "([{":
                    d += 1
                elif u[0] == "op" and u[1] in ")]}":
                    if d == 0:
                        break
                    d -= 1
                if d == 0 and (u[0] == "nl" or isop(u, ";")):
                    specs.append(cur)
                    cur = []
                else:
                    cur.append(u)
                j += 1
            specs.append(cur)
        else:
            cur = []
            while j < len(ft) and not (ft[j][0] == "nl" or isop(ft[j], ";")):
                cur.append(ft[j])
                j += 1
            specs.append(cur)
        for s in specs:
            if not s:
                continue
            ids, prev = [], None
            for u in s:
                if u[0] == "id" and (not ids or isop(prev, ",")):
                    ids.append(u)
                elif not isop(u, ","):
                    break
                prev = u
            val = None
            sh = [u[1] if u[0] in ("id", "op") else u[0] for u in s]
            if len(s) in (3, 4) and sh[-2] == "=" and literal(s[-1]) is not None:
                if len(s) == 3 or s[1][0] == "id":
                    val = literal(s[-1])
            elif len(s) == 6 and sh[1] == "=" and s[2][0] == "id" and sh[3] == "(" and sh[5] == ")":
                val = literal(s[4])
            for u in ids:
                out.setdefault(u[1], []).append((val if len(ids) == 1 else None, "%s:%d" % (where, u[2])))
        k = j + 1
    return out


def resolve(names, files):
    table = {}
    for where, src in files:
        for n, recs in consts(src, where).items():
            table.setdefault(n, []).extend(recs)
    values = []
    for n in names:
        recs = table.get(n, [])
        if len(recs) != 1 or recs[0][0] is None or not VALUE.match(recs[0][0]):
            got = ", ".join(r[1] for r in recs) or "none"
            raise Refuse("%s element %s: want one top-level string constant in the package, got %s" % (IDENT, n, got))
        values.append(recs[0][0])
    return values


def guard(files, kinds_where):
    for where, src in files:
        if where.endswith("_test.go") or where == kinds_where:
            continue
        ks = refs(tokens(src, where))
        if ks:
            raise Refuse("%s:%d names %s in code; a parsed read cannot vouch for its runtime value"
                         % (where, tokens(src, where)[ks[0]][2], IDENT))


def run(args, cwd=None, env=None):
    try:
        p = subprocess.run(args, cwd=cwd, env=env, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    except OSError as e:
        raise Refuse("%s: %s" % (args[0], e))
    return p.returncode, p.stdout, p.stderr


def git(*args):
    rc, out, err = run(["git"] + list(args))
    if rc != 0:
        raise Refuse("git %s: rc %d: %s" % (" ".join(args), rc, err.decode("utf-8", "replace").strip()[:200]))
    return out


def text(b, where):
    try:
        return b.decode("utf-8")
    except UnicodeDecodeError:
        raise Refuse("%s: not UTF-8" % where)


def binrev(binp, want):
    if not HEX40.match(want):
        raise Refuse("WANT_SHA %r is not 40 hex (is T1_G_BIN_SHA passed to the child?)" % want)
    if not os.path.isfile(binp):
        raise Refuse("BIN %r is not a file" % binp)
    env = dict(os.environ, GOTOOLCHAIN="local", GOFLAGS="")
    rc, out, err = run(["go", "version", "-m", binp], cwd="/", env=env)
    if rc != 0:
        raise Refuse("go version -m: rc %d: %s" % (rc, err.decode("utf-8", "replace").strip()[:200]))
    revs, mods = [], []
    for ln in out.decode("utf-8", "replace").splitlines():
        f = ln.split()
        if len(f) == 2 and f[0] == "build" and f[1].startswith("vcs.revision="):
            revs.append(f[1].split("=", 1)[1])
        if len(f) == 2 and f[0] == "build" and f[1].startswith("vcs.modified="):
            mods.append(f[1].split("=", 1)[1])
    if len(revs) != 1 or len(mods) != 1:
        raise Refuse("BIN carries %d vcs.revision and %d vcs.modified lines; want one each" % (len(revs), len(mods)))
    if mods[0] != "false":
        raise Refuse("BIN vcs.modified=%s; want false" % mods[0])
    if revs[0] != want:
        raise Refuse("BIN vcs.revision %s is not WANT_SHA %s" % (revs[0], want))
    return revs[0]


def atrev(rev):
    full = git("rev-parse", "--verify", "--quiet", rev + "^{commit}").decode().strip()
    if not HEX40.match(full):
        raise Refuse("REV %r does not name one commit" % rev)
    blob = git("rev-parse", "--verify", "--quiet", full + ":" + KINDS).decode().strip()
    kinds = text(git("cat-file", "blob", blob), KINDS)
    names = declaration(kinds, KINDS)
    pkg = [(KINDS, kinds)]
    for p in git("ls-tree", "--name-only", full, PKG).decode().splitlines():
        if p.endswith(".go") and not p.endswith("_test.go") and p != KINDS:
            pkg.append((p, text(git("cat-file", "blob", full + ":" + p), p)))
    values = resolve(names, pkg)
    rc, out, err = run(["git", "grep", "-l", "-F", "-e", IDENT, full, "--", "*.go"])
    if rc != 0:
        raise Refuse("git grep for %s at %s: rc %d" % (IDENT, full, rc))
    hits = [ln.split(":", 1)[1] for ln in out.decode().splitlines() if ln.startswith(full + ":")]
    if KINDS not in hits:
        raise Refuse("git grep at %s does not list %s" % (full, KINDS))
    others = [p for p in hits if not p.endswith("_test.go") and p != KINDS]
    guard([(p, text(git("cat-file", "blob", full + ":" + p), p)) for p in others], KINDS)
    return full, blob, names, values, len(others)


def line2(names, values):
    return "%s names [%s] values [%s]" % (IDENT, " ".join(names), " ".join(values))


def main(argv):
    if len(argv) >= 2 and argv[0] == "--file":
        files = []
        for p in argv[1:]:
            with open(p, "rb") as f:
                files.append((p, text(f.read(), p)))
        names = declaration(files[0][1], files[0][0])
        values = resolve(names, files)
        guard(files, files[0][0])
        print("TEST MODE --file %s, %d package files" % (os.path.basename(argv[1]), len(files)))
        print(line2(names, values))
        return 3
    if len(argv) == 2 and argv[0] == "--rev":
        full, blob, names, values, n = atrev(argv[1])
        print("TEST MODE --rev: %s at %s blob %s; other non-test files naming it outside code: %d"
              % (KINDS, full, blob, n))
        print(line2(names, values))
        return 3
    if len(argv) != 2 or argv[0].startswith("-"):
        raise Refuse("usage: s2kinds.py BIN WANT_SHA | --rev REV | --file KINDS [GO...]")
    rev = binrev(argv[0], argv[1])
    full, blob, names, values, n = atrev(rev)
    print("%s at %s blob %s (BIN vcs.revision, vcs.modified false; T1_G_BIN_SHA %s); "
          "other non-test files naming it outside code: %d" % (KINDS, full, blob, argv[1], n))
    print(line2(names, values))
    return 0


if __name__ == "__main__":
    try:
        sys.exit(main(sys.argv[1:]))
    except Refuse as e:
        print("s2kinds: REFUSE: %s" % e, file=sys.stderr)
        sys.exit(2)
    except Exception as e:
        print("s2kinds: REFUSE: %s: %s" % (type(e).__name__, e), file=sys.stderr)
        sys.exit(2)
