#!/bin/sh
# j2probe.sh p1 NAME | p2 NAME: T2 pre-run addendum 5 section 2 (B3; the director's send 132 on QA's 11:40:40Z
# proposal). QA writes it and never launches it: its launch word comes with the director's re-cut word (2c).
#
# P1 (2a): no session, no model, no paid call. G's binary at T1_G_BIN_SHA, run.sh's G setup with RK_SETUP_ONLY=yes
#   (run.sh:132, "setup only, no session"; addendum 5 cites :125, stale), then G's installed hook commands driven
#   directly with harness-shaped payloads, each through hook.sh as a session runs them, so hooks.log has the shape
#   judge/j3.sh reads. S1 (b) only: the managed spec through real discovery (RK_MANAGED, required). S1 (a), a
#   project spec in a scratch copy, is not run: an untrusted repository spec's boundary rules are withheld
#   (internal/graph/engine/rules.go:11-13, :30 at b60d296b), and trust is bound by `rashomon spec trust`, which reads
#   the digest's first 8 characters typed by a person at a terminal and refuses under an agent (cmd/rashomon/spec.go
#   trustSpec); its bindings carry a per-store MAC (D173). (a) awaits the director's ruling.
#   Sequence, one session id: SessionStart (source startup), UserPromptSubmit (t2/prompt.txt, the frozen prompt; the
#   prompt path is where the conflict base is written: recap pending -> preflightNote -> graphBriefWith ->
#   writeConflictBase), then per direction PreToolUse(Edit), the edit applied as an exact-once replace, and
#   PostToolUse(Edit) with its tool_response; then one Stop (stop_hook_active false). Payload fields as the
#   product's own replay builds them (internal/backtest/replay.go payload) plus transcript_path, prompt and source.
#   The hook environment is what Claude Code set for hooks in T1c's G runs (t1c-g18 to g20 hook-env.txt):
#   CLAUDECODE=1, CLAUDE_CODE_ENTRYPOINT=sdk-cli, AI_AGENT=claude-code_<RK_CLAUDE_VERSION>_harness and
#   CLAUDE_PROJECT_DIR=<tree>; hook.sh then exports CLAUDE_CODE_ENTRYPOINT=cli as in every run. CLAUDE_CONFIG_DIR is a
#   scratch directory under the run (p1cfg) holding only the hook.sh-wrapped settings.json, built from the setup's
#   settings.installed.json by wrap.sh's own rewrite, so the hooks read their entries present (internal/hook/
#   coverage.go Resolve) and RK_CONFIG_DIR's settings.json is never touched by the replay; transcript_path names an
#   empty file under p1cfg/projects/ (no session, no transcript).
#   The two seeded edits (written here before any launch; both old strings unique at T1_TREE_SHA 5f851724):
#     pgraph-in   internal/platform/digests.go gains  _ "github.com/altrace-dev-role/rashomon/pgraph"
#     pgraph-out  pgraph/log.go gains                  _ "github.com/altrace-dev-role/rashomon/internal/store"
#   The overlay maps an import by the index root's go.mod module path (internal/hook/overlaywrite.go overlayImports,
#   gopg ImportUnit), so a pgraph file's import of .../rashomon/internal/store maps to unit:go:internal/store although
#   pgraph is its own module.
#   PASS (2a): zero boundary_cross before the edits; exactly one per direction after (conflict_check records, rule ids
#   pgraph-in and pgraph-out), on real files: judge/s1cross.sh reads 0 and 0 before, nontest 1 and 1 after; no go.mod
#   line changed and git diff names the two files alone. Recorded: each caution's channel and rule id, read as j3.sh
#   reads them (a hooks.log section holding T2K_CAUTION_KEY and the rule id; post = the post section, stop = a recap
#   section), with the verb that printed it; none = "caution trigger not available". P1's post channel is hook
#   stdout, delivered and not "reached" (2b: reached is what a model was shown); P2 reads reached.
#   Gates printed in order, so a reading of none names where it stopped: the session_content marker, the conflict
#   base and its rule ids, the overlay files, the conflict_check records.
# P2 (2b, D16 ruled (i)): one capped G session, run.sh G with RK_MAX_TURNS=15 and RK_BUDGET_USD=1.00 (fixed here,
#   never the launcher's) on the frozen prompt, then t2/judge/j2.py (D15': the sealed j2b.py, copied cmp-equal) on
#   the run; its output is RUN/j2b-probe.out (D16' names t2-skel/j2b-probe.out: the director copies it there). A probe,
#   never a replicate: it decides only whether the caution capability is available and whether J2's instrument
#   reads; its cost is printed from result.json.
# Guards (2c, P1 and P2), cheapest first, every refusal rc 2 before anything is written under RK_RUNS/NAME:
#   RK_T2_J2PROBE_GO=yes; the required environment (RK_RUNS RK_GOCACHE_BASE RK_BIN_G RK_PGRAPH RK_PGRAPH_G
#   RK_GO_TOOLCHAIN_BIN RK_SEED RK_KNOWLEDGE RK_METER_FILE); RASHOMON_MANAGED_SETTINGS_PATH unset in the launching
#   shell; pins.txt sourced as run.sh sources it, T1_G_BIN_SHA and T1_G_PGRAPH_SHA ruled (not TBD); RK_CONFIG_DIR, the
#   CLI and its version from pins.txt as batch.sh sets them; P1 needs RK_MANAGED; P2 needs t2/judge/j2.py and a
#   keys.txt with no TBD; RK_BIN_G and RK_PGRAPH_G each embed vcs.revision at its pin and vcs.modified is not true;
#   the seam at T1_G_BIN_SHA (internal/store/conflictcheck.go and cmd/rashomon/conflictpost.go present in RK_REPO,
#   default this repository), else "caution trigger not available by construction"; RUN, its gocache clone and its
#   log absent; the configuration's account equal to the meter's (B454); P2: `claude auth status` loggedIn true. Then
#   the meter, read twice 20 s apart, each a whole number at or under T2_METER_LAUNCH_PCT (reading 1 refuses before
#   the wait); free disk, swap and the swap files logged (B669: a whole-GiB drop is swap). Then its own gocache clone
#   RK_RUNS/NAME.gocache (cp -c -R from RK_GOCACHE_BASE, batch.sh's one_run), passed to run.sh as RK_GOCACHE and
#   exported as GOCACHE for the setup's pgraph sync and index too, measured and removed by that exact path on exit,
#   the removal verified (batch.sh gcdrop).
# Exit 0: the probe's reading is written (whatever it reads). 1: a harness failure (setup, an edit that cannot apply,
# a base not clean). 2: a refusal. Log: RK_RUNS/NAME.j2probe.log.
set -u
MODE=${1:-}; NAME=${2:-}
HERE=$(cd "$(dirname "$0")" && pwd)
T2=$(cd "$HERE/.." && pwd)
H=$(cd "$HERE/../.." && pwd)
refuse() { echo "j2probe.sh: $*" >&2; exit 2; }
case "$MODE" in p1|p2) ;; *) refuse "usage: j2probe.sh p1 NAME | p2 NAME" ;; esac
case "$NAME" in ''|.*|*[!A-Za-z0-9._-]*) refuse "NAME [$NAME] is not a plain run name" ;; esac
[ "${RK_T2_J2PROBE_GO:-}" = yes ] || refuse "RK_T2_J2PROBE_GO is not yes; its launch word comes with the director's re-cut word (addendum 5 2c)"
for v in RK_RUNS RK_GOCACHE_BASE RK_BIN_G RK_PGRAPH RK_PGRAPH_G RK_GO_TOOLCHAIN_BIN RK_SEED RK_KNOWLEDGE RK_METER_FILE; do
  eval "val=\${$v:-}"; [ -n "$val" ] || refuse "$v unset"
