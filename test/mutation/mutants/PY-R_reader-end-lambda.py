# PY-R, the Python reader of pylite/4: every func and method node carries
# end, the 1-based last line of its body, a number and no text, the lines of
# a call in the body and of a def nested in it included and no blank or
# comment line counted; a lambda's commas are its own, so a lambda as a
# default or as an argument is one parameter or one argument, a lambda in a
# lambda's parameters closing at its own colon first; and the reader is
# pylite version 4. One row per Break clause of the tests in
# internal/graph/lang/pylite/pyr_test.go, each killed by the test it names.
scan = "internal/graph/lang/pylite/scan.go"
pylite = "internal/graph/lang/pylite/pylite.go"

m("PY-R end stays on the line the parameters close on", scan,
  "\t\t\tf.defs[len(f.defs)-1].end = i + 1\n",
  "\t\t\t// end not moved past the parameters\n",
  "TestPYR_EveryDefCarriesTheLineItsBodyEndsOn")
m("PY-R a def node writes no end", pylite,
  ', "end": strconv.Itoa(d.end)}}},', "}}},",
  "TestPYR_EveryDefCarriesTheLineItsBodyEndsOn")
m("PY-R every comma at depth zero splits, a lambda's own too", scan,
  "\t\t\tif depth == 0 && open == 0 {\n",
  "\t\t\tif depth == 0 {\n",
  "TestPYR_ALambdasCommasAreItsOwn")
m("PY-R the first colon closes every open lambda", scan,
  "\t\t\t\topen--\n",
  "\t\t\t\topen = 0\n",
  "TestPYR_ALambdasCommasAreItsOwn")
m("PY-R the reader stays pylite/3", pylite,
  '\tprovVersion   = "4"\n', '\tprovVersion   = "3"\n',
  "TestPYR_TheReaderIsPyliteFour")
