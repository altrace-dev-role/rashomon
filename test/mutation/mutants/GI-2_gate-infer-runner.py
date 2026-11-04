# GI-2 (card GATE-INFER2): the next prompt's owed note binds a runner whose
# row is more than two words or holds the interpreter's -m (`python3 -m
# pytest`, `uv run pytest`, `npm run test`): the gate slot admits a row of
# the binary's runner lists, and nothing else its one-or-two-word rule
# refuses. An owed note whose name the slot still refuses is a render_refused
# steer line, once per turn end, never a silent drop. DE R87: the name is
# compared with the row's exact string, and three words no list writes stay
# refused; a masked run's note names its gate by the row too (TestGI2v_).
S = "internal/steer/render/slot/slot.go"
N = "cmd/rashomon/gateinfer_note.go"
V = "internal/verify/steer_masked.go"

m("GI-2 Gate refuses a runner's row", S,
  "err != nil && shape.RunnerNameID(s) == 0 {", "err != nil && (shape.RunnerNameID(s) == 0 || true) {",
  "TestGI2s_|TestGI2n_TheNote|TestGI2na_|TestGI2v_")
m("GI-2 Gate admits whatever its words refuse", S,
  "err != nil && shape.RunnerNameID(s) == 0 {", "err != nil && shape.RunnerNameID(s) == 0 && false {", "TestGI2s_")
m("GI-2 Gate admits a three-word name no list writes", S,
  "\tif len(words) > 2 {\n", "\tif len(words) > 3 {\n", "TestGI2s_")
m("GI-2 a row is looked up by its first word", S,
  "shape.RunnerNameID(s) == 0 {", "shape.RunnerNameID(strings.SplitN(s, \" \", 2)[0]) == 0 {", "TestGI2s_")
m("GI-2 a row is matched with its case folded", S,
  "shape.RunnerNameID(s) == 0 {", "shape.RunnerNameID(strings.ToLower(s)) == 0 {", "TestGI2s_")
m("GI-2 a row is matched with its spaces collapsed", S,
  "shape.RunnerNameID(s) == 0 {", "shape.RunnerNameID(strings.Join(strings.Fields(s), \" \")) == 0 {", "TestGI2s_")
m("GI-2 a masked note names a three-word row by its program", V,
  "slot.Gate(runner).Validate() == nil {", "slot.Gate(runner).Validate() == nil && strings.Count(runner, \" \") < 2 {",
  "TestGI2v_AMaskedNoteNamesItsGateByTheRunnersRow")
m("GI-2 a refused name drops silently", N,
  "if err != nil && o.Gate == \"\" {", "if err != nil {", "TestGI2n_ARefused")
m("GI-2 an unnamed owed note leaves a trace", N,
  "if err != nil && o.Gate == \"\" {", "if err != nil && false {", "TestGI2n_ARefused")
m("GI-2 a refused line is not given", N,
  " ||\n\t\t\t\tscanText(b, sp[3]) == string(steer.SuppressedByRenderRefused))", ")", "TestGI2n_ARefused")
