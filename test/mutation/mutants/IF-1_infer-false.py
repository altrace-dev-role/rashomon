# IF-1: one tool's forms bind one gate, and a gate with no run never "ran"
# (card INFER-FALSE1, B302 and B303 from QA-2's FHV on ea6f100c). Each break
# restores one false line and names the tests that hold it.
K = "internal/shape/masked_name.go"
V = "internal/recap/value.go"
D = "internal/recap/done.go"
m("IF-1 python3 -m pytest keys apart from pytest", K,
  "\t\tfor _, w := range sameWork {\n\t\t\tif w[0] == name {\n\t\t\t\tname = w[1]\n\t\t\t}\n\t\t}\n", "",
  "TestIF1v_|TestIF1_SameWorkIsRows")
m("IF-1 a gate with no run is said to have ran", V,
  "\t\tif g.LastRun == nil {\n\t\t\t// No run of the gate is in the record",
  "\t\tif false {\n\t\t\t// No run of the gate is in the record", "TestIF1_AGateWithNoRunNeverRan")
m("IF-1 a silent done row says no test or build ran", D,
  "if s = fitted(v, prefix+doneLead); s == \"\" {\n\t\t\treturn \"\", false\n",
  "if s = fitted(v, prefix+doneLead); s == \"\" {\n\t\t\ts = noRunSentence\n", "TestIF1_AGateWithNoRunNeverRan")
