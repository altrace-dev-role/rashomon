#!/bin/sh
# a3.sh RUNDIR: A3 consumers by behaviour, A4 correctness and A5 unknown-never-zero (section 7.4), by driving a binary
# built from the arm's final tree through the recorded hook sequence against the pre-registered fixtures.
#   For each fixture (pgraph-synced copy): install the binary's hooks into a throwaway config dir (watch --yes,
#   consent --yes, a fresh RASHOMON_HOME), then feed SessionStart (startup), UserPromptSubmit, one PostToolUse Edit per
#   symbol file, and Stop, with cwd the fixture root; keep every hook's stdout. The renderers read are the ones the
#   platform read appears in on the pinned tree: the PostToolUse blast line and the Stop's platform line (verified
#   2026-11-04: rashomon report has no platform section, so it is captured as information only).
#   Parse rule (pre-registered): on a hook stdout line that names the symbol (its short name with its type for
#   methods), drop "in N package(s)" counts, collect the remaining integers; two integers = (proven, inclusive);
#   one integer followed or preceded by an unknown marker (unknown, ?, +, "or more", "at least") = (n, unknown).
#   A4 match = the pair equals oracle.py's (proven, inclusive); A5 pass = the reflect symbol's inclusive value is not a
#   bare 0 (an unknown marker, or a number other than 0, passes); anything else is unparsed and goes to a blinded read.
# Environment: RK_PGRAPH (pinned), RK_FIXTURES (where the judge prepares dispatch/ and dispatch-reflect/ as git
# repositories, synced; the blast read and the Stop find the project by its git root), RK_GOSLOT (optional),
# RK_GO_TOOLCHAIN_BIN (go >= 1.26 for pgraph), RK_A3_BIN (optional: skip the build, use this binary), RK_AI_AGENT
# (the AI_AGENT value Claude Code sets for hook commands; the hooks identify the harness by it, with CLAUDECODE=1 and
# CLAUDE_CODE_ENTRYPOINT=cli; default claude-code_<RK_CLAUDE_VERSION>_harness, steer.ClientVersion's form).
set -u
D=${1:?RUNDIR}; D=${D%/}
HERE=$(cd "$(dirname "$0")" && pwd)
: "${RK_PGRAPH:?RK_PGRAPH}"; : "${RK_FIXTURES:?RK_FIXTURES}"
RK_AI_AGENT=${RK_AI_AGENT:-claude-code_${RK_CLAUDE_VERSION:-2-1-280}_harness}
. "$HERE/../pins.env"
J=$D/judge; mkdir -p "$J"
BIN=${RK_A3_BIN:-}
if [ -z "$BIN" ]; then
  BIN=$J/rashomon-arm
  JOB="export GOTOOLCHAIN=go1.27.3; go build -o $BIN ./cmd/rashomon"
  if [ -n "${RK_GOSLOT:-}" ]; then "$RK_GOSLOT" "a3-build-$(basename "$D")" "$D/tree" sh -c "$JOB" > "$J/a3-build.txt" 2>&1
  else (cd "$D/tree" && sh -c "$JOB") > "$J/a3-build.txt" 2>&1; fi
  [ -x "$BIN" ] || { echo "a3: build failed"; tail -20 "$J/a3-build.txt"; python3 - "$D" <<'PY'
import json, os, sys
D = sys.argv[1]; json.dump({"run": os.path.basename(D), "build": "failed", "a3": "unparsed", "a4": "unparsed", "a5": "unparsed"}, open(os.path.join(D, "judge", "a3.json"), "w"), indent=1)
PY
  exit 1; }
