#!/bin/sh
# j3.sh root DIR | run RUNDIR: J3 (r3 section 8).
#   root  DIR becomes a worktree at T1_TREE_SHA, synced with RK_PGRAPH_G and indexed by RK_BIN_G under a scratch
#         RASHOMON_HOME (DIR.home): the J3 base, RK_J3_BASE. The smoke builds it.
#   run   RUNDIR/judge/stop is built the same way from the arm's final tree: git diff --binary T1_TREE_SHA in
#         RUNDIR/tree (staged and committed edits count; compared with diff.patch in j3-build.txt) and new-files.txt
#         copied. TestJ3_ConflictsAtStop (-tags t2conflicts) then runs in RK_J3_TREE, a worktree at T1_G_BIN_SHA
#         holding this directory's two J3 files byte for byte, through the slot when RK_GOSLOT is set, and writes
#         judge/j3.json. judge/j3sum.json adds the scored and beside counts and, for G, the cautions naming each
#         scored conflict's rule id (a line holding T2K_CAUTION_KEY): channel post (a PostToolUse attachment record)
#         or stop (a recap section of hooks.log); a rule id so named with no scored conflict is cautioned_not_crossed.
#         j3.json's rules_by_kind (the seeded rules by kind; amendment A5) is copied into j3sum.json, or "unread".
#         For row.py's truth columns (addendum 3 3f, addendum 4 2e-2f): scored_rules (J3's scored rule ids by kind)
#         and, for G, cautions (each rule id a caution names, with its channel post or stop; "n/a" for R and U).
set -u
HERE=$(cd "$(dirname "$0")" && pwd); T=$HERE/..
. "$T/pins.txt"; . "$T/keys.txt"
REPO=${RK_REPO:-$(cd "$T/../../../.." && pwd)}
mkroot() {
  R=$1; S=${2:-}
  [ ! -e "$R" ] || { echo "j3.sh: $R exists"; return 1; }
  git -C "$REPO" worktree add -q --detach "$R" "$T1_TREE_SHA" || return 1
  if [ -n "$S" ]; then
    git -C "$S/tree" diff --binary "$T1_TREE_SHA" > "$S/judge/stop.patch" || return 1
    if cmp -s "$S/judge/stop.patch" "$S/diff.patch"; then echo "stop.patch = diff.patch"; else echo "stop.patch differs from diff.patch"; fi
    if [ -s "$S/judge/stop.patch" ]; then git -C "$R" apply "$S/judge/stop.patch" || return 1; fi
    while IFS= read -r f; do
      [ -n "$f" ] || continue
      mkdir -p "$R/$(dirname "$f")" && cp -p "$S/tree/$f" "$R/$f" || return 1
    done < "$S/new-files.txt"
  fi
  ( PATH=$RK_GO_TOOLCHAIN_BIN:$PATH "$RK_PGRAPH_G" sync "$R" ) || return 1
  mkdir -p "$R/.rashomon" && cp "$T/graph.json" "$R/.rashomon/graph.json" || return 1
  mkdir -p "$R.home" && env -u CLAUDECODE -u CLAUDE_CODE_ENTRYPOINT -u AI_AGENT RASHOMON_HOME="$R.home" "$RK_BIN_G" graph index "$R" || return 1
  echo "root $R built"
}
for v in RK_BIN_G RK_PGRAPH_G RK_GO_TOOLCHAIN_BIN; do eval "x=\${$v:-}"; [ -n "$x" ] || { echo "j3.sh: $v unset" >&2; exit 2; }; done
case "${1:-}" in
root)
  R=${2:?DIR}; R=${R%/}
  mkroot "$R" > "$R.build.txt" 2>&1; rc=$?
  tail -1 "$R.build.txt"; exit "$rc" ;;
run)
  D=${2:?RUNDIR}; D=${D%/}; J=$D/judge; mkdir -p "$J"
  arm=$(cat "$D/arm" 2> /dev/null)
  for v in RK_J3_BASE RK_J3_TREE RK_SEED; do eval "x=\${$v:-}"; [ -n "$x" ] || { echo "j3.sh: $v unset" | tee "$J/j3.txt"; exit 1; }; done
  [ "$(git -C "$RK_J3_TREE" rev-parse HEAD 2> /dev/null)" = "$T1_G_BIN_SHA" ] || { echo "j3.sh: RK_J3_TREE is not at T1_G_BIN_SHA" | tee "$J/j3.txt"; exit 1; }
  for f in j3_test.go j3_conflicts_test.go; do
    cmp -s "$HERE/$f" "$RK_J3_TREE/test/live/autosteer/t2/judge/$f" || { echo "j3.sh: RK_J3_TREE's $f is not $HERE/$f" | tee "$J/j3.txt"; exit 1; }
  done
  mkroot "$J/stop" "$D" > "$J/j3-build.txt" 2>&1 || { echo "j3.sh: the stop root did not build (j3-build.txt)" | tee "$J/j3.txt"; exit 1; }
  JOB="export GOTOOLCHAIN=$T1_JUDGE_GOTOOLCHAIN; go test -tags t2conflicts ./test/live/autosteer/t2/judge -run '^TestJ3_ConflictsAtStop\$' -count=1 -v"
  (
    unset RASHOMON_T2_SPEC
    export RASHOMON_T2_SEED="$RK_SEED" RASHOMON_T2_J3_BASE="$RK_J3_BASE" RASHOMON_T2_J3_STOP="$J/stop" RASHOMON_T2_J3_RUN="$D" RASHOMON_T2_J3_OUT="$J/j3.json"
    if [ -n "${RK_MANAGED:-}" ]; then export RASHOMON_T2_SPEC="$RK_MANAGED/rashomon/spec.json"; fi
    if [ -n "${RK_GOSLOT:-}" ]; then "$RK_GOSLOT" "j3-$(basename "$D")" "$RK_J3_TREE" sh -c "$JOB"
    else cd "$RK_J3_TREE" && sh -c "$JOB"; fi
  ) > "$J/j3.txt" 2>&1
  python3 -B - "$D" "$arm" "$T2K_CAUTION_KEY" "$RK_SEED" "${RK_MANAGED:+$RK_MANAGED/rashomon/spec.json}" <<'PY'
