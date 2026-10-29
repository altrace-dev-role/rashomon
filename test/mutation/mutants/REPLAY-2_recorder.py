# B2b (docs/autosteer-prescan.md): the rebuilt timeline is replayed through the
# live recorder's own entry points into a world of its own. The replay writes
# nowhere else and reads no clock but the event's historical time, pins its
# host guard, starts each session itself, feeds the recorder the declaration
# from the use line's cwd and the execution from the result line's, stores
# duration_ms null and lets nothing read it, evaluates each Stop from the
# records before it and never calls one Verified past a gap, and holds
# final-message text within a budget, as the recorder keeps it.
RP = "cmd/rashomon/replay.go"
RT = "cmd/rashomon/recap_turn.go"
MN = "cmd/rashomon/main.go"
HG = "cmd/rashomon/hostguard.go"
PO = "cmd/rashomon/post.go"
P = "internal/replay/payload.go"
S = "internal/replay/scan.go"
R = "internal/replay/replay.go"
HP = "internal/hook/post.go"
VC = "internal/verify/classify.go"

# Isolation: the world's store and configuration, the environment around them,
# and nothing left behind.
m("replay-2  the store root is left to the environment", RP,
  "\t\t\"RASHOMON_HOME\":                  s(w.root),\n", "\t\t\"RASHOMON_HOME\":                  nil,\n", "TestREPLAY7_|TestREPLAY7b_")
m("replay-2  the Claude Code configuration is left to the environment", RP,
  "\t\t\"CLAUDE_CONFIG_DIR\":              s(w.config),\n", "\t\t\"CLAUDE_CONFIG_DIR\":              nil,\n", "TestREPLAY7_|TestREPLAY7b_")
m("replay-2  a world may be made on the real store", RP,
  "\t\tif within(scratch, a) || within(a, scratch) {\n", "\t\tif false {\n", "TestREPLAY7_")
m("replay-2  a world may be made in a directory with things in it", RP,
  "\tif entries, err := os.ReadDir(scratch); err == nil && len(entries) > 0 {\n", "\tif false {\n", "TestREPLAY7_")
m("replay-2  the caller's environment is not restored", RP,
  "\t\t\t\t_ = os.Setenv(k, p.val)\n", "\t\t\t\t_ = os.Setenv(k, \"\")\n", "TestREPLAY7_|TestREPLAY7b_")
m("replay-2  the recorder's clock is left set", RP,
  "\tdefer func() { entryNow, stopSeen = prevNow, prevSeen }()\n", "\tdefer func() { stopSeen = prevSeen }()\n", "TestREPLAY7_")
m("replay-2  the Stop tap is left set", RP,
  "\tdefer func() { entryNow, stopSeen = prevNow, prevSeen }()\n", "\tdefer func() { entryNow = prevNow }()\n", "TestREPLAY7_")
m("replay-2  the entry points' output is kept without being asked for", RP,
  "\tif r.cfg.KeepOutput {\n", "\tif true {\n", "TestREPLAY7_")

# Time: the entry points read the event's time, never the present.
m("replay-2  the replay's clock is the present", RP,
  "\tentryNow = func() time.Time { return w.clock }\n", "\tentryNow = time.Now\n", "TestREPLAY8_|TestREPLAY10_")
m("replay-2  the Stop reads the present", RT,
  "\tnow := entryNow()\n", "\tnow := time.Now()\n", "TestREPLAY8_|TestREPLAY10_")
m("replay-2  the probe reads the present", MN,
  "\t\t\trunErr = hook.RunProbeFromPayload(sig, phase, payload, st, entryNow)\n", "\t\t\trunErr = hook.RunProbeFromPayload(sig, phase, payload, st, time.Now)\n", "TestREPLAY8_|TestREPLAY10_")
m("replay-2  a post reads the present", PO,
  "\t\tp := hook.NewPost(st, entryNow)\n", "\t\tp := hook.NewPost(st, time.Now)\n", "TestREPLAY8_|TestREPLAY10_")
IMPORTS["replay-2  a post reads the present"] = (PO, '\t"io"\n', '\t"io"\n\t"time"\n')
m("replay-2  a declaration reads the present", HG,
  "\th := hook.New(st, entryNow)\n", "\th := hook.New(st, time.Now)\n", "TestREPLAY8_|TestREPLAY10_")
IMPORTS["replay-2  a declaration reads the present"] = (HG, '\t"io"\n', '\t"io"\n\t"time"\n')
m("replay-2  an event with no time reads the present", P,
  "\tt, err := time.Parse(time.RFC3339Nano, ts)\n\tif err != nil {\n\t\tu.add(NoTimestamp)\n\t\treturn\n\t}\n",
  "\tt, err := time.Parse(time.RFC3339Nano, ts)\n\tif err != nil {\n\t\tt = time.Now()\n\t}\n", "TestREPLAY4b_")

# Host guard: pinned, not inherited.
m("replay-2  the host guard is left to the environment", RP,
  "\t\t\"CLAUDE_CODE_ENTRYPOINT\":         s(replayEntrypoint),\n", "\t\t\"CLAUDE_CODE_ENTRYPOINT\":         nil,\n", "TestREPLAY7b_|TestREPLAY9_")
m("replay-2  the field holdout is left to the environment", RP,
  "\t\t\"RASHOMON_HOLDOUT\":               s(\"off\"),\n", "\t\t\"RASHOMON_HOLDOUT\":               nil,\n", "TestREPLAY7b_")
m("replay-2  CI in the caller's environment reaches the recorder", RP,
  "\t\t\"CI\":                             nil,\n", "", "TestREPLAY7b_|TestREPLAY9_")

