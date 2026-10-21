#!/bin/sh
# The hook-contract checks, HC-1 to HC-6 with fm-11 to fm-13, run against a
# real Claude Code session on this machine.
#
# They need an authenticated `claude` on PATH, and they spend real sessions:
# nine short ones, a resumed turn and a /compact. They were first run on
# 2026-10-20 and 21, against 2.1.304 and 2.1.305. Each item prints what it
# sees now, then a "then:" line saying what was seen on those dates, so a
# change in Claude Code shows as a difference between the two. Nothing here is
# asserted by machine, as in l-items.sh: these are read by a person.
#
# Nothing a session sends is printed or kept. Every hook installed here hands
# its payload to test/live/hc_shape.py, and every stream and log a check reads
# goes through it too; what comes out is key names, counts, sizes, exit codes,
# yes or no, values from closed lists, and digests it compares but never shows
# (H-108 holds it to that). All scratch is under one directory from mktemp,
# removed on exit with the transcripts and session-env directories Claude Code
# writes for the sessions started here. TMPDIR points inside that directory
# while the checks run: Claude Code keeps per-session files under
# $TMPDIR/claude-<uid>, and the cache-break-state ones record the model, API
# betas and tool names a session ran with. What Claude Code writes elsewhere
# for its own use -- its caches of remote settings, for one -- is not this
# script's to remove.
#
# Unlike l-items.sh this never writes ~/.claude/settings.json: every hook goes
# in through --settings. It refuses to run when that file exists all the same,
# because --settings adds to the user's settings instead of replacing them
# (HC-5), and a hook already there would fire beside these and be read as
# theirs.
set -eu
CFG=${CLAUDE_CONFIG_DIR:-$HOME/.claude}
if [ -e "$CFG/settings.json" ]; then
  echo "refusing: $CFG/settings.json exists; move it aside first" >&2; exit 1
fi
if ! command -v claude > /dev/null 2>&1; then
  echo "refusing: no claude on PATH" >&2; exit 1
fi
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
T=$(mktemp -d "${TMPDIR:-/tmp}/rashomon-hc.XXXXXX")
SIDS=
# Children HC-4 leaves alive are killed, then every session this script
# started loses its transcript and its session-env directory, then the scratch
# goes. The session-env directory goes with rmdir and not rm: these checks set
# no session environment, so a non-empty one holds something this script did
# not put there, and it is left for a person to look at.
cleanup() {
  set +e
  for f in "$T"/hc4/*/*.pid; do [ -f "$f" ] && kill "$(cat "$f")" 2> /dev/null; done
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
trap 'exit 130' INT
trap 'exit 143' TERM
# Paths go into JSON and into hook command lines unquoted; refusing an odd one
# is cheaper than quoting for both.
case $T in *[!A-Za-z0-9/._-]*) echo "refusing: the scratch path needs quoting this script does not do" >&2; exit 1 ;; esac
mkdir "$T/tmp"
export TMPDIR="$T/tmp"
cp "$ROOT/test/live/hc_shape.py" "$T/shape.py"
SHAPE="python3 -I $T/shape.py"

uuid() { python3 -I -c 'import uuid; print(uuid.uuid4())'; }
token() { python3 -I -c 'import secrets; print(secrets.token_hex(4))'; }
TO=
if command -v timeout > /dev/null 2>&1; then TO="timeout 600"; fi
# cl OUT ARGS...: one claude run, stdin closed, stdout to OUT and stderr beside
# it, never to the terminal; RC is how it exited. The prompt always comes
# straight after -p: --tools and --allowedTools take every word after them.
cl() { out=$1; shift; RC=0; $TO claude "$@" < /dev/null > "$out" 2> "$out.stderr" || RC=$?; }
# transcript SID: the file Claude Code keeps that session in, or -.
transcript() {
  for f in "$CFG"/projects/*/"$1".jsonl; do
    if [ -f "$f" ]; then echo "$f"; return 0; fi
  done
  echo -
}
# group MATCHER TIMEOUT CMD...: one matcher group running each CMD; - for no matcher.
group() {
  m=; [ "$1" = - ] || m="\"matcher\": \"$1\", "
  printf '[{%s"hooks": [' "$m"; sep=
  to=$2; shift 2
  for c in "$@"; do printf '%s{"type": "command", "command": "%s", "timeout": %s}' "$sep" "$c" "$to"; sep=', '; done
  printf ']}]'
}
item() { echo; echo "################ $* ################"; }
was() { echo "  then: $*"; }

