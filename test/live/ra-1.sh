#!/bin/sh
# RA-1: the release-acceptance evidence rows of RELEASE-ACCEPTANCE.md (A1, A2,
# A4, A5, A7, A8), the no-false-red gate row (A13, release gate G11) and the
# MVP smoke rows (A14-A21, plan card c1), one command each, evidence saved.
#
#   test/live/ra-1.sh [ROW ...]    ROW is A1 A2 A4 A5 A7 A8 A13 A14 A15 A16 A17
#                                  A18 A19 A20 A21; A1 A2 A4 A7 A8 by default
#   RA_MVP=1 test/live/ra-1.sh     the MVP smoke, A14-A21 (with ROW arguments:
#                                  those rows and A14-A21)
#
# RUN IT FROM A PLAIN TERMINAL, NEVER FROM AN AGENT SESSION. `rashomon watch`,
# `rashomon spec trust` and `rashomon purge` each ask a person on /dev/tty and
# refuse an agent session (internal/selfguard: the CLAUDECODE variable Claude
# Code sets in every process it starts). This script checks the same variable
# first and refuses, exit 2, before it does anything else; it never passes
# --yes, never unsets CLAUDECODE for a child, and never answers a prompt. The
# person at the terminal answers y at watch (A2, A4, A7, A8), at spec trust
# (A2, A4, A7) and at purge (A8).
#
# Rows, prerequisites, what is saved ($EVID = .handoff/evidence by default,
# untracked; override with RA_EVIDENCE):
#
#   A1  needs the tag $RA_TAG (default v1.2.0-alpha.1) to exist; network, go,
#       curl, unzip, shasum. Installs by go install, install.sh and the plugin
#       zip from the release assets, each into a scratch directory under
#       $EVID/A1/. Saves A1/install.txt: `rashomon version` and the sha256 of
#       each binary, and the verdict. Refuses, exit 2, when the tag does not
#       exist. install.sh runs as the user would, with no pinned version, so
#       it also shows what releases/latest serves (RA_INSTALL_PIN=1 pins it).
#   A2  needs the RC binary (below), claude on PATH and ~/.claude-probe logged
#       in with no settings.json. Two sessions of an INTERACTIVE claude, in
#       your terminal, each in its own scratch Go project. Before the
#       sessions the script runs spec trust in both projects (you answer y,
#       twice) for a spec that declares one required gate, unit =
#       `go test ./...`: without a trusted spec and a verification index
#       nothing can raise a note in a fresh store (internal/hook/steerstep.go
#       judge; internal/verify/steer.go staleEvidence). Then, per session, the
#       script prints the prompt on its own lines (and copies it to the
#       clipboard when pbcopy exists), you press Enter, claude starts under
#       the probe's CLAUDE_CONFIG_DIR. CLAUDE CODE MAY FIRST ASK WHETHER TO
#       TRUST THE FOLDER: accept it. Then type or paste the prompt EXACTLY,
#       approve the tool calls and type /exit. The prompt has the model run a
#       green go test, then edit a file, which is stale-evidence: the declared
#       gate was green, then this call wrote, and no later run is recorded;
#       the note fires at that Edit's PostToolUse. Session one reads as
#       Claude Code 2.1.200: the version is SPOOFED (AI_AGENT, which
#       steer.ClientVersion reads, is set to the 2.1.200 harness form for the
#       hook only, by a wrapper the watch entries point at), and the floor,
#       2.1.280 for PostToolUse (internal/steer/client.go), is what is under
#       test. Session two runs on the real version. Nothing else is spoofed:
#       the entry point stays cli, the real surface. Saves A2/: claude
#       version, spec-trust-<mode>.txt, both transcripts, both steer.ndjson,
#       both <mode>.prompt-fired markers (UserPromptSubmit reached the store),
#       verdict.txt. PASS: spoofed, the once-per-session "a note was built and
#       not sent" notice recorded once, a stale-evidence line shadowed, no
#       note in the agent's transcript; real, the stale-evidence line
#       delivered, "rashomon record: gate 'unit' ..." in the agent's
#       transcript, and no notice. INCONCLUSIVE (exit 3), not FAIL, when a
#       session never got the prompt (no user prompt in its transcript and no
#       UserPromptSubmit in the store: claude was started and exited with
#       nothing typed) or the model did not run go test and edit a file.
#   A4  needs the same, plus go. In a scratch Go project with a trusted spec
#       that declares `go test ./...`: an interactive session that edits and
#       runs no test (the done line at Stop), then one that edits and runs go
#       test (the verified line). The terminal is captured with script(1) when
#       it exists; a line the capture misses is asked of you at the terminal
#       and the verdict says it is your word. Saves A4/: transcripts, both
#       `rashomon report --session <id>`, status.txt, verdict.txt. The spec
#       declares the gate and no rule: a protect_path rule over the file the
#       session edits is a binding violation at Stop (SPEC-RULES1), which turns
#       the verified turn into "N record finding(s)", so A18 owns the rules.
#
# RA_HEADLESS=1 runs A2 and A4 as one `claude -p` per session instead, with
# AI_AGENT spoofed and, because claude -p is the SDK surface where notes are
# shut by design (internal/steer/notice.go), CLAUDE_CODE_ENTRYPOINT forced to
# cli. That is a SMOKE CHECK of the script and the plumbing. It is NOT
# release evidence: forcing the entry point makes rashomon treat a headless
# run as the CLI, which no user sees. verdict.txt names the mode that ran.
#   A7  the report half of A4's run (A4 runs first if it is not listed).
#       Saves A7/: the two reports and a verdict on what they name.
#   A8  needs the RC binary only; no claude run. In a scratch config
#       directory, once seeded with an unrelated settings.json and once with
#       none: sha256 of settings.json, watch, detach, purge, sha256 again; the
#       hashes must be equal. Saves A8/uninstall.txt.
#
#   A5  (RELEASE-ACCEPTANCE A5; MASKED-OUTCOME1 #149) needs the same, plus
#       python3 and pytest on PATH (INCONCLUSIVE, exit 3, with that reason when
#       either is missing). Three interactive sessions over a scratch Python
#       project with one pytest test, a trusted spec declaring one gate, unit =
#       `pytest -q`: (1) an edit, then an unpiped `pytest -q`, green: the
#       verified line at Stop; (2) an edit, then `pytest -q | tail -5`, green:
#       the verified line with the provenance clause "(read from its output;
#       the pipe hid the exit status)" and no false alarm (no "hid its result"
#       line, no masked-status note); (3) in a project whose test fails, an
#       edit, then `pytest -q | tail -5`: the failure line at Stop, read from
#       the output, and no verified line. Saves A5/: pytest version, spec
#       trust, three transcripts and steer logs, three reports, verdict.txt.
#       PASS: all three. INCONCLUSIVE when a prompt was never typed or the model
#       did not run the exact command.
#   A13 (release gate G11, no false red; GATEOUT-ZERO1 #180) needs the same,
#       plus go and make. A Go project whose Makefile `test:` target runs
#       `go test -v ./...` and whose test logs "0 failed" with t.Log (go prints
#       a passing package's log only under -v; before #180 pytest's bare fail
#       form matched "0 failed" anywhere in a delegated make run's output, so
#       the gate read red over a run that exited 0). One session: an edit, then
#       `make test`. PASS: the verified line at Stop and no "failed" or
#       "exited" line for the gate. FAIL, with that reason, when a green make
#       test is read red: the build under test does not have #180.
#
# The MVP smoke, A14-A21 (RA_MVP=1 or named rows), runs the rows that each
# stand on one card merged on main. They share one watch and one scratch Go
# work tree (git init) whose spec, trusted once (you answer y), declares a test gate
# (unit = go test ./...), a protect_path rule (parse-owner over internal/
# parse) and a forbid_command rule (no-uname over `uname`), and one to three
# interactive sessions (the first always; the second only when A20 is
# selected, the third only when A17 is). Raw evidence is in $EVID/MVP/; each
# row keeps its own directory with its verdict.txt. Rows that need a note to
# reach the agent need Claude Code 2.1.280 or later (PostToolUse's floor,
# internal/steer/client.go): on an older one A18, A19 and A20 are
# INCONCLUSIVE. Session one's prompt: add a comment line to
# internal/parse/lexer.go, run uname -s, run go test ./... , reply done.
#   A14 report --ledger (LEDGER-WIRE1): `rashomon report --ledger --session ID`
#       for session one prints the header (point, spec, task), the declared
#       gate's gate_after_write row met, parse-owner's spec_rule row unmet, a
#       no-uname row (unknown here: this build's hook records no command text,
#       so the Stop cannot judge forbid_command; unmet if a build records it),
#       and prints the same bytes twice. Saves A14/ledger.txt, verdict.txt.
#   A15 the brief at session start (BRIEF1): `rashomon brief DIR` prints the
#       header and the instructions, build, tests (the spec's declared gate,
#       "(spec, trusted)"), spec, task, packages and verify lines; session
#       one's terminal shows the brief when it starts (you may be asked to
#       confirm what the capture missed). A later session's "last session:"
#       line is reported, not judged. Saves A15/brief.verb.txt, verdict.txt.
#   A16 `rashomon status` shows hooks fired vs installed (STATUS-VERBS1a):
#       status --json read right after session one: every one of the eight
#       events installed, none flagged not firing, and PreToolUse,
#       PostToolUse, SessionStart, UserPromptSubmit and Stop fired in the
#       newest session (SessionEnd is reported, not judged: it is session
#       end's). Saves A16/status.txt, status.json, events.txt, verdict.txt.
#   A17 an approved lesson is delivered once (LEARN-DELIVER1): needs a
#       terminal for two steps. The script runs `rashomon learn import` on a
#       one-row lessons file, then `rashomon learn approve keep-diffs-small`,
#       which shows a six-digit code ON YOUR TERMINAL: type it back there.
#       Then session three, which has two prompts in one session (type the
#       first, wait for the reply, type the second). PASS: the lesson note is
#       in the agent's transcript once, one delivered learned-fact line in the
#       steer log, and the lesson lists as probation. Saves A17/: import,
#       approve and list output, transcript, steer log, verdict.txt.
#   A18 a forbidden command and a protected path are raised at the call and
#       at Stop (SPEC-RULES1 1a): at the call, session one's agent transcript
#       holds "this 'uname' call matched spec rule no-uname." and the steer
#       log holds a user-only integrity line for parse-owner (the agent is not
#       told); at Stop, the terminal shows "N record finding(s) in this turn"
#       with N at least 1 (the protect_path violation; 2 where a build records
#       command text). Saves A18/: transcript, steer log, verdict.txt.
#   A19 `rashomon why <id>` explains a delivered note (REPORT1b): the id is
#       read from session one's delivered platform-rule steer line; why
#       prints its pattern ("platform rule"), when and after which call, the
#       state "delivered, delivery delivered", the text the agent was sent
#       and the rule it came from. Saves A19/why.txt, verdict.txt.
#   A20 `rashomon steer off --project` stops notes for that project while
#       recording continues (STEER-CLI1): session two runs the same uname -s
#       after `steer off --project`. PASS: the note was built and logged with
#       suppressed_by steer_off, none reached the agent's transcript, the
#       session is in the report (declarations recorded), `steer` and `steer
#       log --all` say off, and `steer on --project` turns it back. Needs
#       session one's delivered note as the control, else INCONCLUSIVE. Saves
#       A20/: steer off, state, log and on output, transcript, steer log,
#       report, verdict.txt.
#   A21 CHANNEL-EVIDENCE1 (#159), the half that can be run live: after the
#       sessions above, in which Claude Code did show the delivered notes in
#       its transcript, no channel-evidence-* marker exists in the store (a
#       shadow needs two consecutive Stops that find a delivered note absent;
#       a healthy channel must not shadow). The other half, a note that Claude
#       Code does not show twice in a row, cannot be produced without hiding
#       notes from Claude Code or faking its transcript, so it is NOT driven
#       here (the unit and acceptance tests own it). Saves A21/markers.txt,
#       verdict.txt.
#
# `RA_HEADLESS=1` runs the MVP sessions as claude -p too (with the entry point
# forced to cli through the wrapper, as A2 does): a smoke check; lines the
# terminal prints are looked for in the stream only.
#
# The RC binary is $RA_BIN, else the install.sh binary A1 saved
# ($EVID/A1/prefix/bin/rashomon), and it must print $RA_TAG without the v
# (RA_ANY_VERSION=1 skips that check).
#
# Claude Code runs under CLAUDE_CONFIG_DIR, $HOME/.claude-probe by default,
# and a scratch RASHOMON_HOME; ~/.claude and the user's store are never read
# or written (the script refuses a CLAUDE_CONFIG_DIR that is ~/.claude).
# watch writes the probe's settings.json and the script's exit trap detaches
# it, so the directory ends with none, as p-tru-1.sh needs.
#
# Exit 0 when every row passed, 1 when any failed, 3 when none failed but a
# session did not do what the prompt asked (nothing to judge), 2 when it
# refused to start. Every external command runs bounded.
set -eu

