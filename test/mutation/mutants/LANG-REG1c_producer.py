# LANG-REG1c, the producer string: a reader registered indexed that states
# its producer (lang.Versioned) is unindexed under an index header naming
# any other string for its language, compared for equality alone; pylite
# states the string its records carry, from one pair of constants. One row
# per clause, each killed by the test it names. Written, not run: for the
# end sweep.
lang = "internal/graph/lang/lang.go"
pylite = "internal/graph/lang/pylite/pylite.go"

m("LANG-REG1c a stated producer is never compared", lang,
  "\tif v, ok := e.r.(Versioned); ok && producers[l] != v.Producer() {\n\t\treturn l, true\n\t}\n", "",
  "TestRegistry_UnindexedComparesTheProducerString")
m("LANG-REG1c a stated producer is unindexed when it matches", lang,
  "ok && producers[l] != v.Producer()", "ok && producers[l] == v.Producer()",
  "TestRegistry_UnindexedComparesTheProducerString")
m("LANG-REG1c pylite states a string its records do not carry", pylite,
  'return provExtractor + "/" + provVersion }', 'return provExtractor + "/" + provVersion + "x" }',
  "TestBuild_PyProducerIsTheReadersOwn")