done
[ -z "${RASHOMON_MANAGED_SETTINGS_PATH+x}" ] || refuse "RASHOMON_MANAGED_SETTINGS_PATH is set in the launching environment; only run.sh and this script set it"
unset RK_PROMPT_FILE RK_MODEL RK_MAX_TURNS RK_BUDGET_USD RK_SETUP_ONLY
export RK_SLICE=t2
. "$T2/pins.txt"
for v in T1_G_BIN_SHA T1_G_PGRAPH_SHA; do
  eval "val=\${$v:-}"; case "$val" in ''|TBD) refuse "$v is [$val] in pins.txt; the probe reads G at its pin (addendum 5 section 1)" ;; esac
done
case "${T2_METER_LAUNCH_PCT:-}" in ''|*[!0-9]*) refuse "T2_METER_LAUNCH_PCT is [${T2_METER_LAUNCH_PCT:-}] in pins.txt, not a whole number" ;; esac
if [ -n "${RK_CONFIG_DIR:-}" ] && [ "$(cd -- "$RK_CONFIG_DIR" 2> /dev/null && pwd -P)" != "$T1_CONFIG_DIR" ]; then
  refuse "RK_CONFIG_DIR is $RK_CONFIG_DIR; pins.txt names $T1_CONFIG_DIR"
