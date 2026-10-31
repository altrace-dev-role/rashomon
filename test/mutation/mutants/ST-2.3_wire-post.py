# Card 2.3: post wired to the steer step. The step runs on a failed runner
# call whose execution landed, judges it once against the run, and prints
# what Route returns after the steer line is durable. regression-location is
# raised by the red run itself after an earlier green, with the relevant
# writes strictly between the two runs named, and rendered from the record:
# the degraded row when no path is left to name, a refusal over a clamped
# number. Consent is the steer_disclosure marker (D84). The kill row is the
# named test's.
H = "internal/hook/steerstep.go"
P = "internal/hook/post.go"
C = "cmd/rashomon/post.go"
V = "internal/verify/steer.go"
S = "internal/store/landexec.go"
E = "cmd/rashomon/steerenv.go"

# The trigger (R3): each condition dropped.
m("ST-2.3 the trigger ignores the event", H,
  "\tfailure := event == FailureEvent\n", "\tfailure := true\n",
  "TestST_StepsIsTheTriggerTable")
m("ST-2.3 the trigger ignores the landed seq", H,
  "\treturn landedSeq > 0 && (", "\treturn (",
  "TestST_StepsIsTheTriggerTable")
m("ST-2.3 the trigger ignores the runner", H,
  "(failure && runner ||", "(failure ||",
  "TestST_StepsIsTheTriggerTable")

# The step (R4): the consent threshold, attendance, the flush before the print.
m("ST-2.3 consent needs more than the disclosure version", H,
  "\treturn err == nil && n >= steer.DisclosureVersion\n", "\treturn err == nil && n > steer.DisclosureVersion\n",
  "TestST_SteerEnvBecomesTheContext")
m("ST-2.3 CI is attended", "internal/hook/promptcontext.go",
  "\tunattended := env.CI\n", "\tunattended := false\n",
  "TestST_SteerEnvBecomesTheContext")
m("ST-2.3 dontAsk is attended", "internal/hook/promptcontext.go",
  "\tif unattended || permissionMode == \"dontAsk\" {\n", "\tif unattended {\n",
  "TestST_SteerEnvBecomesTheContext")
m("ST-2.3 the step prints before its line is durable", H,
  "\tif err := p.st.Flush(); err != nil {\n\t\treturn nil\n\t}\n", "",
  "TestST_TheLineIsDurableBeforeItsNote",
  platform="darwin")
# The break is judged on darwin only: off darwin no append is deferred, Flush
# finds nothing dirty and returns at once (store.Flush), so dropping it
# changes nothing a process can observe. On darwin the test sees the step's
# full sync of the steer log after the line's barrier go missing.
m("ST-2.3 the step matches the raw id, which no record stores over the cap", P,
  "\tp.toolUseID = store.CapID(pl.ToolUseID)\n", "\tp.toolUseID = pl.ToolUseID\n",
  "TestST_OverCapIdMatchesItsRun")
# The guard after a failed record flush (R1): only where no marker could be
# written does the run read Complete, so only there would the step append a
# durable line it cannot print.
m("ST-2.3 a post whose flush and marker both failed runs the step", C,
  "\t\t\tfmt.Fprintln(stderr, flushLostLine)\n\t\t\treturn nil\n\t\t}\n",
  "\t\t\tfmt.Fprintln(stderr, flushLostLine)\n\t\t}\n",
  "TestST_FailedFlushCitesNothing",
  platform="darwin")  # skipWithoutAFlush: off darwin a deferred post issues no flush to fail
m("ST-2.3 consent read from consent.json, not the marker", E,
  "\t\tConsent:      store.SteerDisclosureMarker,\n",
  "\t\tConsent:      func(root string) (int, error) { _ = store.SteerDisclosureMarker; return hook.ConsentSteerDisclosure(root) },\n",
  "TestST_NoConsentPrintsAFlagOnly")

# The store (R2).
m("ST-2.3 LandExecution reports a spilled execution as landed", S,
  "\t\treturn 0, s.SpillExecution(rec)", "\t\treturn 1, s.SpillExecution(rec)",
  "TestREC_LandExecutionReturnsTheSeq")

# The producer (R6).
m("ST-2.3 a red that did not run the gate (126, 127) still locates", V,
  "\treturn code != 126 && code != 127\n", "\t_ = code\n\treturn true\n", "TestSTR44_")
m("ST-2.3 a red of another call is taken as this run", V,
  " || g.LastRun == nil || g.LastRun.ToolUseID != toolUseID {\n", " || g.LastRun == nil {\n",
  "TestSTR43i_")