# ---- refuse an agent session, before anything else ------------------------
if [ -n "${CLAUDECODE:-}" ]; then
  echo "refusing: this is an agent session (CLAUDECODE is set). watch, spec trust and purge refuse one and are not bypassed here. Run test/live/ra-1.sh from a plain terminal." >&2
  exit 2
fi

ROOT=$(cd "$(dirname "$0")/../.." && pwd)
TAG=${RA_TAG:-v1.2.0-alpha.1}
VER=${TAG#v}
REPO=${RA_REPO:-altrace-dev-role/rashomon}
EVID=${RA_EVIDENCE:-$ROOT/.handoff/evidence}
CFG=${CLAUDE_CONFIG_DIR:-$HOME/.claude-probe}
HEADLESS=${RA_HEADLESS:-0}
if [ "$HEADLESS" = 1 ]; then MODE="headless smoke check, NOT release evidence"; else MODE="interactive"; fi
SPOOF_VER=2.1.200
SPOOF_AGENT=claude-code_2-1-200_harness
MVP_ROWS="A14 A15 A16 A17 A18 A19 A20 A21"
ALL_ROWS="A1 A2 A4 A5 A7 A8 A13 $MVP_ROWS"
ROWS=$*
if [ "${RA_MVP:-0}" = 1 ]; then ROWS="$ROWS $MVP_ROWS"; fi
case $ROWS in *[A-Za-z0-9]*) ;; *) ROWS="A1 A2 A4 A7 A8" ;; esac
for r in $ROWS; do
  case " $ALL_ROWS " in
    *" $r "*) ;;
    *) echo "refusing: $r is not a row this script drives ($ALL_ROWS)" >&2; exit 2 ;;
  esac
done
# canonical order; A7 reads A4's run, so it brings A4 with it
SEL=" $ROWS "
case $SEL in *" A7 "*) SEL="$SEL A4 " ;; esac
ROWS=
for r in $ALL_ROWS; do case $SEL in *" $r "*) ROWS="$ROWS $r" ;; esac; done
NEED_CLAUDE=0; NEED_TTY=0
for r in $ROWS; do
  case $r in A1 | A8) ;; *) NEED_CLAUDE=1 ;; esac
  case $r in A1) ;; *) NEED_TTY=1 ;; esac
done
CFGABS=$(cd "$CFG" 2> /dev/null && pwd -P || echo "$CFG")
if [ "$CFGABS" = "$(cd "$HOME/.claude" 2> /dev/null && pwd -P || echo /nonexistent)" ]; then
  echo "refusing: CLAUDE_CONFIG_DIR is ~/.claude, which this script never touches" >&2; exit 2
fi
if [ "$NEED_TTY" = 1 ] && ! (: < /dev/tty) 2> /dev/null; then
  echo "refusing: no terminal; watch, spec trust and purge ask on /dev/tty. Run it from your own terminal." >&2; exit 2
fi
if [ "$NEED_CLAUDE" = 1 ]; then
  command -v claude > /dev/null 2>&1 || { echo "refusing: no claude on PATH" >&2; exit 2; }
  [ -d "$CFG" ] || { echo "refusing: $CFG does not exist; log claude in under it once first" >&2; exit 2; }
  if [ -e "$CFG/settings.json" ]; then
    echo "refusing: $CFG/settings.json exists; its hooks would fire beside the probe's" >&2; exit 2
  fi
fi

T=$(mktemp -d "${TMPDIR:-/tmp}/rashomon-ra1.XXXXXX")
# the physical path: watch writes the binary's symlink-resolved path into
# every entry (internal/install Executable), and swap matches that text
T=$(cd "$T" && pwd -P)
case $T in *[!A-Za-z0-9/._-]*) echo "refusing: the scratch path needs quoting this script does not do" >&2; rm -rf "$T"; exit 2 ;; esac
mkdir -p "$EVID"

# ---- bounded commands -----------------------------------------------------
TOCMD=
if command -v timeout > /dev/null 2>&1; then TOCMD=timeout
elif command -v gtimeout > /dev/null 2>&1; then TOCMD=gtimeout; fi
# bounded SECONDS cmd args...: an external command, killed at the bound.
bounded() {
  s=$1; shift
  if [ -n "$TOCMD" ]; then "$TOCMD" "$s" "$@"
  else perl -e 'alarm shift; exec @ARGV or exit 127' "$s" "$@"; fi
}
uuid() { python3 -I -c 'import uuid; print(uuid.uuid4())'; }
sha() { shasum -a 256 "$1" | cut -d' ' -f1; }
shaf() { if [ -e "$1" ]; then sha "$1"; else echo absent; fi; }

# ---- state the exit trap undoes -------------------------------------------
WATCHED=0   # the probe config holds watch's entries
SWAPPED=0   # those entries point at the spoof wrapper
SIDS=
cleanup() {
  set +e
  if [ "$SWAPPED" = 1 ]; then swap "$WRAP" "$REAL"; fi
  if [ "$WATCHED" = 1 ]; then
    bounded 60 env CLAUDE_CONFIG_DIR="$CFG" RASHOMON_HOME="$HOME_R" "$REAL" detach > "$T/detach.out" 2>&1
    cat "$T/detach.out"
    [ -e "$CFG/settings.json" ] && echo "WARNING: $CFG/settings.json is still there after detach" >&2
  fi
  for s in $SIDS; do
    for f in "$CFG"/projects/*/"$s".jsonl; do
      [ -e "$f" ] || continue
      rm -rf "$f" "${f%.jsonl}"; rmdir "$(dirname "$f")" 2> /dev/null
    done
    rmdir "$CFG/session-env/$s" 2> /dev/null
  done
  rm -rf "$T"
}
trap cleanup EXIT
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM

worst=0
verdict() { # verdict ROW PASS|FAIL|INCONCLUSIVE file
  case $1 in A1 | A8) echo "$1 $2" | tee -a "$3" ;; *) echo "$1 $2 ($MODE)" | tee -a "$3" ;; esac
  case $2 in
    FAIL) worst=1 ;;
    INCONCLUSIVE) [ "$worst" = 1 ] || worst=3 ;;
  esac
}
# run_tee FILE cmd...: run on the terminal (the person answers there), copy
# the output to FILE, and keep the command's own status in $RC.
run_tee() {
  f=$1; shift
  { "$@" 2>&1; echo $? > "$T/rc"; } | tee "$f"
  RC=$(cat "$T/rc")
}
has() { grep -qF -- "$1" "$2" 2> /dev/null; } # has TEXT FILE
yn() { if "$@"; then echo yes; else echo no; fi; } # yn CMD...: yes when it succeeds
rx() { grep -aEq -- "$1" "$2" 2> /dev/null; } # rx REGEX FILE
Q="'"

# chk FILE WHAT GOT WANT: one verdict line "<what>: <got> (want <x>)"; any
# GOT other than WANT turns OKALL off.
OKALL=1
chk() {
  echo "$2: $3 (want $4)" >> "$1"
  [ "$3" = "$4" ] || OKALL=0
}
# finish ROW FILE: PASS when every chk agreed, else FAIL.
finish() { if [ "$OKALL" = 1 ]; then verdict "$1" PASS "$2"; else verdict "$1" FAIL "$2"; fi; }

# attach_count FILE TEXT: how many transcript records hold TEXT in their
# "attachment", which is what a hook gave the agent (hc_shape.py reads the
# same key); 0 with no file.
attach_count() {
  [ -f "$1" ] || { echo 0; return 0; }
  python3 -I -c '
import json, sys
n = 0
for l in open(sys.argv[1], errors="replace"):
    try:
        r = json.loads(l)
    except ValueError:
        continue
    a = r.get("attachment") if isinstance(r, dict) else None
    if isinstance(a, dict) and sys.argv[2] in json.dumps(a, ensure_ascii=False):
        n += 1
print(n)' "$1" "$2"
}
# agent_count FILE TEXT: attach_count, or, when no attachment holds TEXT and a
# non-user transcript line does, that line count (a Claude Code that files a
# hook's context under another key must not read as "never delivered").
agent_count() {
  n=$(attach_count "$1" "$2")
  if [ "$n" = 0 ] && [ -f "$1" ]; then n=$(grep -F -- "$2" "$1" | grep -vc '"type":"user"' || true); fi
  echo "${n:-0}"
}

# steer_q count|id FILE PATTERN [FIELD=VALUE | ~TEXT]...: over the steer log's
# lines of PATTERN that have each field equal to VALUE ("null" for a null) and
# hold each TEXT: how many, or the first one's id.
steer_q() {
  mode=$1; f=$2; p=$3; shift 3
  if [ ! -f "$f" ]; then if [ "$mode" = count ]; then echo 0; else echo; fi; return 0; fi
  python3 -I -c '
import json, sys
mode, f, p = sys.argv[1:4]
conds = sys.argv[4:]
n, first = 0, ""
for l in open(f, errors="replace"):
    try:
        d = json.loads(l)
    except ValueError:
        continue
    if d.get("pattern") != p:
        continue
    ok = True
    for c in conds:
        if c.startswith("~"):
            ok = ok and c[1:] in l
        else:
            k, v = c.split("=", 1)
            g = d.get(k)
            ok = ok and ("null" if g is None else str(g)) == v
    if ok:
        n += 1
        first = first or str(d.get("id", ""))
print(n if mode == "count" else first)' "$mode" "$f" "$p" "$@"
}

# ver_ge A B: dotted version A is at least B.
ver_ge() {
  python3 -I -c '
import re, sys
def v(s):
    m = re.match(r"(\d+(?:\.\d+)*)", s.strip())
    return [int(x) for x in m.group(1).split(".")] if m else []
sys.exit(0 if v(sys.argv[1]) >= v(sys.argv[2]) else 1)' "$1" "$2"
}

# typed DIR NAME PHRASE: yes when the session's prompt was submitted (the
# store's UserPromptSubmit marker, or a user line in the transcript with PHRASE).
typed() {
  if [ -f "$1/$2.prompt-fired" ] || grep '"type":"user"' "$1/$2.transcript.jsonl" 2> /dev/null | grep -qF -- "$3"; then echo yes; else echo no; fi
}
# user_said TRANSCRIPT TEXT: a user line of the transcript holds TEXT.
user_said() { grep '"type":"user"' "$1" 2> /dev/null | grep -qF -- "$2"; }
# ran_cmd TRANSCRIPT COMMAND: a Bash call whose command is exactly COMMAND.
ran_cmd() { grep '"name":"Bash"' "$1" 2> /dev/null | grep -qF -- "\"command\":\"$2\""; }

# ---- the RC binary --------------------------------------------------------
pick_bin() {
  BIN=${RA_BIN:-$EVID/A1/prefix/bin/rashomon}
  [ -x "$BIN" ] || { echo "refusing: no RC binary at $BIN; run A1 first or set RA_BIN" >&2; exit 2; }
  got=$("$BIN" version 2> /dev/null | head -n 1)
  if [ "${RA_ANY_VERSION:-}" != 1 ] && [ "$got" != "$VER" ]; then
    echo "refusing: $BIN prints '$got', not $VER (RA_ANY_VERSION=1 to run anyway)" >&2; exit 2
  fi
  mkdir -p "$T/real"; cp "$BIN" "$T/real/rashomon"; chmod 0755 "$T/real/rashomon"
  REAL=$T/real/rashomon
}

# ---- the spoof wrapper: A2 only -------------------------------------------
# claude sets AI_AGENT in a hook's environment itself, so exporting it here
# would be overwritten. The wrapper is what the entries point at for A2: it
# sets AI_AGENT for the hook only (RA_SPOOF_AGENT; steer.ClientVersion reads
# it, harness form claude-code_<dashed version>_harness), then execs the real
# binary. THE VERSION IS SPOOFED in the spoofed session; the floor is what is
# under test. The entry point is left alone, except under RA_HEADLESS=1:
# claude -p reports sdk-cli, the SDK surface, shut by design whatever the
# version (steer/notice.go), so the smoke check forces cli for the hook.
make_wrapper() {
  mkdir -p "$T/wrap"; WRAP=$T/wrap/rashomon
  cat > "$WRAP" << SH
#!/bin/sh
[ -n "\${RA_SPOOF_AGENT:-}" ] && AI_AGENT=\$RA_SPOOF_AGENT && export AI_AGENT
[ -n "\${RA_SPOOF_ENTRYPOINT:-}" ] && CLAUDE_CODE_ENTRYPOINT=\$RA_SPOOF_ENTRYPOINT && export CLAUDE_CODE_ENTRYPOINT
exec $REAL "\$@"
SH
  chmod 0755 "$WRAP"
}
# swap FROM TO: repoint every entry in the probe's settings.json. It is
# swapped back before detach, and the hash is checked, so detach restores the
# file byte for byte.
swap() {
  python3 -I -c '
import sys
p, a, b = sys.argv[1:4]
s = open(p).read()
n = s.count(a)
if n == 0:
    sys.exit("swap: no entry names " + a)
open(p, "w").write(s.replace(a, b))
print("swap: %d occurrences" % n)' "$CFG/settings.json" "$1" "$2"
}

# wrap_on / wrap_off: the entries point at the wrapper (and back). The MVP rows
# use it under RA_HEADLESS=1 only, so the entry point the hook sees is cli
# (claude_run passes RA_SPOOF_ENTRYPOINT); AI_AGENT is not spoofed.
wrap_on() {
  [ "$SWAPPED" = 1 ] && return 0
  [ -n "${WRAP:-}" ] || make_wrapper
  swap "$REAL" "$WRAP" > /dev/null; SWAPPED=1
}
wrap_off() {
  [ "$SWAPPED" = 1 ] || return 0
  swap "$WRAP" "$REAL" > /dev/null; SWAPPED=0
}

# ---- watch once for the claude rows ---------------------------------------
HOME_R=$T/home
setup_watch() { # setup_watch OUTFILE
  [ "$WATCHED" = 1 ] && return 0
  pick_bin
  export CLAUDE_CONFIG_DIR=$CFG RASHOMON_HOME=$HOME_R
  echo "watch: answer y at the terminal (and n to the Agent SDK question)."
  WATCHED=1
  run_tee "$1" "$REAL" watch
  if [ "$RC" != 0 ]; then echo "watch exited $RC; nothing to run" >&2; exit 2; fi
  SHA_WATCH=$(sha "$CFG/settings.json")
}

# finish_session NAME OUT: copy the session's transcript and its steer log out
# of the probe's directories, before the exit trap removes the transcript.
finish_session() {
  for f in "$CFG"/projects/*/"$SID".jsonl; do
    [ -f "$f" ] && cp "$f" "$2/$1.transcript.jsonl"
  done
  if [ -f "$HOME_R/runs/$SID/steer.ndjson" ]; then cp "$HOME_R/runs/$SID/steer.ndjson" "$2/$1.steer.ndjson"; fi
  # the store's marker that UserPromptSubmit fired: a prompt was submitted
  if [ -e "$HOME_R/runs/$SID/fired-UserPromptSubmit" ]; then : > "$2/$1.prompt-fired"; fi
}

