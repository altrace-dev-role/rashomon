#!/usr/bin/env python3
"""channel.py RUNDIR ARM: the B448 channel check for one R or G run. Prints counts; exits 1 when the channel was closed.

Reads hook-env.txt (what Claude Code set, recorded by hook.sh before its override), hooks.log (the prompt hook's
stdout: additionalContext under a `recap pending` header), store/runs/*/steer.ndjson (no row suppressed by no_mirror or
unverified_channel, the two suppressions the sdk surface causes) and transcript.jsonl (the hook_additional_context
attachment, the half Claude Code shows the model; test/live/p-tru-1.sh reads the same). G must have delivered at least
one prompt-hook additionalContext and one attachment (the graph brief renders on the slice's prompt); R may have had
nothing to say at the prompt, so R passes on the recorded environment and the absence of the two suppressions alone.
Counts only: no prompt, note or transcript text is printed.
"""
import glob, json, os, re, sys

D, arm = sys.argv[1].rstrip("/"), sys.argv[2]
env_path = os.path.join(D, "hook-env.txt")
env = open(env_path).read() if os.path.exists(env_path) else ""
ctx = 0
p = os.path.join(D, "hooks.log")
if os.path.exists(p):
    cur = False
    for line in open(p, errors="replace"):
        m = re.match(r"^== \S+ (\S+)(.*) rc=\d+$", line.rstrip("\n"))
        if m:
            cur = m.group(1) == "recap" and m.group(2).strip().startswith("pending")
        elif cur and "additionalContext" in line:
            ctx += 1
suppressed = {}
for f in glob.glob(os.path.join(D, "store", "runs", "*", "steer.ndjson")):
    for line in open(f, errors="replace"):
        try:
            d = json.loads(line)
        except ValueError:
            continue
        if d.get("state") == "suppressed":
            k = d.get("suppressed_by") or "?"
            suppressed[k] = suppressed.get(k, 0) + 1
att = 0
t = os.path.join(D, "transcript.jsonl")
if os.path.exists(t):
    att = sum(1 for l in open(t, errors="replace") if "hook_additional_context" in l)
closed = [k for k in ("no_mirror", "unverified_channel") if suppressed.get(k)]
ok = bool(env) and not closed and (arm != "G" or (ctx >= 1 and att >= 1))
print("channel %s (%s): hook-env %s; prompt-hook additionalContext %d; transcript attachments %d; suppressed %s"
      % ("open" if ok else "CLOSED", os.path.basename(D), "recorded" if env else "missing", ctx, att, json.dumps(suppressed, sort_keys=True)))
sys.exit(0 if ok else 1)
