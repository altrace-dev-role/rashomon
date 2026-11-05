#!/bin/sh
# a6.sh RUNDIR: A6 architecture compliance on the arm's final tree, every check through the slot when RK_GOSLOT is set.
#   gofmt -l over the changed and new .go files is empty; go vet ./...; TestRubricLAW10_ (test/acceptance);
#   TestRES2_NoPackageLevelRegexpCompile (test/perf); TestPRD1_ (test/prd) with the PF-PL7 row moved off "none";
#   the disclosure package's tests. Writes judge/a6.txt (full output) and judge/a6.json (pass or fail per check).
#   T1b: gofmt is judged by its exit status and its output together (B450: T1's a6 read the output alone, so a missing
#   gofmt printed pass); nothing else differs from T1's a6.sh.
set -u
D=${1:?RUNDIR}; D=${D%/}
mkdir -p "$D/judge"
FILES=$( { sed -n 's/^.\{3\}//p' "$D/status.txt"; cat "$D/new-files.txt"; } | grep '\.go$' | sort -u | tr '\n' ' ')
cat > "$D/judge/a6-job.sh" <<EOJ
export GOTOOLCHAIN=go1.27.3 GOFLAGS=-mod=mod
r() { name=\$1; shift; if "\$@" > "$D/judge/a6-\$name.txt" 2>&1; then echo "\$name pass"; else echo "\$name fail"; fi; }
if [ -n "$FILES" ]; then if out=\$(gofmt -l $FILES 2>&1) && [ -z "\$out" ]; then echo "gofmt pass"; else echo "gofmt fail"; echo "\$out" > "$D/judge/a6-gofmt.txt"; fi; else echo "gofmt pass"; fi
r vet go vet ./...
r law10 go test ./test/acceptance -run 'TestRubricLAW10_' -count=1
r res2 go test ./test/perf -run 'TestRES2_NoPackageLevelRegexpCompile' -count=1
r prd1 go test ./test/prd -run 'TestPRD1_' -count=1
python3 -c "import json,sys; t=json.load(open('test/prd/truth_table.json')); r=[x for x in t['rows'] if x['id']=='PF-PL7'][0]; print('pl7-moved', 'pass' if r['status']!='none' else 'fail', r['status'])"
r disclose go test ./internal/disclose -count=1
EOJ
if [ -n "${RK_GOSLOT:-}" ]; then "$RK_GOSLOT" "a6-$(basename "$D")" "$D/tree" sh "$D/judge/a6-job.sh" > "$D/judge/a6.txt" 2>&1
else (cd "$D/tree" && sh "$D/judge/a6-job.sh") > "$D/judge/a6.txt" 2>&1; fi
python3 - "$D" <<'PY'
import json, os, re, sys
D = sys.argv[1]
res = {}
for line in open(os.path.join(D, "judge", "a6.txt")):
    m = re.match(r"^(gofmt|vet|law10|res2|prd1|pl7-moved|disclose) (pass|fail)", line)
    if m:
        res[m.group(1)] = m.group(2) == "pass"
out = {"run": os.path.basename(D), "checks": res, "a6": bool(res) and all(res.get(k) for k in ("gofmt", "vet", "law10", "res2", "prd1", "pl7-moved", "disclose"))}
json.dump(out, open(os.path.join(D, "judge", "a6.json"), "w"), indent=1)
print(json.dumps(out))
PY
