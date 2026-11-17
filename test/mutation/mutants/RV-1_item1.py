# RV-1 item 1: the Stop reads a final message's done claim only on a turn the
# main agent acted in (turnActed): a call of this prompt at or before the
# Stop, with no AgentID, of a class that writes or runs (writesOrRuns, an
# allowlist), and not proven in no project. Expected's rows keep the
# session's write. Each break below either reads done on a turn that only
# relayed, delegated, searched or looked (TestRV1_1a_, TestRV1_1b_), hands the
# narrowed flag to expected (TestRV1_1c_), or drops a clause that makes a
# turn act, so a turn that did act reads no claim (one row per clause).
C = "internal/verify/cover.go"
S = "internal/verify/stop.go"
GUARD = "\t\tif d.PromptID == nil || *d.PromptID != promptID || d.Seq > at || d.AgentID != nil || !writesOrRuns(d) {\n"
PLACE = "\t\tif p := projectOf(d); p == nil || *p != noProject {\n\t\t\treturn true\n\t\t}\n"
FIRST = "\tif d.SchemaVersion < 3 || d.Shape.MayWrite {\n"
CASE = "\tcase shape.VerbWrite, shape.VerbExecute, shape.VerbTest, shape.VerbVCS, shape.VerbPackage:\n"
m("RV-1 item 1 done is read on every turn the session wrote", S,
  "snap.Gates, wrote && turnActed(run, in.PromptID, pt.Seq), SourceFinalMessage,",
  "snap.Gates, wrote, SourceFinalMessage,",
  "TestRV1_1a_|TestRV1_1b_|TestRV1_1c_")
m("RV-1 item 1 a subagent's call acts for the main agent", C,
  GUARD, GUARD.replace(" || d.AgentID != nil", ""),
  "TestRV1_1a_")
m("RV-1 item 1 acts alone decides, with no class or project filter", C,
  GUARD + "\t\t\tcontinue\n\t\t}\n" + PLACE,
  GUARD.replace("!writesOrRuns(d)", "!acts(d)") + "\t\t\tcontinue\n\t\t}\n\t\treturn true\n",
  "TestRV1_1a_|TestRV1_1b_")
m("RV-1 item 1 a call after the Stop's point acts for this turn", C,
  GUARD, GUARD.replace(" || d.Seq > at", ""),
  "TestRV1_1a_")
m("RV-1 item 1 a call of another prompt acts for this turn", C,
  GUARD, GUARD.replace("d.PromptID == nil || *d.PromptID != promptID || ", ""),
  "TestRV1_1a_")
m("RV-1 item 1 expected reads the narrowed flag", S,
  "disclosed: v.Scan.Disclosed, wrote: wrote})",
  "disclosed: v.Scan.Disclosed, wrote: wrote && turnActed(run, in.PromptID, pt.Seq)})",
  "TestRV1_1c_")
m("RV-1 item 1 a test run makes no act", C,
  CASE, CASE.replace(" shape.VerbTest,", ""),
  "TestRV1_1a_")
m("RV-1 item 1 version control and package tools make no act", C,
  CASE, CASE.replace(", shape.VerbVCS, shape.VerbPackage", ""),
  "TestRV1_1a_")
m("RV-1 item 1 a write makes no act", C,
  CASE, CASE.replace(" shape.VerbWrite,", ""),
  "TestRV1_1a_")
m("RV-1 item 1 a program run makes no act", C,
  CASE, CASE.replace(" shape.VerbExecute,", ""),
  "TestRV1_1a_|TestRV1_1b_|TestSC1f_")
m("RV-1 item 1 a line that may write makes no act", C,
  FIRST, FIRST.replace(" || d.Shape.MayWrite", ""),
  "TestRV1_1a_")
m("RV-1 item 1 a record before may_write says it acts on nothing", C,
  FIRST, FIRST.replace("d.SchemaVersion < 3 || ", ""),
  "TestRV1_1a_")
m("RV-1 item 1 an act proven in no project counts", C,
  PLACE, "\t\treturn true\n",
  "TestRV1_1a_|TestRV1_1b_")
m("RV-1 item 1 an act the record does not place makes none", C,
  PLACE, PLACE.replace("p == nil || *p != noProject", "p != nil && *p != noProject"),
  "TestRV1_1a_")
m("RV-1 item 1 an empty prompt id reads as no act", C,
  "\tif promptID == \"\" {\n\t\treturn true\n\t}\n",
  "\tif promptID == \"\" {\n\t\treturn false\n\t}\n",
  "TestRV1_1a_")
