# Runs inside each real interpreter image, as uid 1000: python oracle_hostile.py <root>
# Prints, as JSON, the version and every regular .py file under <root> that this interpreter cannot compile.
# Unreadable entries are R5's concern (exit 2), not the compile guarantee, so they are listed apart.
import json, os, sys
root = sys.argv[1]
fails, unreadable = [], []
for d, dirs, files in os.walk(root):
    dirs.sort()
    for f in sorted(files):
        p = os.path.join(d, f)
        if not f.endswith(".py") or not os.path.isfile(p):
            continue
        rel = os.path.relpath(p, root)
        try:
            src = open(p, "rb").read()
        except (IOError, OSError):
            unreadable.append(rel)
            continue
        try:
            compile(src, rel, "exec", 0, True)
        except (SyntaxError, ValueError, TypeError, MemoryError, RecursionError if sys.version_info[0] > 2 else RuntimeError, OverflowError):
            fails.append(rel)
print(json.dumps({"version": sys.version.split()[0], "fail": fails, "unreadable": unreadable}))
