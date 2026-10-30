#!/usr/bin/env bash
# demo1.sh BIN [--pace] -- one Claude Code session in three turns, replayed
# through the hook lines `rashomon watch --yes` installs: each payload is piped
# through `sh -c` into its installed line, from the project directory, as
# Claude Code runs it (card DEMO1; D105, D107, D112). Per beat it prints a
# caption and what the line printed -- the systemMessage unwrapped -- or
# "(silent)", then the session's report.
#
# Turn 1 runs go test green, edits calc.go and runs go test red: the post
# flags the regression and the Stop names the recorded failure. Turn 2 edits
# two files and claims a pass with no run: the done flag carries turn 1's red
# forward. Turn 3 edits and runs go test green: verified. Each beat appends
# what Claude Code would to the session's transcript, at its transcript_path,
# before the line runs, so the report reads the session as a live one left it.
#
# Deterministic: fixed ids, one cwd, no backdating and no edit of the record.
# The note's minutes are the gap between two recorded calls, so they read 0;
# --pace waits 60 s before the red, so they read 1. The run's paths, the
# install id, the binary's path and the report's stamp are masked, as the
# header says. Everything is written under one mktemp directory, removed on
# exit. test/acceptance's TestDEMO1_TranscriptMatchesGolden diffs the output
# against test/acceptance/testdata/demo1.golden.
set -u

# A sealed environment: the run reads nothing of the shell it starts from.
# CLAUDECODE (watch refuses an agent session) and CLAUDE_PROJECT_DIR (H1
# evidence naming another project) go, and so does every RASHOMON_* knob.
if [ "${DEMO1_SEALED:-}" != 1 ]; then
  exec env -i DEMO1_SEALED=1 PATH="$PATH" TMPDIR="${TMPDIR:-/tmp}" bash "$0" "$@"
fi

bin=${1:?usage: demo1.sh BIN [--pace]}
pace=${2:-}
case $bin in /*) ;; *) bin=$PWD/$bin ;; esac
[ -x "$bin" ] || { echo "demo1: $bin is not an executable" >&2; exit 2; }

T=$(mktemp -d "${TMPDIR%/}/rashomon-demo1.XXXXXX") || exit 1
trap 'rm -rf "$T"' EXIT
mkdir -p "$T/home" "$T/config" "$T/user" "$T/proj" "$T/transcripts"
P=$T/proj
TR=$T/transcripts/demo-1.jsonl
SID=demo-1
: > "$TR"
export HOME=$T/user RASHOMON_HOME=$T/home CLAUDE_CONFIG_DIR=$T/config \
  RASHOMON_MANAGED_SETTINGS_PATH=$T/config/managed-settings.json \
  CLAUDE_CODE_ENTRYPOINT=cli AI_AGENT=claude-code_2-1-280_harness CI= \
  PYTHONIOENCODING=utf-8 PYTHONDONTWRITEBYTECODE=1
ID=

# say prints its argument with the run's own values masked. Each pattern is a
# variable: bash 3.2 misparses a quoted pattern that opens with a slash. The
# installed lines carry the binary's resolved path (install.go resolves
# os.Executable through every link), which under /var or /tmp, or for a link
# to the binary, is not the path it was given: the leaf's links first, each
# read from its own physical directory, then the directory.
rbin=$bin
for _ in 1 2 3 4 5 6 7 8 9 10; do
  [ -L "$rbin" ] || break
  link=$(readlink "$rbin")
  case $link in /*) rbin=$link ;; *) rbin=$(cd -P "${rbin%/*}" && pwd -P)/$link ;; esac
done
rbin=$(cd -P "${rbin%/*}" && pwd -P)/${rbin##*/}
qrbin="'$rbin'"
qbin="'$bin'"
ptmp="/private$T"
# The brief's first line shows a long root as "..." and its tail from a
# separator (brief's shown), so the scratch directory can print as any of
# its tails: each, from a separator, masks as <tmp> with the dots.
tails=()
t=$ptmp
while :; do
  tails+=("$t")
  case $t in /*/*) t=/${t#/*/} ;; *) break ;; esac