echo "claude --version: $(claude --version 2> /dev/null | head -n 1)"
echo "run at: $(date -u +%Y-%m-%dT%H:%M:%SZ)"
for m in /etc/claude-code/managed-settings.json "/Library/Application Support/ClaudeCode/managed-settings.json"; do
  if [ -e "$m" ]; then echo "managed settings present ($m): its hooks fire beside these"; fi
done
if [ -n "${RASHOMON_BIN:-}" ]; then BIN=$RASHOMON_BIN; else BIN=$T/rashomon; (cd "$ROOT" && go build -o "$BIN" ./cmd/rashomon); fi
export RASHOMON_HOME="$T/store" HC_BIN="$BIN"

item "HC-1: systemMessage and additionalContext in one JSON object, on SessionStart and UserPromptSubmit"
D=$T/hc1; mkdir -p "$D/proj"
X=$(token); CS=HC1-CTX-START-$X; SS=HC1-SM-START-$X; CU=HC1-CTX-UPS-$X; SU=HC1-SM-UPS-$X
cat > "$D/emit.sh" << SH
#!/bin/sh
cat > /dev/null
case "\$1" in
  SessionStart) echo '{"systemMessage": "$SS", "hookSpecificOutput": {"hookEventName": "SessionStart", "additionalContext": "$CS"}}' ;;
  UserPromptSubmit) echo '{"systemMessage": "$SU", "hookSpecificOutput": {"hookEventName": "UserPromptSubmit", "additionalContext": "$CU"}}' ;;
esac
SH
cat > "$D/settings.json" << JSON
{"hooks": {"SessionStart": $(group - 20 "sh $D/emit.sh SessionStart"), "UserPromptSubmit": $(group - 20 "sh $D/emit.sh UserPromptSubmit")}}
JSON
S1=$(uuid); SIDS="$SIDS $S1"
cd "$D/proj"
cl "$D/stream.ndjson" -p "List every token that begins with HC1- and appears anywhere in your context, one per line, and nothing else." \
  --session-id "$S1" --settings "$D/settings.json" --tools "" --max-turns 1 --output-format stream-json --verbose
echo "  claude exit $RC"
$SHAPE hc1 "$D/stream.ndjson" "$(transcript "$S1")" "$CS" "$SS" "$CU" "$SU"
was "2026-10-20, 2.1.304: both hooks exit_code 0, outcome success; the reply named both additionalContext values and neither" \
  "systemMessage; UserPromptSubmit's systemMessage came as an informational event and SessionStart's in none; the" \
  "transcript held a hook_additional_context and a hook_system_message attachment for each event"

item "HC-2: one prompt_id per turn, on UserPromptSubmit and that turn's PreToolUse, PostToolUse and Stop"
D=$T/hc2; mkdir -p "$D/proj"
REC="$SHAPE record $D/events.ndjson hc2"
cat > "$D/probe.sh" << 'SH'
#!/bin/sh
# rashomon probe as an installed entry runs it, keeping only how many bytes it
# printed on each stream and how it exited (fm-11).
"$HC_BIN" probe "$1" > "$HC_DIR/probe.$$.out" 2> "$HC_DIR/probe.$$.err"
rc=$?
echo "${HC_RUN:-other} $1 $rc $(wc -c < "$HC_DIR/probe.$$.out") $(wc -c < "$HC_DIR/probe.$$.err")" >> "$HC_DIR/probe.log"
rm -f "$HC_DIR/probe.$$.out" "$HC_DIR/probe.$$.err"
SH
cat > "$D/settings.json" << JSON
{"hooks": {"SessionStart": $(group - 20 "$REC" "sh $D/probe.sh start"), "UserPromptSubmit": $(group - 20 "$REC"),
  "PreToolUse": $(group Bash 20 "$REC"), "PostToolUse": $(group Bash 20 "$REC"), "Stop": $(group - 20 "$REC"),
  "PreCompact": $(group - 20 "$REC"), "PostCompact": $(group - 20 "$REC"), "SessionEnd": $(group - 20 "$REC" "sh $D/probe.sh end")}}