# claude_run NAME PROMPT TOOLS DIR OUT [ENV...]: RA_HEADLESS only. One bounded
# claude -p session in DIR; sets SID and SCREEN (the stream).
claude_run() {
  name=$1 prompt=$2 tools=$3 dir=$4 out=$5; shift 5
  SID=$(uuid); SIDS="$SIDS $SID"
  mkdir -p "$out"
  SCREEN=$out/$name.stream.ndjson
  (cd "$dir" && bounded 600 env -u CI RASHOMON_HOLDOUT=off RA_SPOOF_ENTRYPOINT=cli "$@" \
    claude -p "$prompt" --session-id "$SID" --allowedTools "$tools" \
    --model haiku --max-turns 8 --output-format stream-json --verbose \
    < /dev/null > "$SCREEN" 2> "$out/$name.stderr") || echo "claude exit $? ($name)" >> "$out/$name.stderr"
  if [ -n "${PROMPT2:-}" ]; then
    (cd "$dir" && bounded 600 env -u CI RASHOMON_HOLDOUT=off RA_SPOOF_ENTRYPOINT=cli "$@" \
      claude -p "$PROMPT2" --resume "$SID" --allowedTools "$tools" \
      --model haiku --max-turns 8 --output-format stream-json --verbose \
      < /dev/null >> "$SCREEN" 2>> "$out/$name.stderr") || echo "claude exit $? ($name, second prompt)" >> "$out/$name.stderr"
  fi
  finish_session "$name" "$out"
}

# interactive_run NAME PROMPT TOOLS DIR OUT [ENV...]: an interactive claude on
# the terminal, bounded at 30 minutes; the person types PROMPT, approves the
# tool calls and types /exit. Sets SID and SCREEN (a script(1) capture, or
# none). TOOLS is not used: the person approves each call.
interactive_run() {
  name=$1 prompt=$2 dir=$4 out=$5; shift 5
  SID=$(uuid); SIDS="$SIDS $SID"
  mkdir -p "$out"
  SCREEN=$out/$name.typescript
  copied=
  if command -v pbcopy > /dev/null 2>&1 && printf '%s' "$prompt" | pbcopy 2> /dev/null; then copied=yes; fi
  {
    echo
    echo "================================================================"
    echo "== $name: claude starts in $dir under $CFG"
    echo "================================================================"
    echo "1. Claude Code may FIRST ask whether you trust this folder. ACCEPT it"
    echo "   (choose Yes); the session cannot start until you do."
    echo "2. Then TYPE (or paste) this prompt EXACTLY, and press Enter:"
    echo
    echo "$prompt"
    echo
    if [ -n "$copied" ]; then echo "   (the prompt is on your clipboard: paste it with Cmd-V)"; fi
    if [ -n "${PROMPT2:-}" ]; then
      echo "   THEN, when it has answered, TYPE (or paste) this SECOND prompt EXACTLY, and press Enter:"
      echo
      echo "$PROMPT2"
      echo
    fi
    echo "3. APPROVE each tool call it asks about."
    echo "4. When it says done (to the last prompt), type /exit."
    echo "If you type /exit without the prompt, this row cannot be judged."
    echo
    printf 'Press Enter to start claude. '
  } > /dev/tty
  read -r _ < /dev/tty
  cmd="claude --session-id $SID --permission-mode default --model haiku"
  tout=
  if [ -n "$TOCMD" ] && [ "$TOCMD" = timeout ]; then tout="timeout --foreground 1800"
  elif [ -n "$TOCMD" ]; then tout="$TOCMD --foreground 1800"; fi
  if command -v script > /dev/null 2>&1; then
    case $(uname -s) in
      Darwin) (cd "$dir" && $tout env -u CI RASHOMON_HOLDOUT=off "$@" script -q "$SCREEN" $cmd < /dev/tty > /dev/tty 2>&1) || true ;;
      *) (cd "$dir" && $tout env -u CI RASHOMON_HOLDOUT=off "$@" script -q -c "$cmd" "$SCREEN" < /dev/tty > /dev/tty 2>&1) || true ;;
    esac
  else
    SCREEN=/nonexistent
    (cd "$dir" && bounded 1800 env -u CI RASHOMON_HOLDOUT=off "$@" $cmd < /dev/tty > /dev/tty 2>&1) || true
  fi
  finish_session "$name" "$out"
}

# session NAME PROMPT TOOLS DIR OUT [ENV...]: the mode's runner.
session() {
  if [ "$HEADLESS" = 1 ]; then claude_run "$@"; else interactive_run "$@"; fi
}
# msession: session, with the wrapper in place under RA_HEADLESS=1.
msession() {
  if [ "$HEADLESS" = 1 ]; then wrap_on; fi
  session "$@"
  if [ "$HEADLESS" = 1 ]; then wrap_off; fi
}

# trust_spec DIR OUTFILE LABEL: spec trust for DIR's spec; you answer y.
trust_spec() {
  echo "spec trust ($3): answer y at the terminal."
  (cd "$1" && run_tee "$2" "$REAL" spec trust .claude/rashomon/spec.json)
  RC=$(cat "$T/rc")
  [ "$RC" = 0 ] || { echo "spec trust exited $RC; nothing to run" >&2; exit 2; }
}

# attest SCREENFILE TEXT: SEEN is yes (found in the terminal capture), yes (the
# person's word, asked on the terminal) or no. Headless never asks.
# attest SCREEN LINE [REGEX]: the line is in the capture as written, or, with
# REGEX, a line the extended regex matches (a line that has two forms).
attest() {
  if has "$2" "$1" || { [ -n "${3:-}" ] && grep -Eq -- "$3" "$1" 2> /dev/null; }; then SEEN="yes (found in the terminal capture)"; return 0; fi
  if [ "$HEADLESS" = 1 ]; then SEEN=no; return 0; fi
  printf 'The capture does not hold this line. Did the terminal show it after the reply?\n  %s\n[y/N] ' "$2" > /dev/tty
  read -r ans < /dev/tty
  case $ans in y | Y) SEEN="yes (the person's word)" ;; *) SEEN=no ;; esac
}

# steer_summary FILE: pattern, state, suppressed_by, version, notice per line; never a value.
steer_summary() {
  [ -f "$1" ] || { echo "(no steer.ndjson)"; return 0; }
  python3 -I -c '
import json, sys
for l in open(sys.argv[1]):
    d = json.loads(l)
    print(d.get("pattern"), d.get("state"), d.get("suppressed_by"), d.get("client_version"), d.get("notice"))' "$1"
}

scratch_project() { # scratch_project DIR: the guide's beat 2 and 5 project
  mkdir -p "$1/internal/parse" "$1/.claude/rashomon"
  printf 'module example.com/rkalpha\n\ngo 1.25\n' > "$1/go.mod"
  printf 'package parse\n' > "$1/internal/parse/lexer.go"
  printf 'package parse\n\nimport "testing"\n\nfunc TestLexer(t *testing.T) {}\n' > "$1/internal/parse/lexer_test.go"
}

