#!/bin/sh
# a2.sh RUNDIR: A2 reuse by structure (section 7.4), judged with pgraph on the arm's final tree.
#   pass = the diff touches a non-test .go file under internal/platform
#      and the final tree's PROVEN call graph holds a `calls` edge from a symbol in cmd/rashomon or internal/verify
#          to a function or method of internal/platform that is new against the pinned tree's log or whose
#          declaration the diff touched
#      and no parallel implementation: no function new against the pinned log outside internal/platform and pgraph
#          calls pgraph.DecodeLog, and no non-platform hunk of the diff adds a ".pgraph" or "pgraph.log" literal.
# Environment: RK_PGRAPH (the pinned pgraph binary), RK_BASELINE_LOG (the pinned tree's pgraph.log; its sha256 must
# equal T1_BASELINE_LOG_SHA256 in pins.txt), RK_GOSLOT (optional slot wrapper for the sync), RK_GO_TOOLCHAIN_BIN
# (a go >= 1.26 first on PATH: pgraph pins GOTOOLCHAIN=local, so the PATH go must satisfy pgraph/go.mod).
set -u
D=${1:?RUNDIR}; D=${D%/}
HERE=$(cd "$(dirname "$0")" && pwd)
. "$HERE/../pins.txt"
: "${RK_PGRAPH:?RK_PGRAPH}"; : "${RK_BASELINE_LOG:?RK_BASELINE_LOG}"
mkdir -p "$D/judge"
got=$(shasum -a 256 "$RK_BASELINE_LOG" | cut -c1-64)
[ "$got" = "$T1_BASELINE_LOG_SHA256" ] || { echo "a2: baseline log sha $got is not the pinned $T1_BASELINE_LOG_SHA256" >&2; exit 2; }
SYNC="export PATH=${RK_GO_TOOLCHAIN_BIN:-/nonexistent}:\$PATH; $RK_PGRAPH sync ."
if [ -n "${RK_GOSLOT:-}" ]; then "$RK_GOSLOT" "a2-$(basename "$D")" "$D/tree" sh -c "$SYNC" > "$D/judge/a2-sync.txt" 2>&1
else (cd "$D/tree" && sh -c "$SYNC") > "$D/judge/a2-sync.txt" 2>&1; fi
[ -f "$D/tree/.pgraph/pgraph.log" ] || { echo "a2: sync wrote no log"; cat "$D/judge/a2-sync.txt"; exit 1; }
MOD=$(sed -n 's/^module //p' "$D/tree/go.mod" | head -1)
python3 - "$D" "$RK_BASELINE_LOG" "$MOD" <<'PY'
import json, os, re, sys
D, base_path, mod = sys.argv[1:4]
def rows(p):
    out = set()
    with open(p, encoding="utf-8") as f:
        assert f.readline().rstrip("\n") == "pgraph-log v0", "log header"
        for line in f:
            x = line.rstrip("\n").split("\t")
            if len(x) == 8 and x[0] == "+":
                out.add(tuple(x[1:5]))
    return out
base, final = rows(base_path), rows(os.path.join(D, "tree", ".pgraph", "pgraph.log"))
plat = mod + "/internal/platform"
def pkg(sym):
    body = sym.split(":", 1)[1]
    head, _, tail = body.rpartition("/")
    return head + "/" + tail.split(".")[0] if head else tail.split(".")[0]
def short(sym):
    return sym.split(":", 1)[1].rpartition("/")[2].split(".", 1)[1] if "." in sym.split(":", 1)[1].rpartition("/")[2] else ""
declared_final = {o for s, p, o, t in final if p == "declares"}
declared_base = {o for s, p, o, t in base if p == "declares"}
new_syms = declared_final - declared_base
new_platform = {o for o in new_syms if o.split(":")[0] in ("func", "method") and pkg(o) == plat}
# functions whose declaration the diff touched, by short name, in platform files
touched_platform = set()
cur = None
for line in open(os.path.join(D, "diff.patch"), encoding="utf-8", errors="replace"):
    if line.startswith("+++ "):
        cur = line[6:].strip()
    elif cur and cur.startswith("internal/platform/") and cur.endswith(".go") and not cur.endswith("_test.go"):
        m = re.match(r"^[+ @].*?func (?:\([^)]*\*?(\w+)\) )?(\w+)\(", line)
        if m:
            touched_platform.add((m.group(1) + "." if m.group(1) else "") + m.group(2))
diff_touches_platform = any(l.startswith("+++ b/internal/platform/") and l.strip().endswith(".go") and not l.strip().endswith("_test.go")
                            for l in open(os.path.join(D, "diff.patch"), encoding="utf-8", errors="replace"))
new_files = [l.strip() for l in open(os.path.join(D, "new-files.txt"))] if os.path.exists(os.path.join(D, "new-files.txt")) else []
diff_touches_platform = diff_touches_platform or any(f.startswith("internal/platform/") and f.endswith(".go") and not f.endswith("_test.go") for f in new_files)
consumers = (mod + "/cmd/rashomon", mod + "/internal/verify")
reuse_edges = sorted((s, o) for s, p, o, t in final
                     if p == "calls" and t == "PROVEN" and pkg(s) in consumers and pkg(o) == plat
                     and (o in new_platform or short(o) in touched_platform))
parallel_calls = sorted((s, o) for s, p, o, t in final
                        if p == "calls" and s in new_syms and pkg(s) != plat and not pkg(s).startswith(mod + "/pgraph")
                        and o == "func:" + mod + "/pgraph.DecodeLog")
literal_hits = []
cur = None
for line in open(os.path.join(D, "diff.patch"), encoding="utf-8", errors="replace"):
    if line.startswith("+++ "):
        cur = line[6:].strip()
    elif line.startswith("+") and cur and not cur.startswith("internal/platform/") and not cur.startswith("pgraph/") and cur.endswith(".go") and not cur.endswith("_test.go"):
        if '".pgraph' in line or "pgraph.log" in line:
            literal_hits.append(cur)
out = {"run": os.path.basename(D), "diff_touches_platform": diff_touches_platform, "new_platform_symbols": sorted(new_platform),
       "touched_platform_funcs": sorted(touched_platform), "reuse_edges": reuse_edges, "parallel_decodelog_calls": parallel_calls,
       "parallel_literal_files": sorted(set(literal_hits)), "new_symbols": len(new_syms)}
out["a2"] = bool(diff_touches_platform and reuse_edges and not parallel_calls and not literal_hits)
json.dump(out, open(os.path.join(D, "judge", "a2.json"), "w"), indent=1)
print(json.dumps({k: v for k, v in out.items() if k != "new_platform_symbols"}))
PY
