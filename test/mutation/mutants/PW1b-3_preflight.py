# PW1b-ON-230 step 3: the preflight platform line. decode's keep sees each
# record's object as well as its subject (a symbol's callers are the records
# whose object it is; ARCH-INLINE1 reads the same).
D = "internal/platform/decode.go"

m("PW1b-3 keep sees the subject twice", D,
  "!keep(c[1], c[3])", "!keep(c[1], c[1])", "TestPW1b3_")

# The prompt's symbol read (Symbols): PROVEN calls only, once per caller
# whatever the config; a name matched on its boundary; at most maxNames
# names and maxDecls declarations; only the walk file's pair; every package
# the answer rests on fresh; an id's package the one its kind's shape fits.
# No row drops the !vouched.on test: with no walk file the digests check
# refuses the pair too (vouched.digests is ""), so that row is equivalent.
S = "internal/platform/symbols.go"

m("PW1b-3 every tier counts", S,
  "\t\tcase f.tier != tierProven:\n", "\t\tcase false:\n", "TestPW1b3_SymbolsNames")
m("PW1b-3 a may-call counts", S,
  "\t\tcase f.predicate == predCalls:\n", "\t\tcase f.predicate == predCalls || f.predicate == predMayCall:\n",
  "TestPW1b3_SymbolsNames")
m("PW1b-3 a call counts once per config", S,
  "\t\t\tif !slices.Contains(callers[f.object], f.subject) {\n", "\t\t\tif true {\n", "TestPW1b3_SymbolsNames")
m("PW1b-3 a name matches inside a word", S,
  "(rest[j-1] == '.' || rest[j-1] == '/')", "true", "TestPW1b3_SymbolsNames")
m("PW1b-3 a name in three packages is given", S,
  "len(ids) > maxDecls", "len(ids) > maxDecls+1", "TestPW1b3_SymbolsNames")
m("PW1b-3 no cap on names", S,
  "\tnames = names[:min(len(names), maxNames)]\n", "", "TestPW1b3_TheReadLooksUp")
m("PW1b-3 a caller's package need not be fresh", S,
  "\t\tif p == \"\" || !fr.fresh(p) {\n", "\t\tif p == \"\" {\n", "TestPW1b3_AStaleCaller")
m("PW1b-3 the log is not checked against the walk file", S,
  "MaxLogBytes, vouched.log, hook", "MaxLogBytes, \"\", hook", "TestPW1b3_OnlyAVouched")
m("PW1b-3 the digests are not checked against the walk file", S,
  "\tif err != nil || dg.sum != vouched.digests {\n", "\tif err != nil {\n", "TestPW1b3_OnlyAVouched")
m("PW1b-3 packageIn ignores the kind's shape", S,
  "!listed || !fitsKind(kind, rest[i+1:])", "!listed", "TestPW1b3_PackageIn")
m("PW1b-3 packageIn takes the longest prefix", S,
  "\t\tif found != \"\" {\n\t\t\treturn \"\"\n\t\t}\n", "\t\tif found != \"\" {\n\t\t\treturn found\n\t\t}\n",
  "TestPW1b3_PackageIn")
