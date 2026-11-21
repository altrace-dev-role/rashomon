#!/bin/sh
# selftest.sh SCRATCH: drive.py and read.py against fake-claude.py in real tmux (no network, no model, no meter).
# SCRATCH must not exist. Cases: a usage error; "plain" (expected verdicts: REJECTED_EXIT_LINE none,
# PLAN_POST_LINE_FOR_ACCEPTED_EXIT yes, RECORD_ORDER_ACCEPTED precedes yes, PLAN_PRE_LINE none); "pre" (the fake adds a
# Pre plan line on both calls: PLAN_PRE_LINE FLAGGED, and REJECTED_EXIT_LINE present naming PreToolUse, since read.py
# reports a line on the rejected call from any channel rather than hiding it); "unknown" (the fake shows a dialog
# drive.py has no rule for: drive.py must abort, exit 1, never answer it, and still clean up). Prints ok or FAIL per
# check; exit 0 only when every check holds.
set -u
S=${1:?SCRATCH}
HERE=$(cd "$(dirname "$0")" && pwd)
J=$(cd "$HERE/.." && pwd)
[ -e "$S" ] && { echo "selftest: $S exists" >&2; exit 2; }
mkdir -p "$S"
fail=0
check() { if eval "$2"; then echo "ok   $1"; else echo "FAIL $1"; fail=1; fi; }
out=$(cd "$S" && python3 -I "$J/drive.py" --bogus x 2>&1); rc=$?
check "usage: an unknown flag exits 2 with a j1live_usage line" '[ "$rc" = 2 ] && echo "$out" | grep -q j1live_usage'
for c in plain pre unknown; do
  R=$S/$c
  mkdir -p "$R/run" "$R/cfg" "$R/tree" "$R/fake"
  SID=$(uuidgen | tr 'A-Z' 'a-z')
  pre=0; [ "$c" = pre ] && pre=1
  unk=0; [ "$c" = unknown ] && unk=1
  ( cd "$R/tree" && env -u CLAUDECODE -u CLAUDE_CODE_ENTRYPOINT -u AI_AGENT RK_RUN_DIR="$R/run" CLAUDE_CONFIG_DIR="$R/cfg" \
      J1_REAL_CLAUDE="$HERE/fake-claude.py" FAKE_OUT="$R/fake" FAKE_PRE_LINE=$pre FAKE_UNKNOWN=$unk \
      python3 -I "$J/drive.py" -p "the prompt" --model claude-sonnet-5 --session-id "$SID" --allowedTools "Read,Edit" \
      --permission-mode acceptEdits --max-turns 150 --max-budget-usd 15 --output-format json \
      < /dev/null > "$R/run/result.json" 2> "$R/run/stderr.txt" )
  echo $? > "$R/run/exit"
  cp "$R/cfg/projects/selftest/$SID.jsonl" "$R/run/transcript.jsonl" 2> /dev/null
  python3 -I "$J/read.py" "$R/run" > "$R/run/j1-read.txt" 2>&1
  echo "--- case $c: drive exit $(cat "$R/run/exit"); fake: $(tr '\n' ';' < "$R/fake/fake.log" 2> /dev/null)"
  check "$c: the fake saw no wrong choice and left by /exit" '! grep -q FAIL "$R/fake/fake.log" && grep -q "exit via /exit" "$R/fake/fake.log"'
  check "$c: env.sh removed, env-names.txt kept" '[ ! -e "$R/run/j1/env.sh" ] && [ -s "$R/run/j1/env-names.txt" ]'
  if [ "$c" = unknown ]; then
    check "unknown: drive.py exit 1, subtype j1live_abort naming the unknown dialog" '[ "$(cat "$R/run/exit")" = 1 ] && grep -q "\"subtype\": \"j1live_abort\"" "$R/run/result.json" && grep -q "unknown dialog unchanged" "$R/run/result.json"'
    check "unknown: the dialog was escaped, never answered" 'grep -q "unknown escaped" "$R/fake/fake.log"'
    continue
  fi
  check "$c: drive.py exit 0, result subtype j1live" '[ "$(cat "$R/run/exit")" = 0 ] && grep -q "\"subtype\": \"j1live\"" "$R/run/result.json"'
  check "$c: steps show trust, reject, accept in order" 'awk -F"\t" "\$3==\"trust\"&&\$4==\"enter\"{t=NR} \$4==\"reject\"{r=NR} \$4==\"accept\"{a=NR} END{exit !(t&&r>t&&a>r)}" "$R/run/j1/steps.tsv"'
  check "$c: PLAN_POST_LINE_FOR_ACCEPTED_EXIT yes" 'grep -q "^PLAN_POST_LINE_FOR_ACCEPTED_EXIT yes" "$R/run/j1-read.txt"'
  check "$c: RECORD_ORDER_ACCEPTED precedes yes" 'grep -q "^RECORD_ORDER_ACCEPTED precedes yes" "$R/run/j1-read.txt"'
  if [ "$c" = pre ]; then
    check "pre: PLAN_PRE_LINE FLAGGED" 'grep -q "^PLAN_PRE_LINE FLAGGED" "$R/run/j1-read.txt"'
    check "pre: REJECTED_EXIT_LINE present, naming PreToolUse" 'grep -Eq "^REJECTED_EXIT_LINE present: record [0-9]+ PreToolUse:ExitPlanMode;" "$R/run/j1-read.txt"'
  else
    check "plain: PLAN_PRE_LINE none" 'grep -q "^PLAN_PRE_LINE none" "$R/run/j1-read.txt"'
    check "plain: REJECTED_EXIT_LINE none" 'grep -q "^REJECTED_EXIT_LINE none" "$R/run/j1-read.txt"'
  fi
done
[ "$fail" = 0 ] && echo "selftest: all checks hold" || echo "selftest: FAILED"
exit "$fail"