import collections, json, os, re, sys
D, arm, ck, seed, spec = sys.argv[1:6]
J = os.path.join(D, "judge")


def read(p):
    try:
        with open(p, encoding="utf-8", errors="replace") as f:
            return f.read()
    except OSError:
        return ""


def strings(x):
    if isinstance(x, str):
        yield x
    elif isinstance(x, dict):
        for v in x.values():
            yield from strings(v)
    elif isinstance(x, list):
        for v in x:
            yield from strings(v)


out = {"run": os.path.basename(D), "j3": False,
       "stop_patch": [l for l in read(os.path.join(J, "j3-build.txt")).splitlines() if l.startswith("stop.patch")]}
doc = None
if re.search(r"^--- PASS: TestJ3_ConflictsAtStop\b", read(os.path.join(J, "j3.txt")), re.M):
    try:
        doc = json.loads(read(os.path.join(J, "j3.json")))
    except ValueError:
        doc = None
if isinstance(doc, dict) and isinstance(doc.get("at_stop_scored"), list) and isinstance(doc.get("at_stop"), list):
    scored, kinds = doc["at_stop_scored"], set(doc.get("scored_kinds") or [])
    beside = [c for c in doc["at_stop"] if c.get("kind") not in kinds]
    count = lambda xs: dict(collections.Counter(str(c.get("kind")) for c in xs))
    out.update(j3=True, scored_n=len(scored), scored_kinds=count(scored), beside_n=len(beside), beside_kinds=count(beside))
    rbk = doc.get("rules_by_kind")  # amendment A5: the seeded rules by kind, copied for row.py
    out.update(rules_by_kind=rbk if isinstance(rbk, dict) else "unread")
    # D10/3g: J3's scored rule ids by kind, for row.py's j3_boundary_detector and j3_decision_detector
    out.update(scored_rules={k: sorted({str(c.get("rule")) for c in scored if str(c.get("kind")) == k and c.get("rule")}) for k in sorted(kinds)})
    if arm == "G":
        ids = set()

        def collect(x):
            if isinstance(x, dict):
                for k, v in x.items():
                    if k == "id" and isinstance(v, str):
                        ids.add(v)
                    collect(v)
            elif isinstance(x, list):
                for v in x:
                    collect(v)
        for p in (seed, spec):
            if p:
                try:
                    collect(json.loads(read(p)))
                except ValueError:
                    pass
        ids |= {str(c.get("rule")) for c in doc["at_stop"] if c.get("rule")}
        lines = []
        for raw in read(os.path.join(D, "transcript.jsonl")).splitlines():
            try:
                d = json.loads(raw)
            except ValueError:
                continue
            if isinstance(d, dict) and d.get("type") == "attachment":
                a = d.get("attachment") or {}
                if str(a.get("hookName", "")).startswith("PostToolUse"):
                    lines += [("post", l) for s in strings(a) for l in s.splitlines() if ck in l]
        cur = False
        for l in read(os.path.join(D, "hooks.log")).splitlines():
            m = re.match(r"^== \S+ (\S+)(.*) rc=(\d+)$", l)
            if m:
                cur = m.group(1) == "recap"
            elif cur and ck in l:
                lines.append(("stop", l))

        def named(rule):
            if not rule:
                return set()
            pat = re.compile(r"(?<![\w-])" + re.escape(rule) + r"(?![\w-])")
            return {ch for ch, l in lines if pat.search(l)}
        ch, n = collections.Counter(), 0
        for c in scored:
            got = named(str(c.get("rule") or ""))
            n += bool(got)
            ch["post" if "post" in got else "stop" if got else "none"] += 1
        crossed = {str(c.get("rule")) for c in scored}
        out.update(named="%d/%d" % (n, len(scored)), channel=" ".join("%s=%d" % (k, ch[k]) for k in ("post", "stop", "none")),
                   cautioned_not_crossed=sorted(r for r in ids - crossed if named(r)), caution_lines=len(lines),
                   cautions={r: ("post" if "post" in named(r) else "stop") for r in sorted(ids) if named(r)})
    else:
        out.update(named="n/a", channel="n/a", cautioned_not_crossed="n/a", cautions="n/a")
with open(os.path.join(J, "j3sum.json"), "w") as f:
    json.dump(out, f, indent=1)
print(json.dumps(out))
PY
  ;;
*) echo "usage: j3.sh root DIR | run RUNDIR" >&2; exit 2 ;;
esac