JSON
S2=$(uuid); SIDS="$SIDS $S2"
export HC_DIR="$D"
cd "$D/proj"
export HC_RUN=1
cl "$D/run1.json" -p "Use the Bash tool to run: echo hc2-one. Then use the Bash tool to run: echo hc2-two. Then reply with the single word: done." \
  --session-id "$S2" --settings "$D/settings.json" --allowedTools "Bash(echo:*)" --max-turns 4 --output-format json
R1=$RC
export HC_RUN=2
cl "$D/run2.json" -p "Use the Bash tool to run: echo hc2-three. Then reply with the single word: done." \
  --resume "$S2" --settings "$D/settings.json" --allowedTools "Bash(echo:*)" --max-turns 3 --output-format json
R2=$RC
# Run 3 is fm-13's, on the same session; its records stay out of HC-2's turns.
export HC_RUN=3
cl "$D/run3.json" -p "/compact" --resume "$S2" --settings "$D/settings.json" --output-format json
R3=$RC
unset HC_RUN HC_DIR
echo "  claude exit: run 1 $R1, run 2 (--resume) $R2"
$SHAPE hc2 "$D/events.ndjson"
was "2026-10-20, 2.1.304: the prompt_id on every PreToolUse, PostToolUse and Stop equal to that turn's UserPromptSubmit;" \
  "two Bash calls in one turn sharing it under distinct tool_use_ids; the resumed turn a new one; SessionStart without one"

item "HC-3: the PostToolUseFailure error string for a hook-rejected commit, a silent failure and an exit 3"
D=$T/hc3; mkdir -p "$D/proj"; cd "$D/proj"
git init -q .
git config user.name hc3; git config user.email hc3@example.invalid
git config core.hooksPath .git/hooks; git config commit.gpgsign false
MARK="hc3-$(token): subject must start with a ticket prefix"
printf '#!/bin/sh\necho "%s" >&2\nexit 1\n' "$MARK" > .git/hooks/commit-msg
chmod +x .git/hooks/commit-msg
REC="$SHAPE record $D/events.ndjson hc3"
cat > "$D/settings.json" << JSON
{"hooks": {"PreToolUse": $(group Bash 20 "$REC"), "PostToolUse": $(group Bash 20 "$REC"), "PostToolUseFailure": $(group Bash 20 "$REC")}}
JSON
S3=$(uuid); SIDS="$SIDS $S3"
export HC_MARKER="$MARK" HC_RUN=1
cl "$D/out.json" -p "Use the Bash tool three times, one command per call, in this order, and carry on when one fails: git commit --allow-empty -m 'no ticket prefix', then false, then exit 3. Then reply with the single word: done." \
  --session-id "$S3" --settings "$D/settings.json" --allowedTools "Bash(git commit:*),Bash(false),Bash(exit 3)" --max-turns 6 --output-format json
unset HC_MARKER HC_RUN
echo "  claude exit $RC"
$SHAPE hc3 "$D/events.ndjson"
was "2026-10-20, 2.1.304: PostToolUseFailure and no PostToolUse for each; 'Exit code 1' and the commit-msg hook's stderr" \
  "line after one newline; 'Exit code 1' alone for false; 'Exit code 3' alone for exit 3; no tool_response and no" \
  "exit-code field; is_interrupt false"

