# py2.7: python r8_breakdown.py <root> <removed.json> <arm.json>  -> every R8 miss with its module and whether the
# scanned tree itself ships that module at the miss's top-level root (stdlib/<m>.py or stdlib/<m>/)
import collections, json, os, sys
sys.path.insert(0, os.path.join(os.path.dirname(os.path.abspath(__file__))))
import truth_p2 as T
root, removed, arm = sys.argv[1:4]
removed = json.load(open(removed))["removed"]
for k, (since, src) in T.SUPPLEMENT.items():
    removed.setdefault(k, {"removed_in": since, "source": src})
exc, std, amb, unp = T.expected(root, removed)
arm = json.load(open(arm))
got = collections.Counter()
for f in arm["findings"]:
    if f["rule"] == "PY3-STDLIB":
        got[(f["file"], f["line"])] += 1
# module per (file, line), from the source
import ast
mods = {}
for (rel, ln, sev, vers), n in std.items():
    if got[(rel, ln)] >= 1:
        got[(rel, ln)] -= 1
        continue
    tree = ast.parse(open(os.path.join(root, rel)).read())
    m = "?"
    for node in ast.walk(tree):
        if getattr(node, "lineno", 0) == ln and isinstance(node, (ast.Import, ast.ImportFrom)):
            names = [a.name for a in node.names] if isinstance(node, ast.Import) else [node.module]
            tops = [x.split(".")[0] for x in names if x.split(".")[0] in removed]
            m = tops[0] if tops else "?"
    first = rel.split("/")[0]
    ships = os.path.exists(os.path.join(root, first, m + ".py")) or os.path.isdir(os.path.join(root, first, m))
    mods[(rel, ln)] = (m, removed.get(m, {}).get("removed_in"), ships, sev)
c = collections.Counter((v[2], v[1]) for v in mods.values())
print("misses %d; by (tree ships module, removed_in): %s" % (len(mods), sorted(c.items())))
for ships in (False, True):
    ex = sorted((k, v) for k, v in mods.items() if v[2] == ships)[:8]
    print("ships=%s e.g." % ships, ["%s:%d %s %s" % (k[0], k[1], v[0], v[1]) for k, v in ex])
mc = collections.Counter(v[0] for v in mods.values() if not v[2])
print("not shipped, by module:", mc.most_common(15))
