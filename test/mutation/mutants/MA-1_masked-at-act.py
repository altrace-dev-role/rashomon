# MA-1 masked status at the act (card MASKED-AT-ACT1). Each break removes one
# rule: the post's hint and its four conditions, the trigger at PostToolUse,
# the producer's event, state and own-call tests, the five places a VERIFY
# pattern is mapped (raised, facts, kind, ceiling, key) and its vocabulary,
# the note's renderer, runner words and stage words, the stage table, and the
# report's key; and names the test that holds it.
V = "internal/verify/steer_masked.go"
H = "internal/hook/post.go"
m("MA-1 the post never hints a hidden run", H,
  "\tp.landedHidden = hiddenRun(&rec)\n", "", "TestMA1_TheMaskedExitIsSaidAtTheAct")
m("MA-1 a failed line is a hidden run", H,
  "\treturn rec.Outcome == store.ExecOK && rec.RunnerDigest != nil && !rec.Backgrounded && shape.StatusHidden(rec.StatusMasked)\n",
  "\treturn rec.RunnerDigest != nil && !rec.Backgrounded && shape.StatusHidden(rec.StatusMasked)\n", "TestMA1h_")
m("MA-1 a line with no runner is a hidden run", H,
  "\treturn rec.Outcome == store.ExecOK && rec.RunnerDigest != nil && !rec.Backgrounded && shape.StatusHidden(rec.StatusMasked)\n",
  "\treturn rec.Outcome == store.ExecOK && !rec.Backgrounded && shape.StatusHidden(rec.StatusMasked)\n", "TestMA1h_")
m("MA-1 a backgrounded launch is a hidden run", H,
  "\treturn rec.Outcome == store.ExecOK && rec.RunnerDigest != nil && !rec.Backgrounded && shape.StatusHidden(rec.StatusMasked)\n",
  "\treturn rec.Outcome == store.ExecOK && rec.RunnerDigest != nil && shape.StatusHidden(rec.StatusMasked)\n", "TestMA1h_")
m("MA-1 a status in view is a hidden run", H,
  "\treturn rec.Outcome == store.ExecOK && rec.RunnerDigest != nil && !rec.Backgrounded && shape.StatusHidden(rec.StatusMasked)\n",
  "\treturn rec.Outcome == store.ExecOK && rec.RunnerDigest != nil && !rec.Backgrounded && rec.StatusMasked != nil\n",
  "TestMA1h_")
m("MA-1 a hidden run does not step", "internal/hook/steerstep.go",
  " ||\n\t\thidden && post", "", "TestST_StepsIsTheTriggerTable")
m("MA-1 a hidden run steps at its failure event too", "internal/hook/steerstep.go",
  "\t\thidden && post", "\t\thidden", "TestST_StepsIsTheTriggerTable")
m("MA-1 masked status is raised at a failure event", V,
  "\tif event != string(steer.EventPostToolUse) {\n\t\treturn nil\n\t}\n\tvar out []Finding\n", "\tvar out []Finding\n",
  "TestMA1b_")
m("MA-1 any state of this call's run raises it", V,
  "\t\tif g.State != MaskedInconclusive || g.LastRun == nil", "\t\tif g.LastRun == nil", "TestMA1b_")
m("MA-1 a later call raises it for the masked run", V,
  " || g.LastRun.ToolUseID != toolUseID {\n\t\t\tcontinue", " {\n\t\t\tcontinue", "TestMA1b_")
m("MA-1 the producer is not called", "internal/verify/steer.go",
  "\tfacts = append(facts, maskedStatus(snap, c.runs, pt, event, toolUseID)...)\n", "", "TestMA1a_")
m("MA-1 VERIFY does not raise masked-status", "internal/verify/tosteer.go",
  "\t\tpattern.MaskedStatus, pattern.SubtaskGate, pattern.PlatformRule,", "\t\tpattern.SubtaskGate, pattern.PlatformRule,",
  "TestMA1a_")
m("MA-1 a masked fact does not fit its pattern", "internal/verify/tosteer.go",
  "\tcase pattern.RegressionLocation, pattern.StaleEvidence, pattern.MaskedStatus, pattern.SubtaskGate:\n",
  "\tcase pattern.RegressionLocation, pattern.StaleEvidence, pattern.SubtaskGate:\n", "TestMA1a_")
m("MA-1 masked-status has no kind to map", "internal/verify/tosteer.go",
  "\tcase pattern.RegressionLocation, pattern.StaleEvidence, pattern.MaskedStatus:\n\t\treturn \"\", \"\"\n",
  "\tcase pattern.RegressionLocation, pattern.StaleEvidence:\n\t\treturn \"\", \"\"\n", "TestMA1a_")
m("MA-1 the card raises it before the call", "internal/verify/tosteer_card.go",
  "\tcase id == pattern.MaskedStatus && event == post:\n", "\tcase id == pattern.MaskedStatus && event == pre:\n",
  "TestMA1a_")
m("MA-1 the key is not the gate", "internal/verify/tosteer_card.go",
  "\tcase pattern.MaskedStatus:\n\t\treturn string(f.Gate.GateID), \"\"\n",
  "\tcase pattern.MaskedStatus:\n\t\treturn \"masked\", \"\"\n", "TestMA1a_")
m("MA-1 a masked state contradicts nothing", "internal/verify/tosteer_vocab.go",
  "\t\treturn g.State == MaskedInconclusive && g.LastRunSeq != nil\n", "\t\treturn false\n", "TestMA1a_")
m("MA-1 the note is not rendered", "internal/verify/steer.go",
  "\t\t\tsf.Message = renderMasked(run, f)\n", "\t\t\tsf.Message = render.Message{}\n", "TestMA1a_")
m("MA-1 the note names the program, not the runner", V,
  "\tif runner != \"\" && slot.Gate(runner).Validate() == nil {\n", "\tif false && runner != \"\" && slot.Gate(runner).Validate() == nil {\n",
  "TestMA1a_")
m("MA-1 an unlisted stage is named as one", V,
  "\tcase *by == \"other\":\n\t\treturn \"a pipe\"\n", "", "TestMA1c_")
m("MA-1 an unknown stage word is named as a pipe stage", V,
  "\tcase by == nil || shape.MaskedByID(*by) == 0:\n", "\tcase by == nil:\n", "TestMA1d_")
m("MA-1 the stage table has no line", "internal/steer/render/slot/enum.go",
  "\"the pipe to cut\", \"a pipe\", \"its line\",\n", "\"the pipe to cut\", \"a pipe\",\n", "TestMA1c_|TestMA1d_")
m("MA-1 the report does not key a masked note by its gate", "internal/recap/impact.go",
  "string(pattern.StaleEvidence), string(pattern.MaskedStatus),\n", "string(pattern.StaleEvidence),\n", "TestMA1i_")
# Two breaks remove a package's last use in steer_masked.go.
IMPORTS["MA-1 masked status is raised at a failure event"] = (
    V, '\t"github.com/altrace-dev-role/rashomon/internal/steer"\n', "")
IMPORTS["MA-1 an unknown stage word is named as a pipe stage"] = (
    V, '\t"github.com/altrace-dev-role/rashomon/internal/shape"\n', "")