done
say() {
  local s=$1 t
  for t in "${tails[@]}"; do s=${s//"...$t"/"<tmp>"}; done
  s=${s//"$qrbin"/rashomon}
  s=${s//"$rbin"/rashomon}
  s=${s//"$qbin"/rashomon}
  s=${s//"$bin"/rashomon}
  s=${s//"$ptmp"/"<tmp>"}
  s=${s//"$T"/"<tmp>"}
  [ -n "$ID" ] && s=${s//"$ID"/"<install>"}
  printf '%s\n' "$s"
}

# unwrap is a hook's stdout as a person reads it: a lone systemMessage's text,
# indented under its beat; anything else verbatim.
unwrap() {
  python3 -c '
import json, sys
raw = sys.stdin.read()
try:
    d = json.loads(raw)
    text = d["systemMessage"] if isinstance(d, dict) and list(d) == ["systemMessage"] else None
except ValueError:
    text = None
for line in (text if text is not None else raw.rstrip("\n")).split("\n"):
    print("      " + line)
'
}

# run EVENT PAYLOAD runs EVENT's installed line on PAYLOAD; prints its stdout,
# then any stderr or nonzero exit, which a hook line never has.
run() {
  local cmd err rc out
  eval "cmd=\${cmd_$1}"
  err=$T/stderr
  out=$(printf '%s' "$2" | (cd "$P" && sh -c "$cmd") 2>"$err"); rc=$?
  printf '%s' "$out"
  [ -s "$err" ] && printf '\n(stderr: %s)' "$(cat "$err")"
  [ $rc -eq 0 ] || printf '\n(exit %d)' $rc
}

# beat CAPTION OUTPUT prints one beat.
beat() {
  if [ -z "$2" ]; then
    say "$(printf '  %-43s (silent)' "$1")"
  else
    say "  $1"
    say "$(printf '%s' "$2" | unwrap)"
  fi
}

session() { printf '{"hook_event_name":"%s","session_id":"%s","transcript_path":"%s","cwd":"%s"}' "$1" "$SID" "$TR" "$P"; }
prompt() { printf '{"hook_event_name":"UserPromptSubmit","session_id":"%s","transcript_path":"%s","cwd":"%s","permission_mode":"default","prompt":"%s"}' "$SID" "$TR" "$P" "$1"; }
stop() { printf '{"hook_event_name":"Stop","session_id":"%s","transcript_path":"%s","cwd":"%s","permission_mode":"default","stop_hook_active":false,"last_assistant_message":"%s"}' "$SID" "$TR" "$P" "$1"; }
common() { printf '"session_id":"%s","transcript_path":"%s","cwd":"%s","permission_mode":"default","prompt_id":"%s","tool_name":"%s","tool_input":%s,"tool_use_id":"%s"' "$SID" "$TR" "$P" "$@"; }

# call PROMPT ID TOOL INPUT OUTCOME runs one tool call, PreToolUse then its
# post, and prints the two lines' output together.
call() {
  local a b
  asked "$2" "$3" "$4"
  a=$(run PreToolUse "{\"hook_event_name\":\"PreToolUse\",$(common "$1" "$3" "$4" "$2")}")
  case $5 in
    ok-bash) b=$(run PostToolUse "{\"hook_event_name\":\"PostToolUse\",$(common "$1" "$3" "$4" "$2"),\"tool_response\":{\"stdout\":\"ok  \\texample.com/calc\\t0.01s\\n\",\"stderr\":\"\",\"interrupted\":false}}") ;;
    ok-edit) b=$(run PostToolUse "{\"hook_event_name\":\"PostToolUse\",$(common "$1" "$3" "$4" "$2"),\"tool_response\":{\"output\":\"\"}}") ;;
    exit1) b=$(run PostToolUseFailure "{\"hook_event_name\":\"PostToolUseFailure\",$(common "$1" "$3" "$4" "$2"),\"error\":\"Exit code 1\",\"is_interrupt\":false,\"duration_ms\":30}") ;;
  esac
  case $5 in
    ok-bash) got "$1" "$2" 'ok  \texample.com/calc\t0.01s' false ;;
    ok-edit) got "$1" "$2" 'The file has been updated successfully.' false ;;
    exit1) got "$1" "$2" 'Exit code 1\n--- FAIL: TestDiscount (0.00s)\nFAIL\texample.com/calc\t0.01s' true ;;
  esac
  printf '%s%s' "$a" "$b"
}

# tx TYPE PROMPT MESSAGE appends one transcript line, as Claude Code writes the
# session's JSONL beside the hooks: uuids and stamps from the line count, so a
# subshell's append numbers the same as the shell's.
tx() {
  local n prev=null
  n=$(( $(wc -l < "$TR") + 1 ))
  [ $n -gt 1 ] && prev=$(printf '"00000000-0000-4000-8000-%012d"' $((n - 1)))
  printf '{"parentUuid":%s,"isSidechain":false,%s"type":"%s","message":%s,"uuid":"00000000-0000-4000-8000-%012d","timestamp":"2026-10-28T10:%02d:00.000Z","sessionId":"%s","cwd":"%s","version":"2.1.280"}\n' \
    "$prev" "${2:+\"promptId\":\"$2\",}" "$1" "$3" $n $n "$SID" "$P" >> "$TR"
}
# said PROMPT TEXT, asked ID TOOL INPUT, got PROMPT ID TEXT IS_ERROR and
# answered PROMPT TEXT are the four lines a turn writes.
said() { tx user "$1" "{\"role\":\"user\",\"content\":\"$2\"}"; }
asked() { tx assistant "" "{\"model\":\"claude-sonnet-5\",\"id\":\"msg_$1\",\"type\":\"message\",\"role\":\"assistant\",\"content\":[{\"type\":\"tool_use\",\"id\":\"$1\",\"name\":\"$2\",\"input\":$3}],\"stop_reason\":\"tool_use\"}"; }
got() { tx user "$1" "{\"role\":\"user\",\"content\":[{\"tool_use_id\":\"$2\",\"type\":\"tool_result\",\"content\":\"$3\",\"is_error\":$4}]}"; }
answered() { tx assistant "" "{\"model\":\"claude-sonnet-5\",\"id\":\"msg_end_$1\",\"type\":\"message\",\"role\":\"assistant\",\"content\":[{\"type\":\"text\",\"text\":\"$2\"}],\"stop_reason\":\"end_turn\"}"; }
gotest() { call "$1" "$2" Bash '{"command":"go test ./..."}' "$3"; }
edit() { call "$1" "$2" Edit "{\"file_path\":\"$P/$3\",\"old_string\":\"a\",\"new_string\":\"b\"}" ok-edit; }

# Each turn's final text, once: finish writes it to the transcript and hands
# it to Stop as last_assistant_message, which is all the Stop line reads of
# it (cmd/rashomon/recap_turn.go:90-95, never the transcript). Turn 1 claims
# nothing, turn 2 claims a pass with no run, turn 3 says what the record shows.
reply1="Added the discount to calc.go; go test exits 1 in TestDiscount."
reply2="All 42 tests pass."
reply3="go test passes after the util.go edit."

# finish PROMPT TEXT ends a turn: the final text into the transcript, then
# the Stop line on it.
finish() {
  answered "$1" "$2"
  beat "Stop: \"$2\"" "$(run Stop "$(stop "$2")")"
}

say "rashomon demo 1: one Claude Code session, three turns, each hook line run as installed."
say "Masked: <tmp> is the run's scratch directory, <install> its install id, <generated> the report's clock; the binary prints as rashomon."
say ""

"$bin" watch --yes >"$T/screen.log" 2>&1 || { echo "demo1: watch --yes failed:" >&2; cat "$T/screen.log" >&2; exit 1; }
say "setup: rashomon watch --yes (exit 0; its screen went to a log, as --yes means)"
eval "$(python3 - "$CLAUDE_CONFIG_DIR/settings.json" <<'PY'
import json, shlex, sys
hooks = json.load(open(sys.argv[1]))["hooks"]
for event, groups in hooks.items():
    cmds = [h["command"] for g in groups for h in g.get("hooks", []) if " --install " in h.get("command", "")]
    if len(cmds) == 1:
        print("cmd_%s=%s" % (event, shlex.quote(cmds[0])))
PY
)"
events="PreToolUse PostToolUse PostToolUseFailure SessionStart SessionEnd Stop StopFailure UserPromptSubmit"
for e in $events; do
  eval "c=\${cmd_$e:-}"
  [ -n "$c" ] || { echo "demo1: no installed line for $e" >&2; exit 1; }
done
ID=${cmd_PreToolUse##* --install }
say "setup: the eight lines it installed in settings.json, one per event:"
for e in $events; do
  eval "c=\${cmd_$e}"
  say "$(printf '  %-20s%s' "$e" "$c")"
done
say ""

say "session start"
beat "SessionStart" "$(run SessionStart "$(session SessionStart)")"
say ""

say "turn 1, prompt p1: \"Add the discount to calc.go and run the tests.\""
said p1 "Add the discount to calc.go and run the tests."
beat "UserPromptSubmit" "$(run UserPromptSubmit "$(prompt "Add the discount to calc.go and run the tests.")")"
beat "Bash go test ./... -> ok" "$(gotest p1 toolu_p1_test1 ok-bash)"
beat "Edit calc.go" "$(edit p1 toolu_p1_edit calc.go)"
if [ "$pace" = --pace ]; then
  say "  (60 s pass before the next run, so the note reads 1 min)"
  sleep 60
fi
beat "Bash go test ./... -> exit 1" "$(gotest p1 toolu_p1_test2 exit1)"
finish p1 "$reply1"
say ""

say "turn 2, prompt p2: \"Fix it.\""
said p2 "Fix it."
beat "UserPromptSubmit" "$(run UserPromptSubmit "$(prompt "Fix it.")")"
beat "Edit calc.go" "$(edit p2 toolu_p2_edit1 calc.go)"
beat "Edit util.go" "$(edit p2 toolu_p2_edit2 util.go)"
finish p2 "$reply2"
say ""

say "turn 3, prompt p3: \"Run the tests.\""
said p3 "Run the tests."
beat "UserPromptSubmit" "$(run UserPromptSubmit "$(prompt "Run the tests.")")"
beat "Edit util.go" "$(edit p3 toolu_p3_edit util.go)"
beat "Bash go test ./... -> ok" "$(gotest p3 toolu_p3_test ok-bash)"
finish p3 "$reply3"
say ""

say "session end"
beat "SessionEnd" "$(run SessionEnd "$(session SessionEnd)")"
say ""

say "\$ rashomon report --session $SID"
report=$("$bin" report --session "$SID" </dev/null 2>&1)
say "$(printf '%s\n' "$report" | sed -E 's/^(rashomon report -- generated ).*/\1<generated>/')"
