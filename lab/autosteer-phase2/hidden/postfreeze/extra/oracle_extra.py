# Runs inside each interpreter image as uid 1000: python oracle_extra.py <root>
# Per case: entry types (lstat) and the compile verdict of every regular .py file. JSON out.
import json, os, stat, sys
root = sys.argv[1]
out = {"version": sys.version.split()[0], "cases": {}}
for case in sorted(os.listdir(root)):
    cdir = os.path.join(root, case)
    entries, compiled = {}, {}
    try:
        names = sorted(os.listdir(cdir))
    except OSError as e:
        out["cases"][case] = {"listdir_error": type(e).__name__}
        continue
    for n in names:
        p = os.path.join(cdir, n)
        st = os.lstat(p)
        m = st.st_mode
        kind = ("symlink" if stat.S_ISLNK(m) else "dir" if stat.S_ISDIR(m) else "socket" if stat.S_ISSOCK(m)
                else "fifo" if stat.S_ISFIFO(m) else "regular" if stat.S_ISREG(m) else "other")
        entries[n] = kind
        if kind == "regular" and n.endswith(".py"):
            try:
                src = open(p, "rb").read()
            except (IOError, OSError) as e:
                compiled[n] = "unreadable"
                continue
            try:
                compile(src, n, "exec", 0, True)
                compiled[n] = "ok"
            except Exception as e:
                compiled[n] = "%s: %s" % (type(e).__name__, str(getattr(e, "msg", e))[:60])
    if case in ("many_files", "very_deep_dirs"):
        entries = {"entries": len(entries)}
        compiled = {"compiled_ok": sum(1 for v in compiled.values() if v == "ok")}
    out["cases"][case] = {"entries": entries, "compile": compiled}
print(json.dumps(out, indent=1, sort_keys=True))