fi
export RK_CONFIG_DIR="$T1_CONFIG_DIR" RK_CLAUDE_BIN="$T1_CLAUDE_BIN" RK_CLAUDE_VERSION="$T1_CLAUDE_VERSION"
CK=$( . "$T2/keys.txt" 2> /dev/null; printf '%s' "${T2K_CAUTION_KEY:-}" )
case "$CK" in ''|TBD) refuse "T2K_CAUTION_KEY is [$CK] in keys.txt" ;; esac
if [ "$MODE" = p1 ]; then
  [ -n "${RK_MANAGED:-}" ] || refuse "P1 needs RK_MANAGED (S1 (b)). S1 (a) awaits the director's ruling: an untrusted repository spec's rules are withheld (engine/rules.go:30); trust is a person's act at a terminal (spec.go trustSpec)"
else
  [ -f "$T2/judge/j2.py" ] || refuse "t2/judge/j2.py is absent; D15' copies the sealed j2b.py there first"
  if grep -n '=TBD' "$T2/keys.txt" >&2; then refuse "keys.txt holds the TBD values above; fill it at the re-cut"; fi
fi
export PATH="$RK_GO_TOOLCHAIN_BIN:$PATH"
for tool in go python3 perl git uuidgen shasum; do command -v "$tool" > /dev/null 2>&1 || refuse "$tool is not on PATH"; done
for f in "$RK_BIN_G" "$RK_PGRAPH" "$RK_PGRAPH_G" "$RK_SEED" "$RK_METER_FILE" "$H/run.sh" "$H/hook.sh" "$T2/judge/s1cross.sh" "$T2/prompt.txt"; do
  [ -f "$f" ] || refuse "missing $f"
done
[ -x "$H/hook.sh" ] || refuse "$H/hook.sh is not executable; the installed entries run it as a command"
for pair in "$RK_BIN_G:$T1_G_BIN_SHA" "$RK_PGRAPH_G:$T1_G_PGRAPH_SHA"; do
  bin=${pair%:*}; want=${pair##*:}
  bi=$(go version -m "$bin" 2> /dev/null)
  printf '%s\n' "$bi" | grep -q "vcs.revision=$want" || refuse "$bin: embedded vcs.revision is not the pinned $want"
  printf '%s\n' "$bi" | grep -q 'vcs.modified=true' && refuse "$bin: built from a modified tree (vcs.modified=true), so not the code at $want"
done
RK_REPO=${RK_REPO:-$(cd "$H/../../.." && pwd)}
for p in internal/store/conflictcheck.go cmd/rashomon/conflictpost.go; do
  git -C "$RK_REPO" cat-file -e "$T1_G_BIN_SHA:$p" 2> /dev/null || refuse "caution trigger not available by construction: $p is absent at T1_G_BIN_SHA $T1_G_BIN_SHA (the W-M2.4 seam writes conflict_check; without it no record names a rule)"
done
mkdir -p "$RK_RUNS" && RK_RUNS=$(cd "$RK_RUNS" && pwd -P) || refuse "RK_RUNS [$RK_RUNS] cannot be made or resolved"
export RK_RUNS RK_REPO
D=$RK_RUNS/$NAME; GC=$RK_RUNS/$NAME.gocache; LOG=$RK_RUNS/$NAME.j2probe.log
for p in "$D" "$GC" "$LOG"; do [ -e "$p" ] && refuse "$p exists; pick a new NAME"; done
acct() { python3 -I -B -c 'import json,sys; a=(json.load(open(sys.argv[1])).get("oauthAccount") or {}); print(a.get("emailAddress"), a.get("accountUuid"))' "$1" 2> /dev/null || echo unreadable; }
ac=$(acct "$RK_CONFIG_DIR/.claude.json"); am=$(acct "${RK_METER_ACCOUNT_JSON:-$HOME/.claude.json}")
case "$ac" in *None*|unreadable) refuse "the configuration's account is unreadable [$ac]" ;; esac
[ "$ac" = "$am" ] || refuse "config account [$ac] is not the meter's [$am] (B454)"
if [ "$MODE" = p2 ]; then
  [ -x "$RK_CLAUDE_BIN" ] || refuse "$RK_CLAUDE_BIN is not executable"
  AUTH=$(env -u CLAUDECODE -u CLAUDE_CODE_ENTRYPOINT -u AI_AGENT CLAUDE_CONFIG_DIR="$RK_CONFIG_DIR" "$RK_CLAUDE_BIN" auth status 2>&1)
  printf '%s\n' "$AUTH" | python3 -I -B -c 'import json,sys; sys.exit(0 if json.load(sys.stdin).get("loggedIn") is True else 1)' 2> /dev/null \
    || refuse "auth status on $RK_CONFIG_DIR is not loggedIn true"