item "HC-4: a hook's child, across the hook returning, its timeout kill, and the end of the session"
D=$T/hc4; mkdir -p "$D/proj"
cat > "$D/stop.sh" << 'SH'
#!/bin/sh
# A Stop hook. $1: where each child leaves its pid and marker; $2: how long the
# children sleep before writing the marker; $3: how long this hook then holds
# on, inside its timeout or past it.
cat > /dev/null
mkdir -p "$1"
child='echo $$ > "$1.pid"; sleep "$2"; : > "$1.marker"'
sh -c "$child" plain "$1/plain" "$2" < /dev/null > /dev/null 2>&1 &
if command -v setsid > /dev/null 2>&1; then
  setsid sh -c "$child" setsid "$1/setsid" "$2" < /dev/null > /dev/null 2>&1 &
  (setsid sh -c "$child" double "$1/double" "$2" < /dev/null > /dev/null 2>&1 &)
fi
date +%s > "$1/spawned"
sleep "$3"
SH
# alive PIDFILE: yes, no (gone, or a zombie nobody reaped), or not spawned.
alive() {
  [ -f "$1" ] || { echo "not spawned"; return 0; }
  st=$(ps -o stat= -p "$(cat "$1")" 2> /dev/null) || st=
  case $st in '' | *Z*) echo no ;; *) echo yes ;; esac
}
# hc4 VARIANT SLEEP HOLD TIMEOUT
hc4() {
  V=$D/$1
  cat > "$V.json" << JSON
{"hooks": {"Stop": $(group - "$4" "sh $D/stop.sh $V $2 $3")}}
JSON
  S=$(uuid); SIDS="$SIDS $S"
  cd "$D/proj"
  cl "$V.ndjson" -p "Reply with the single word: done." --session-id "$S" --settings "$V.json" --tools "" --max-turns 1 --output-format stream-json --verbose
  echo "  claude exit $RC"
  $SHAPE hooks "$V.ndjson"
  if [ ! -f "$V/spawned" ]; then echo "  the Stop hook spawned nothing: it did not run"; return 0; fi
  sleep 1
  for k in plain setsid double; do alive "$V/$k.pid" > "$V/$k.alive"; done
  deadline=$(($(cat "$V/spawned") + $2 + 2))
  while [ "$(date +%s)" -lt "$deadline" ]; do sleep 1; done
  for k in plain setsid double; do
    case $k in plain) n="background child, stdio on /dev/null, not waited on" ;; setsid) n="setsid child" ;; double) n="setsid child, double-forked" ;; esac
    mk=no; [ -e "$V/$k.marker" ] && mk=yes
    echo "  $n: alive after claude exited $(cat "$V/$k.alive"); wrote its marker $mk"
  done
}
echo "  -- the hook returns at once and the session ends straight after (L-4's (a) and (c) in one run)"
hc4 ret 15 0 30
was "2026-10-20, 2.1.304: the background and setsid children alive after claude exited and both wrote their markers:" \
  "setsid is not needed to outlive the hook or the session (the double-forked child was not tried)"
echo "  -- the hook holds on past its 2 s timeout"
hc4 kill 6 10 2
was "2026-10-20, 2.1.304: hook_response exit_code 1, outcome cancelled; the hook's tree killed, the setsid child with it;" \
  "only the double-forked setsid child survived and wrote its marker"

item "HC-5: a nested headless run from inside a Stop hook, tool-less and under its own --session-id"
D=$T/hc5; mkdir -p "$D/proj" "$D/nested"
O=$(uuid); N1=$(uuid); N2=$(uuid); SIDS="$SIDS $O $N1 $N2"
echo "hc5-$(token) hc6-$(token)" > "$D/markers"
read -r M1 M2 < "$D/markers"
cat > "$D/nested.sh" << 'SH'
#!/bin/sh
# The outer session's Stop hook: two nested runs, one after the other. $1 is
# the check's directory, $2 and $3 the nested sessions' ids. The markers come
# from a file and not from here: Claude Code keeps a hook's command line in the
# outer session's transcript, which HC-6 searches.
cat > /dev/null
D=$1; read -r M1 M2 < "$D/markers"
python3 -I -c 'import time; print(time.time())' > "$D/window"
cd "$D/nested"
rc=0
claude -p "Reply with exactly the word PONG and nothing else. (marker $M1)" --session-id "$2" --tools "" --max-turns 1 \
  --settings "$D/nested.json" --output-format stream-json --verbose --debug-file "$D/n1.debug" \
  < /dev/null > "$D/n1.stream" 2> "$D/n1.stderr" || rc=$?