# ============================================================ A1 ============
row_A1() {
  A=$EVID/A1; rm -rf "$A"; mkdir -p "$A/gobin" "$A/prefix/bin" "$A/plugin"
  out=$A/install.txt; : > "$out"
  tag_ok=0
  if command -v gh > /dev/null 2>&1 && bounded 60 gh api "repos/$REPO/git/ref/tags/$TAG" > /dev/null 2>&1; then tag_ok=1
  elif bounded 60 curl -fsSL --proto '=https' -o /dev/null "https://api.github.com/repos/$REPO/releases/tags/$TAG" > /dev/null 2>&1; then tag_ok=1; fi
  if [ "$tag_ok" != 1 ]; then
    echo "refusing: the tag $TAG does not exist in $REPO yet (or is not readable from here); A1 needs the release. Nothing was installed." >&2
    rm -rf "$A"; exit 2
  fi
  {
    echo "A1 install routes for $TAG, run at $(date -u +%Y-%m-%dT%H:%M:%SZ) on $(uname -sm)"
    echo "wanted: rashomon version prints $VER on all three; the sha256s are recorded facts, not compared (the three builds differ)"
  } >> "$out"
  ok=1

  # route 1: go install, from a directory outside any module
  mkdir -p "$T/gi"
  echo "== route 1: go install github.com/$REPO/cmd/rashomon@$TAG" >> "$out"
  if (cd "$T/gi" && GOBIN=$A/gobin GOFLAGS= bounded 600 go install "github.com/$REPO/cmd/rashomon@$TAG") >> "$out" 2>&1; then
    v=$("$A/gobin/rashomon" version 2>&1 | head -n 1)
    { echo "version: $v"; echo "sha256: $(sha "$A/gobin/rashomon")"
      echo "build info: $(go version -m "$A/gobin/rashomon" | grep -E '^[[:space:]]*(mod|path)[[:space:]]' | tr '\n' ' ')"; } >> "$out"
    if [ "$v" != "$VER" ]; then
      echo "MISMATCH: route 1 prints '$v' (main.go takes the module version from the build info when no ldflags set one)" >> "$out"
      ok=0
    fi
  else echo "route 1 FAILED to install" >> "$out"; ok=0; fi

  # route 2: install.sh from this checkout, into a scratch prefix, as a user runs it
  echo "== route 2: sh install.sh into $A/prefix/bin" >> "$out"
  pin=
  if [ "${RA_INSTALL_PIN:-}" = 1 ]; then pin=$TAG; fi
  if RASHOMON_REPO=$REPO RASHOMON_INSTALL_DIR=$A/prefix/bin RASHOMON_VERSION=$pin bounded 300 sh "$ROOT/install.sh" >> "$out" 2>&1; then
    v=$("$A/prefix/bin/rashomon" version 2>&1 | head -n 1)
    { echo "version: $v"; echo "sha256: $(sha "$A/prefix/bin/rashomon")"; } >> "$out"
    [ "$v" = "$VER" ] || { echo "MISMATCH: route 2 prints '$v'" >> "$out"; ok=0; }
  else echo "route 2 FAILED to install (with no RA_INSTALL_PIN it resolved releases/latest)" >> "$out"; ok=0; fi

  # route 3: the plugin zip from the release assets
  echo "== route 3: the plugin marketplace zip from the $TAG assets" >> "$out"
  got=0
  if command -v gh > /dev/null 2>&1 && bounded 300 gh release download "$TAG" --repo "$REPO" --pattern rashomon-plugin.zip --dir "$A/plugin" --clobber >> "$out" 2>&1; then got=1
  elif bounded 300 curl -fsSL --proto '=https' -o "$A/plugin/rashomon-plugin.zip" "https://github.com/$REPO/releases/download/$TAG/rashomon-plugin.zip" >> "$out" 2>&1; then got=1; fi
  if [ "$got" = 1 ]; then
    echo "zip sha256: $(sha "$A/plugin/rashomon-plugin.zip")" >> "$out"
    os=$(uname -s | tr '[:upper:]' '[:lower:]'); arch=$(uname -m)
    case $arch in x86_64) arch=amd64 ;; aarch64) arch=arm64 ;; esac
    mkdir -p "$A/plugin/x"
    unzip -q -o "$A/plugin/rashomon-plugin.zip" -d "$A/plugin/x" >> "$out" 2>&1 || true
    b=$A/plugin/x/bin/rashomon-$os-$arch
    if [ -f "$b" ]; then
      chmod 0755 "$b"
      v=$("$b" version 2>&1 | head -n 1)
      { echo "recorder: bin/rashomon-$os-$arch"; echo "version: $v"; echo "sha256: $(sha "$b")"; } >> "$out"
      [ "$v" = "$VER" ] || { echo "MISMATCH: route 3 prints '$v'" >> "$out"; ok=0; }
    else echo "route 3: the zip holds no bin/rashomon-$os-$arch" >> "$out"; ok=0; fi
  else echo "route 3 FAILED to fetch rashomon-plugin.zip" >> "$out"; ok=0; fi

  if [ "$ok" = 1 ]; then verdict A1 PASS "$out"; else verdict A1 FAIL "$out"; fi
}

# ============================================================ A2 ============
row_A2() {
  A=$EVID/A2; rm -rf "$A"; mkdir -p "$A"
  setup_watch "$A/watch.txt"
  make_wrapper
  swap "$REAL" "$WRAP" > /dev/null; SWAPPED=1
  claude --version > "$A/claude-version.txt" 2>&1 || true
  realv=$(head -n 1 "$A/claude-version.txt")
  P="Run go test ./... once. Then add one comment line to internal/parse/lexer.go. Run no other command. Reply with only the word done."
  # A trusted spec with one required test gate, in each project, answered y
  # on the terminal as A4 does: stale-evidence fires in a fresh store only
  # for a trusted spec's required gate (no learned index exists yet).
  for mode in spoofed real; do
    D=$T/a2-$mode; scratch_project "$D"
    printf '{"rashomon_spec": 1, "gates": [{"id": "unit", "kind": "test", "argv": ["go", "test", "./..."]}]}\n' > "$D/.claude/rashomon/spec.json"
    echo "spec trust ($mode project): answer y at the terminal."
    (cd "$D" && run_tee "$A/spec-trust-$mode.txt" "$REAL" spec trust .claude/rashomon/spec.json)
    RC=$(cat "$T/rc")
    [ "$RC" = 0 ] || { echo "spec trust exited $RC; nothing to run" >&2; exit 2; }
  done
  for mode in spoofed real; do
    D=$T/a2-$mode
    if [ "$mode" = spoofed ]; then
      echo "NOTE: the next session reads as Claude Code $SPOOF_VER. The version is spoofed; the floor is under test."
      session spoofed "$P" "Bash(go test:*) Edit" "$D" "$A" RA_SPOOF_AGENT=$SPOOF_AGENT
    else
      session real "$P" "Bash(go test:*) Edit" "$D" "$A"
    fi
  done
  v=$A/verdict.txt; : > "$v"
  {
    echo "mode: $MODE"
    echo "A2 run at $(date -u +%Y-%m-%dT%H:%M:%SZ); real claude: $realv; spoofed session: $SPOOF_VER, AI_AGENT=$SPOOF_AGENT for the hook only (a spoofed VERSION; the floor is what is under test)"
    echo "spec: gate unit = go test ./... (required), trusted at spec trust in each project"
    echo "-- steer lines, spoofed (pattern state suppressed_by client_version notice)"; steer_summary "$A/spoofed.steer.ndjson"
    echo "-- steer lines, real"; steer_summary "$A/real.steer.ndjson"
  } >> "$v"
  nctx="rashomon record: gate 'unit'"
  # was the prompt ever submitted: a user line holding it in the transcript,
  # or the store's UserPromptSubmit marker
  typed_sp=no; typed_re=no
  if [ -f "$A/spoofed.prompt-fired" ] || grep '"type":"user"' "$A/spoofed.transcript.jsonl" 2> /dev/null | grep -qF "Reply with only the word done"; then typed_sp=yes; fi
  if [ -f "$A/real.prompt-fired" ] || grep '"type":"user"' "$A/real.transcript.jsonl" 2> /dev/null | grep -qF "Reply with only the word done"; then typed_re=yes; fi
  # did the model do what the prompt asked: run go test, edit a file
  did_sp=no; did_re=no
  # (the prompt itself names go test, so look for a Bash tool call that runs it)
  grep '"name":"Bash"' "$A/spoofed.transcript.jsonl" 2> /dev/null | grep -qF "go test" && has '"name":"Edit"' "$A/spoofed.transcript.jsonl" && did_sp=yes
  grep '"name":"Bash"' "$A/real.transcript.jsonl" 2> /dev/null | grep -qF "go test" && has '"name":"Edit"' "$A/real.transcript.jsonl" && did_re=yes
  sp_notice=$(grep -c '"notice":"channel"' "$A/spoofed.steer.ndjson" 2> /dev/null || true)
  sp_ver=no; has "\"client_version\":\"$SPOOF_VER\"" "$A/spoofed.steer.ndjson" && sp_ver=yes
  sp_built=no; grep '"pattern":"stale-evidence"' "$A/spoofed.steer.ndjson" 2> /dev/null | grep -qF '"suppressed_by":"unverified_channel"' && sp_built=yes
  sp_ctx=no; has "$nctx" "$A/spoofed.transcript.jsonl" && sp_ctx=yes
  re_ctx=no; has "$nctx" "$A/real.transcript.jsonl" && re_ctx=yes
  re_line=no; grep '"pattern":"stale-evidence"' "$A/real.steer.ndjson" 2> /dev/null | grep -qF '"state":"delivered"' && re_line=yes
  re_notice=$(grep -c '"notice":"channel"' "$A/real.steer.ndjson" 2> /dev/null || true)
  {
    echo "spoofed: the prompt was submitted: $typed_sp (want yes); the model ran go test and edited: $did_sp (want yes)"
    echo "real: the prompt was submitted: $typed_re (want yes); the model ran go test and edited: $did_re (want yes)"
    echo "spoofed: notice lines recorded: ${sp_notice:-0} (want 1)"
    echo "spoofed: the steer log names client version $SPOOF_VER: $sp_ver (want yes)"
    echo "spoofed: a stale-evidence line shadowed (unverified_channel): $sp_built (want yes)"
    echo "spoofed: note in the agent's transcript: $sp_ctx (want no)"
    echo "real: a stale-evidence line delivered in the steer log: $re_line (want yes)"
    echo "real: note in the agent's transcript: $re_ctx (want yes)"
    echo "real: notice lines recorded: ${re_notice:-0} (want 0)"
  } >> "$v"
  if [ "$typed_sp" = no ] || [ "$typed_re" = no ]; then
    who=
    [ "$typed_sp" = no ] && who="spoofed"
    [ "$typed_re" = no ] && who="${who:+$who and }real"
    echo "the prompt was never typed in the $who session: no user prompt in its transcript and no UserPromptSubmit in the store (claude was started and exited with nothing typed, or the folder-trust dialog was not accepted). Not a product failure; run A2 again and type the prompt." >> "$v"
    verdict A2 INCONCLUSIVE "$v"
  elif [ ! -f "$A/spoofed.steer.ndjson" ] || [ ! -f "$A/real.steer.ndjson" ]; then
    echo "no steer record for a session: the hooks did not run, or no call reached the steer step" >> "$v"
    verdict A2 INCONCLUSIVE "$v"
  elif [ "$did_sp" = no ] || [ "$did_re" = no ]; then
    echo "a session did not run go test and edit a file as the prompt asked: nothing to judge" >> "$v"
    verdict A2 INCONCLUSIVE "$v"
  elif [ "${sp_notice:-0}" = 1 ] && [ "$sp_ver" = yes ] && [ "$sp_built" = yes ] && [ "$sp_ctx" = no ] && [ "$re_line" = yes ] && [ "$re_ctx" = yes ] && [ "${re_notice:-0}" = 0 ]; then
    verdict A2 PASS "$v"
  else verdict A2 FAIL "$v"; fi
  swap "$WRAP" "$REAL" > /dev/null; SWAPPED=0
  [ "$(sha "$CFG/settings.json")" = "$SHA_WATCH" ] || echo "WARNING: settings.json differs from watch's after the swap back" >&2
}

