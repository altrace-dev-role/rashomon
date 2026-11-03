# TOKEN-BUDGET1: the model is sent only the preflight lines that list
# something; the brief, the evidence and the unknowns are the person's aside.
# A sparse line whose lists name nothing is left out, a note left with its
# header alone has no model text, and one with no line at all is refused.
A = "internal/steer/render/aside.go"

m("TB-1 a line that lists nothing is sent", A,
  "\t\tif sparse && !(head && i == 0) && listsNothing(names, slots) {\n",
  "\t\tif sparse && !(head && i == 0) && listsNothing(names, slots) && false {\n", "TestTB1_AModelLine")
m("TB-1 the header alone is sent", A,
  "\tif sparse && head && len(kept) == 1 {\n", "\tif false {\n", "TestTB1_AModelLine")
m("TB-1 a line that lists is left out", A,
  "\t\t\tif l.Lists() {\n\t\t\t\treturn false\n", "\t\t\tif l.Lists() && false {\n\t\t\t\treturn false\n",
  "TestTB1_AModelLine")
m("TB-1 a line with no list is left out", A,
  "\treturn lists\n", "\treturn lists || true\n", "TestTB1_AModelLine")
m("TB-1 the aside goes to the model", A,
  "\treturn text, aside, nil\n", "\treturn text + \"\\n\" + aside, aside, nil\n", "TestTB1_TheModelText")
m("TB-1 a note with no line at all prints", A,
  "\tcase text == \"\" && aside == \"\":\n\t\treturn ErrNothing\n", "\tcase false:\n\t\treturn ErrNothing\n",
  "TestTB1_ANoteWithNoLine")
m("TB-1 the aside is outside the cap", A,
  "\tcase len(text)+len(aside) > MaxBriefingMessageBytes:\n", "\tcase len(text) > MaxBriefingMessageBytes:\n",
  "TestLRN17r_TheUnknownsLineGoesFirst")

# What lists: a named item, listed, left out by a check, digested or counted
# in the record form; never a count of items given earlier, withheld or
# learned.
L = "internal/steer/render/slot/lists.go"

m("TB-1 a path left out does not list", L,
  "len(s.paths)+len(s.digests)+s.more > 0", "len(s.paths)+len(s.digests) > 0", "TestTB1_ListsIsWhatALineNames")
m("TB-1 a digest does not list", L,
  "len(s.paths)+len(s.digests)+s.more > 0", "len(s.paths)+s.more > 0", "TestTB1_ListsIsWhatALineNames")
m("TB-1 rules given earlier or withheld list", L,
  "len(r.items)+r.more > 0", "len(r.items)+r.more+r.earlier+r.withheld > 0", "TestTB1_ListsIsWhatALineNames")
m("TB-1 learned gates list", L,
  "len(g.items)+g.more > 0", "len(g.items)+g.more+g.learned > 0", "TestTB1_ListsIsWhatALineNames")
m("TB-1 counted symbols do not list", L,
  "len(s.items)+s.more > 0", "len(s.items) > 0", "TestTB1_ListsIsWhatALineNames")
m("TB-1 an empty value list lists", L,
  "len(l.values) > 0", "true", "TestTB1_ListsIsWhatALineNames")
m("TB-1 a zero count lists", L,
  "c > 0", "true", "TestTB1_ListsIsWhatALineNames")

# The templates: the unknowns and the brief are the aside's.
R = "internal/steer/render/render.go"

m("TB-1 the unknowns line is sent", R,
  "\t\t\tformat: preflightHead + preflightKnown,\n\t\t\taside:  preflightEvidence + preflightUnknown,\n",
  "\t\t\tformat: preflightHead + preflightKnown + preflightUnknown,\n\t\t\taside:  preflightEvidence,\n",
  "TestTB1_ANoteWithNothing|TestTB1b_")
m("TB-1 the brief is sent", R,
  "\t\t\tformat: preflightHead + preflightKnown,\n\t\t\taside:  firstBrief + preflightEvidence + preflightUnknown,\n",
  "\t\t\tformat: preflightHead + firstBrief + preflightKnown,\n\t\t\taside:  preflightEvidence + preflightUnknown,\n",
  "TestPF1i_FirstPromptCarriesTheBrief")

# The router: a note with nothing for the model prints its aside alone as a
# flag; a note's or a flag's aside follows its text in the person's line.
D = "internal/steer/decide.go"

m("TB-1 a note with no model text stays a nudge", D,
  "\t\tif p.level == LevelNudge && f.Message.Text() == \"\" {\n", "\t\tif false {\n", "TestTB1_ANoteWithNothing")
m("TB-1 an aside alone is refused", D,
  "\t\tcase f.Message.Text() == \"\" && f.Message.Aside() == \"\" || slotErr != nil:\n",
  "\t\tcase f.Message.Text() == \"\" || slotErr != nil:\n", "TestTB1_ANoteWithNothing")
m("TB-1 the aside is not printed", D,
  "\tcase aside == \"\":\n\t\treturn user\n", "\tcase true:\n\t\treturn user\n", "TestTB1_ANoteWithNothing|TestPF1r_")

# The note: a new evidence kind is new to the person, so it still makes a
# note, one with no model text.
N = "internal/intent/note.go"

m("TB-1 a new evidence kind makes no note", N,
  "\tfor _, k := range in.Evidence {\n\t\tfresh = fresh || !d.Evidence[k]\n\t}\n", "",
  "TestTB1_ANewEvidenceKind|TestTB1_ANoteWithNothingProven")
