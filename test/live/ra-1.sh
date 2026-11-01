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
#       the verified turn into "N record finding(s)".
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
  case $1 in A2 | A4 | A7) echo "$1 $2 ($MODE)" | tee -a "$3" ;; *) echo "$1 $2" | tee -a "$3" ;; esac
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
    echo "3. APPROVE each tool call it asks about."
    echo "4. When it says done, type /exit."
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

for r in $ROWS; do "row_$r"; done
echo "evidence: $EVID"
exit "$worst"