# ============================================================ A4 ============
A4_DONE=0
row_A4() {
  A=$EVID/A4; rm -rf "$A"; mkdir -p "$A"
  setup_watch "$A/watch.txt"
  D=$T/a4; scratch_project "$D"
  printf '{"rashomon_spec": 1, "gates": [{"id": "unit", "kind": "test", "argv": ["go", "test", "./..."]}]}\n' > "$D/.claude/rashomon/spec.json"
  echo "spec trust: answer y at the terminal."
  (cd "$D" && run_tee "$A/spec-trust.txt" "$REAL" spec trust .claude/rashomon/spec.json)
  RC=$(cat "$T/rc")
  [ "$RC" = 0 ] || { echo "spec trust exited $RC; nothing to run" >&2; exit 2; }
  "$REAL" status > "$A/status.txt" 2>&1 || true
  session done "Add one comment line to internal/parse/lexer.go. Run no commands. Reply with only the word done." "Edit" "$D" "$A"
  SID_DONE=$SID; SCREEN_DONE=$SCREEN
  session verified "Add a second comment line to internal/parse/lexer.go, then run go test ./... Reply with only the word done." "Edit Bash(go test:*)" "$D" "$A"
  SID_VER=$SID; SCREEN_VER=$SCREEN
  "$REAL" report --session "$SID_DONE" > "$A/done.report.txt" 2>&1 || true
  "$REAL" report --session "$SID_VER" > "$A/verified.report.txt" 2>&1 || true
  echo "$SID_DONE" > "$A/done.session"; echo "$SID_VER" > "$A/verified.session"
  v=$A/verdict.txt; : > "$v"
  dl="rashomon: done was claimed; the record shows 1 file edited, no test or build run after the last edit."
  vl="rashomon: verified - go test passed after the last edit."
  vl_re='rashomon: verified - (go( test)?|'"'"'unit'"'"')( and [^ ]+)* passed after the last edit\.'
  attest "$SCREEN_DONE" "$dl"; d=$SEEN
  attest "$SCREEN_VER" "$vl" "$vl_re"; e=$SEEN
  crossd=no; grep -Eq -- "$vl_re" "$SCREEN_DONE" 2> /dev/null && crossd=yes
  {
    echo "mode: $MODE"
    echo "A4 run at $(date -u +%Y-%m-%dT%H:%M:%SZ); binary $("$REAL" version)"
    echo "edit, no run: the done line at Stop: $d (want yes); a verified line there: $crossd (want no)"
    echo "edit, then go test: the verified line at Stop: $e (want yes)"
  } >> "$v"
  if [ ! -s "$A/done.report.txt" ] || has "no sessions recorded" "$A/done.report.txt" || has "no sessions recorded" "$A/verified.report.txt"; then
    echo "no session was recorded: the hooks did not run" >> "$v"; verdict A4 INCONCLUSIVE "$v"
  elif [ "${d#yes}" != "$d" ] && [ "${e#yes}" != "$e" ] && [ "$crossd" = no ]; then verdict A4 PASS "$v"
  else
    echo "if a line is missing from the stream, check the transcript and the report: the line may print where the stream does not carry it" >> "$v"
    verdict A4 FAIL "$v"
  fi
  A4_DONE=1
}

# ============================================================ A7 ============
row_A7() {
  A=$EVID/A7; rm -rf "$A"; mkdir -p "$A"
  v=$A/verdict.txt; : > "$v"
  src=$EVID/A4
  ok=1
  for k in done verified; do
    cp "$src/$k.report.txt" "$A/$k.report.txt" 2> /dev/null || { echo "no A4 $k report" >> "$v"; ok=0; continue; }
    sid=$(cat "$src/$k.session")
    for want in "session $sid" "declarations recorded:" "executions recorded:" "coverage:"; do
      if has "$want" "$A/$k.report.txt"; then echo "$k: has '$want'" >> "$v"; else echo "$k: MISSING '$want'" >> "$v"; ok=0; fi
    done
    echo "$k: edits named: $(grep -c 'by tool:.*\(Edit\|Write\)' "$A/$k.report.txt") line(s); gates named (by program): $(grep -c 'by program:.*go' "$A/$k.report.txt") line(s)" >> "$v"
  done
  echo "notes: the report does not print the steer notes; they are in $src/*.steer.ndjson (REPORT1 owns the first-screen story)" >> "$v"
  if [ "$ok" = 1 ]; then verdict A7 PASS "$v"; else verdict A7 FAIL "$v"; fi
}

# ============================================================ A8 ============
row_A8() {
  A=$EVID/A8; rm -rf "$A"; mkdir -p "$A"; out=$A/uninstall.txt; : > "$out"
  pick_bin
  allok=1
  for variant in seeded absent; do
    cfg=$T/a8-$variant-cfg; hm=$T/a8-$variant-home; mkdir -p "$cfg"
    if [ $variant = seeded ]; then
      printf '{\n  "permissions": {"allow": ["Bash(ls:*)"]},\n  "env": {"RA_SEED": "1"},\n  "hooks": {"PreToolUse": [{"matcher": "Bash", "hooks": [{"type": "command", "command": "true"}]}]}\n}\n' > "$cfg/settings.json"
    fi
    s0=$(shaf "$cfg/settings.json")
    echo "== A8 ($variant settings.json), scratch config $cfg, store $hm" | tee -a "$out"
    echo "sha256 before watch:   $s0" | tee -a "$out"
    echo "watch: answer y (and n to the Agent SDK question)."
    run_tee "$T/w.out" env CLAUDE_CONFIG_DIR="$cfg" RASHOMON_HOME="$hm" "$REAL" watch; echo "-- watch (exit $RC)" >> "$out"; cat "$T/w.out" >> "$out"
    [ "$RC" = 0 ] || { echo "watch exited $RC" | tee -a "$out"; allok=0; continue; }
    echo "sha256 after watch:    $(shaf "$cfg/settings.json")" | tee -a "$out"
    echo "-- detach" >> "$out"
    bounded 60 env CLAUDE_CONFIG_DIR="$cfg" RASHOMON_HOME="$hm" "$REAL" detach > "$T/d.out" 2>&1 || echo "detach exited $?" >> "$T/d.out"
    cat "$T/d.out" | tee -a "$out"
    s1=$(shaf "$cfg/settings.json")
    echo "sha256 after detach:   $s1" | tee -a "$out"
    echo "purge: answer y at the terminal."
    run_tee "$T/p.out" env CLAUDE_CONFIG_DIR="$cfg" RASHOMON_HOME="$hm" "$REAL" purge; echo "-- purge (exit $RC)" >> "$out"; cat "$T/p.out" >> "$out"
    s2=$(shaf "$cfg/settings.json")
    echo "sha256 after purge:    $s2" | tee -a "$out"
    stg=present; [ -e "$hm" ] || stg=gone
    echo "store after purge: $stg (want gone)" | tee -a "$out"
    if [ "$s0" = "$s1" ] && [ "$s0" = "$s2" ] && [ "$stg" = gone ] && [ "$RC" = 0 ]; then echo "$variant: hashes equal" | tee -a "$out"
    else echo "$variant: DIFFERENT or not purged" | tee -a "$out"; allok=0; fi
  done
  if [ "$allok" = 1 ]; then verdict A8 PASS "$out"; else verdict A8 FAIL "$out"; fi
}

# ============================================================ A5 ============
py_project() { # py_project DIR PASS|FAIL: one pytest test, a trusted-spec gate unit = pytest -q
  mkdir -p "$1/.claude/rashomon"
  printf 'def add(a, b):\n    return a + b\n' > "$1/calc.py"
  want=4
  if [ "$2" = FAIL ]; then want=5; fi
  printf 'from calc import add\n\n\ndef test_add():\n    assert add(2, 2) == %s\n' "$want" > "$1/test_calc.py"
  printf '%s\n' '{"rashomon_spec": 1, "gates": [{"id": "unit", "kind": "test", "argv": ["pytest", "-q"]}]}' > "$1/.claude/rashomon/spec.json"
}

