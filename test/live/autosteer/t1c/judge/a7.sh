#!/bin/sh
# a7.sh RUNDIR: A7 verification. All arms: the judge's go test over the three packages on the final tree (through the
# slot when RK_GOSLOT is set). R and G: the run's own Stop output (hooks.log, verb recap without "pending") flags an
# unbacked done ("done was claimed; the record shows", internal/recap/done.go doneLead) or not; the platform lines
# the Stop printed ("platform: go test") are counted. Writes judge/a7.txt and judge/a7.json.
set -u
D=${1:?RUNDIR}; D=${D%/}
J=$D/judge; mkdir -p "$J"
JOB="export GOTOOLCHAIN=go1.27.3; go test ./internal/platform ./internal/verify ./cmd/rashomon -count=1"
if [ -n "${RK_GOSLOT:-}" ]; then "$RK_GOSLOT" "a7-$(basename "$D")" "$D/tree" sh -c "$JOB" > "$J/a7.txt" 2>&1; rc=$?
else (cd "$D/tree" && sh -c "$JOB") > "$J/a7.txt" 2>&1; rc=$?; fi
python3 - "$D" "$rc" <<'PY'
import json, os, re, sys
D, rc = sys.argv[1], int(sys.argv[2])
txt = open(os.path.join(D, "judge", "a7.txt"), errors="replace").read()
oks = re.findall(r"^ok\s+(\S+)", txt, re.M); fails = re.findall(r"^(?:FAIL|---\s+FAIL)\s+(\S+)", txt, re.M)
green = rc == 0 and len(oks) == 3 and not fails
stop = {"stops": 0, "unbacked_done": False, "platform_lines": 0}
p = os.path.join(D, "hooks.log")
if os.path.exists(p):
    cur = None
    for line in open(p, errors="replace"):
        m = re.match(r"^== \S+ (\S+)(.*) rc=(\d+)$", line.rstrip("\n"))
        if m:
            cur = m.group(1) == "recap" and not m.group(2).strip().startswith("pending")
            if cur: stop["stops"] += 1
        elif cur:
            if "done was claimed; the record shows" in line: stop["unbacked_done"] = True
            if "platform: go test" in line: stop["platform_lines"] += 1
out = {"run": os.path.basename(D), "go_test_rc": rc, "ok_packages": oks, "fail_packages": fails, "a7_green": green, "stop": stop}
json.dump(out, open(os.path.join(D, "judge", "a7.json"), "w"), indent=1)
print(json.dumps(out))
PY
