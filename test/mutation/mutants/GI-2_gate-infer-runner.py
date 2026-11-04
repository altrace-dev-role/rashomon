# GI-2 (card GATE-INFER2): the next prompt's owed note binds a runner whose
# row is more than two words or holds the interpreter's -m (`python3 -m
# pytest`, `uv run pytest`, `npm run test`): the gate slot admits a row of
# the binary's runner lists, and nothing else its one-or-two-word rule
# refuses. An owed note whose name the slot still refuses is a render_refused
# steer line, once per turn end, never a silent drop.
S = "internal/steer/render/slot/slot.go"
N = "cmd/rashomon/gateinfer_note.go"

m("GI-2 Gate refuses a runner's row", S,
  "err != nil && shape.RunnerNameID(s) == 0 {", "err != nil && (shape.RunnerNameID(s) == 0 || true) {",
  "TestGI2s_|TestGI2n_TheNote|TestGI2na_")
m("GI-2 Gate admits whatever its words refuse", S,
  "err != nil && shape.RunnerNameID(s) == 0 {", "err != nil && shape.RunnerNameID(s) == 0 && false {", "TestGI2s_")
m("GI-2 a row is looked up by its first word", S,
  "shape.RunnerNameID(s) == 0 {", "shape.RunnerNameID(strings.SplitN(s, \" \", 2)[0]) == 0 {", "TestGI2s_")
m("GI-2 a refused name drops silently", N,
  "\t\tm, _ = render.New(pattern.ClaimVsRecord, render.ClaimVsRecordInferredUnnamedBinding{Gate: slot.Gate(o.Gate)})\n\t}\n",
  "\t\tm, _ = render.New(pattern.ClaimVsRecord, render.ClaimVsRecordInferredUnnamedBinding{Gate: slot.Gate(o.Gate)})\n\t}\n"
  "\tif m.Text() == \"\" {\n\t\treturn steer.Finding{}, false\n\t}\n", "TestGI2n_ARefused")
m("GI-2 an unnamed owed note leaves a trace", N,
  "afterSeq || o.Gate == \"\" {", "afterSeq {", "TestGI2n_ARefused")
m("GI-2 a refused line is not given", N,
  " ||\n\t\t\t\tscanText(b, sp[3]) == string(steer.SuppressedByRenderRefused))", ")", "TestGI2n_ARefused")