fi
prepare() { # prepare FIXTURE SOURCE : a git repository holding the fixture, pgraph-synced; kept across runs under RK_FIXTURES
  fx=$1; src=$2; dst=$RK_FIXTURES/$fx
  [ -f "$dst/.pgraph/pgraph.walk" ] && [ -d "$dst/.git" ] && return 0
  rm -rf "$dst"; mkdir -p "$dst"
  case "$src" in
    sha:*) git -C "$D/tree" archive "${src#sha:}" "pgraph/testdata/$fx" | tar -x -C "$dst" --strip-components 3 -f - ;;   # pgraph/testdata/dispatch/ stripped: the module lands at $dst
    *) (cd "$src" && tar -cf - .) | (cd "$dst" && tar -xf -) ;;
  esac
  # The Stop's platform row is judged only for a package whose directory holds a _test.go and only under a go test
  # gate the project's own files declare (verify.platformObligations: Facts known, goTestGate, Tests; the gate from
  # go.mod alone stands for a convention and is excluded). The fixture therefore carries one CI workflow step and one
  # empty test file per package; neither calls an oracle symbol, so the oracle numbers are unchanged (verified by
  # oracle.py on the synced copy). Pre-registered before any run.
  mkdir -p "$dst/.github/workflows"
  printf 'name: ci\non: [push]\njobs:\n  test:\n    runs-on: ubuntu-latest\n    steps:\n      - run: go test ./...\n' > "$dst/.github/workflows/ci.yml"
  for d in $(find "$dst" -name '*.go' -not -name '*_test.go' -not -path '*/.git/*' -exec dirname {} \; | sort -u); do
    [ -f "$d/fixture_test.go" ] && continue
    pkg=$(grep -h -m1 '^package ' "$d"/*.go | head -1 | awk '{print $2}')
    printf 'package %s\n\nimport "testing"\n\n// TestFixture gives the package a test file; the platform read owes a go test only to a package that has one.\nfunc TestFixture(t *testing.T) {}\n' "$pkg" > "$d/fixture_test.go"
  done
  (cd "$dst" && git init -q && git add -A && git -c user.name=judge -c user.email=judge@localhost commit -q -m "T1 fixture $fx") || return 1
  SYNC="export PATH=${RK_GO_TOOLCHAIN_BIN:-/nonexistent}:\$PATH; $RK_PGRAPH sync ."
  if [ -n "${RK_GOSLOT:-}" ]; then "$RK_GOSLOT" "a3-sync-$fx" "$dst" sh -c "$SYNC" > "$J/a3-$fx-sync.txt" 2>&1
  else (cd "$dst" && sh -c "$SYNC") > "$J/a3-$fx-sync.txt" 2>&1; fi
  [ -f "$dst/.pgraph/pgraph.walk" ] || { echo "a3: sync of $fx failed"; cat "$J/a3-$fx-sync.txt"; return 1; }
}
mkdir -p "$RK_FIXTURES"
prepare dispatch "sha:$T1_TREE_SHA" || exit 2
prepare dispatch-reflect "$HERE/../testdata/dispatch-reflect" || exit 2
replay() { # replay FIXTURE TAG SID FILE : one install, one session, one Edit of FILE, then Stop; stdout of every hook kept
  # One symbol's file per session: the Stop prints at most two package lines (recap.maxPlatformLines) and sends the
  # rest to the ledger, so a three-package session would hide the third symbol from an arm that keeps the cap.
  fx=$1; tag=$2; sid=$3; f=$4; id=$fx-$tag
  root=$(cd "$RK_FIXTURES/$fx" && pwd -P)   # the physical path: Claude Code reports cwd and file paths with symlinks resolved
  cfg=$J/a3-$id-cfg; home=$J/a3-$id-home; rm -rf "$cfg" "$home"; mkdir -p "$cfg" "$home"; echo '{}' > "$cfg/settings.json"
  env -u CLAUDECODE -u CLAUDE_CODE_ENTRYPOINT -u AI_AGENT CLAUDE_CONFIG_DIR=$cfg RASHOMON_HOME=$home "$BIN" watch --yes > "$J/a3-$id-install.txt" 2>&1 || { echo "a3: watch --yes failed for $id"; cat "$J/a3-$id-install.txt"; return 1; }
  env -u CLAUDECODE -u CLAUDE_CODE_ENTRYPOINT -u AI_AGENT CLAUDE_CONFIG_DIR=$cfg RASHOMON_HOME=$home "$BIN" consent --yes >> "$J/a3-$id-install.txt" 2>&1
  tp=$J/a3-$id-transcript.jsonl; : > "$tp"
  hook() { # hook EVENT JSON : run the installed command for EVENT with JSON on stdin from the fixture root
    ev=$1; body=$2
    cmd=$(python3 -c 'import json,sys; d=json.load(open(sys.argv[1])); e=(d.get("hooks") or {}).get(sys.argv[2]) or []; print(e[0]["hooks"][0]["command"] if e else "")' "$cfg/settings.json" "$ev")
    [ -n "$cmd" ] || { echo "a3: no $ev hook installed"; return 1; }
    printf '%s' "$body" | (cd "$root" && env CLAUDECODE=1 CLAUDE_CODE_ENTRYPOINT=cli AI_AGENT=$RK_AI_AGENT CLAUDE_CONFIG_DIR=$cfg RASHOMON_HOME=$home RASHOMON_HOLDOUT=off CI= CLAUDE_PROJECT_DIR=$root sh -c "$cmd") > "$J/a3-$id-$ev.txt" 2>> "$J/a3-$id-stderr.txt"
    echo "$ev rc=$?" >> "$J/a3-$id-events.txt"
  }
  pid=p1-$id
  hook SessionStart "{\"hook_event_name\":\"SessionStart\",\"session_id\":\"$sid\",\"prompt_id\":\"$pid\",\"transcript_path\":\"$tp\",\"cwd\":\"$root\",\"source\":\"startup\"}"
  hook UserPromptSubmit "{\"hook_event_name\":\"UserPromptSubmit\",\"session_id\":\"$sid\",\"prompt_id\":\"$pid\",\"transcript_path\":\"$tp\",\"cwd\":\"$root\",\"prompt\":\"touch the fixture\"}"
  # the Edit lands as a no-op content edit so the fixture's digests stay as synced (the read is of the symbols' callers)
  hook PreToolUse "{\"hook_event_name\":\"PreToolUse\",\"session_id\":\"$sid\",\"prompt_id\":\"$pid\",\"transcript_path\":\"$tp\",\"cwd\":\"$root\",\"permission_mode\":\"acceptEdits\",\"tool_name\":\"Edit\",\"tool_use_id\":\"toolu_a3_$tag\",\"tool_input\":{\"file_path\":\"$root/$f\",\"old_string\":\"package\",\"new_string\":\"package\"}}"
  hook PostToolUse "{\"hook_event_name\":\"PostToolUse\",\"session_id\":\"$sid\",\"prompt_id\":\"$pid\",\"transcript_path\":\"$tp\",\"cwd\":\"$root\",\"permission_mode\":\"acceptEdits\",\"tool_name\":\"Edit\",\"tool_use_id\":\"toolu_a3_$tag\",\"tool_input\":{\"file_path\":\"$root/$f\",\"old_string\":\"package\",\"new_string\":\"package\"},\"tool_response\":{\"filePath\":\"$root/$f\",\"success\":true}}"
  hook Stop "{\"hook_event_name\":\"Stop\",\"session_id\":\"$sid\",\"prompt_id\":\"$pid\",\"transcript_path\":\"$tp\",\"cwd\":\"$root\",\"stop_hook_active\":false,\"last_assistant_message\":\"done\"}"
  env -u CLAUDECODE -u CLAUDE_CODE_ENTRYPOINT -u AI_AGENT CLAUDE_CONFIG_DIR=$cfg RASHOMON_HOME=$home "$BIN" report --session "$sid" > "$J/a3-$id-report.txt" 2>&1
  cp "$cfg/settings.json" "$J/a3-$id-settings.json"
}
# one session per oracle symbol (t1/oracle-symbols.txt: fixture, id, file, shape); tag = the file's directory
replay dispatch shape "a3000000-0000-4000-8000-000000000001" shape/shape.go || exit 1
replay dispatch app   "a3000000-0000-4000-8000-000000000003" app/app.go || exit 1
replay dispatch extra "a3000000-0000-4000-8000-000000000004" extra/extra.go || exit 1
replay dispatch-reflect extra "a3000000-0000-4000-8000-000000000002" extra/byname.go || exit 1
python3 "$HERE/oracle.py" "$RK_FIXTURES/dispatch" "$RK_FIXTURES/dispatch-reflect" > "$J/oracle.json" || { echo "a3: oracle shapes failed"; exit 2; }
RK_T1_DIR="$HERE/.." python3 - "$D" "$J/oracle.json" <<'PY'
import glob, json, os, re, sys
D, oracle_path = sys.argv[1:3]
J = os.path.join(D, "judge")
oracle = json.load(open(oracle_path))
UNKNOWN = re.compile(r"unknown|\?|\+|or more|at least", re.I)
def short_names(sym):
    tail = sym.split(":", 1)[1].rpartition("/")[2]          # app.Total | shape.Circle.Area
    parts = tail.split(".")
    return [".".join(parts[1:])] + ([parts[-1]] if len(parts) > 2 else [])
def parse_line(line):
    s = re.sub(r"\bin \d+ packages?\b", " ", line)
    ints = [int(x) for x in re.findall(r"(?<![\w.])\d+(?![\w.])", s)]
    if len(ints) == 2:
        return ints[0], ints[1]
    if len(ints) == 1 and UNKNOWN.search(s):
        return ints[0], "unknown"
    return None
symfile = {}
for line in open(os.path.join(os.environ["RK_T1_DIR"], "oracle-symbols.txt")):
    cols = line.rstrip("\n").split("\t")
    if len(cols) >= 4 and not line.startswith("#"):
        symfile[cols[1]] = cols[2]
def renderer_lines(fx, sym):
    tag = os.path.dirname(symfile[sym]).split("/")[-1]; id_ = "%s-%s" % (fx, tag)
    out = {}
    for r, ev in (("post", "PostToolUse"), ("stop", "Stop")):
        p = os.path.join(J, "a3-%s-%s.txt" % (id_, ev))
        out[r] = open(p, errors="replace").read().splitlines() if os.path.exists(p) else []
    p = os.path.join(J, "a3-%s-report.txt" % id_)
    out["report"] = open(p, errors="replace").read().splitlines() if os.path.exists(p) else []
    return out
res = {}
for sym, o in oracle.items():
    fx = o["fixture"]; lines = renderer_lines(fx, sym); names = short_names(sym)
    per = {}
    for r, ls in lines.items():
        hits = [l for l in ls if any(re.search(r"(?<![\w.])" + re.escape(n) + r"(?![\w])", l) for n in names)]
        parsed = [parse_line(l) for l in hits]
        parsed = [p for p in parsed if p is not None]
        per[r] = {"lines_naming_symbol": len(hits), "parsed": parsed[:1]}
    rendered = {r: v["parsed"][0] for r, v in per.items() if v["parsed"] and r in ("post", "stop")}
    verdict = {"renderers": per, "a3": "both" if len(rendered) == 2 else ("one" if rendered else "neither")}
    if o["shape"] == "reflect":
        vals = list(rendered.values())
        verdict["a5"] = "unparsed" if not vals else ("pass" if all(v[1] != 0 for v in vals) else "fail")
    else:
        vals = list(rendered.values())
        verdict["a4"] = "unparsed" if not vals else ("match" if all(v == (o["proven"], o["inclusive"]) for v in vals) else "mismatch")
    res[sym] = verdict
a4 = [v["a4"] for v in res.values() if "a4" in v]; a5 = [v["a5"] for v in res.values() if "a5" in v]
out = {"run": os.path.basename(D), "symbols": res,
       "a3": "both" if all(v["a3"] == "both" for v in res.values()) else ("neither" if all(v["a3"] == "neither" for v in res.values()) else "mixed"),
       "a4": "match" if a4 and all(x == "match" for x in a4) else ("unparsed" if all(x == "unparsed" for x in a4) else "mismatch"),
       "a5": a5[0] if a5 else "unparsed"}
json.dump(out, open(os.path.join(J, "a3.json"), "w"), indent=1)
print(json.dumps({k: v for k, v in out.items() if k != "symbols"}))
for sym, v in res.items():
    print(" ", sym.split("/")[-1], {r: x["parsed"] for r, x in v["renderers"].items()}, v.get("a4", v.get("a5")))
PY