m("ST-2.3 any red reason is a candidate", V,
  "\tcase FailedOutput:\n\t\treturn g.LastRun.OutcomeFrom == SourceOutput\n\t}\n\treturn false\n", "\tcase FailedOutput:\n\t\treturn g.LastRun.OutcomeFrom == SourceOutput\n\t}\n\treturn true\n",
  "TestSTR43k_")
m("ST-2.3 a red with no earlier green is a candidate", V,
  "g.State != Red || g.LastGreen == nil || g.LastRun == nil ||", "g.State != Red || g.LastRun == nil ||",
  "TestSTR43d_")
m("ST-2.3 the green run's own writes are between", V,
  "return writes[i].Seq > lo }); i < len(writes) && writes[i].Seq < hi; i++ {",
  "return writes[i].Seq >= lo }); i < len(writes) && writes[i].Seq < hi; i++ {", "TestSTR43b_")
m("ST-2.3 the red run's own writes are between", V,
  "i < len(writes) && writes[i].Seq < hi; i++", "i < len(writes) && writes[i].Seq <= hi; i++", "TestSTR43b_")
m("ST-2.3 a red with nothing written between locates", V,
  "\t\tif fact.WritesSince == 0 {\n\t\t\tcontinue\n\t\t}\n", "", "TestSTR43c_")
m("ST-2.3 an irrelevant write counts as between", V,
  "\t\tif !Relevant(w.Class) {\n\t\t\tcontinue\n\t\t}\n", "", "TestSTR43c_")
m("ST-2.3 a path written twice is named twice", V,
  "\t\t\tcontinue\n\t\t}\n\t\tp := PathSlot(*w.Path)\n\t\tif seen[p] {\n\t\t\tcontinue\n\t\t}\n",
  "\t\t\tcontinue\n\t\t}\n\t\tp := PathSlot(*w.Path)\n", "TestSTR43b_")
m("ST-2.3 a path outside the slot charset is named", V,
  "\t\tif p.Valid() && len(f.Paths) < MaxPaths {\n\t\t\tf.Paths = append(f.Paths, p)\n\t\t} else {\n\t\t\tf.PathsOmitted++\n\t\t}\n\t}\n}\n\n// programOf",
  "\t\tif len(f.Paths) < MaxPaths {\n\t\t\tf.Paths = append(f.Paths, p)\n\t\t} else {\n\t\t\tf.PathsOmitted++\n\t\t}\n\t}\n}\n\n// programOf", "TestSTR43b_")
m("ST-2.3 a program outside its charset is kept", V,
  "\t\tif p := Program(programOf(runs, last)); p.Valid() {\n", "\t\tif p := Program(programOf(runs, last)); true {\n",
  "TestST_ProgramFallsBackToTheKind")
m("ST-2.3 other agents counted as the point's own", V,
  "\t\tif !sameAgent(w.AgentID, agent) {\n", "\t\tif sameAgent(w.AgentID, agent) {\n", "TestSTR43e_")

# The renderer (R7, D85).
m("ST-2.3 the degraded row is never chosen", V,
  "\t\t}\n\t}\n\tif fromOutput {\n\t\tm, _ := render.New(pattern.RegressionLocation, ",
  "\t\t}\n\t}\n\tif true {\n\t\treturn render.Message{}\n\t}\n\tif fromOutput {\n\t\tm, _ := render.New(pattern.RegressionLocation, ",
  "TestSTR43f_")
m("ST-2.3 every named path left out refuses the note", V,
  "\t\tif !errors.Is(err, slot.ErrNoPathsLeft) {\n\t\t\treturn m\n\t\t}\n\t}\n\tif fromOutput {\n\t\tm, _ := render.New(pattern.RegressionLocation, ",
  "\t\t_ = err\n\t\treturn m\n\t}\n\tif fromOutput {\n\t\tm, _ := render.New(pattern.RegressionLocation, ",
  "TestSTR43j_")
m("ST-2.3 minutes over the slot's range are clamped", V,
  "\tminutes := slot.Minutes(max(0, (red-green)/60_000))\n",
  "\tminutes := slot.Minutes(min(10_080, max(0, (red-green)/60_000)))\n", "TestSTR43g_")
m("ST-2.3 and k more leaves out the writes that name no path", V,
  "\t\tunlisted := g.PathsOmitted + unnamedBetween(run.Declarations, *g.LastGreenSeq, *g.LastRunSeq)\n",
  "\t\tunlisted := g.PathsOmitted\n", "TestSTR43b_")
