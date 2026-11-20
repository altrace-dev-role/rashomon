# CONSENT-GATE2 (card x315): the overlay writer keeps a session's edit of a
# file in the session graph log only where the reader that claims the file
# states its session edits are kept (lang.Kept, read through
# Registry.SessionKept), and a reader of the registry a hook builds states so
# only where the consent paragraph saying what a yes keeps names files of its
# language: Go yes, Python no. One row per Break clause of each test, killed
# by that test alone. The Go-set row moved here from LANG-REG1_hook-lines.py:
# its test's .py edit keeps nothing now, so its killer is the kept-reader
# witness's test.
pylite = "internal/graph/lang/pylite/pylite.go"
gopg = "internal/graph/lang/gopg/gopg.go"
langgo = "internal/graph/lang/lang.go"
overlay = "internal/hook/overlaywrite.go"

pynot = "func (Resolver) SessionKept() bool { return false }\n"
pykept = "func (Resolver) SessionKept() bool { return !false }\n"

# TestConsentGate2_AKeptReaderIsNamedByTheKeptParagraph. Breaks: state
# pylite kept; let the registry call kept every reader that states anything.
m("CONSENT-GATE2 pylite states kept, against the words", pylite, pynot, pykept,
  "^TestConsentGate2_AKeptReaderIsNamedByTheKeptParagraph$")
m("CONSENT-GATE2 the registry calls kept every reader that states", langgo,
  "\treturn ok && k.SessionKept()\n",
  "\treturn ok && (k.SessionKept() || ok)\n",
  "^TestConsentGate2_AKeptReaderIsNamedByTheKeptParagraph$")

# TestConsentGate2_APythonEditKeepsNothingInTheSessionLog. Breaks: let the
# writer keep a group whatever the claiming reader states; state pylite
# kept. Its premise row: gopg states not kept, and the .go edit keeps none.
m("CONSENT-GATE2 the writer keeps a group whatever the reader states", overlay,
  "\tif !g.reg.SessionKept(at, under) {\n",
  "\tif !g.reg.SessionKept(at, under) && under == \"\" {\n",
  "^TestConsentGate2_APythonEditKeepsNothingInTheSessionLog$")
m("CONSENT-GATE2 pylite states kept, at the hook", pylite, pynot, pykept,
  "^TestConsentGate2_APythonEditKeepsNothingInTheSessionLog$")
m("CONSENT-GATE2 gopg states not kept", gopg,
  "func (Resolver) SessionKept() bool { return true }\n",
  "func (Resolver) SessionKept() bool { return !true }\n",
  "^TestConsentGate2_APythonEditKeepsNothingInTheSessionLog$")

# TestLANGREG1_APythonEditKeepsNoOverlayGroup. Break: state pylite kept,
# and the log holds the file's group.
m("CONSENT-GATE2 pylite states kept, end to end", pylite, pynot, pykept,
  "^TestLANGREG1_APythonEditKeepsNoOverlayGroup$")

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
