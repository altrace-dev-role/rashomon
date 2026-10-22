#!/bin/sh
# The three live items, run against a real Claude Code session on this machine.
#
# They need an authenticated `claude` on PATH. They write to the real
# ~/.claude/settings.json -- that is the test -- and remove what they wrote.
# A pre-existing settings.json is not handled: the script refuses rather than
# merge with it, because the point of the exercise is a known starting state.
#
# Each item prints what it observed; read the output. Nothing here is asserted
# by machine on purpose: these are the items the specification says are read
# by a person.
set -eu
LIVE=${LIVE:-/tmp/rashomon-live}
BIN=$LIVE/rashomon
export RASHOMON_HOME=$LIVE/store
if [ -e "$HOME/.claude/settings.json" ]; then
  echo "refusing: $HOME/.claude/settings.json exists; move it aside first" >&2; exit 1
fi
rm -rf "$LIVE"; mkdir -p "$LIVE/work"
go build -o "$BIN" ./cmd/rashomon
trap 'touch "$LIVE/l3.go"; "$BIN" detach >/dev/null 2>&1 || true; rm -f "$HOME/.claude/settings.json"; claude mcp remove -s user rashomon-ping >/dev/null 2>&1 || true' EXIT

# L-2 needs a foreign PreToolUse hook already present under a narrow matcher,
# leaving a sentinel keyed by tool_use_id.
cat > "$LIVE/foreign.sh" <<SH
#!/bin/sh
python3 -c 'import sys,json; print(json.load(sys.stdin)["tool_use_id"])' >> "$LIVE/foreign.log"
SH
chmod +x "$LIVE/foreign.sh"
# L-3 needs a gate between one Bash call finishing and the next one starting.
# A PostToolUse hook is that gate: Claude Code does not hand a tool's result
# back to the model until every PostToolUse hook for it has returned, and
# hooks for one event run side by side, so ours lands its execution record
# while this one is still holding the result. It holds nothing until L-3 arms
# it, and gives up on its own well inside its timeout so a dead script cannot
# hang the session.
cat > "$LIVE/gate.sh" <<SH
#!/bin/sh
cat > /dev/null
[ -e "$LIVE/l3.armed" ] || exit 0
i=0; while [ ! -e "$LIVE/l3.go" ] && [ \$i -lt 300 ]; do sleep 0.2; i=\$((i+1)); done
SH
chmod +x "$LIVE/gate.sh"
mkdir -p "$HOME/.claude"
printf '{"hooks": {"PreToolUse": [{"matcher": "Bash", "hooks": [{"type": "command", "command": "%s", "timeout": 5}]}], "PostToolUse": [{"matcher": "Bash", "hooks": [{"type": "command", "command": "%s", "timeout": 90}]}]}}\n' "$LIVE/foreign.sh" "$LIVE/gate.sh" > "$HOME/.claude/settings.json"

# L-1 needs an mcp__* tool: the smallest stdio server that offers one.
cat > "$LIVE/mcp_ping.py" <<'PY'
import json, sys
def reply(i, r):
    sys.stdout.write(json.dumps({"jsonrpc": "2.0", "id": i, "result": r}) + "\n"); sys.stdout.flush()
for line in sys.stdin:
    if not line.strip(): continue
    m = json.loads(line); method, i = m.get("method"), m.get("id")
    if method == "initialize": reply(i, {"protocolVersion": m["params"].get("protocolVersion", "2024-11-05"), "capabilities": {"tools": {}}, "serverInfo": {"name": "rashomon-ping", "version": "0"}})
    elif method == "tools/list": reply(i, {"tools": [{"name": "ping", "description": "Replies pong.", "inputSchema": {"type": "object", "properties": {}}}]})
    elif method == "tools/call": reply(i, {"content": [{"type": "text", "text": "pong"}]})
    elif method == "ping": reply(i, {})
    elif i is not None: sys.stdout.write(json.dumps({"jsonrpc": "2.0", "id": i, "error": {"code": -32601, "message": "method not found"}}) + "\n"); sys.stdout.flush()
PY
claude mcp add -s user rashomon-ping -- python3 "$LIVE/mcp_ping.py" >/dev/null

"$BIN" watch --yes
cd "$LIVE/work"
uuid() { python3 -c 'import uuid; print(uuid.uuid4())'; }

echo; echo "################ L-1: Bash + Write + mcp__* in one session ################"
S1=$(uuid)
claude -p "Do exactly these three things using tools, in this order, then stop. 1) Use the Bash tool to run: echo l1-bash. 2) Use the Write tool to create a file named l1.txt containing the single word hello. 3) Call the MCP tool mcp__rashomon-ping__ping with no arguments. After all three, reply with the single word: done." \
  --session-id "$S1" --allowedTools "Bash(echo:*),Write,mcp__rashomon-ping__ping" --max-turns 8 --output-format json < /dev/null > "$LIVE/l1.out"
