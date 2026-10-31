#!/bin/sh
# RA-1: the release-acceptance evidence rows A1, A2, A4, A7 and A8 of
# RELEASE-ACCEPTANCE.md, one command each, evidence saved.
#
#   test/live/ra-1.sh [ROW ...]    ROW is A1 A2 A4 A7 A8; all five by default
#
# RUN IT FROM A PLAIN TERMINAL, NEVER FROM AN AGENT SESSION. `rashomon watch`,
# `rashomon spec trust` and `rashomon purge` each ask a person on /dev/tty and
# refuse an agent session (internal/selfguard: the CLAUDECODE variable Claude
# Code sets in every process it starts). This script checks the same variable
# first and refuses, exit 2, before it does anything else; it never passes
# --yes, never unsets CLAUDECODE for a child, and never answers a prompt. The
# person at the terminal answers y at watch (A2, A4, A7, A8), at spec trust
# (A4, A7) and at purge (A8).
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
#       in with no settings.json. Two `claude -p` sessions (haiku): a green
#       go test, then an edit. One reads as Claude Code 2.1.200, below the
#       PostToolUse floor 2.1.280 (internal/steer/client.go); one reads as the
#       real version. Saves A2/: claude version, both streams, both
#       transcripts, both steer.ndjson, and verdict.txt. Spoofed: the once-
#       per-session "a note was built and not sent to the agent" line, no
#       note in the agent's context. Real: the stale-evidence note in the
#       agent's context.
#   A4  needs the same, plus go. In a scratch Go project with a trusted spec
#       that declares `go test ./...`: a session that edits and runs no test
#       (the done line at Stop), then a session that edits and runs go test
#       (the verified line). Saves A4/: both streams and transcripts, both
#       `rashomon report --session <id>`, status.txt, verdict.txt.
#   A7  the report half of A4's run (A4 runs first if it is not listed).
#       Saves A7/: the two reports and a verdict on what they name.
#   A8  needs the RC binary only; no claude run. In a scratch config
#       directory, once seeded with an unrelated settings.json and once with
#       none: sha256 of settings.json, watch, detach, purge, sha256 again; the
#       hashes must be equal. Saves A8/uninstall.txt.
#
# The RC binary is $RA_BIN, else the install.sh binary A1 saved
# ($EVID/A1/prefix/bin/rashomon), and it must print $RA_TAG without the v
# (RA_ANY_VERSION=1 skips that check). `go install` is not the default: a
# go install build carries no -X main.version, so it prints dev.
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
SPOOF_VER=2.1.200
SPOOF_AGENT=claude-code_2-1-200_harness
ROWS=${*:-A1 A2 A4 A7 A8}
for r in $ROWS; do
  case $r in
    A1 | A2 | A4 | A7 | A8) ;;
    *) echo "refusing: $r is not a row this script drives (A1 A2 A4 A7 A8)" >&2; exit 2 ;;
  esac
