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
  "!listed || !fitsKind(kind, rest[i+1:])", "!listed || !fitsKind(kind, rest[i+1:]) && false",
  "TestPW1b3_PackageIn")
m("PW1b-3 packageIn takes the longest prefix", S,
  "\t\tif found != \"\" {\n\t\t\treturn \"\"\n\t\t}\n", "\t\tif found != \"\" {\n\t\t\treturn found\n\t\t}\n",
  "TestPW1b3_PackageIn")

# The wiring: intent asks the graph for the request's identifiers (each
# once, at most four) and says the symbols-unknown only when it proved
# none; a proven symbol makes the note new; the first prompt's note gives
# the symbols in the unknowns' place; the v4 record counts them. The slot
# leaves out and counts a name it refuses, screens names for secrets, says
# "1 package", and its record form names nothing. The prompt path hands
# intent the read; the turn end's owed packages reach the next prompt,
# inside the gate's note or alone.
I = "internal/intent/intent.go"
N = "internal/intent/note.go"
SL = "internal/steer/render/slot/preflight.go"
PF = "cmd/rashomon/preflight.go"
GN = "cmd/rashomon/gateinfer_note.go"

m("PW1b-3 Resolve asks the graph for nothing", I,
  "\t\tin.Symbols = p.Symbols(idents)\n", "", "TestPW1b3_ResolveAsks")
m("PW1b-3 the symbols-unknown is said whatever the graph proved", I,
  "say(symbols && len(in.Symbols) == 0, unknownSymbols)", "say(symbols, unknownSymbols)", "TestPW1b3_ResolveAsks")
m("PW1b-3 every identifier is asked", I,
  "if len(idents) < MaxSymbolNames && !slices.Contains(idents, string(tok)) {",
  "if !slices.Contains(idents, string(tok)) {", "TestPW1b3_ResolveAsks")
m("PW1b-3 a symbol is not new to the session", N,
  "fresh := len(paths) > 0 || len(rules) > 0 || len(in.Symbols) > 0", "fresh := len(paths) > 0 || len(rules) > 0",
  "TestPW1b3_ASymbolIsNew")
m("PW1b-3 the first note keeps its unknowns over the symbols", N,
  "\tif sf != nil {\n\t\treturn render.TaskPreflightFirstSymbolsBinding{",
  "\tif false {\n\t\treturn render.TaskPreflightFirstSymbolsBinding{", "TestPW1b3_TheNoteGives")
m("PW1b-3 the v4 record names the symbols", N,
  "\t\tc := sf.Counted()\n", "\t\tc := *sf\n", "TestPW1b3_TheV4Record")
m("PW1b-3 a refused symbol is rendered", SL,
  "\t\tif it.check() != nil {\n\t\t\tmore++\n\t\t\tcontinue\n\t\t}\n", "", "TestPW1b3_SymbolFacts")
m("PW1b-3 a symbol's name is not screened", SL,
  "\tif err := denied(\"symbol\", f.Name); err != nil {\n\t\treturn err\n\t}\n", "", "TestPW1b3_SymbolFacts")
m("PW1b-3 one package reads as many", SL,
  "\tif f.Packages == 1 {\n", "\tif false {\n", "TestPW1b3_SymbolFacts")
m("PW1b-3 the counted form names the symbols", SL,
  "\treturn SymbolFacts{more: len(s.items) + s.more, counted: true}\n", "\treturn s\n", "TestPW1b3_SymbolFacts")
m("PW1b-3 the prompt reads no graph", PF,
  ",\n\t\t\tSymbols: promptSymbols(root)})", "})", "TestPW1b3_APromptNaming")
m("PW1b-3 the owed packages are not carried", GN,
  "o.Seq, o.Packages = v.Snap.Point.Seq, pkgs", "o.Seq, o.Packages = v.Snap.Point.Seq, nil", "TestPW1b3_TheNextPrompt")
m("PW1b-3 the owed packages never ride the gate's note", GN,
  "\t\tif pkgs != nil {\n\t\t\tforms = append(forms, render.ClaimVsRecordInferredPlatformBinding",
  "\t\tif false {\n\t\t\tforms = append(forms, render.ClaimVsRecordInferredPlatformBinding", "TestPW1b3_TheNextPrompt")
m("PW1b-3 the owed packages need a gate", GN,
  "\t} else if pkgs != nil {\n\t\tforms = append(forms, render.ClaimVsRecordPlatformBinding{Packages: *pkgs})\n\t}\n", "\t}\n",
  "TestPW1b3_TheNextPrompt")

# The steer line's symbols slot (B327): slotsFrom records it, or Route drops
# the whole note on the real binary; the counted form keeps its count.
W = "internal/steer/slots.go"

m("PW1b-3 the steer line cannot record the symbols", W,
  "slot.EnumList, slot.SymbolFacts:\n", "slot.EnumList:\n",
  "TestSlotsOf_EverySlotTypeHasARow|TestPW1b3_TheLineHoldsCountedSymbolsAsACount")
m("PW1b-3 the counted symbols lose their count", W,
  "More: v.More(), Names: []string{}", "More: 0, Names: []string{}",
  "TestPW1b3_TheLineHoldsCountedSymbolsAsACount")
