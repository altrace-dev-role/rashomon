# B2a (docs/autosteer-prescan.md): the replay turns a transcript into the
# timeline of hook events the live recorder would have received. It orders
# events by position in the file and never by call, tells a confirmed denial
# from a call whose result was not read, puts a Stop only where the transcript
# itself establishes one and marks every other end unknown, never fills a field
# the transcript cannot supply, and carries a taint from every gap on. The
# history reader gives it each call's use and result positions and the result
# line's own cwd, prompt and response keys, bounded and counted.
R = "internal/replay/replay.go"
S = "internal/replay/scan.go"
P = "internal/replay/payload.go"
H = "internal/history/history.go"

# Ordering by position.
m("replay-1  events are ordered by call, not by position", R,
  "\tif a.Pos != b.Pos {\n\t\treturn a.Pos.Before(b.Pos)\n\t}\n", "\tif a.Seq != b.Seq {\n\t\treturn a.Seq < b.Seq\n\t}\n", "TestREPLAY2_|FuzzReplayTimeline")
m("replay-1  events are placed before an earlier call's result is known", R,
  "\t\tif e == nil || through != nil && through.Before(e.Pos) {\n", "\t\tif e == nil {\n", "TestREPLAY2_|FuzzReplayTimeline")

# A denial is not a missing result.
m("replay-1  a denial reads as a missing result", P,
  "\te.Denied = c.Outcome == history.Denied\n", "\te.Denied = false\n", "TestREPLAY2_|TestREPLAY1_")
m("replay-1  a missing result reads as a denial", P,
  "\te.Missing = c.Outcome == history.OutcomeNone\n", "\te.Missing = c.Outcome == history.OutcomeNone || c.Outcome == history.Denied\n", "TestREPLAY2_")
m("replay-1  a denied call gets an execution", P,
  "\tif c.Outcome == history.OutcomeNone || c.Outcome == history.Denied {\n\t\treturn nil\n", "\tif c.Outcome == history.OutcomeNone {\n\t\treturn nil\n", "TestREPLAY2_|TestREPLAY1_|FuzzReplayTimeline")

# Where a turn ends.
m("replay-1  the Stop stands at the start of its last line", S,
  "\tm.end = history.Pos{Line: n, Block: max(len(blocks), 1)}\n", "\tm.end = history.Pos{Line: n, Block: 0}\n", "TestREPLAY1b_|TestREPLAY2_|TestREPLAY3_")
m("replay-1  a reply that is not end_turn is a Stop", S,
  "\tcase m.stopReason != \"end_turn\":\n", "\tcase false:\n", "TestREPLAY3_")
m("replay-1  a turn cut after a tool_result is a Stop", S,
  "\t\tif last || o.unreadable {\n\t\t\tsc.unknownStop(o.last, o.lastSession, StopNotEndTurn)\n", "\t\tif false {\n\t\t\tsc.unknownStop(o.last, o.lastSession, StopNotEndTurn)\n", "TestREPLAY3_")
m("replay-1  an interrupted turn keeps its Stop", S,
  "\tcase o.interrupted:\n", "\tcase false:\n", "TestREPLAY3_")
m("replay-1  a turn that went on after an end_turn has one Stop", S,
  "\tif m := o.msg; m != nil && m.id != h.Message.ID && m.stopReason == \"end_turn\" {\n", "\tif m := o.msg; false && m != nil && m.id != h.Message.ID && m.stopReason == \"end_turn\" {\n", "TestREPLAY3_")
m("replay-1  an unreadable line after the last reply goes unnoticed", S,
  "\tcase o.unreadable:\n", "\tcase false:\n", "TestREPLAY3_")
m("replay-1  a system line ends a turn", S,
  "\t\tif h.IsSidechain {\n\t\t\t// SubagentStop is not an installed event, and a sidechain line\n", "\t\tif h.Type == \"system\" && cur != nil && cur.msg != nil {\n\t\t\tcur.msg.stopReason = \"end_turn\"\n\t\t}\n\t\tif h.IsSidechain {\n\t\t\t// SubagentStop is not an installed event, and a sidechain line\n", "TestREPLAY3_")

# A field the transcript cannot supply is never filled.
m("replay-1  a subagent's declaration is rebuilt without its agent_type", P,
  "\tu.when(c.Sidechain, SidechainAgentType)\n\tu.when(c.CWD == \"\", NoCWD)\n", "\tu.when(c.CWD == \"\", NoCWD)\n", "TestREPLAY4_")
