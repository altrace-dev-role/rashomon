# FALSE-FIRE1 STATUS-PROBE: a line that hides its runner's status and prints it
# is read by the status it printed (shape.PrintedStatus), recorded as
# gate_output printed_pass or printed_fail, read by verify as the run's exit,
# and left out of the report's masked-runs sentence. Each shape row is killed
# by its row of TestFF1_PrintedStatus; the rest by the FF1 acceptance tests.

S = "internal/shape/masked_probe.go"
P = "TestFF1_PrintedStatus"
m("FF-1 $? reads a runner that is not its pipe's last stage", S,
  "\t\tif run >= 0 && stage == stages-1 {\n", "\t\tif run >= 0 {\n", P)
m("FF-1 zsh pipestatus counts from 0", S,
  "\t\t\tfrom = 1 // zsh counts from 1\n", "\t\t\tfrom = 0\n", P)
m("FF-1 two different statuses read", S,
  "\tif conflict || !seen {\n", "\tif !seen {\n", P)
m("FF-1 a line that holds the template in part reads", S,
  "\treturn got, len(line) == 0\n", "\treturn got, true\n", P)
m("FF-1 leading zeros read", S,
  "\t\tif n == 0 || n > 3 {\n", "\t\tif n == 0 {\n", P)
m("FF-1 a status past 255 reads", S,
  "\t\tif v > 255 {\n", "\t\tif v > 1<<20 {\n", P)
m("FF-1 a literal dollar reads", S,
  "\t\tif t.meta || t.ticks > 0 || t.litDollar {\n", "\t\tif t.meta || t.ticks > 0 {\n", P)
m("FF-1 a run after || reads", S,
  "\t\t\t\tif op != \"&&\" {\n", "\t\t\t\tif op == \"\" {\n", P)
m("FF-1 a run with a hiding flag reads", S,
  "\t\t\tif !isSimple || s.hides || it.bg", "\t\t\tif !isSimple || it.bg", P)
m("FF-1 a variable assigned twice reads", S,
  "\t\t\tif run >= 0 && rStage == rStages-1 && p.namedOnce(name) {\n", "\t\t\tif run >= 0 && rStage == rStages-1 {\n", P)
m("FF-1 a redirected echo reads", S,
  "\tif len(ws) < 2 || len(ws) != c.end-c.start {\n", "\tif len(ws) < 2 {\n", P)
m("FF-1 the first run decides", S,
  "\treturn lines, runs - 1, true\n", "\treturn lines, 0, true\n", P)
m("FF-1 a line that hides two runners reads", S,
  "\tif !ok || len(hidden) != 1 {\n", "\tif !ok || len(hidden) == 0 {\n", P)

# The lexer's literal dollars.
T = "internal/shape/tokenize.go"
m("FF-1 a dollar in single quotes is not marked literal", T,
  "\tl.w.litDollar = l.w.litDollar || strings.IndexByte(s[i+1:i+1+j], '$') >= 0\n", "", P)
m("FF-1 an escaped dollar in double quotes is not marked literal", T,
  "\t\t\t\tw.litDollar = w.litDollar || n == '$'\n", "", P)

# The post hook writes the word; verify and report read it.
m("FF-1 the post hook never writes the printed word", "internal/hook/post.go",
  "\t\t\t\trec.GateOutput = &printed\n", "\t\t\t\t_ = printed\n",
  "TestFF1_APrintedFailureIsTheRunsOutcome")
m("FF-1 a printed 0 is recorded as a fail", "internal/hook/gateoutput.go",
  "\tif code == 0 {\n\t\treturn store.GateOutput{Kind: gateout.KindPrintedPass", "\tif code < 0 {\n\t\treturn store.GateOutput{Kind: gateout.KindPrintedPass",
  "TestFF1_F1_EveryStatusCapturedAndPrinted")
V = "internal/verify/gates.go"
m("FF-1 a printed run stays masked in verify", V,
  "\treturn statusHidden(r) && !r.FromOutput && !r.FromPrinted\n", "\treturn statusHidden(r) && !r.FromOutput\n",
  "TestFF1_APrintedFailureIsTheRunsOutcome")
m("FF-1 a printed failure is not red", V,
  " ||\n\t\tr.FromPrinted && r.Output.Kind == outPrintedFail\n", "\n",
  "TestFF1_APrintedFailureIsTheRunsOutcome")
m("FF-1 the masked-runs sentence counts a printed run", "internal/report/account.go",
  "\t\tif readFromPrinted(outcomeRecord(executed[d.ToolUseID])) {\n", "\t\tif false && readFromPrinted(outcomeRecord(executed[d.ToolUseID])) {\n",
  "TestFF1_APrintedFailureIsTheRunsOutcome|TestFF1_F4_ZshPipestatus")