"$BIN" report --json --session "$S1" | tee "$LIVE/l1-report.json" | python3 -c '
import sys, json
s = json.load(sys.stdin)["sessions"][0]; t = s["transcripts"][0]
print("by_tool:", s["declarations"]["by_tool"])
print("executions recorded:", s["executions"]["recorded"])
print("declared without an execution:", s["declarations"]["without_execution"])
print("coverage:", s["coverage"]["state"], s["coverage"]["reasons"])
print("ids in transcript:", t["ids_in_transcript"], "| recorded:", t["ids_recorded"], "| missing either way:", t["missing_from_store"], t["missing_from_transcript"])'

echo; echo "################ L-2: the foreign hook fired alongside ours ################"
echo "sentinel ids:"; cat "$LIVE/foreign.log"
echo "our Bash declarations for them:"
python3 - "$RASHOMON_HOME" "$LIVE/foreign.log" <<'PY'
import sys, json, glob, os
ours = {json.loads(l)["tool_use_id"]: json.loads(l)["tool_name"] for f in glob.glob(os.path.join(sys.argv[1], "runs", "*", "records.ndjson")) for l in open(f) if '"declaration"' in l}
for sid in open(sys.argv[2]).read().split(): print(" ", sid, "->", ours.get(sid, "not in this store (another session's store, or another install)"))
PY

echo; echo "################ L-3: our entry removed mid-session ################"
# The detach has to fall after the first call has finished and before the
# second has started, or the item measures a race instead of a removal. Asking
# for `sleep 25` did not give that: Claude Code ran it in the background, or
# issued the second call straight away, and the second call landed within a
# second of the first, on either side of the detach. So nothing here relies on
# how long a command takes:
#  - the second command needs the first one's output, a word the model cannot
#    know beforehand, so the two cannot be issued together;
#  - the gate above holds the first call's result back from the model until
#    our execution record for it has landed, detach has run, and Claude Code
#    has had a few seconds to notice the edit -- it learns of it through a
#    file watcher, not when the write returns.
# What that should leave, and why it is what the expected line below says: the
# first call recorded with its execution; no hook of ours firing again, so the
# second call is in the transcript and not in the store (transcript_mismatch,
# and execution_mismatch for its result) and no end is recorded
# (run_not_closed). No record says hook_entry_absent, because that is written
# only by one of our hooks that runs and finds its entry gone; seeing it, or
# the second call recorded, means Claude Code was still running the old hooks.
S3=$(uuid)
W3=l3-$(uuid | cut -c1-8)
printf '#!/bin/sh\necho %s\n' "$W3" > l3-word.sh; chmod +x l3-word.sh
touch "$LIVE/l3.armed"
claude -p "Use the Bash tool to run exactly this command, in the foreground: ./l3-word.sh -- it prints one word. Wait for its output. Then use the Bash tool to run: echo followed by that word. Then reply with the single word: done." \
  --session-id "$S3" --allowedTools "Bash(./l3-word.sh),Bash(echo:*)" --max-turns 6 --output-format json < /dev/null > "$LIVE/l3.out" &
CPID=$!
i=0; while [ $i -lt 240 ] && ! grep -qs '"execution"' "$RASHOMON_HOME/runs/$S3/records.ndjson"; do sleep 0.5; i=$((i+1)); done
if [ $i -lt 240 ]; then echo "first call's execution record landed, its result held at the gate; detaching:"
else echo "no execution record after 120s; detaching anyway, so read what follows as a broken run:"; fi
"$BIN" detach
sleep 3
touch "$LIVE/l3.go"
wait $CPID
"$BIN" report --json --session "$S3" | tee "$LIVE/l3-report.json" | python3 -c '
import sys, json
word = sys.argv[1]
s = json.load(sys.stdin)["sessions"][0]; t = s["transcripts"][0]
second = set()
for l in open(t["path"]):
    m = json.loads(l).get("message")
    if isinstance(m, dict) and isinstance(m.get("content"), list):
        for c in m["content"]:
            if isinstance(c, dict) and c.get("type") == "tool_use" and c.get("name") == "Bash" and c.get("input", {}).get("command", "").startswith("echo") and word in c["input"]["command"]:
                second.add(c["id"])
print("declarations recorded:", s["declarations"]["recorded"], "| executions recorded:", s["executions"]["recorded"])
print("coverage:", s["coverage"]["state"], s["coverage"]["reasons"], "| end recorded:", s["coverage"]["end_recorded"])
print("ids in transcript:", t["ids_in_transcript"], "| recorded:", t["ids_recorded"], "| missing_from_store:", t["missing_from_store"])
print("expected: declarations 1, executions 1 | second call in transcript: yes, recorded: no | coverage: unverified [run_not_closed, transcript_mismatch, execution_mismatch]")
print("observed: declarations %d, executions %d | second call in transcript: %s, recorded: %s | coverage: %s [%s]" % (
    s["declarations"]["recorded"], s["executions"]["recorded"], "yes" if second else "no",
    "yes" if second - set(t["missing_from_store"]) else "no", s["coverage"]["state"], ", ".join(s["coverage"]["reasons"])))' "$W3"