# The Stop lines A5 and A13 read (internal/recap/value.go): rowSentence's
# RowVerified "verified - <gate> passed after the last edit" and RowRed
# "<gate> failed after the last edit (call N)", each followed by readFromOutput
# " (read from its output; the pipe hid the exit status)" when the outcome came
# from the output. A declared gate is named by its id in the slot's quote
# ('unit', SNG1 in internal/recap/stop_names_gate_test.go), a learned one by its
# program word, so the regexes take either.
row_A5() {
  A=$EVID/A5; rm -rf "$A"; mkdir -p "$A"; v=$A/verdict.txt; : > "$v"; OKALL=1
  if ! command -v python3 > /dev/null 2>&1 || ! command -v pytest > /dev/null 2>&1; then
    echo "pytest is not on PATH (python3: $(command -v python3 || echo missing); pytest: $(command -v pytest || echo missing)); A5 needs both. Nothing was run; this is not a product failure." >> "$v"
    verdict A5 INCONCLUSIVE "$v"; return 0
  fi
  setup_watch "$A/watch.txt"
  pytest --version > "$A/pytest-version.txt" 2>&1 || true
  DP=$T/a5-pass; DF=$T/a5-fail
  py_project "$DP" PASS; py_project "$DF" FAIL
  trust_spec "$DP" "$A/spec-trust-pass.txt" "A5 passing project"
  trust_spec "$DF" "$A/spec-trust-fail.txt" "A5 failing project"
  c_plain="pytest -q"; c_pipe="pytest -q | tail -5"
  session plain "Reply with only the word done when finished. Steps: 1. Add one comment line to calc.py. 2. Run exactly this one command, with nothing added: $c_plain" "Edit Bash(pytest:*)" "$DP" "$A"
  SA1=$SID; SC1=$SCREEN
  session piped "Reply with only the word done when finished. Steps: 1. Add one comment line to calc.py. 2. Run exactly this one command, with nothing added: $c_pipe" "Edit Bash(pytest:*) Bash(tail:*)" "$DP" "$A"
  SA2=$SID; SC2=$SCREEN
  session failing "Reply with only the word done when finished. Steps: 1. Add one comment line to calc.py. 2. Run exactly this one command, with nothing added: $c_pipe" "Edit Bash(pytest:*) Bash(tail:*)" "$DF" "$A"
  SA3=$SID; SC3=$SCREEN
  for n in 1 2 3; do
    eval "sid=\$SA$n"
    case $n in 1) nm=plain ;; 2) nm=piped ;; *) nm=failing ;; esac
    bounded 60 "$REAL" report --session "$sid" > "$A/$nm.report.txt" 2>&1 || true
  done
  {
    echo "mode: $MODE"
    echo "A5 run at $(date -u +%Y-%m-%dT%H:%M:%SZ); binary $("$REAL" version); $(head -n 1 "$A/pytest-version.txt")"
    echo "spec: gate unit = pytest -q (required), trusted at spec trust in each project"
    echo "-- steer lines, unpiped (pattern state suppressed_by client_version notice)"; steer_summary "$A/plain.steer.ndjson"
    echo "-- steer lines, piped green"; steer_summary "$A/piped.steer.ndjson"
    echo "-- steer lines, piped failing"; steer_summary "$A/failing.steer.ndjson"
  } >> "$v"
  t1=$(typed "$A" plain "only the word done"); t2=$(typed "$A" piped "only the word done"); t3=$(typed "$A" failing "only the word done")
  d1=no; d2=no; d3=no
  if ran_cmd "$A/plain.transcript.jsonl" "$c_plain" && has '"name":"Edit"' "$A/plain.transcript.jsonl"; then d1=yes; fi
  if ran_cmd "$A/piped.transcript.jsonl" "$c_pipe" && has '"name":"Edit"' "$A/piped.transcript.jsonl"; then d2=yes; fi
  if ran_cmd "$A/failing.transcript.jsonl" "$c_pipe" && has '"name":"Edit"' "$A/failing.transcript.jsonl"; then d3=yes; fi
  ok_re="verified - (${Q}unit${Q}|pytest) passed after the last edit"
  fl_re="(${Q}unit${Q}|pytest) failed (after the last edit|in this turn)"
  fo="read from its output; the pipe hid the exit status"
  attest "$SC1" "verified - 'unit' passed after the last edit" "$ok_re"; g1=$SEEN
  c1=$(yn rx "$fo" "$SC1")
  attest "$SC2" "verified - 'unit' passed after the last edit" "$ok_re"; g2=$SEEN
  attest "$SC2" "$fo" "read from its output"; p2=$SEEN
  f2=$(yn rx 'hid its result|exit status was hidden by its line' "$SC2")
  m2=$(steer_q count "$A/piped.steer.ndjson" masked-status)
  attest "$SC3" "failed after the last edit" "$fl_re"; g3=$SEEN
  attest "$SC3" "$fo" "read from its output"; p3=$SEEN
  vv3=$(yn rx 'verified - .* passed after the last edit' "$SC3")
  yes_of() { case $1 in yes*) echo yes ;; *) echo no ;; esac; }
  {
    echo "(1) unpiped: the prompt was submitted: $t1 (want yes); the model edited and ran exactly '$c_plain': $d1 (want yes)"
    echo "(2) piped green: the prompt was submitted: $t2 (want yes); the model edited and ran exactly '$c_pipe': $d2 (want yes)"
    echo "(3) piped failing: the prompt was submitted: $t3 (want yes); the model edited and ran exactly '$c_pipe': $d3 (want yes)"
  } >> "$v"
  chk "$v" "(1) unpiped pytest -q green: the verified line at Stop [$g1]" "$(yes_of "$g1")" yes
  chk "$v" "(1) unpiped: a 'read from its output' clause" "$c1" no
  chk "$v" "(2) pytest -q | tail -5 green: the verified line at Stop [$g2]" "$(yes_of "$g2")" yes
  chk "$v" "(2) piped green: the 'read from its output; the pipe hid the exit status' clause [$p2]" "$(yes_of "$p2")" yes
  chk "$v" "(2) piped green: a 'hid its result' or 'hidden by its line' line (false alarm)" "$f2" no
  chk "$v" "(2) piped green: masked-status lines in the steer log" "${m2:-0}" 0
  chk "$v" "(3) failing pytest -q | tail -5: the failure line at Stop [$g3]" "$(yes_of "$g3")" yes
  chk "$v" "(3) failing: the 'read from its output' clause [$p3]" "$(yes_of "$p3")" yes
  chk "$v" "(3) failing: a verified line" "$vv3" no
  if [ "$t1" = no ] || [ "$t2" = no ] || [ "$t3" = no ]; then
    echo "a prompt was never typed in a session (no user prompt in its transcript and no UserPromptSubmit in the store: claude was started and exited with nothing typed, or the folder-trust dialog was not accepted). Not a product failure; run A5 again and type the prompts." >> "$v"
    verdict A5 INCONCLUSIVE "$v"
  elif [ "$d1" = no ] || [ "$d2" = no ] || [ "$d3" = no ]; then
    echo "a session did not edit a file and run the exact command asked: nothing to judge" >> "$v"
    verdict A5 INCONCLUSIVE "$v"
  else finish A5 "$v"; fi
}

# ============================================================ A13 ===========
go_green_project() { # go_green_project DIR: make test runs go test -v; the test logs "0 failed"
  mkdir -p "$1/.claude/rashomon"
  printf 'module example.com/rkgreen\n\ngo 1.25\n' > "$1/go.mod"
  printf 'package green\n' > "$1/green.go"
  printf 'package green\n\nimport "testing"\n\nfunc TestReport(t *testing.T) { t.Log("0 failed") }\n' > "$1/green_test.go"
  printf 'test:\n\tgo test -v ./...\n' > "$1/Makefile"
  printf '%s\n' '{"rashomon_spec": 1, "gates": [{"id": "unit", "kind": "test", "argv": ["make", "test"]}]}' > "$1/.claude/rashomon/spec.json"
}

# What alpha.1 misread: gateout's pytest fail form was a bare `\d+ (failed|error)`
# matched anywhere in a line, and a delegated run (make, npm) is matched against
# every table, so the log line "green_test.go:5: 0 failed" read summary_fail and
# the gate went red over a make test that exited 0 (internal/gateout/zero_test.go
# TestGZ1_AZeroCountOrALogLineIsNotAFailure, internal/hook/gateoutput_zero_test.go
# TestGZ1b_AGreenDelegatedRunIsNotReadRed, on origin/fix/gateout-zero1; the
# classifier was run on this exact output at the branch base and read it
# summary_fail). go prints a passing package's t.Log only under -v, which is why
# the Makefile's test target is `go test -v ./...`.
row_A13() {
  A=$EVID/A13; rm -rf "$A"; mkdir -p "$A"; v=$A/verdict.txt; : > "$v"; OKALL=1
  if ! command -v go > /dev/null 2>&1 || ! command -v make > /dev/null 2>&1; then
    echo "go and make are both needed (go: $(command -v go || echo missing); make: $(command -v make || echo missing)). Nothing was run; this is not a product failure." >> "$v"
    verdict A13 INCONCLUSIVE "$v"; return 0
  fi
  setup_watch "$A/watch.txt"
  D=$T/a13; go_green_project "$D"
  trust_spec "$D" "$A/spec-trust.txt" "A13 project"
  session green "Reply with only the word done when finished. Steps: 1. Add one comment line to green.go. 2. Run exactly this one command, with nothing added: make test" "Edit Bash(make:*)" "$D" "$A"
  SA1=$SID; SC1=$SCREEN
  bounded 60 "$REAL" report --session "$SA1" > "$A/green.report.txt" 2>&1 || true
  {
    echo "mode: $MODE"
    echo "A13 run at $(date -u +%Y-%m-%dT%H:%M:%SZ); binary $("$REAL" version); $(go version)"
    echo "project: Makefile test: go test -v ./...; green_test.go logs '0 failed'; spec gate unit = make test (required), trusted"
    echo "-- steer lines (pattern state suppressed_by client_version notice)"; steer_summary "$A/green.steer.ndjson"
  } >> "$v"
  t1=$(typed "$A" green "only the word done")
  d1=no
  if ran_cmd "$A/green.transcript.jsonl" "make test" && has '"name":"Edit"' "$A/green.transcript.jsonl"; then d1=yes; fi
  attest "$SC1" "verified - 'unit' passed after the last edit" "verified - (${Q}unit${Q}|make) passed after the last edit"
  g1=$SEEN
  red=$(yn rx "(${Q}unit${Q}|make) (failed|exited [0-9]+) (after the last edit|in this turn)" "$SC1")
  echo "the prompt was submitted: $t1 (want yes); the model edited and ran exactly 'make test': $d1 (want yes)" >> "$v"
  case $g1 in yes*) g1y=yes ;; *) g1y=no ;; esac
  chk "$v" "a green make test whose output holds '0 failed': the verified line at Stop [$g1]" "$g1y" yes
  chk "$v" "a failed or exited line for the gate (a false red)" "$red" no
  if [ "$t1" = no ]; then
    echo "the prompt was never typed (no user prompt in the transcript and no UserPromptSubmit in the store). Not a product failure; run A13 again and type the prompt." >> "$v"
    verdict A13 INCONCLUSIVE "$v"
  elif [ "$d1" = no ]; then
    echo "the session did not edit a file and run make test as asked: nothing to judge" >> "$v"
    verdict A13 INCONCLUSIVE "$v"
  else
    if [ "$red" = yes ]; then echo "a make test that exited 0 was read red: the build under test does not have GATEOUT-ZERO1 (#180); alpha.1 reads this output as a failure" >> "$v"; fi
    finish A13 "$v"
  fi
}

# ========================================================= MVP smoke ========
# One watch, one project, one to three sessions for A14-A21 (header). Raw
# evidence under $EVID/MVP; every row copies what it judges.
MVP_DONE=0; MVP_SKIP=; MVP_FLOOR=no; MVP_CLAUDE=
S1_TYPED=no; S1_DID=no; S2_TYPED=no; S2_DID=no; S3_OK=no; S3_WHY=
S1=; S2=; S3=; S1_SCREEN=/nonexistent; S2_SCREEN=/nonexistent; S3_SCREEN=/nonexistent
MVP_SPEC='{"rashomon_spec": 1, "gates": [{"id": "unit", "kind": "test", "argv": ["go", "test", "./..."]}], "rules": [{"id": "parse-owner", "kind": "protect_path", "why": "the parser belongs to the platform team", "hard_control": "code review by the platform team", "paths": ["internal/parse"]}, {"id": "no-uname", "kind": "forbid_command", "why": "the smoke test forbids uname", "hard_control": "the sandbox", "command": {"argv": ["uname"]}}]}'
LESSON_ID=keep-diffs-small
LESSON_TEXT="Keep each change small and name the file you changed."