fi
meter() { python3 -I -B -c 'import json,sys; d=json.load(open(sys.argv[1])); print(d.get("ts"), d.get("rate_limits"))' "$RK_METER_FILE" 2>&1; }
# The meter gate (the re-cut: q4-answer.txt items 1-4; the director 14:13 PST 11-14 3a, 3d): t2/meter.py at the launch
# line; a HOLD re-reads every 30 s up to T2_METER_HOLD_MAX_S. Its ledger RK_RUNS/NAME.meter.txt is this launch's own,
# so no D: the five-hour line is T2_METER_5H_LAUNCH_PCT. It writes nothing on a refusal.
mg=$(python3 -I -B "$T2/meter.py" gate --file "$RK_METER_FILE" --ledger "$RK_RUNS/$NAME.meter.txt" --name "$NAME" --weekly "${T2_METER_LAUNCH_PCT:-}" --weekly-key T2_METER_LAUNCH_PCT --max-age "${T2_METER_MAX_AGE_S:-}" --hold-max "${T2_METER_HOLD_MAX_S:-}" --five-line "${T2_METER_5H_PCT:-}" --mult "${T2_METER_5H_MULT:-}" --five-launch "${T2_METER_5H_LAUNCH_PCT:-}"); mrc=$?
[ "$mrc" = 0 ] || refuse "meter gate rc $mrc (meter.py): ${mg:-the lines above}"

say() { echo "$(date -u +%Y-%m-%dT%H:%M:%SZ) $*" | tee -a "$LOG"; }
say "=== j2probe $MODE $NAME; G $RK_BIN_G at $T1_G_BIN_SHA; pgraph-G $RK_PGRAPH_G at $T1_G_PGRAPH_SHA; repo $RK_REPO; harness $(git -C "$H" rev-parse --short HEAD 2> /dev/null); script $(shasum -a 256 "$0" | cut -c1-16)"
say "meter gate (meter.py): [$mg]; weekly at $T2_METER_LAUNCH_PCT or under (T2_METER_LAUNCH_PCT); config $RK_CONFIG_DIR account [$ac]"
say "stop fold at T1_G_BIN_SHA: $(git -C "$RK_REPO" grep -q 'conflictsSeen = foldConflictChecks' "$T1_G_BIN_SHA" -- cmd/rashomon/recap_conflicts.go 2> /dev/null && echo present || echo 'absent (the Stop names no conflict)')"
say "free disk: $(df -k /System/Volumes/Data | tail -1)"
say "swap: $(sysctl -n vm.swapusage 2>&1)"
say "swap files: $(ls -l /System/Volumes/VM 2>&1 | awk '/swapfile/ { printf "%s %s; ", $NF, $5 }')"

