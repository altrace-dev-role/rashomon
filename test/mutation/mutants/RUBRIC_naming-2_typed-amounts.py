# Rubric naming-2: nanodollars and token counts are distinct types, made by
# price alone. These breaks are the ones the types cannot stop -- a kind's
# count or price taken from the wrong kind inside price -- and the exact
# by_kind assertions are what catch them.
m("RUBRIC naming-2 output is priced at the input rate", "internal/spend/amounts.go",
  "\tp.output.nano = nanoUSD(t.Output * rt.Output)", "\tp.output.nano = nanoUSD(t.Output * rt.Input)",
  "TestRubricNAMING2_EachKindIsPricedAtItsOwnRate")
m("RUBRIC naming-2 cache reads carry the output count", "internal/spend/amounts.go",
  "\t\tcacheRead:  kindCost{tokens: tokenCount(t.CacheRead)},", "\t\tcacheRead:  kindCost{tokens: tokenCount(t.Output)},",
  "TestRubricNAMING2_EachKindIsPricedAtItsOwnRate")
m("RUBRIC naming-2 the output row is priced at the cache-read figure", "internal/spend/spend.go",
  "\t\tkind(&s.ByKind.Output, p.output)", "\t\tkind(&s.ByKind.Output, p.cacheRead)",
  "TestRubricNAMING2_EachKindIsPricedAtItsOwnRate")
