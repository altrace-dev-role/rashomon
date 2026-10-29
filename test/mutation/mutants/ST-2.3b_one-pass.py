# Card 2.3b: the run reader decodes a line once. A line in the writer's form
# has its pair read from its first bytes (prefixPair) and is decoded once into
# its kind's record; any other line takes the head-first path it always did.
# On every path a record whose decoded pair is not the pair it was read under
# is Skipped (decodeAs, D97). The prefix rows are equivalent at
# classifyRunLine's level, where the fallback reads the same line the same
# way, so their kill is prefixPair's own table (A11).
R = "internal/store/readprefix.go"
D = "internal/store/read.go"

# prefixPair (R1, A11): each bound of the writer's form loosened or tightened.
m("ST-2.3b the prefix reads a four-digit version", R,
  "\tprefixDigits = 3\n", "\tprefixDigits = 4\n", "TestREC_PrefixReadsEveryKindVersion")
m("ST-2.3b the prefix declines a three-digit version", R,
  "\tprefixDigits = 3\n", "\tprefixDigits = 2\n", "TestREC_PrefixReadsEveryKindVersion")
m("ST-2.3b the prefix needs no comma after the version", R,
  "\tif n == 0 || n > prefixDigits || n == len(rest) || rest[n] != ',' {\n",
  "\tif n == 0 || n > prefixDigits {\n", "TestREC_PrefixReadsEveryKindVersion")
m("ST-2.3b the prefix's kind table drops a kind", R,
  "\t\tnames[kind] = kind\n", "\t\tif kind != TypeTerminal {\n\t\t\tnames[kind] = kind\n\t\t}\n",
  "TestREC_PrefixReadsEveryKindVersion")
m("ST-2.3b the prefix is read past leading whitespace", R,
  "\trest, ok := cutPrefix(line, prefixType)\n",
  "\trest, ok := cutPrefix(bytes.TrimLeft(line, \" \\t\"), prefixType)\n",
  "TestREC_PrefixReadsEveryKindVersion")

# decodeAs (R2, A8, D97): the one check every path makes.
m("ST-2.3b a record whose pair changed is collected", R,
  "\tif k, v := rec.pair(); k != kind || v != version {\n\t\treturn errPairChanged\n\t}\n", "",
  "TestREC_DuplicateTypeKeyIsSkipped|TestREC_OnePassEqualsTwoPass")
m("ST-2.3b a body that does not decode is collected", R,
  "\tif err := json.Unmarshal(line, rec); err != nil {\n\t\treturn err\n\t}\n",
  "\t_ = json.Unmarshal(line, rec)\n", "TestREC_MalformedBodyAfterValidPrefixIsSkipped")

# classifyRunLine (R1, A8): the fast path's branches.
m("ST-2.3b a line not in the writer's form is Skipped", D,
  "\tif !ok {\n\t\tclassifyByHead(run, line)\n\t\treturn\n\t}\n",
  "\tif !ok {\n\t\trun.Skipped++\n\t\treturn\n\t}\n", "TestREC_ForeignOrderFallsBack")
m("ST-2.3b the fast path accepts a refused pair", D,
  "\tif !AcceptsKind(kind, version) {\n\t\tvar head runHead\n", "\tif false {\n\t\tvar head runHead\n",
  "TestREC_RefusedPairStillReadsCritical")
m("ST-2.3b a refused pair's critical key is not read", D,
  "\t\tpassOver(run, head.Critical)\n\t\treturn\n\t}\n\tif readAccepted(run, line, kind, version) != nil {\n",
  "\t\trun.Skipped++\n\t\treturn\n\t}\n\tif readAccepted(run, line, kind, version) != nil {\n",
  "TestREC_RefusedPairStillReadsCritical")
m("ST-2.3b a refused pair is passed over under its last key", D,
  "\t\tif decodeAs(line, &head, kind, version) != nil {\n", "\t\tif json.Unmarshal(line, &head) != nil {\n",
  "TestREC_DuplicateTypeKeyIsSkipped")