done
# canonical order; A7 reads A4's run, so it brings A4 with it
SEL=" $ROWS "
case $SEL in *" A7 "*) SEL="$SEL A4 " ;; esac
ROWS=
for r in A1 A2 A4 A7 A8; do case $SEL in *" $r "*) ROWS="$ROWS $r" ;; esac; done
NEED_CLAUDE=0; NEED_TTY=0
for r in $ROWS; do
  case $r in A2 | A4 | A7) NEED_CLAUDE=1 ;; esac
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
  echo "$1 $2" | tee -a "$3"
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
# claude sets AI_AGENT and CLAUDE_CODE_ENTRYPOINT itself in a hook's
# environment, so exporting them here would be overwritten. The wrapper is
# what the entries point at for A2: it sets the two for the hook only, then
# execs the real binary. RA_SPOOF_AGENT sets AI_AGENT (steer.ClientVersion
# reads it, harness form claude-code_<dashed version>_harness). The entry
# point is forced to cli because claude -p reports sdk-cli, the SDK surface,
# which is shut by design whatever the version (steer/notice.go), so no
# version question would be asked of it.
make_wrapper() {
  mkdir -p "$T/wrap"; WRAP=$T/wrap/rashomon
  cat > "$WRAP" << SH
#!/bin/sh
[ -n "\${RA_SPOOF_AGENT:-}" ] && AI_AGENT=\$RA_SPOOF_AGENT && export AI_AGENT
CLAUDE_CODE_ENTRYPOINT=cli; export CLAUDE_CODE_ENTRYPOINT
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

# claude_run NAME PROMPT TOOLS DIR OUTDIR [ENV...]: one bounded claude -p
# session in DIR; sets SID, copies the stream and the transcript to OUTDIR.
claude_run() {
  name=$1 prompt=$2 tools=$3 dir=$4 out=$5; shift 5
  SID=$(uuid); SIDS="$SIDS $SID"
  mkdir -p "$out"
  (cd "$dir" && bounded 600 env -u CI RASHOMON_HOLDOUT=off "$@" \
    claude -p "$prompt" --session-id "$SID" --allowedTools "$tools" \
    --model haiku --max-turns 8 --output-format stream-json --verbose \
    < /dev/null > "$out/$name.stream.ndjson" 2> "$out/$name.stderr") || echo "claude exit $? ($name)" >> "$out/$name.stderr"
  for f in "$CFG"/projects/*/"$SID".jsonl; do
    [ -f "$f" ] && cp "$f" "$out/$name.transcript.jsonl"
  done
  S_ROOT=$HOME_R
  if [ -f "$S_ROOT/runs/$SID/steer.ndjson" ]; then cp "$S_ROOT/runs/$SID/steer.ndjson" "$out/$name.steer.ndjson"; fi
}

# steer_summary FILE: pattern, state, suppressed_by per line; never a value.
steer_summary() {
  [ -f "$1" ] || { echo "(no steer.ndjson)"; return 0; }
  python3 -I -c '
import json, sys
for l in open(sys.argv[1]):
    d = json.loads(l)
    print(d.get("pattern"), d.get("state"), d.get("suppressed_by"))' "$1"
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
    echo "wanted: rashomon version prints $VER on all three"
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
      echo "MISMATCH: route 1 prints '$v'. A go install build carries no -X main.version, so it prints dev unless main.go reads the build info; the module version above says which tag it is." >> "$out"
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
  for mode in spoofed real; do
    D=$T/a2-$mode; scratch_project "$D"
    if [ "$mode" = spoofed ]; then
      claude_run spoofed "$P" "Bash(go test:*) Edit" "$D" "$A" RA_SPOOF_AGENT=$SPOOF_AGENT
    else
      claude_run real "$P" "Bash(go test:*) Edit" "$D" "$A"
    fi
  done
  v=$A/verdict.txt; : > "$v"
  {
    echo "A2 run at $(date -u +%Y-%m-%dT%H:%M:%SZ); real claude: $realv; spoofed: $SPOOF_VER (AI_AGENT=$SPOOF_AGENT, CLAUDE_CODE_ENTRYPOINT=cli for the hook only)"
    echo "-- steer lines, spoofed (pattern state suppressed_by)"; steer_summary "$A/spoofed.steer.ndjson"
    echo "-- steer lines, real"; steer_summary "$A/real.steer.ndjson"
  } >> "$v"
  nline="a note was built and not sent to the agent (Claude Code $SPOOF_VER, PostToolUse"
  nctx="rashomon record: gate"
  sp_notice=no; has "$nline" "$A/spoofed.stream.ndjson" && sp_notice=yes
  sp_ctx=no; has "$nctx" "$A/spoofed.transcript.jsonl" && sp_ctx=yes
  re_ctx=no; has "$nctx" "$A/real.transcript.jsonl" && re_ctx=yes
  re_notice=no; has "a note was built and not sent" "$A/real.stream.ndjson" && re_notice=yes
  {
    echo "spoofed: once-per-session line in the stream: $sp_notice (want yes)"
    echo "spoofed: note in the agent's transcript: $sp_ctx (want no)"
    echo "real: note in the agent's transcript: $re_ctx (want yes)"
    echo "real: not-sent line in the stream: $re_notice (want no)"
  } >> "$v"
  if [ ! -f "$A/spoofed.steer.ndjson" ] || [ ! -f "$A/real.steer.ndjson" ]; then
    echo "no steer record for a session: the hooks did not run, or the model skipped the edit" >> "$v"
    verdict A2 INCONCLUSIVE "$v"
  elif [ "$sp_notice" = yes ] && [ "$sp_ctx" = no ] && [ "$re_ctx" = yes ] && [ "$re_notice" = no ]; then
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
  printf '{"rashomon_spec": 1, "gates": [{"id": "unit", "kind": "test", "argv": ["go", "test", "./..."]}], "rules": [{"id": "parse-owner", "kind": "protect_path", "why": "the parser belongs to the platform team", "hard_control": "code review by the platform team", "paths": ["internal/parse"]}]}\n' > "$D/.claude/rashomon/spec.json"
  echo "spec trust: answer y at the terminal."
  (cd "$D" && run_tee "$A/spec-trust.txt" "$REAL" spec trust .claude/rashomon/spec.json)
  RC=$(cat "$T/rc")
  [ "$RC" = 0 ] || { echo "spec trust exited $RC; nothing to run" >&2; exit 2; }
  "$REAL" status > "$A/status.txt" 2>&1 || true
  claude_run done "Add one comment line to internal/parse/lexer.go. Run no commands. Reply with only the word done." "Edit" "$D" "$A"
  SID_DONE=$SID
  claude_run verified "Add a second comment line to internal/parse/lexer.go, then run go test ./... Reply with only the word done." "Edit Bash(go test:*)" "$D" "$A"
  SID_VER=$SID
  "$REAL" report --session "$SID_DONE" > "$A/done.report.txt" 2>&1 || true
  "$REAL" report --session "$SID_VER" > "$A/verified.report.txt" 2>&1 || true
  echo "$SID_DONE" > "$A/done.session"; echo "$SID_VER" > "$A/verified.session"
  v=$A/verdict.txt; : > "$v"
  dl="rashomon: done was claimed; the record shows 1 file edited, no test or build run after the last edit."
  vl="rashomon: verified - go passed after the last edit."
  d=no; has "$dl" "$A/done.stream.ndjson" && d=yes
  e=no; has "$vl" "$A/verified.stream.ndjson" && e=yes
  crossd=no; has "$vl" "$A/done.stream.ndjson" && crossd=yes
  {
    echo "A4 run at $(date -u +%Y-%m-%dT%H:%M:%SZ); binary $("$REAL" version)"
    echo "edit, no run: the done line at Stop: $d (want yes); a verified line there: $crossd (want no)"
    echo "edit, then go test: the verified line at Stop: $e (want yes)"
  } >> "$v"
  if [ ! -s "$A/done.report.txt" ] || has "no sessions recorded" "$A/done.report.txt" || has "no sessions recorded" "$A/verified.report.txt"; then
    echo "no session was recorded: the hooks did not run" >> "$v"; verdict A4 INCONCLUSIVE "$v"
  elif [ "$d" = yes ] && [ "$e" = yes ] && [ "$crossd" = no ]; then verdict A4 PASS "$v"
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

for r in $ROWS; do "row_$r"; done
echo "evidence: $EVID"
exit "$worst"
