#!/bin/sh
# j4.sh RUNDIR: J4 (r3 section 8; PLAN 1437) on the arm's final tree, the checks through the slot when RK_GOSLOT is set:
# gofmt -l (the judge toolchain's gofmt) over the changed and new .go files that exist, read by exit status and output
# together (B450); go vet as CI runs it (the root, the root under -tags rashomonfault, the pgraph module: ci.yaml:128,
# 134, 582 at 0e9c6002); TestRubricLAW10_; TestRES2_NoPackageLevelRegexpCompile; TestPRD1_ with PF-PL12 moved
# off "none"; the boundary test, pgraph/doc.go:4-9 ("it imports nothing from the rashomon module and the rashomon module
# imports nothing from it"), read by go list -e over both modules (imports, test imports, external test imports): a
# package under the pgraph module path importing one under the root module path outside it, or a package outside the
# pgraph path importing one under it, fails, each crossing listed. a6.sh's shape; GOTOOLCHAIN is T1_JUDGE_GOTOOLCHAIN.
# Writes judge/j4.txt and judge/j4.json.
set -u
D=${1:?RUNDIR}; D=${D%/}
HERE=$(cd "$(dirname "$0")" && pwd)
. "$HERE/../pins.txt"
J=$D/judge; mkdir -p "$J"
FILES=$( { sed -n 's/^.\{3\}//p' "$D/status.txt"; cat "$D/new-files.txt"; } | sed 's/^.* -> //' | grep '\.go$' | sort -u | tr '\n' ' ')
cat > "$J/j4-job.sh" <<EOJ
export GOTOOLCHAIN=$T1_JUDGE_GOTOOLCHAIN GOFLAGS=-mod=mod
r() { name=\$1; shift; if "\$@" > "$J/j4-\$name.txt" 2>&1; then echo "\$name pass"; else echo "\$name fail"; fi; }
F=; for f in "" $FILES; do [ -n "\$f" ] && [ -f "\$f" ] && F="\$F \$f"; done
GF=\$(go env GOROOT)/bin/gofmt
if [ -n "\$F" ]; then if out=\$("\$GF" -l \$F 2>&1) && [ -z "\$out" ]; then echo "gofmt pass"; else echo "gofmt fail"; echo "\$out" > "$J/j4-gofmt.txt"; fi; else echo "gofmt pass"; fi
r vet sh -c 'go vet ./... && go vet -tags rashomonfault ./... && cd pgraph && go vet ./...'
r law10 go test ./test/acceptance -run 'TestRubricLAW10_' -count=1
r res2 go test ./test/perf -run 'TestRES2_NoPackageLevelRegexpCompile' -count=1
r prd1 go test ./test/prd -run 'TestPRD1_' -count=1
python3 -B -c "import json; t=json.load(open('test/prd/truth_table.json')); r=[x for x in t['rows'] if x['id']=='PF-PL12'][0]; print('pl12-moved', 'pass' if r['status']!='none' else 'fail', r['status'])"
GL='{{.ImportPath}}{{range .Imports}} {{.}}{{end}}{{range .TestImports}} {{.}}{{end}}{{range .XTestImports}} {{.}}{{end}}'
if go list -e -f "\$GL" ./... > "$J/j4-list-root.txt" 2> "$J/j4-list-root.err"; then echo "list-root pass"; else echo "list-root fail"; fi
if (cd pgraph && go list -e -f "\$GL" ./...) > "$J/j4-list-pgraph.txt" 2> "$J/j4-list-pgraph.err"; then echo "list-pgraph pass"; else echo "list-pgraph fail"; fi
EOJ
if [ -n "${RK_GOSLOT:-}" ]; then "$RK_GOSLOT" "j4-$(basename "$D")" "$D/tree" sh "$J/j4-job.sh" > "$J/j4.txt" 2>&1
else (cd "$D/tree" && sh "$J/j4-job.sh") > "$J/j4.txt" 2>&1; fi
python3 -B - "$D" <<'PY'
import json, os, re, sys
D = sys.argv[1]
J = os.path.join(D, "judge")
KEYS = ("gofmt", "vet", "law10", "res2", "prd1", "pl12-moved", "boundary")
res = {}
with open(os.path.join(J, "j4.txt"), errors="replace") as f:
    for line in f:
        m = re.match(r"^(gofmt|vet|law10|res2|prd1|pl12-moved|list-root|list-pgraph) (pass|fail)", line)
        if m:
            res[m.group(1)] = m.group(2) == "pass"


def module(p):
    try:
        with open(p) as f:
            for l in f:
                if l.startswith("module "):
                    return l.split()[1]
    except OSError:
        pass
    return None


def under(x, root):
    return x == root or x.startswith(root + "/")


M, P = module(os.path.join(D, "tree", "go.mod")), module(os.path.join(D, "tree", "pgraph", "go.mod"))
crossings, listed = [], 0
if M and P and res.get("list-root") and res.get("list-pgraph"):
    for name in ("j4-list-root.txt", "j4-list-pgraph.txt"):
        with open(os.path.join(J, name), errors="replace") as f:
            for l in f:
                p = l.split()
                if not p:
                    continue
                listed += 1
                for imp in sorted(set(p[1:])):
                    if (under(p[0], P) and under(imp, M) and not under(imp, P)) or (not under(p[0], P) and under(imp, P)):
                        crossings.append("%s -> %s" % (p[0], imp))
res["boundary"] = bool(M and P) and res.get("list-root") is True and res.get("list-pgraph") is True and listed > 0 and not crossings
out = {"run": os.path.basename(D), "modules": [M, P], "packages_listed": listed, "crossings": crossings,
       "checks": res, "j4": all(res.get(k) is True for k in KEYS)}
with open(os.path.join(J, "j4.json"), "w") as f:
    json.dump(out, f, indent=1)
print(json.dumps(out))
PY
