# RV-1 item 4: a task-preflight note is a nudge only where its model lines
# tell the agent something the request did not, a spec rule or gate, a
# symbol the graph proved or a lesson that rode; a note that lists only the
# paths the request named is a flag, which reaches the person and never the
# agent, and the graph brief keeps its place. Each break below either sends a
# paths-only note to the agent again (TestRV1_4a_, TestRV1_4c_), routes the
# graph brief ahead of a note that lists a rule (TestRV1_4b_), or turns a note
# that does list something into a flag (one row per clause of noteLevel).
N = "internal/intent/note.go"
P = "cmd/rashomon/preflight.go"
CLAUSE = "\tif rode || pf.Rules.Lists() || pf.Gates.Lists() || (sf != nil && sf.Lists()) {\n"
m("RV-1 item 4 every note is a nudge again", N,
  "MaxLevel: noteLevel(pf, sf, rode),",
  "MaxLevel: steer.LevelNudge,",
  "TestRV1_4a_|TestRV1_4c_")
m("RV-1 item 4 the graph brief routes ahead of the prompt's notes", P,
  "\tfound := promptFindings(in, proj.lessons, lines, run.LastSeq(), digest)\n"
  "\tif f, ok := graphFinding(gb, keyGraph, run.LastSeq(), digest); graphed && ok {\n"
  "\t\tfound = append(found, f)\n"
  "\t}\n",
  "\tvar found []steer.Finding\n"
  "\tif f, ok := graphFinding(gb, keyGraph, run.LastSeq(), digest); graphed && ok {\n"
  "\t\tfound = append(found, f)\n"
  "\t}\n"
  "\tfound = append(found, promptFindings(in, proj.lessons, lines, run.LastSeq(), digest)...)\n",
  "TestRV1_4b_")
m("RV-1 item 4 a spec rule makes no nudge", N,
  CLAUSE, CLAUSE.replace("pf.Rules.Lists() || ", ""),
  "TestRV1_4b_|TestDONE1f_|TestRV1_4t_")
m("RV-1 item 4 a gate makes no nudge", N,
  CLAUSE, CLAUSE.replace("pf.Gates.Lists() || ", ""),
  "TestRV1_4b_")
m("RV-1 item 4 a lesson that rode makes no nudge", N,
  CLAUSE, CLAUSE.replace("rode || ", ""),
  "TestLRN17r_ALessonRides")
m("RV-1 item 4 a proven symbol makes no nudge", N,
  CLAUSE, CLAUSE.replace(" || (sf != nil && sf.Lists())", ""),
  "TestRV1_4e_")
# A fire that holds only host frames reads no graph: the break below builds
# the graph brief on it again, and the refused brief's evidence file is
# written over the one the turn's typed fire wrote.
m("RV-1 item 4 a frames-only fire builds the graph brief", P,
  "\tif !p.framed {\n\t\tgb, graphed = graphBriefWith(",
  "\tif true {\n\t\tgb, graphed = graphBriefWith(",
  "TestRV1_4pin_")
# measure1's channel control rides on its own task file, whose builtin rule
# makes the control's note a nudge: each break below leaves the control
# workspaces without it, puts it in every arm, or drops it from the
# pre-registration.
S = "scripts/measure1.sh"
TASK = "\tmkdir -p \"$WS/.claude/rashomon/task\"\n\tcp \"$M/control-task.json\" \"$WS/.claude/rashomon/task/current.json\"\n"
m("RV-1 item 4 the control task file reaches no control workspace", S,
  "\t\tnewarm\n\t" + TASK.replace("\n\t", "\n\t\t"),
  "\t\tnewarm\n",
  "TestMS1zd_")
m("RV-1 item 4 the control task file is in every arm", S,
  "\tWS=\"$A/ws-$(hex8)\"\n\tmkdir \"$WS\"\n",
  "\tWS=\"$A/ws-$(hex8)\"\n" + TASK,
  "TestMS1zd_")
m("RV-1 item 4 the control task file is not pre-registered", S,
  " \"$M/control-prompt.txt\" \"$M/control-task.json\" ",
  " \"$M/control-prompt.txt\" ",
  "TestMS1zd_")