mvp_run() {
  [ "$MVP_DONE" = 1 ] && return 0
  MVP_DONE=1
  M=$EVID/MVP; rm -rf "$M"; mkdir -p "$M"
  if ! command -v go > /dev/null 2>&1; then MVP_SKIP="go is not on PATH; the MVP smoke project is a Go project. Nothing was run."; return 0; fi
  setup_watch "$M/watch.txt"
  claude --version > "$M/claude-version.txt" 2>&1 || true
  MVP_CLAUDE=$(head -n 1 "$M/claude-version.txt")
  if ver_ge "$MVP_CLAUDE" 2.1.280; then MVP_FLOOR=yes; fi
  MD=$T/mvp; scratch_project "$MD"
  # a work tree: the verbs below (brief, report --ledger, why) run from a plain
  # terminal, where nothing but a .git names the project root a spec is
  # discovered up to (spec.WalkRoot; Claude Code's own hooks also have
  # CLAUDE_PROJECT_DIR). git init when git is there, else a bare .git directory.
  (cd "$MD" && git init -q 2> /dev/null) || mkdir -p "$MD/.git"
  printf '%s\n' "$MVP_SPEC" > "$MD/.claude/rashomon/spec.json"
  trust_spec "$MD" "$M/spec-trust.txt" "MVP project"
  (cd "$MD" && bounded 60 "$REAL" brief "$MD") > "$M/brief.verb.txt" 2>&1 || true

  # session one: the call-time raises, the Stop line, the ledger, the status, the why
  msession rules "Do exactly these three steps in order, and run no other command: (1) add one comment line to internal/parse/lexer.go; (2) run the command uname -s; (3) run the command go test ./... Then reply with only the word done." "Edit Bash(uname:*) Bash(go test:*)" "$MD" "$M"
  S1=$SID; S1_SCREEN=$SCREEN
  (cd "$MD" && bounded 60 "$REAL" status --json) > "$M/status.json" 2> "$M/status.json.err" || true
  (cd "$MD" && bounded 60 "$REAL" status) > "$M/status.txt" 2>&1 || true
  S1_TYPED=$(typed "$M" rules "only the word done")
  if ran_cmd "$M/rules.transcript.jsonl" "uname -s" && ran_cmd "$M/rules.transcript.jsonl" "go test ./..." && has '"name":"Edit"' "$M/rules.transcript.jsonl"; then S1_DID=yes; fi

  # session two (A20): the same uname call with the project's notes off
  case " $ROWS " in *" A20 "*)
    steer_off_rc=0
    (cd "$MD" && bounded 60 "$REAL" steer off --project) > "$M/steer-off.txt" 2>&1 || steer_off_rc=$?
    echo "$steer_off_rc" > "$M/steer-off.rc"
    msession steeroff "Run the command uname -s and no other command. Reply with only the word done." "Bash(uname:*)" "$MD" "$M"
    S2=$SID; S2_SCREEN=$SCREEN
    (cd "$MD" && bounded 60 "$REAL" steer) > "$M/steer-state-off.txt" 2>&1 || true
    (cd "$MD" && bounded 60 "$REAL" steer log --all) > "$M/steer-log.txt" 2>&1 || true
    (cd "$MD" && bounded 60 "$REAL" steer on --project) > "$M/steer-on.txt" 2>&1 || true
    bounded 60 "$REAL" report --session "$S2" > "$M/steeroff.report.txt" 2>&1 || true
    S2_TYPED=$(typed "$M" steeroff "only the word done")
    if ran_cmd "$M/steeroff.transcript.jsonl" "uname -s"; then S2_DID=yes; fi
    ;;
  esac

  # session three (A17): import, approve on the terminal, two prompts in one session
  case " $ROWS " in *" A17 "*)
    printf '{"schema": "lessons/1", "lessons": [{"id": "%s", "text": "%s"}]}\n' "$LESSON_ID" "$LESSON_TEXT" > "$T/lessons.json"
    irc=0
    (cd "$MD" && bounded 60 "$REAL" learn import "$T/lessons.json") > "$M/learn-import.txt" 2>&1 || irc=$?
    echo "$irc" > "$M/learn-import.rc"
    echo "learn approve $LESSON_ID: a six-digit code is shown on your terminal; type it back there."
    (cd "$MD" && run_tee "$M/learn-approve.txt" "$REAL" learn approve "$LESSON_ID") || true
    arc=$(cat "$T/rc")
    echo "$arc" > "$M/learn-approve.rc"
    (cd "$MD" && bounded 60 "$REAL" learn) > "$M/learn-list.txt" 2>&1 || true
    if [ "$irc" = 0 ] && [ "$arc" = 0 ]; then
      PROMPT2="Reply with only the word ok."
      msession learn "Reply with only the word done." "Read" "$MD" "$M"
      PROMPT2=
      S3=$SID; S3_SCREEN=$SCREEN; S3_OK=yes
    else S3_WHY="learn import exited $irc and learn approve exited $arc (you type the code at approve; a wrong code approves nothing)"; fi
    ;;
  esac
}

# mvp_ready ROW FILE NEEDS: the shared sessions ran once; INCONCLUSIVE (return
# 1) where they cannot be judged. NEEDS: none, s1 (session one typed and done),
# note (s1, and a Claude Code at PostToolUse's floor).
mvp_ready() {
  mvp_run
  if [ -n "$MVP_SKIP" ]; then echo "$MVP_SKIP" >> "$2"; verdict "$1" INCONCLUSIVE "$2"; return 1; fi
  case $3 in s1 | note)
    if [ "$S1_TYPED" = no ]; then
      echo "the prompt was never typed in session one (no user prompt in its transcript and no UserPromptSubmit in the store: claude was started and exited with nothing typed, or the folder-trust dialog was not accepted). Not a product failure; run again and type the prompt." >> "$2"
      verdict "$1" INCONCLUSIVE "$2"; return 1
    fi
    if [ "$S1_DID" = no ]; then
      echo "session one did not edit internal/parse/lexer.go and run exactly 'uname -s' and 'go test ./...' as asked: nothing to judge" >> "$2"
      verdict "$1" INCONCLUSIVE "$2"; return 1
    fi ;;
  esac
  if [ "$3" = note ] && [ "$MVP_FLOOR" = no ]; then
    echo "claude is '$MVP_CLAUDE', below 2.1.280, the floor of PostToolUse (internal/steer/client.go): notes are shut by design there, so no note can be judged. Not a product failure." >> "$2"
    verdict "$1" INCONCLUSIVE "$2"; return 1
  fi
  return 0
}

mvp_head() { # mvp_head ROW FILE
  { echo "mode: $MODE"
    echo "$1 run at $(date -u +%Y-%m-%dT%H:%M:%SZ); binary $("$REAL" version); claude $MVP_CLAUDE"
    echo "project: scratch Go project; spec gate unit = go test ./..., rules parse-owner (protect_path internal/parse) and no-uname (forbid_command uname), trusted at spec trust"; } >> "$2"
}

# ============================================================ A14 ===========
row_A14() {
  A=$EVID/A14; rm -rf "$A"; mkdir -p "$A"; v=$A/verdict.txt; : > "$v"; OKALL=1
  mvp_ready A14 "$v" s1 || return 0
  mvp_head A14 "$v"
  rc=0
  (cd "$MD" && bounded 60 "$REAL" report --ledger --session "$S1") > "$A/ledger.txt" 2> "$A/ledger.err" || rc=$?
  (cd "$MD" && bounded 60 "$REAL" report --ledger --session "$S1") > "$A/ledger.again.txt" 2> /dev/null || true
  l=$A/ledger.txt
  chk "$v" "report --ledger --session <session one>: exit status" "$rc" 0
  chk "$v" "header: the point line (point: seq N, required gates as recorded)" "$(yn rx '^point: seq [0-9]+, required gates as recorded' "$l")" yes
  chk "$v" "header: the spec line names the trusted spec (spec: sha256:... (trusted))" "$(yn rx '^spec: sha256:[0-9a-f]+ \(trusted\)$' "$l")" yes
  chk "$v" "header: the task line" "$(yn has 'task: outside spec_digest, so a change since the Stop is not named' "$l")" yes
  chk "$v" "row: gate_after_write for the declared gate unit is met, citing a call, family spec:unit" "$(yn rx '^ob:[0-9a-f]+ [a-z]+ gate_after_write spec met [0-9,]+ spec:unit$' "$l")" yes
  chk "$v" "row: spec_rule for parse-owner is unmet, citing the write" "$(yn rx '^ob:[0-9a-f]+ [a-z]+ spec_rule spec unmet [0-9,]+ spec:parse-owner$' "$l")" yes
  chk "$v" "row: spec_rule for no-uname is unmet (command text recorded) or unknown (not recorded)" "$(yn rx '^ob:[0-9a-f]+ [a-z]+ spec_rule spec (unmet [0-9,]+|unknown -) spec:no-uname$' "$l")" yes
  chk "$v" "a second derivation prints the same bytes" "$(yn cmp -s "$l" "$A/ledger.again.txt")" yes
  echo "no-uname's row as printed: $(grep -a 'spec:no-uname' "$l" || echo none)" >> "$v"
  finish A14 "$v"
}

# ============================================================ A15 ===========
row_A15() {
  A=$EVID/A15; rm -rf "$A"; mkdir -p "$A"; v=$A/verdict.txt; : > "$v"; OKALL=1
  mvp_ready A15 "$v" s1 || return 0
  mvp_head A15 "$v"
  b=$A/brief.verb.txt; cp "$M/brief.verb.txt" "$b"
  chk "$v" "rashomon brief DIR: the first line names the root" "$(yn rx '^rashomon brief: ' "$b")" yes
  for k in instructions build tests spec task packages verify; do
    chk "$v" "brief verb: a '$k' line" "$(yn rx "^  $k: " "$b")" yes
  done
  chk "$v" "brief verb: the build line names go.mod" "$(yn rx '^  build: .*go\.mod' "$b")" yes
  chk "$v" "brief verb: the tests line is the spec's declared gate, trusted" "$(yn has '  tests: "go test ./..." (spec, trusted)' "$b")" yes
  chk "$v" "brief verb: the spec line names the file and its trust" "$(yn rx '^  spec: \.claude/rashomon/spec\.json trusted \(sha256:' "$b")" yes
  attest "$S1_SCREEN" "rashomon brief: "; hd=$SEEN
  attest "$S1_SCREEN" "(spec, trusted)"; tl=$SEEN
  case $hd in yes*) hy=yes ;; *) hy=no ;; esac
  case $tl in yes*) ty=yes ;; *) ty=no ;; esac
  chk "$v" "session one's start: the brief's first line on the terminal [$hd]" "$hy" yes
  chk "$v" "session one's start: its tests line names the trusted spec gate [$tl]" "$ty" yes
  later=no
  for f in "$S2_SCREEN" "$S3_SCREEN"; do if has "last session:" "$f"; then later=yes; fi; done
  echo "reported, not judged: a later session's start shows a 'last session:' line: $later (a later session ran only with A20 or A17 selected)" >> "$v"
  finish A15 "$v"
}

# ============================================================ A16 ===========
row_A16() {
  A=$EVID/A16; rm -rf "$A"; mkdir -p "$A"; v=$A/verdict.txt; : > "$v"; OKALL=1
  mvp_ready A16 "$v" s1 || return 0
  mvp_head A16 "$v"
  cp "$M/status.json" "$M/status.txt" "$A/"
  python3 -I -c '
import json, sys
try:
    d = json.load(open(sys.argv[1]))
except Exception as e:
    print("unreadable", e)
    sys.exit(0)
print("session", d.get("session"))
print("unknown", len(d.get("unknown") or []))
for e in d.get("events") or []:
    print(e.get("event"), "installed=" + ("yes" if e.get("installed") else "no"), "newest=" + ("yes" if e.get("newest_session") else "no"),
          "notfiring=" + ("yes" if e.get("not_firing") else "no"), "sessions=%s/%s" % (e.get("sessions"), e.get("of")))' "$A/status.json" > "$A/events.txt" 2>&1
  ev=$A/events.txt
  chk "$v" "status --json: the newest session is session one" "$(yn rx "^session $S1\$" "$ev")" yes
  chk "$v" "status --json: blocks status could not read" "$(grep -a '^unknown ' "$ev" | cut -d' ' -f2)" 0
  for e in PreToolUse PostToolUse PostToolUseFailure SessionStart SessionEnd Stop StopFailure UserPromptSubmit; do
    chk "$v" "$e is installed" "$(yn rx "^$e installed=yes " "$ev")" yes
    chk "$v" "$e is not flagged as not firing" "$(yn rx "^$e .* notfiring=no " "$ev")" yes
  done
  for e in PreToolUse PostToolUse SessionStart UserPromptSubmit Stop; do
    chk "$v" "$e fired in the newest session" "$(yn rx "^$e installed=[a-z]+ newest=yes " "$ev")" yes
  done
  echo "reported, not judged: SessionEnd fired in the newest session: $(yn rx '^SessionEnd installed=[a-z]+ newest=yes ' "$ev") (it is the exit's; PostToolUseFailure and StopFailure fire only on a failure)" >> "$v"
  finish A16 "$v"
}

