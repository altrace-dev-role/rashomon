# py2.7, in the pinned image: python truth_guarded_r2.py <lib> <xdiff-dir>
# Guarded PY3-STDLIB imports under Phase 2 ruling R2 (P2-rulings-1.txt):
# - The try has a bare except, or a handler naming ImportError, ModuleNotFoundError, Exception,
#   BaseException or StandardError, alone or in a tuple.
# - The import is in the try body or one of its handler bodies.
# - Excluded: else and finally blocks, and def or lambda bodies nested in the try.
# Replaces truth27.py's guard, which saw only the try body and only ImportError, Exception or bare.
# Positive control first: a hand-written module with known answers.
import ast, json, os, sys

CATCHES = {"ImportError", "ModuleNotFoundError", "Exception", "BaseException", "StandardError"}


def catches_import_error(node):
    for h in node.handlers:
        if h.type is None:
            return True
        names = h.type.elts if isinstance(h.type, ast.Tuple) else [h.type]
        if any(isinstance(n, ast.Name) and n.id in CATCHES for n in names):
            return True
    return False


def imports_in(stmts, out):
    for s in stmts:
        if isinstance(s, (ast.FunctionDef, ast.Lambda)):
            continue  # runs later, outside the try
        if isinstance(s, (ast.Import, ast.ImportFrom)):
            out.add(s.lineno)
        for child in ast.iter_child_nodes(s):
            if isinstance(child, ast.stmt):
                imports_in([child], out)
            elif isinstance(child, ast.excepthandler):
                imports_in(child.body, out)


def guarded_lines(tree):
    out = set()
    for node in ast.walk(tree):
        if isinstance(node, ast.TryExcept) and catches_import_error(node):
            imports_in(node.body, out)
            for h in node.handlers:
                imports_in(h.body, out)
    return out


CONTROL = """try:
    import Queue
except ImportError:
    import queue as Queue
try:
    import queue
except (ValueError, StandardError):
    import Queue
else:
    import cPickle
try:
    def f():
        import urllib2
    class C:
        import urlparse
except:
    pass
try:
    import HTMLParser
except ValueError:
    pass
import ConfigParser
try:
    try:
        import cStringIO
    finally:
        import StringIO
except ImportError:
    pass
"""
got = sorted(guarded_lines(ast.parse(CONTROL)))
assert got == [2, 4, 6, 8, 15, 25, 27], got

if __name__ == "__main__" and len(sys.argv) == 3:
    lib, xd = sys.argv[1], sys.argv[2]
    cache = {}
    for arm in ("opus", "sonnet", "blank"):
        data = json.load(open(os.path.join(xd, arm + ".json")))
        stdlib = guarded = 0
        for f in data["findings"]:
            if f["rule"] != "PY3-STDLIB" or f["severity"] != "blocker":
                continue
            path = f["file"]
            if path not in cache:
                try:
                    cache[path] = guarded_lines(ast.parse(open(os.path.join(lib, path)).read(), path))
                except SyntaxError:
                    cache[path] = set()
            stdlib += 1
            guarded += f["line"] in cache[path]
        print("%-6s PY3-STDLIB blockers %d, guarded under R2 %d (%.0f%%)" % (arm, stdlib, guarded, 100.0 * guarded / max(stdlib, 1)))
    print("control ok")
