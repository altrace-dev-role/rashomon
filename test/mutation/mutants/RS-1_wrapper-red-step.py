# RS-1 (card WRAPPER-RED-STEP1): a red read in a wrapper steps the post as a
# runner's red does -- red by its runner's failing summary alone, at either
# post event -- and regression-location and stale-evidence say a wrapped run
# ran inside another command, never that a pipe hid it, never that a wrapped
# green exited 0; a wrapped red is named by the runner whose table read it.
P = "internal/hook/post.go"
SS = "internal/hook/steerstep.go"
V = "internal/verify/steer.go"

# hook: the trigger.
m("RS-1 a wrapper's red is never read", P,
  "\t\treturn k == \"summary_fail\" && rec.Outcome != store.ExecInterrupted\n", "\t\treturn false\n",
  "TestRS1_RedReadIsAWrappersFailingSummary")
m("RS-1 a wrapper's fail line reads red", P,
  "return k == \"summary_fail\" && rec.Outcome", "return (k == \"summary_fail\" || k == \"fail_line\") && rec.Outcome",
  "TestRS1_RedReadIsAWrappersFailingSummary")
m("RS-1 an interrupted wrapper reads red", P,
  "k == \"summary_fail\" && rec.Outcome != store.ExecInterrupted\n", "k == \"summary_fail\"\n",
  "TestRS1_RedReadIsAWrappersFailingSummary")
m("RS-1 a wrapper's red steps at PostToolUse alone", SS,
  "redRead && (failure || post)", "redRead && post",
  "TestST_StepsIsTheTriggerTable")

# verify: the fact.
m("RS-1 the fact never marks a wrapped red", V,
  "RedWrapped:  g.LastRun.Wrapped == WrapRead,", "RedWrapped:  false,",
  "TestRS1_TheFactSaysWhichRunWasWrapped")
m("RS-1 the regression fact never marks a wrapped green", V,
  "GreenWrapped: g.LastGreen.Wrapped == WrapRead}\n\t\tif g.LastRun.ExitCode", "GreenWrapped: false}\n\t\tif g.LastRun.ExitCode",
  "TestRS1_TheFactSaysWhichRunWasWrapped")
m("RS-1 a wrapped red is named by its wrapper", V,
  "\t\tif fact.RedWrapped && g.LastRun.GateOutput != nil {\n", "\t\tif false && g.LastRun.GateOutput != nil {\n",
  "TestRS1_TheFactSaysWhichRunWasWrapped")
m("RS-1 the stale fact never marks a wrapped green", V,
  "OutcomeFrom: OutcomeFrom(g.LastGreen.OutcomeFrom.String()), GreenWrapped: g.LastGreen.Wrapped == WrapRead}",
  "OutcomeFrom: OutcomeFrom(g.LastGreen.OutcomeFrom.String()), GreenWrapped: false}",
  "TestRS1_AWrappedGreenRaisesStaleEvidence")

# verify: the row.
m("RS-1 a wrapped red takes the pipe's row", V,
  "\t\tvar err error\n\t\tswitch {\n\t\tcase fromOutput && g.RedWrapped:\n", "\t\tvar err error\n\t\tswitch {\n\t\tcase false:\n",
  "TestRS1_TheRowSaysWhereTheRunWasRead")
m("RS-1 a wrapped red with no path takes the pipe's row", V,
  "\tvar m render.Message\n\tswitch {\n\tcase fromOutput && g.RedWrapped:\n", "\tvar m render.Message\n\tswitch {\n\tcase false:\n",
  "TestRS1_TheRowSaysWhereTheRunWasRead")
m("RS-1 a wrapped green is said to have exited 0", V,
  "\t\tcase g.GreenWrapped:\n\t\t\t// A green read in a wrapper", "\t\tcase false:\n\t\t\t// A green read in a wrapper",
  "TestRS1_TheRowSaysWhereTheRunWasRead")
m("RS-1 a wrapped green with no path is said to have exited 0", V,
  "\tcase g.GreenWrapped:\n\t\tm, _ = render.New(pattern.RegressionLocation, render.RegressionLocationWrappedGreenDegradedBinding{",
  "\tcase false:\n\t\tm, _ = render.New(pattern.RegressionLocation, render.RegressionLocationWrappedGreenDegradedBinding{",
  "TestRS1_TheRowSaysWhereTheRunWasRead")
m("RS-1 a wrapped green goes stale in the pipe's row", V,
  "\t\tvar err error\n\t\tswitch {\n\t\tcase fromOutput && g.GreenWrapped:\n", "\t\tvar err error\n\t\tswitch {\n\t\tcase false:\n",
  "TestRS1_TheRowSaysWhereTheRunWasRead")
m("RS-1 a wrapped green with no path goes stale in the pipe's row", V,
  "\tvar m render.Message\n\tswitch {\n\tcase fromOutput && g.GreenWrapped:\n", "\tvar m render.Message\n\tswitch {\n\tcase false:\n",
  "TestRS1_TheRowSaysWhereTheRunWasRead")