# ============================================================ A17 ===========
row_A17() {
  A=$EVID/A17; rm -rf "$A"; mkdir -p "$A"; v=$A/verdict.txt; : > "$v"; OKALL=1
  mvp_ready A17 "$v" none || return 0
  mvp_head A17 "$v"
  for f in learn-import.txt learn-approve.txt learn-list.txt; do cp "$M/$f" "$A/" 2> /dev/null || true; done
  chk "$v" "learn import: exit status" "$(cat "$M/learn-import.rc" 2> /dev/null || echo none)" 0
  chk "$v" "learn import: the row is added at proposed" "$(yn has 'imported 1 lessons at proposed' "$A/learn-import.txt")" yes
  chk "$v" "learn approve (code typed on the terminal): exit status" "$(cat "$M/learn-approve.rc" 2> /dev/null || echo none)" 0
  chk "$v" "learn approve: 'approved $LESSON_ID'" "$(yn has "approved $LESSON_ID" "$A/learn-approve.txt")" yes
  chk "$v" "learn lists the lesson past proposed (probation or proven)" "$(yn rx "^$LESSON_ID (probation|proven):" "$A/learn-list.txt")" yes
  if [ "$S3_OK" != yes ]; then
    echo "session three did not run: $S3_WHY. Not a product failure (nothing was approved, or the code was mistyped); run A17 again." >> "$v"
    verdict A17 INCONCLUSIVE "$v"; return 0
  fi
  cp "$M/learn.transcript.jsonl" "$M/learn.steer.ndjson" "$A/" 2> /dev/null || true
  u1=$(yn user_said "$A/learn.transcript.jsonl" "only the word done")
  u2=$(yn user_said "$A/learn.transcript.jsonl" "only the word ok")
  echo "session three: first prompt typed: $u1 (want yes); second prompt typed: $u2 (want yes)" >> "$v"
  if [ "$u1" = no ] || [ "$u2" = no ]; then
    echo "a prompt of session three was never typed: with one prompt the once-a-session rule is not exercised. Not a product failure; run A17 again and type both prompts." >> "$v"
    verdict A17 INCONCLUSIVE "$v"; return 0
  fi
  nt=$(agent_count "$A/learn.transcript.jsonl" "lesson $LESSON_ID, approved for this project")
  nl=$(steer_q count "$A/learn.steer.ndjson" learned-fact state=delivered key="$LESSON_ID")
  chk "$v" "the lesson note in the agent's transcript (records)" "$nt" 1
  chk "$v" "delivered learned-fact lines for the lesson in the steer log" "$nl" 1
  echo "reported: the delivering event: $(steer_q count "$A/learn.steer.ndjson" learned-fact state=delivered key="$LESSON_ID" event=UserPromptSubmit) line(s) at UserPromptSubmit" >> "$v"
  finish A17 "$v"
}

# ============================================================ A18 ===========
row_A18() {
  A=$EVID/A18; rm -rf "$A"; mkdir -p "$A"; v=$A/verdict.txt; : > "$v"; OKALL=1
  mvp_ready A18 "$v" note || return 0
  mvp_head A18 "$v"
  cp "$M/rules.transcript.jsonl" "$M/rules.steer.ndjson" "$A/" 2> /dev/null || true
  tr=$A/rules.transcript.jsonl; sl=$A/rules.steer.ndjson
  echo "-- steer lines, session one (pattern state suppressed_by client_version notice)" >> "$v"; steer_summary "$sl" >> "$v"
  n1=$(agent_count "$tr" "this 'uname' call matched spec rule no-uname.")
  chk "$v" "at the call: the forbidden-command note is in the agent's transcript (records, want 1 or more)" "$([ "${n1:-0}" -ge 1 ] && echo yes || echo no)" yes
  chk "$v" "at the call: a delivered platform-rule line for no-uname in the steer log" "$(steer_q count "$sl" platform-rule state=delivered '~no-uname' | awk '{print ($1>=1)?"yes":"no"}')" yes
  chk "$v" "at the call: an integrity line for parse-owner in the steer log (the user-only flag)" "$(steer_q count "$sl" integrity '~parse-owner' | awk '{print ($1>=1)?"yes":"no"}')" yes
  chk "$v" "at the call: the protected-path flag is NOT in the agent's transcript (records)" "$(agent_count "$tr" "which spec rule parse-owner protects")" 0
  attest "$S1_SCREEN" "record finding(s) in this turn" '[0-9]+ record finding\(s\) in this turn'; sf=$SEEN
  case $sf in yes*) sy=yes ;; *) sy=no ;; esac
  chk "$v" "at Stop: the line carries 'N record finding(s) in this turn' [$sf]" "$sy" yes
  cnt=$(grep -aEo '[0-9]+ record finding\(s\) in this turn' "$S1_SCREEN" 2> /dev/null | head -n 1 | cut -d' ' -f1 || true)
  if [ -n "$cnt" ]; then chk "$v" "at Stop: N is at least 1 (the parse-owner violation; 2 where the build records command text)" "$([ "$cnt" -ge 1 ] && echo yes || echo no)" yes; fi
  echo "reported: N in the capture: ${cnt:-not read from the capture}; the ledger's rows for both rules are A14's" >> "$v"
  finish A18 "$v"
}

# ============================================================ A19 ===========
row_A19() {
  A=$EVID/A19; rm -rf "$A"; mkdir -p "$A"; v=$A/verdict.txt; : > "$v"; OKALL=1
  mvp_ready A19 "$v" note || return 0
  mvp_head A19 "$v"
  id=$(steer_q id "$M/rules.steer.ndjson" platform-rule state=delivered '~no-uname')
  if [ -z "$id" ]; then
    echo "session one's steer log holds no delivered platform-rule line to ask why of (A18 says why no note was delivered)" >> "$v"
    verdict A19 INCONCLUSIVE "$v"; return 0
  fi
  pfx=$(printf '%s' "$id" | cut -c1-8)
  rc=0
  (cd "$MD" && bounded 60 "$REAL" why "$pfx" --session "$S1") > "$A/why.txt" 2>&1 || rc=$?
  bad=0
  (cd "$MD" && bounded 60 "$REAL" why zzzzzzzz --session "$S1") > "$A/why-unknown.txt" 2>&1 || bad=$?
  w=$A/why.txt
  chk "$v" "rashomon why $pfx --session <session one>: exit status" "$rc" 0
  chk "$v" "the first line names the prefix, the session and the line id" "$(yn rx "^rashomon why $pfx \\(session $S1, line $id\\)\$" "$w")" yes
  chk "$v" "what: the pattern's title and its key" "$(yn rx '^  what: platform rule, key ' "$w")" yes
  chk "$v" "when: the time, the event and the call it followed" "$(yn rx '^  when: [0-9:]+ at PostToolUse, after #[0-9]+' "$w")" yes
  chk "$v" "state: delivered, sent to the agent and shown to you" "$(yn rx '^  state: delivered, delivery delivered: sent to the agent and shown to you$' "$w")" yes
  chk "$v" "sent to the agent: the note's text" "$(yn rx "^  sent to the agent: rashomon record: this ${Q}uname${Q} call matched spec rule no-uname\\.\$" "$w")" yes
  chk "$v" "source: the rule it came from" "$(yn rx '^  source: rules .*no-uname' "$w")" yes
  chk "$v" "an unknown prefix is refused (exit status nonzero)" "$([ "$bad" != 0 ] && echo yes || echo no)" yes
  chk "$v" "an unknown prefix: the refusal says no line has that id" "$(yn has 'has an id starting' "$A/why-unknown.txt")" yes
  finish A19 "$v"
}

# ============================================================ A20 ===========
row_A20() {
  A=$EVID/A20; rm -rf "$A"; mkdir -p "$A"; v=$A/verdict.txt; : > "$v"; OKALL=1
  mvp_ready A20 "$v" note || return 0
  mvp_head A20 "$v"
  if [ "$S2_TYPED" = no ] || [ "$S2_DID" = no ]; then
    echo "session two (steer off --project, then uname -s): prompt typed: $S2_TYPED, ran exactly 'uname -s': $S2_DID. Not a product failure; run A20 again and type the prompt." >> "$v"
    verdict A20 INCONCLUSIVE "$v"; return 0
  fi
  ctl=$(steer_q count "$M/rules.steer.ndjson" platform-rule state=delivered '~no-uname')
  if [ "${ctl:-0}" -lt 1 ]; then
    echo "no control: session one (notes on) delivered no platform-rule note, so the channel is shut and nothing here shows steer off. A18 says why." >> "$v"
    verdict A20 INCONCLUSIVE "$v"; return 0
  fi
  for f in steer-off.txt steer-state-off.txt steer-log.txt steer-on.txt steeroff.report.txt steeroff.transcript.jsonl steeroff.steer.ndjson; do cp "$M/$f" "$A/" 2> /dev/null || true; done
  echo "-- steer lines, session two (pattern state suppressed_by client_version notice)" >> "$v"; steer_summary "$A/steeroff.steer.ndjson" >> "$v"
  chk "$v" "steer off --project: exit status" "$(cat "$M/steer-off.rc" 2> /dev/null || echo none)" 0
  chk "$v" "steer off --project: says this project is off" "$(yn has 'this project: off,' "$A/steer-off.txt")" yes
  chk "$v" "control, notes on: delivered platform-rule lines in session one (1 or more)" "$([ "${ctl:-0}" -ge 1 ] && echo yes || echo no)" yes
  chk "$v" "notes off: the note was built and logged with suppressed_by steer_off" "$(steer_q count "$A/steeroff.steer.ndjson" platform-rule suppressed_by=steer_off | awk '{print ($1>=1)?"yes":"no"}')" yes
  chk "$v" "notes off: the note is NOT in the agent's transcript (records)" "$(agent_count "$A/steeroff.transcript.jsonl" "call matched spec rule no-uname")" 0
  chk "$v" "recording goes on: the session is in the report (declarations recorded)" "$(yn rx 'declarations recorded: [1-9]' "$A/steeroff.report.txt")" yes
  chk "$v" "recording goes on: the report is not 'no sessions recorded'" "$(yn has 'no sessions recorded' "$A/steeroff.report.txt")" no
  chk "$v" "rashomon steer (notes off) says off for this project" "$(yn has 'this project: off' "$A/steer-state-off.txt")" yes
  chk "$v" "rashomon steer log --all names steer_off" "$(yn has 'steer_off' "$A/steer-log.txt")" yes
  chk "$v" "steer on --project turns it back" "$(yn has 'this project: on' "$A/steer-on.txt")" yes
  finish A20 "$v"
}

# ============================================================ A21 ===========
row_A21() {
  A=$EVID/A21; rm -rf "$A"; mkdir -p "$A"; v=$A/verdict.txt; : > "$v"; OKALL=1
  mvp_ready A21 "$v" note || return 0
  mvp_head A21 "$v"
  ctl=$(steer_q count "$M/rules.steer.ndjson" platform-rule state=delivered '~no-uname')
  if [ "${ctl:-0}" -lt 1 ]; then
    echo "no control: session one delivered no platform-rule note for a Stop to find in the transcript. A18 says why." >> "$v"
    verdict A21 INCONCLUSIVE "$v"; return 0
  fi
  ls "$HOME_R" 2> /dev/null | grep '^channel-evidence-' > "$A/markers.txt" || true
  n=$(wc -l < "$A/markers.txt" | tr -d ' ')
  echo "NOT driven here: the shadow itself (two consecutive Stops that find a delivered note absent from the transcript, Q11) cannot be produced without hiding notes from Claude Code or faking its transcript; the unit and acceptance tests own it. This row checks the other half: a channel that does show its notes is not shadowed." >> "$v"
  chk "$v" "a Stop read a transcript that held the delivered note: channel-evidence-* markers in the store" "$n" 0
  chk "$v" "notes shut as unverified_channel in session one" "$(steer_q count "$M/rules.steer.ndjson" platform-rule suppressed_by=unverified_channel)" 0
  finish A21 "$v"
}

for r in $ROWS; do "row_$r"; done
echo "evidence: $EVID"
exit "$worst"