# The session starts: without a liveness marker no verdict speaks.
m("replay-2  a session's SessionStart names no session", RP,
  "\t\t\t\tif _, err := r.feed(replayStep{install.EventSessionStart, start, e.At}, \"\"); err != nil {\n",
  "\t\t\t\tif _, err := r.feed(replayStep{install.EventSessionStart, []byte(\"{}\"), e.At}, \"\"); err != nil {\n", "TestREPLAY10_|TestREPLAY17_")

# The duration: stored null, never estimated, and read by nothing.
m("replay-2  the recorder stores a duration for a call nobody timed", HP,
  "\tif d == nil || *d <= 0 {\n\t\treturn nil\n\t}\n", "\tif d == nil || *d <= 0 {\n\t\tone := int64(1)\n\t\treturn &one\n\t}\n", "TestREPLAY10_")
m("replay-2  a verdict reads the stored duration", VC,
  "func execSeq(e *store.Execution) int64 {\n\tif e.Seq == nil {\n", "func execSeq(e *store.Execution) int64 {\n\t_ = e.DurationMS\n\tif e.Seq == nil {\n", "TestREPLAY11_")

# cwd: the declaration from the use line, the execution from the result line,
# one call's two halves under one id.
m("replay-2  a declaration takes the result line's cwd", P,
  "\t\te.Payload = marshal(&prePayload{SessionID: c.SessionID, TranscriptPath: w.opt.TranscriptPath, CWD: c.CWD,\n",
  "\t\te.Payload = marshal(&prePayload{SessionID: c.SessionID, TranscriptPath: w.opt.TranscriptPath, CWD: c.ResultCWD,\n", "TestREPLAY10_|TestREPLAY12_")
m("replay-2  an execution takes the use line's cwd", P,
  "TranscriptPath: w.opt.TranscriptPath, CWD: c.ResultCWD, PromptID: c.ResultPromptID,", "TranscriptPath: w.opt.TranscriptPath, CWD: c.CWD, PromptID: c.ResultPromptID,", "TestREPLAY10_|TestREPLAY12_")
m("replay-2  an execution is filed under another id", P,
  "HookEventName: kind.String(), ToolName: c.ToolName, ToolInput: input, ToolUseID: c.ToolUseID}", "HookEventName: kind.String(), ToolName: c.ToolName, ToolInput: input, ToolUseID: c.ToolUseID + \"x\"}", "TestREPLAY10_|TestREPLAY12_")

# Stop-prefix evidence and taint.
m("replay-2  a Stop is placed past every later result", S,
  "\tm.end = history.Pos{Line: n, Block: max(len(blocks), 1)}\n", "\tm.end = history.Pos{Line: int(^uint(0) >> 1), Block: 1}\n", "TestREPLAY13_")
m("replay-2  a tainted Stop is called Verified", RP,
  "\t\t\tstop.Verified = e.Taint.IsZero()\n", "\t\t\tstop.Verified = true\n", "TestREPLAY14_")
m("replay-2  a tainted Stop is downgraded without a reason", RP,
  "\t\t\t\t\ttaintReason = string(e.Gap)\n", "\t\t\t\t\ttaintReason = \"\"\n", "TestREPLAY14_")
m("replay-2  a Stop the transcript cannot supply is fed anyway", RP,
  "\t\t\tif !e.Supported() {\n", "\t\t\tif false {\n", "TestREPLAY14_|TestREPLAY15_")
m("replay-2  a Stop that was not fed names no reason", RP,
  "\t\t\t\t\tstop.Unevaluated = string(e.Unsupported[0])\n", "\t\t\t\t\tstop.Unevaluated = \"\"\n", "TestREPLAY14_|TestREPLAY15_")

# Final-message memory: what the recorder keeps, within a budget, released.
m("replay-2  the budget never refuses a Stop's text", S,
  "\t\t\tif sc.held+len(m.text) > sc.budget {\n", "\t\t\tif false {\n", "TestREPLAY15_")
m("replay-2  a Stop's text is not released when it is replayed", R,
  "\t\t\tw.sc.held -= len(fin.text)\n", "\t\t\t_ = fin\n", "TestREPLAY15_")
m("replay-2  the head kept is short of the recorder's", S,
  "\ti := MessageHeadBytes + finalMargin\n", "\ti := MessageHeadBytes - 1024\n", "TestREPLAY15_|TestREPLAY3b_")
m("replay-2  the tail kept is short of the recorder's", S,
  "\tj := len(s) - MessageTailBytes - finalMargin\n", "\tj := len(s) - MessageTailBytes + 1024\n", "TestREPLAY15_|TestREPLAY3b_")
m("replay-2  a long message is held whole", S,
  "\tif len(s) <= MessageHeadBytes+MessageTailBytes+2*finalMargin {\n", "\tif len(s) <= 1<<30 {\n", "TestREPLAY15_|TestREPLAY3b_")
m("replay-2  the replay's head size is not the recorder's", S,
  "\tMessageHeadBytes = 256 << 10\n", "\tMessageHeadBytes = 255 << 10\n", "TestREPLAY15_")
m("replay-2  a message that cannot be carried live is fed", S,
  "\t\tun.when(m.encoded > maxFinalPayloadBytes-finalPayloadHeadroom, FinalTextTooLarge)\n", "\t\tun.when(m.size > maxFinalPayloadBytes-finalPayloadHeadroom, FinalTextTooLarge)\n", "TestREPLAY15_")

# Rewrites cannot be seen: the flag says so on every session.
m("replay-2  a session is not flagged as having no rewrite evidence", RP,
  "var replayFlags = []string{flagRewriteAbsent, flagDurationNull, flagProbeStartSynthesized, flagProbeEndAbsent}\n",
  "var replayFlags = []string{flagDurationNull, flagProbeStartSynthesized, flagProbeEndAbsent}\n", "TestREPLAY16_")
