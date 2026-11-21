# CONSENT-GATE2 (cards x315, x370/x371): the overlay writer keeps a session's
# edit of a file in the session graph log only where the reader that claims
# the file states its session edits are kept (lang.Kept, read through
# Registry.SessionKept), and, for a reader that states the first consent
# scope its files are kept from (lang.KeptFrom, read through
# Registry.SessionKeptFrom), only where the highest scope on file
# (consent.ScopeOnFile, wired as hook.ConsentScopeOnFile) reaches it; the log
# then holds of that reader's records only the attributes its list names,
# filtered from a copy before the group is built. Go states no scope and is
# kept whole; pylite states 9 and five names. One row per Break clause of
# each test, killed by that test alone. The Go-set row moved here from
# LANG-REG1_hook-lines.py; its killer is the kept-reader witness's test.
pylite = "internal/graph/lang/pylite/pylite.go"
gopg = "internal/graph/lang/gopg/gopg.go"
langgo = "internal/graph/lang/lang.go"
overlay = "internal/hook/overlaywrite.go"
record = "internal/consent/record.go"
wiring = "cmd/rashomon/consent.go"

pykept = "func (Resolver) SessionKept() bool { return true }\n"
pyfrom = "const SessionKeptScope = 9\n"
gate = "\tif from > 0 && !scopeAtLeast(c.st.Root(), from) {\n"

# TestConsentGate2_APythonEditUnderAYesOfNineKeepsItsRecords. Breaks: leave
# pylite unkept; ask the consent of a root other than the store's; keep an
# attribute past the test's list, or drop one it names.
m("CONSENT-GATE2 pylite left unkept", pylite, pykept,
  "func (Resolver) SessionKept() bool { return !true }\n",
  "^TestConsentGate2_APythonEditUnderAYesOfNineKeepsItsRecords$")
m("CONSENT-GATE2 the scope asked of the repository, not the store", overlay, gate,
  "\tif from > 0 && !scopeAtLeast(at, from) {\n",
  "^TestConsentGate2_APythonEditUnderAYesOfNineKeepsItsRecords$")
m("CONSENT-GATE2 a parameter count kept past the list", pylite,
  "\"node sym line\", \"edge calls line\"}", "\"node sym line\", \"node sym arity_min\", \"edge calls line\"}",
  "^TestConsentGate2_APythonEditUnderAYesOfNineKeepsItsRecords$")
m("CONSENT-GATE2 a declaration's line left off the list", pylite,
  "\"node sym symkind\", \"node sym line\", ", "\"node sym symkind\", ",
  "^TestConsentGate2_APythonEditUnderAYesOfNineKeepsItsRecords$")

# TestConsentGate2_APythonEditUnderAYesBelowNineKeepsNothing. Breaks: state
# pylite kept from 8, or with no scope of its own. Its premise row: gopg
# states not kept, and the .go edit keeps none.
m("CONSENT-GATE2 pylite kept from 8", pylite, pyfrom, "const SessionKeptScope = 8\n",
  "^TestConsentGate2_APythonEditUnderAYesBelowNineKeepsNothing$")
m("CONSENT-GATE2 pylite kept with no scope of its own", pylite,
  "func (Resolver) SessionKeptFrom() int { return SessionKeptScope }\n",
  "func (Resolver) SessionKeptFrom() int { return 0 }\n",
  "^TestConsentGate2_APythonEditUnderAYesBelowNineKeepsNothing$")
m("CONSENT-GATE2 gopg states not kept", gopg,
  "func (Resolver) SessionKept() bool { return true }\n",
  "func (Resolver) SessionKept() bool { return !true }\n",
  "^TestConsentGate2_APythonEditUnderAYesBelowNineKeepsNothing$")

# TestConsentGate2_APythonEditWithNoScopeReadKeepsNothing. Breaks: keep the
# group where the reader is unset; take the 9 past the error.
m("CONSENT-GATE2 an unset scope reader reads as a yes", overlay,
  "\tif ConsentScopeOnFile == nil {\n\t\treturn false\n\t}\n",
  "\tif ConsentScopeOnFile == nil {\n\t\treturn true\n\t}\n",
  "^TestConsentGate2_APythonEditWithNoScopeReadKeepsNothing$")
m("CONSENT-GATE2 the scope taken past its read error", overlay,
  "\treturn err == nil && scope >= from\n",
  "\t_ = err\n\treturn scope >= from\n",
  "^TestConsentGate2_APythonEditWithNoScopeReadKeepsNothing$")

# TestConsentGate2_AGoEditNeverReadsTheScope. Break: read the scope on file
# for every reader.
m("CONSENT-GATE2 the scope read for every reader", overlay, gate,
  "\tif !scopeAtLeast(c.st.Root(), from) && from > 0 {\n",
  "^TestConsentGate2_AGoEditNeverReadsTheScope$")

# TestConsentGate2_TheLogKeepsNoAttributeTheTextDoesNotName. Breaks: keep
# every attribute the reader writes (no list); filter the records the post
# holds.
m("CONSENT-GATE2 the log keeps every attribute", overlay,
  "\t\tlogged.Recs = keepAttrs(recs, kept)\n",
  "\t\tlogged.Recs, _ = recs, kept\n",
  "^TestConsentGate2_TheLogKeepsNoAttributeTheTextDoesNotName$")
