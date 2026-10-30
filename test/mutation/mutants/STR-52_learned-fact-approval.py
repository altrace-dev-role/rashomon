# STR-52: a learned fact reaches the model only after a person approved it.
# active.json is the one file the prompt path reads, and ReadActive keeps its
# probation and proven rows only; Save never writes a proposed row there, so
# TestLRN17_ also signs one in by hand, as an agent holding the key could.
# The kill row is LRN-17's first test, declared as the pair in
# shared-tests.txt.
A = "internal/learn/active.go"

m("STR-52 a proposed lesson delivered (the tier filter keeps proposed rows)", A,
  "\t\tif ls.Tier == TierProbation || ls.Tier == TierProven {\n",
  "\t\tif ls.Tier != TierDemoted {\n", "TestLRN17_")