# Base check (addendum 11, 7a; the director 16:32 PST (b)): batch.sh's four lines (:261-264) with two declared
# substitutions only, "$HERE/cachemanifest.py" to "$T2/cachemanifest.py" and "batch refused" to "j2probe refused";
# t2-skel/a11-basecheck-equal.py holds the three copies equal. Then the device preflight (16:32 PST (b)): RK_RUNS
# and the base on one APFS device, where cp -c -R can only clone (cp -c otherwise falls back to a byte copy, B684).
# Addendum 11 7a, ruled 11-13 10:18 PST item 12: the base must be byte-equal to its own physical path (pwd -P).
[ ! -d "$RK_GOCACHE_BASE" ] || { gbp=$(cd -- "$RK_GOCACHE_BASE" && pwd -P); [ "$RK_GOCACHE_BASE" = "$gbp" ]; } || { say "RK_GOCACHE_BASE [$RK_GOCACHE_BASE] is not its physical path [$gbp]; refused, not followed"; exit 2; }
[ -d "$RK_GOCACHE_BASE" ] || { say "RK_GOCACHE_BASE $RK_GOCACHE_BASE is not a directory"; exit 2; }
gm=$RK_RUNS/t2-gocache-base-$(date -u +%Y%m%dT%H%M%SZ)-$$.manifest
gml=$(python3 -I -B "$T2/cachemanifest.py" "$RK_GOCACHE_BASE" "$gm" 2>&1); say "gocache base $RK_GOCACHE_BASE: $gml ($gm)"
[ "$(printf '%s\n' "$gml" | sed -n 's/.* manifest_sha256 \([0-9a-f]*\)$/\1/p')" = "$T2_GOCACHE_BASE_MANIFEST_SHA256" ] || { say "the gocache base manifest is not T2_GOCACHE_BASE_MANIFEST_SHA256 (amend-gocache); j2probe refused"; exit 2; }
devr=$(stat -f %d "$RK_RUNS"); devb=$(stat -f %d "$RK_GOCACHE_BASE/."); devm=$(df -P "$RK_RUNS" | awk 'NR==2 { print $NF }')
[ -n "$devr" ] && [ "$devr" = "$devb" ] && mount | grep -q -F " on $devm (apfs," || { say "RK_RUNS $RK_RUNS (device $devr, mounted on $devm) and RK_GOCACHE_BASE $RK_GOCACHE_BASE (device $devb) are not one APFS device: cp -c -R would byte-copy (B684); j2probe refused"; exit 2; }
GCD=
gcdrop() { [ -n "${GC:-}" ] || return 0; case "$GC" in "$RK_RUNS"/*.gocache) ;; *) say "HARNESS: the gocache clone path [$GC] is not RK_RUNS/NAME.gocache; not removed"; GC=; return 1 ;; esac; if [ -d "$GC" ]; then gk=$(du -sk "$GC" | cut -f1); [ -d "${GCD:-}" ] && printf 'gocache-clone-kib %s\n' "$gk" >> "$GCD/knobs"; rm -rf "$GC"; [ ! -e "$GC" ] || { say "HARNESS: the gocache clone $GC is still present after its drop"; GC=; return 1; }; say "gocache clone $GC removed ($gk KiB)"; fi; GC=; }
[ -e "$GC" ] && { say "HARNESS: $GC exists before its clone; not touched"; exit 2; }
trap gcdrop EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
cp -c -R "$RK_GOCACHE_BASE" "$GC" || { say "HARNESS: the gocache clone $GC failed"; exit 1; }
export GOCACHE="$GC"

if [ "$MODE" = p2 ]; then
  say "P2 start $NAME (G): RK_MAX_TURNS=15 RK_BUDGET_USD=1.00; managed ${RK_MANAGED:-off}"
  RK_MAX_TURNS=15 RK_BUDGET_USD=1.00 RK_GOCACHE="$GC" sh "$H/run.sh" G "$NAME" > "$RK_RUNS/$NAME.launch.txt" 2>&1; rc=$?
  GCD=$D
  say "P2 end rc=$rc: $(tail -1 "$RK_RUNS/$NAME.launch.txt")"
  [ -d "$D" ] && printf 'gocache-base %s\ngocache-clone %s\nj2probe p2\n' "$RK_GOCACHE_BASE" "$RK_RUNS/$NAME.gocache" >> "$D/knobs"
  [ "$rc" -eq 0 ] && [ -f "$D/transcript.jsonl" ] || { say "HARNESS: run.sh rc=$rc, transcript $( [ -f "$D/transcript.jsonl" ] && echo present || echo absent)"; exit 1; }
  say "P2 cost: $(python3 -I -B -c 'import json,sys; d=json.load(open(sys.argv[1])); print("total_cost_usd", d.get("total_cost_usd"), "num_turns", d.get("num_turns"), "subtype", d.get("subtype"))' "$D/result.json" 2>&1)"
  say "P2 j2.py $(shasum -a 256 "$T2/judge/j2.py" | cut -c1-16)"
  python3 -I -B "$T2/judge/j2.py" "$D" > "$D/j2b-probe.out" 2>&1; jrc=$?
  say "P2 j2.py rc=$jrc: $(tr '\n' ' ' < "$D/j2b-probe.out" | cut -c1-600)"
  say "P2 meter after [$(meter)]"
  exit 0
fi

say "P1 setup $NAME (G): RK_SETUP_ONLY=yes; managed $RK_MANAGED"
RK_SETUP_ONLY=yes RK_GOCACHE="$GC" sh "$H/run.sh" G "$NAME" > "$RK_RUNS/$NAME.launch.txt" 2>&1; rc=$?
GCD=$D
say "P1 setup rc=$rc: $(tail -1 "$RK_RUNS/$NAME.launch.txt")"
[ -d "$D" ] && printf 'gocache-base %s\ngocache-clone %s\nj2probe p1\n' "$RK_GOCACHE_BASE" "$RK_RUNS/$NAME.gocache" >> "$D/knobs"
[ "$rc" -eq 0 ] && [ -f "$D/settings.installed.json" ] && [ -d "$D/store" ] || { say "HARNESS: setup rc=$rc or no settings.installed.json or store"; exit 1; }
python3 -I -B - "$D" "$H/hook.sh" "$RK_BIN_G" "$T2/prompt.txt" "$CK" "$RK_MANAGED/managed-settings.json" "$T2/judge/s1cross.sh" \
  "$RK_CLAUDE_VERSION" "${T1_AGENT_GOTOOLCHAIN:-go1.27.3}" "$GC" <<'PY' 2>&1 | tee -a "$LOG"
import glob, json, os, re, subprocess, sys, time, uuid

D, hook, binary, prompt_file, ck, managed, s1cross, version, gotc, gc = sys.argv[1:11]
tree = os.path.join(D, "tree")
P = os.path.join(D, "p1")
os.makedirs(P)

# The seeded edits, one per direction (addendum 5 2a), on real files at T1_TREE_SHA.
EDITS = [
    ("pgraph-in", "internal/platform/digests.go",
     '\t"github.com/altrace-dev-role/rashomon/internal/regfile"\n)\n',
     '\t"github.com/altrace-dev-role/rashomon/internal/regfile"\n\t_ "github.com/altrace-dev-role/rashomon/pgraph"\n)\n'),
    ("pgraph-out", "pgraph/log.go",
     'import (\n\t"fmt"\n\t"sort"\n\t"strings"\n)\n',
     'import (\n\t"fmt"\n\t"sort"\n\t"strings"\n\n\t_ "github.com/altrace-dev-role/rashomon/internal/store"\n)\n'),
]
RULES = [e[0] for e in EDITS]


def harness(msg):
    print("HARNESS: " + msg)
    sys.exit(1)


# wrap.sh install's rewrite, verbatim in effect: an entry whose command starts with the binary (bare, single- or
# double-quoted, by its path or its real path) and a space gets hook.sh in the binary's place.
doc = json.load(open(os.path.join(D, "settings.installed.json")))
forms = []
for b in {binary, os.path.realpath(binary)}:
    forms += [b + " ", "'" + b + "' ", '"' + b + '" ']
ours, others = {}, 0
for event, entries in (doc.get("hooks") or {}).items():
    for entry in entries:
        for h in entry.get("hooks", []):
            cmd = h.get("command", "")
            for form in forms:
                if cmd.startswith(form):
                    h["command"] = hook + " " + cmd[len(form):]
                    ours.setdefault(event, []).append((entry.get("matcher"), h["command"]))
                    break
            else:
                others += 1
cfg = os.path.join(D, "p1cfg")
os.makedirs(os.path.join(cfg, "projects"))
with open(os.path.join(cfg, "settings.json"), "w") as f:
    json.dump(doc, f, indent=2)
print("p1 settings: our entries %s; other entries %d" % (json.dumps({k: len(v) for k, v in sorted(ours.items())}), others))


def matches(matcher, value):
    if matcher in (None, "", "*"):
        return True
    try:
        return re.fullmatch(matcher, value) is not None
    except re.error:
        return value in matcher.split("|")


def command(event, value=None):
    got = [c for m, c in ours.get(event, []) if value is None or matches(m, value)]
    if len(got) != 1:
        harness("%d of our entries for %s%s, not one" % (len(got), event, "" if value is None else " (" + value + ")"))
    return got[0]


CMD = {"SessionStart": command("SessionStart", "startup"), "UserPromptSubmit": command("UserPromptSubmit"),
       "PreToolUse": command("PreToolUse", "Edit"), "PostToolUse": command("PostToolUse", "Edit"), "Stop": command("Stop")}
for k in sorted(CMD):
    print("p1 command %s: %s" % (k, CMD[k]))

sid = str(uuid.uuid4())
pid = str(uuid.uuid4())
slug = re.sub(r"[^A-Za-z0-9]", "-", tree)
os.makedirs(os.path.join(cfg, "projects", slug))
tpath = os.path.join(cfg, "projects", slug, sid + ".jsonl")
open(tpath, "w").close()
with open(os.path.join(P, "session-id"), "w") as f:
    f.write(sid + "\n")
store = os.path.join(D, "store")

env = {k: v for k, v in os.environ.items() if k not in (
    "CLAUDECODE", "CLAUDE_CODE_ENTRYPOINT", "AI_AGENT", "CLAUDE_PROJECT_DIR", "RASHOMON_MANAGED_SETTINGS_PATH",
    "RASHOMON_STEER_ARM", "RASHOMON_HOME", "RK_BIN", "RK_RUN_DIR", "CLAUDE_CONFIG_DIR")}
env.update({"CLAUDECODE": "1", "CLAUDE_CODE_ENTRYPOINT": "sdk-cli", "AI_AGENT": "claude-code_%s_harness" % version,
            "CLAUDE_PROJECT_DIR": tree, "CLAUDE_CONFIG_DIR": cfg, "RASHOMON_HOME": store, "RK_BIN": binary,
            "RK_RUN_DIR": D, "RK_CLAUDE_VERSION": version, "RASHOMON_HOLDOUT": "off",
            "RASHOMON_MANAGED_SETTINGS_PATH": managed, "GOTOOLCHAIN": gotc, "GOCACHE": gc})
print("p1 hook env: CLAUDECODE=1 CLAUDE_CODE_ENTRYPOINT=sdk-cli AI_AGENT=%s CLAUDE_PROJECT_DIR=%s CLAUDE_CONFIG_DIR=%s "
      "RASHOMON_HOME=%s RASHOMON_HOLDOUT=off RASHOMON_MANAGED_SETTINGS_PATH=%s GOTOOLCHAIN=%s"
      % (env["AI_AGENT"], tree, cfg, store, managed, gotc))
n = [0]


def fire(event, payload):
    n[0] += 1
    base = os.path.join(P, "%02d-%s" % (n[0], event))
    body = json.dumps(payload).encode()
    with open(base + ".in.json", "wb") as f:
        f.write(body)
    t0 = time.time()
    try:
        r = subprocess.run(["sh", "-c", CMD[event]], input=body, cwd=tree, env=env, capture_output=True, timeout=120)
    except subprocess.TimeoutExpired:
        harness("%s timed out after 120 s" % event)
    with open(base + ".out", "wb") as f:
        f.write(r.stdout)
    print("p1 fire %02d %s rc=%d ms=%d stdout=%dB stderr=%dB" % (n[0], event, r.returncode, (time.time() - t0) * 1000,
                                                                 len(r.stdout), len(r.stderr)))
    return r


def marker():
    try:
        return open(os.path.join(store, "session_content")).read().strip() or "empty"
    except OSError:
        return "absent"


def records():
    out = []
    for p in sorted(glob.glob(os.path.join(store, "runs", "*", "records.ndjson"))):
        for raw in open(p, encoding="utf-8", errors="replace"):
            try:
                d = json.loads(raw)
            except ValueError:
                continue
            if isinstance(d, dict) and d.get("type") == "conflict_check" and d.get("session_id") == sid:
                out.append(d)
    return out


def crosses(recs):
    got = {}
    for d in recs:
        for r in d.get("rows") or []:
            if r.get("kind") == "boundary_cross":
                got[str(r.get("rule_id"))] = got.get(str(r.get("rule_id")), 0) + 1
    return got


def s1(tag):
    r = subprocess.run(["sh", s1cross, tree], capture_output=True, text=True)
    with open(os.path.join(P, "s1cross-%s.txt" % tag), "w") as f:
        f.write(r.stdout + r.stderr)
    if r.returncode != 0:
        harness("s1cross.sh %s rc=%d: %s" % (tag, r.returncode, r.stderr.strip()))
    got = {}
    for line in r.stdout.splitlines():
        w = line.split()
        if len(w) == 5 and w[0] in RULES and w[1] == "nontest":
            got[w[0]] = (int(w[2]), int(w[4]))
        if w and w[0] == "gomod":
            got["gomod"] = line
    print("p1 s1cross %s: %s" % (tag, json.dumps(got, sort_keys=True)))
    return got


def git(*a):
    return subprocess.run(["git", "-C", tree] + list(a), capture_output=True, text=True).stdout


def bases():
    out = []
    for p in sorted(glob.glob(os.path.join(store, "graph", "unitbrief", "*", "*"))):
        try:
            with open(p, encoding="utf-8") as f:
                head = json.loads(f.readline())
                body = json.loads(f.readline())
        except (OSError, ValueError):
            continue
        if isinstance(head, dict) and "rules_digest" in head and isinstance(body, dict):
            ids = sorted(str(r.get("id", r.get("ID"))) for r in body.get("rules") or [] if isinstance(r, dict))
            out.append("%s commit %s rules %s" % (os.path.basename(p), str(head.get("commit"))[:12], ",".join(ids)))
    return out


before = s1("before")
if any(before.get(r) != (0, 0) for r in RULES):
    harness("the base is not clean: s1cross reads %s before any edit" % json.dumps(before, sort_keys=True))
diff0 = git("diff", "--name-only").split()
if diff0:
    harness("the tree differs from T1_TREE_SHA before any edit: %s" % " ".join(diff0))
print("p1 gate session_content before SessionStart: %s" % marker())
common = {"session_id": sid, "transcript_path": tpath, "cwd": tree}
fire("SessionStart", dict(common, hook_event_name="SessionStart", source="startup"))
print("p1 gate session_content after SessionStart: %s" % marker())
prompt = open(prompt_file, encoding="utf-8").read()
fire("UserPromptSubmit", dict(common, hook_event_name="UserPromptSubmit", permission_mode="acceptEdits", prompt=prompt,
                              prompt_id=pid))
cb = bases()
print("p1 gate conflict base: %s" % ("; ".join(cb) if cb else "none"))
pre_cross = crosses(records())
print("p1 boundary_cross before the edits: %s" % json.dumps(pre_cross, sort_keys=True))
blocked = []
for rule, rel, old, new in EDITS:
    path = os.path.join(tree, rel)
    text = open(path, encoding="utf-8").read()
    if text.count(old) != 1:
        harness("%s: old_string occurs %d times in %s, not once" % (rule, text.count(old), rel))
    tid = "toolu_j2probe_" + uuid.uuid4().hex[:20]
    tin = {"file_path": path, "old_string": old, "new_string": new, "replace_all": False}
    tool = dict(common, permission_mode="acceptEdits", prompt_id=pid, tool_name="Edit", tool_input=tin, tool_use_id=tid)
    r = fire("PreToolUse", dict(tool, hook_event_name="PreToolUse"))
    deny = False
    try:
        deny = (json.loads(r.stdout or b"{}").get("hookSpecificOutput") or {}).get("permissionDecision") == "deny"
    except (ValueError, AttributeError):
        pass
    if r.returncode == 2 or deny:
        print("p1 %s: PreToolUse blocked the edit (rc %d, deny %s); not applied, no post" % (rule, r.returncode, deny))
        blocked.append(rule)
        continue
    with open(path, "w", encoding="utf-8") as f:
        f.write(text.replace(old, new, 1))
    resp = {"filePath": path, "oldString": old, "newString": new, "originalFile": text, "replaceAll": False,
            "userModified": False, "structuredPatch": []}
    fire("PostToolUse", dict(tool, hook_event_name="PostToolUse", tool_response=resp))
fire("Stop", dict(common, hook_event_name="Stop", permission_mode="acceptEdits", stop_hook_active=False))

after = s1("after")
diff1 = sorted(git("diff", "--name-only").split())
ovl = sorted(os.path.relpath(p, store) for p in glob.glob(os.path.join(store, "**", "overlay-*"), recursive=True))
print("p1 gate overlay files: %d %s" % (len(ovl), " ".join(ovl[:6])))
recs = records()
for d in recs:
    print("p1 conflict_check seq %s landed_seq %s commit %s rows %s" % (d.get("seq"), d.get("landed_seq"),
          str(d.get("commit"))[:12], json.dumps([(r.get("kind"), r.get("rule_id")) for r in d.get("rows") or []])))
got = crosses(recs)
print("p1 gate conflict_check records: %d; boundary_cross by rule %s" % (len(recs), json.dumps(got, sort_keys=True)))

lines = []
cur = None
for raw in open(os.path.join(D, "hooks.log"), encoding="utf-8", errors="replace").read().splitlines():
    m = re.match(r"^== \S+ (\S+)(.*) rc=(\d+)$", raw)
    if m:
        cur = (m.group(1), "pending" in m.group(2).split())
    elif cur and ck in raw:
        lines.append(("post" if cur[0] == "post" else "stop" if cur[0] == "recap" else "other",
                      cur[0] + (" pending" if cur[1] else ""), raw))


def named(rule):
    pat = re.compile(r"(?<![\w-])" + re.escape(rule) + r"(?![\w-])")
    return sorted({(ch, verb) for ch, verb, l in lines if pat.search(l)})


print("p1 caution lines (T2K_CAUTION_KEY) in hooks.log: %d" % len(lines))
cautions = {}
for rule in RULES:
    nm = named(rule)
    cautions[rule] = nm
    print("caution %s: %s" % (rule, ", ".join("%s (%s)" % (ch, verb) for ch, verb in nm) if nm else "none"))

checks = [
    ("zero boundary_cross before", pre_cross == {}),
    ("s1cross before 0/0", all(before.get(r) == (0, 0) for r in RULES)),
    ("no edit blocked", not blocked),
    ("exactly one boundary_cross per direction after", all(got.get(r) == 1 for r in RULES) and set(got) <= set(RULES)),
    ("s1cross after nontest 1 each", all((after.get(r) or (None,))[0] == 1 for r in RULES)),
    ("gomod line unchanged", before.get("gomod") == after.get("gomod")),
    ("git diff names the two files alone", diff1 == sorted(rel for _, rel, _, _ in EDITS)),
]
for name, ok in checks:
    print("p1 check %s: %s" % (name, "yes" if ok else "NO"))
print("P1 %s" % ("PASS" if all(ok for _, ok in checks) else "FAIL"))
avail = {r: c for r, c in cautions.items() if c}
if avail:
    print("CAUTION AVAILABLE on hook output: %s (P1's post channel is hook stdout, delivered, not reached: 2b)"
          % "; ".join("%s %s" % (r, ",".join(ch for ch, _ in c)) for r, c in sorted(avail.items())))
else:
    print("caution trigger not available")
PY
grep -q '^HARNESS: ' "$LOG" && exit 1
grep -q -E '^(P1 PASS|P1 FAIL)$' "$LOG" || { say "HARNESS: the replay wrote no P1 verdict"; exit 1; }
say "P1 done; run $D"
exit 0