m("CONSENT-GATE2 the post holds the filtered records", overlay,
  "\treturn group, at, under, true\n",
  "\treturn logged, at, under, true\n",
  "^TestConsentGate2_TheLogKeepsNoAttributeTheTextDoesNotName$")

# TestConsentGate2_ANameKeptForOneKindOfRecordIsLeftOutOnAnother. Break:
# keep a name any kind lists on every kind.
m("CONSENT-GATE2 a listed name kept on every kind", overlay,
  "\t\tmid, last, ok := strings.Cut(rest, \" \")\n\t\tif ok && mid == kind && last == name {\n",
  "\t\t_, last, ok := strings.Cut(rest, \" \")\n\t\tif ok && last == name {\n",
  "^TestConsentGate2_ANameKeptForOneKindOfRecordIsLeftOutOnAnother$")

# TestConsentGate2_APythonEditPastTheBoundKeepsTheCutAndAWholeMarker. Break:
# filter the built group, so the cut and the marker's own names go.
m("CONSENT-GATE2 the built group filtered", overlay,
  "\t_ = log.AppendOverlayGroup(graph.BuildOverlayGroup(logged), func",
  "\t_ = log.AppendOverlayGroup(keepAttrs(graph.BuildOverlayGroup(group), kept), func",
  "^TestConsentGate2_APythonEditPastTheBoundKeepsTheCutAndAWholeMarker$")

# TestConsentGate2_AReaderThatStatesNotKeptKeepsNothing. Breaks: read a
# reader as kept whatever it states; drop the writer's check of it.
m("CONSENT-GATE2 the registry calls kept every reader that states", langgo,
  "\treturn ok && k.SessionKept()\n",
  "\treturn ok && (k.SessionKept() || ok)\n",
  "^TestConsentGate2_AReaderThatStatesNotKeptKeepsNothing$")
m("CONSENT-GATE2 the writer keeps a group whatever the reader states", overlay,
  "\tif !g.reg.SessionKept(at, under) {\n",
  "\tif !g.reg.SessionKept(at, under) && under == \"\" {\n",
  "^TestConsentGate2_AReaderThatStatesNotKeptKeepsNothing$")

# TestScopeOnFile_TheHighestScopeOnFile. Breaks: read the last record's
# scope; answer true or false in place of the number; answer a scope past
# the error.
scopemax = "\tbest, _ := maxBy(recs, func(r Record) int { return r.ScopeVersion })\n"
m("CONSENT-GATE2 the scope on file is the last record's", record,
  scopemax + "\treturn best, nil\n",
  "\tbest := 0\n\tif len(recs) > 0 {\n\t\tbest = recs[len(recs)-1].ScopeVersion\n\t}\n\treturn best, nil\n",
  "^TestScopeOnFile_TheHighestScopeOnFile$")
m("CONSENT-GATE2 the scope on file answered as yes or no", record,
  scopemax + "\treturn best, nil\n", scopemax + "\treturn min(best, 1), nil\n",
  "^TestScopeOnFile_TheHighestScopeOnFile$")
m("CONSENT-GATE2 the scope on file answered past its read error", record,
  "\t\treturn 0, err\n\t}\n" + scopemax, "\t\treturn 0, nil\n\t}\n" + scopemax,
  "^TestScopeOnFile_TheHighestScopeOnFile$")

# TestLANGREG1_APythonEditIsKeptFromAYesOfNine. Breaks: leave the scope
# reader unwired in the binary; state pylite kept from 8, end to end.
m("CONSENT-GATE2 the scope reader left unwired", wiring,
  "\thook.ConsentScopeOnFile = consent.ScopeOnFile\n", "",
  "^TestLANGREG1_APythonEditIsKeptFromAYesOfNine$")
m("CONSENT-GATE2 pylite kept from 8, end to end", pylite, pyfrom,
  "const SessionKeptScope = 8\n",
  "^TestLANGREG1_APythonEditIsKeptFromAYesOfNine$")

# TestLANGREG1_APythonEditAfterAWithdrawnAnswerKeepsNothing. Break: keep an
# edit's group without the session-content marker, so a withdrawn answer
# whose scope stays on file still keeps.
m("CONSENT-GATE2 the session-content marker not read", overlay,
  " || !store.SessionContentMarker(st.Root()) {\n", " {\n",
  "^TestLANGREG1_APythonEditAfterAWithdrawnAnswerKeepsNothing$")

# TestConsentGate2_AKeptReaderOtherThanGoCarriesItsOwnSet. Break: carry the
# Go resolver's set for every language, and the group's set is Go's.
goset = "LANG-REG1 every group carries the Go reader's set"
IMPORTS[goset] = (overlay,
                  "\t\"github.com/altrace-dev-role/rashomon/internal/graph/lang\"\n",
                  "\t\"github.com/altrace-dev-role/rashomon/internal/graph/lang\"\n"
                  "\t\"github.com/altrace-dev-role/rashomon/internal/graph/lang/gopg\"\n")
m(goset, overlay,
  "\t\tCarried: r.OverlayCarried(), Recs: recs}\n",
  "\t\tCarried: gopg.Resolver{}.OverlayCarried(), Recs: recs}\n",
  "^TestConsentGate2_AKeptReaderOtherThanGoCarriesItsOwnSet$")
