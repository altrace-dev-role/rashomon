# F2c: a masked last run with a write its gate covers after it is stale
# (masked_then_writes, maskedThenWritten), keeps its run and its unmet
# exit_unmasked row (exitUnmasked); a write before the masked run leaves it
# masked, the classic catch. A call acts on the turn's projects only where it
# writes or executes (acts): a read whose line may write nothing acts on none,
# and a record before may_write acts. A turn no call of which acts in a
# project is in the projects it only looked in (turnProjects), not in every
# one. A later echo or date stage prints, and sets no may_write of itself
# (readsOrPrints, printOnly). Each break removes one rule and names the tests
# that hold it.
C = "internal/verify/cover.go"
G = "internal/verify/gates.go"
O = "internal/verify/obligations.go"
V = "internal/shape/verb.go"
m("F2c a masked run stays masked whatever came after it", G,
  "\t{maskedThenWritten, Stale, func(*fam) Reason { return MaskedThenWrites }},\n", "", "TestF2d_")
m("F2c a write before the masked run expires it", G,
  "\t\tif w.Seq > f.last.Seq {\n\t\t\treturn true\n", "\t\tif w.Seq > 0 {\n\t\t\treturn true\n", "TestF2d_")
m("F2c a run that is not masked expires as masked", G,
  "func maskedThenWritten(f *fam) bool {\n\tif !masked(f.last) {\n\t\treturn false\n\t}\n",
  "func maskedThenWritten(f *fam) bool {\n", "TestVER12b_")
m("F2c exit_unmasked reads a stale gate as met", O,
  "\tif g.Reason == MaskedThenWrites {\n\t\treturn exitUnmaskedHidden\n\t}\n", "", "TestF2d_")
m("F2c a call that only reads acts", C,
  " || d.Seq > at || acting && !acts(d) {\n", " || d.Seq > at {\n", "TestF2e_|TestF2a_")
m("F2c a turn that only read reads every gate", C,
  "\t\tps, _ = projectsOf(run, promptID, at, false)\n", "\t\tps, _ = projectsOf(run, promptID, at, true)\n",
  "TestF2f_|TestBGO2e_")
m("F2c an act the record does not place falls back to the reads", C,
  "\tif ok && ps == nil {\n", "\tif _ = ok; ps == nil {\n", "TestF2f_|TestF2a_")
m("F2c a record before may_write says its line writes nothing", C,
  "\treturn d.SchemaVersion < 3 || d.Shape.VerbClass", "\treturn d.Shape.VerbClass", "TestF2e_")
m("F2c a read line that may write acts on nothing", C,
  "d.Shape.VerbClass != shape.VerbRead || d.Shape.MayWrite\n", "d.Shape.VerbClass != shape.VerbRead\n", "TestF2e_")
m("F2c a line that executes acts on nothing", C,
  "d.Shape.VerbClass != shape.VerbRead || d.Shape.MayWrite\n", "d.Shape.VerbClass == shape.VerbWrite || d.Shape.MayWrite\n",
  "TestF2e_")
m("F2c a printing stage may write", V,
  "\treturn verbForProgram(prog) == VerbRead || slices.Contains(printOnly, prog)\n",
  "\treturn verbForProgram(prog) == VerbRead\n", "TestF2e_")
m("F2c echo is not a printing stage", V,
  "var printOnly = []string{\"echo\", \"date\"}\n", "var printOnly = []string{\"date\"}\n", "TestF2e_")
m("F2c date is not a printing stage", V,
  "var printOnly = []string{\"echo\", \"date\"}\n", "var printOnly = []string{\"echo\"}\n", "TestF2e_")
