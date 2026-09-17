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
LIVE=${LIVE:-/tmp/attest-live}
BIN=$LIVE/attest
export ATTEST_HOME=$LIVE/store
if [ -e "$HOME/.claude/settings.json" ]; then
  echo "refusing: $HOME/.claude/settings.json exists; move it aside first" >&2; exit 1
fi
rm -rf "$LIVE"; mkdir -p "$LIVE/work"
go build -o "$BIN" ./cmd/attest
trap '"$BIN" detach >/dev/null 2>&1 || true; rm -f "$HOME/.claude/settings.json"; claude mcp remove -s user attest-ping >/dev/null 2>&1 || true' EXIT

# L-2 needs a foreign PreToolUse hook already present under a narrow matcher,
# leaving a sentinel keyed by tool_use_id.
cat > "$LIVE/foreign.sh" <<SH
#!/bin/sh
python3 -c 'import sys,json; print(json.load(sys.stdin)["tool_use_id"])' >> "$LIVE/foreign.log"
SH
chmod +x "$LIVE/foreign.sh"
mkdir -p "$HOME/.claude"
printf '{"hooks": {"PreToolUse": [{"matcher": "Bash", "hooks": [{"type": "command", "command": "%s", "timeout": 5}]}]}}\n' "$LIVE/foreign.sh" > "$HOME/.claude/settings.json"

# L-1 needs an mcp__* tool: the smallest stdio server that offers one.
cat > "$LIVE/mcp_ping.py" <<'PY'
import json, sys
def reply(i, r):
    sys.stdout.write(json.dumps({"jsonrpc": "2.0", "id": i, "result": r}) + "\n"); sys.stdout.flush()
for line in sys.stdin:
    if not line.strip(): continue
    m = json.loads(line); method, i = m.get("method"), m.get("id")
    if method == "initialize": reply(i, {"protocolVersion": m["params"].get("protocolVersion", "2024-11-05"), "capabilities": {"tools": {}}, "serverInfo": {"name": "attest-ping", "version": "0"}})
    elif method == "tools/list": reply(i, {"tools": [{"name": "ping", "description": "Replies pong.", "inputSchema": {"type": "object", "properties": {}}}]})
    elif method == "tools/call": reply(i, {"content": [{"type": "text", "text": "pong"}]})
    elif method == "ping": reply(i, {})
    elif i is not None: sys.stdout.write(json.dumps({"jsonrpc": "2.0", "id": i, "error": {"code": -32601, "message": "method not found"}}) + "\n"); sys.stdout.flush()
PY
claude mcp add -s user attest-ping -- python3 "$LIVE/mcp_ping.py" >/dev/null

"$BIN" watch
cd "$LIVE/work"
uuid() { python3 -c 'import uuid; print(uuid.uuid4())'; }

echo; echo "################ L-1: Bash + Write + mcp__* in one session ################"
S1=$(uuid)
claude -p "Do exactly these three things using tools, in this order, then stop. 1) Use the Bash tool to run: echo l1-bash. 2) Use the Write tool to create a file named l1.txt containing the single word hello. 3) Call the MCP tool mcp__attest-ping__ping with no arguments. After all three, reply with the single word: done." \
  --session-id "$S1" --allowedTools "Bash(echo:*),Write,mcp__attest-ping__ping" --max-turns 8 --output-format json < /dev/null > "$LIVE/l1.out"
"$BIN" report --session "$S1" | tee "$LIVE/l1-report.json" | python3 -c '
import sys, json
s = json.load(sys.stdin)["sessions"][0]; t = s["transcript"]
print("by_tool:", s["declarations"]["by_tool"])
print("coverage:", s["coverage"]["state"], s["coverage"]["reasons"])
print("ids in transcript:", t["ids_in_transcript"], "| recorded:", t["ids_recorded"], "| missing either way:", t["missing_from_store"], t["missing_from_transcript"])'

echo; echo "################ L-2: the foreign hook fired alongside ours ################"
echo "sentinel ids:"; cat "$LIVE/foreign.log"
echo "our Bash declarations for them:"
python3 - "$ATTEST_HOME" "$LIVE/foreign.log" <<'PY'
import sys, json, glob, os
ours = {json.loads(l)["tool_use_id"]: json.loads(l)["tool_name"] for f in glob.glob(os.path.join(sys.argv[1], "runs", "*", "records.ndjson")) for l in open(f) if '"declaration"' in l}
for sid in open(sys.argv[2]).read().split(): print(" ", sid, "->", ours.get(sid, "not in this store (another session's store, or another install)"))
PY

echo; echo "################ L-3: our entry removed mid-session ################"
S3=$(uuid)
claude -p "Use the Bash tool to run: sleep 25. Then use the Bash tool to run: echo after-detach. Then reply with the single word: done." \
  --session-id "$S3" --allowedTools "Bash(sleep:*),Bash(echo:*)" --max-turns 6 --output-format json < /dev/null > "$LIVE/l3.out" &
CPID=$!
i=0; while [ $i -lt 240 ] && ! grep -qs '"declaration"' "$ATTEST_HOME/runs/$S3/records.ndjson"; do sleep 0.5; i=$((i+1)); done
echo "first declaration landed; detaching while the session runs:"; "$BIN" detach
wait $CPID
"$BIN" report --session "$S3" | tee "$LIVE/l3-report.json" | python3 -c '
import sys, json
s = json.load(sys.stdin)["sessions"][0]; t = s["transcript"]
print("declarations recorded:", s["declarations"]["recorded"])
print("coverage:", s["coverage"]["state"], s["coverage"]["reasons"], "| end recorded:", s["coverage"]["end_recorded"])
print("ids in transcript:", t["ids_in_transcript"], "| recorded:", t["ids_recorded"], "| missing_from_store:", t["missing_from_store"])'