echo "$rc" > "$D/n1.exit"
rc=0
printf '%s' "Reply with exactly the word PONG and nothing else. (marker $M2)" | claude -p --session-id "$3" --tools "" \
  --max-turns 1 --strict-mcp-config --settings "$D/nested.json" --no-session-persistence --output-format json \
  > "$D/n2.json" 2> "$D/n2.stderr" || rc=$?
echo "$rc" > "$D/n2.exit"
python3 -I -c 'import time; print(time.time())' >> "$D/window"
SH
RO="$SHAPE record $D/events.ndjson outer"; RN="$SHAPE record $D/events.ndjson nested"
cat > "$D/outer.json" << JSON
{"hooks": {"SessionStart": $(group - 20 "$RO"), "UserPromptSubmit": $(group - 20 "$RO"),
  "PreToolUse": $(group '*' 20 "$RO"), "PostToolUse": $(group '*' 20 "$RO"), "SessionEnd": $(group - 20 "$RO"),
  "Stop": [{"hooks": [{"type": "command", "command": "$RO", "timeout": 20}, {"type": "command", "command": "sh $D/nested.sh $D $N1 $N2", "timeout": 180}]}]}}
JSON
cat > "$D/nested.json" << JSON
{"hooks": {"SessionStart": $(group - 20 "$RN"), "UserPromptSubmit": $(group - 20 "$RN"), "Stop": $(group - 20 "$RN"), "SessionEnd": $(group - 20 "$RN")}}
JSON
touch "$D/since"
cd "$D/proj"
cl "$D/outer.out" -p "Reply with the single word: done." --session-id "$O" --settings "$D/outer.json" --tools "" --max-turns 1 --output-format json
echo "  outer claude exit $RC"
$SHAPE hc5 "$D" "$CFG" "$D/nested" "$N1"
was "2026-10-20, 2.1.304: exit 0, PONG, one turn, no refusal; --tools \"\" gave no tools at all, measured outside a hook (a" \
  "--disallowedTools list left 17); its own --settings hooks fired and none of the outer session's; it looked for the user, managed, project and" \
  "local settings files (none there) and applied --settings as flagSettings, beside them and not instead; remote settings fetched"

item "HC-6: --no-session-persistence, with the prompt on stdin, leaves no transcript"
$SHAPE hc6 "$D" "$CFG" "$N1" "$N2" "$D/since" "$M1" "$M2"
was "2026-10-21, 2.1.305: exit 0, PONG, no transcript under the projects directory and no file under ~/.claude holding" \
  "the prompt's marker; the same invocation without the flag wrote a transcript holding it"

item "fm-11: rashomon probe start and probe end print nothing on stdout (HC-2's sessions)"
$SHAPE fm11 "$T/hc2/probe.log" "$T/store" "$S2"
was "not measured live before; the contract (H-107 holds it offline): stdout 0 bytes and exit 0 from every invocation," \
  "with a coverage record for each phase. SessionStart's stdout reaches the model (HC-1), so a byte here is a byte of context"

item "fm-12: the Stop payload carries prompt_id (HC-2's sessions)"
$SHAPE fm12 "$T/hc2/events.ndjson"
was "2026-10-20, 2.1.304: every Stop carried it, equal to its turn's UserPromptSubmit prompt_id"

item "fm-13: SessionStart fires with source compact after a manual /compact through -p --resume (HC-2's session)"
echo "  claude exit (/compact) $R3"
$SHAPE fm13 "$T/hc2/events.ndjson"
was "2026-10-20, 2.1.304: PreCompact(trigger manual), SessionStart(source compact), PostCompact(trigger manual); the" \
  "compact SessionStart carried prompt_id, the startup and resume ones did not"

echo
echo "every check's scratch, TMPDIR among it, was under one directory, removed on exit with the transcripts and session-env directories of the sessions started here"