m("replay-1  a subagent's execution is rebuilt without its agent_type", P,
  "\tu.when(c.Sidechain, SidechainAgentType)\n\tu.when(c.ResultCWD == \"\", NoResultCWD)\n", "\tu.when(c.ResultCWD == \"\", NoResultCWD)\n", "TestREPLAY4_")
m("replay-1  an interrupted call is rebuilt as an ordinary failure", P,
  "\tcase c.Outcome == history.Interrupted:\n\t\tu.add(InterruptedCall)\n", "\tcase false:\n\t\tu.add(InterruptedCall)\n", "TestREPLAY4_")
m("replay-1  an execution that names no result cwd is rebuilt with none", P,
  "\tu.when(c.ResultCWD == \"\", NoResultCWD)\n", "", "TestREPLAY4_")
m("replay-1  a dropped input is rebuilt as empty", P,
  "\tinput := toolInput(c)\n\tu.when(input == nil, InputDropped)\n\tctx := w.sc.at(c.UsePos.Line)\n", "\tinput := toolInput(c)\n\tctx := w.sc.at(c.UsePos.Line)\n", "TestREPLAY4_")
m("replay-1  a permission mode that may have changed is used", P,
  "\t\tu.when(ctx.permChange != 0 && ctx.permChange < c.UsePos.Line, PermissionModeChanged)\n", "", "TestREPLAY4_")
m("replay-1  a Bash response nobody can read is taken as foreground", P,
  "\t\tu.when(!c.ResultObject, ResponseUnreadable)\n", "", "TestREPLAY4_")
m("replay-1  a background launch the result does not confirm is taken as foreground", P,
  "\t\tu.when(c.ResultObject && c.Backgrounded && !c.ResultBackgroundTaskID && !c.ResultBackgroundedByUser, BackgroundUnconfirmed)\n", "", "TestREPLAY4_")
m("replay-1  a version no fixture shows is rebuilt", P,
  "\tcase !history.Supported(v):\n\t\tu.add(UnsupportedVersion)\n", "\tcase false:\n\t\tu.add(UnsupportedVersion)\n", "TestREPLAY4_")

# Taint.
m("replay-1  an unsupported event is not a gap", P,
  "\tif len(e.Unsupported) > 0 {\n\t\te.Gap = e.Unsupported[0]\n", "\tif false {\n\t\te.Gap = e.Unsupported[0]\n", "TestREPLAY4_|FuzzReplayTimeline")
m("replay-1  a gap taints nothing after it", R,
  "\t\te.Taint = w.first\n", "\t\te.Taint = history.Pos{}\n", "TestREPLAY4_|FuzzReplayTimeline")
m("replay-1  an unreadable line is not a gap", R,
  "\tif !u.IsZero() && u.Before(at) && (w.first.IsZero() || u.Before(w.first)) {\n", "\tif false && !u.IsZero() && u.Before(at) && (w.first.IsZero() || u.Before(w.first)) {\n", "TestREPLAY4_")
m("replay-1  a result history dropped is not a gap", P,
  "\tif e.Gap == \"\" && c.Reason == history.Evicted {\n", "\tif false {\n", "TestREPLAY4_")

# The history reader's positions and result side.
m("replay-1  a result is placed where its call was used", H,
  "\tp.ResultPos = at\n", "\tp.ResultPos = p.UsePos\n", "TestHIST11_|TestHIST10_|FuzzHistCalls|TestREPLAY2_")
m("replay-1  a use is not placed", H,
  "Line: at.Line, UsePos: at,", "Line: at.Line,", "TestHIST11_|TestHIST10_|FuzzHistCalls")
m("replay-1  the result's cwd is the call's", H,
  "\t\tp.ResultCWD = h.CWD\n", "\t\tp.ResultCWD = p.CWD\n", "TestHIST12_|TestHIST10_")
m("replay-1  the result side is not counted against the queue", H,
  "\theld := len(p.ResultCWD) + len(p.ResultPromptID)\n", "\theld := 0\n", "TestHIST13_")
m("replay-1  another tool's error text is dropped", H,
  "\tif p.ToolName == \"Bash\" || b.IsError {\n", "\tif p.ToolName == \"Bash\" {\n", "TestHIST12_")
m("replay-1  a toolUseResult is always an object", H,
  "\tp.ResultObject = tur.object\n", "\tp.ResultObject = true\n", "TestHIST12_|TestHIST10_")
m("replay-1  a null task id is a task", H,
  "backgroundTaskID: set(r.BackgroundTaskID),", "backgroundTaskID: len(r.BackgroundTaskID) > 0,", "TestHIST12_")
