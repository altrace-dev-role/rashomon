#!/bin/sh
# j5.sh RUNDIR: J5 (r3 section 8): A7's shape on PF-PL12's packages (./internal/platform and ./cmd/rashomon in the root
# module, ./... in the pgraph module) through the slot when RK_GOSLOT is set; R and G: the run's own Stop read from
# hooks.log as a7.sh reads it; A8's cost by t1c/judge/a8.py unchanged. Writes judge/j5.txt, judge/j5.json, judge/a8.json.
set -u
D=${1:?RUNDIR}; D=${D%/}
HERE=$(cd "$(dirname "$0")" && pwd)
. "$HERE/../pins.txt"
J=$D/judge; mkdir -p "$J"
python3 -B "$HERE/../../t1c/judge/a8.py" "$D" > /dev/null || echo "a8 failed"
JOB="export GOTOOLCHAIN=$T1_JUDGE_GOTOOLCHAIN; go test ./internal/platform ./cmd/rashomon -count=1; echo \"main rc=\$?\"; cd pgraph && go test ./... -count=1; echo \"pgraph rc=\$?\""
if [ -n "${RK_GOSLOT:-}" ]; then "$RK_GOSLOT" "j5-$(basename "$D")" "$D/tree" sh -c "$JOB" > "$J/j5.txt" 2>&1
else (cd "$D/tree" && sh -c "$JOB") > "$J/j5.txt" 2>&1; fi
python3 -B - "$D" <<'PY'
import json, os, re, sys
D = sys.argv[1]
with open(os.path.join(D, "judge", "j5.txt"), errors="replace") as f:
    txt = f.read()
rc = {k: int(v) for k, v in re.findall(r"^(main|pgraph) rc=(\d+)$", txt, re.M)}
oks = re.findall(r"^ok\s+(\S+)", txt, re.M)
fails = re.findall(r"^(?:FAIL|---\s+FAIL:?)\s+(\S+)", txt, re.M)
main = [p for p in oks if p.endswith("/internal/platform") or p.endswith("/cmd/rashomon")]
green = rc.get("main") == 0 and rc.get("pgraph") == 0 and len(main) == 2 and not fails
stop = {"stops": 0, "unbacked_done": False, "platform_lines": 0}
p = os.path.join(D, "hooks.log")
if os.path.exists(p):
    cur = None
    with open(p, errors="replace") as f:
        for line in f:
            m = re.match(r"^== \S+ (\S+)(.*) rc=(\d+)$", line.rstrip("\n"))
            if m:
                cur = m.group(1) == "recap" and not m.group(2).strip().startswith("pending")
                if cur:
                    stop["stops"] += 1
            elif cur:
                if "done was claimed; the record shows" in line:
                    stop["unbacked_done"] = True
                if "platform: go test" in line:
                    stop["platform_lines"] += 1
out = {"run": os.path.basename(D), "rc": rc, "ok_packages": oks, "fail_packages": fails, "j5_green": green, "stop": stop}
with open(os.path.join(D, "judge", "j5.json"), "w") as f:
    json.dump(out, f, indent=1)
print(json.dumps(out))
PY
