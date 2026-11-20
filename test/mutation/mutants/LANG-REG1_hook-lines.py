# LANG-REG1 (PR 325), the hook's lines for an edit no reader indexes: a
# kind no reader claims is said once per session and kind, up to sixteen
# kinds, then once in a line naming no kind; an unindexed language, an
# unread configuration and a reader with no overlay (the index's facts
# stand) are each said once per session and language or file, under a
# digest marker of their own family, as a system message for the person
# alone, and only inside the overlay writer's gate; a line that does not
# print gives its marker back. With no test registry set the registry is
# the wired one: the seam, whose row is below. One row per Break clause of
# each test, killed by that test alone. Written, not run: for the end sweep.
kindline = "internal/hook/kindline.go"
overlay = "internal/hook/overlaywrite.go"
kindnotice = "internal/store/kindnotice.go"
notice = "internal/store/notice.go"
record = "internal/store/record.go"

openmarker = "\tf, err := os.OpenFile(filepath.Join(s.RunDir(sessionID), name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, fileMode)\n"
storewide = "\tf, err := os.OpenFile(filepath.Join(s.root, name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, fileMode)\n"
isnotice = "\treturn IsKindNoticeFile(name) || IsUnindexedNoticeFile(name) || IsUnreadNoticeFile(name) || IsLastIndexNoticeFile(name)\n"
rawkey = "\tif key := lang.KindKey(rel); printableKind(key) {\n"
rawkeybroken = "\tif key := lang.KindKey(rel); true {\n"
silence = "\tif s.NoticeTaken(sessionID, name) {\n"
silenced = "\tif s.NoticeTaken(sessionID, name) || s.NoticeTaken(sessionID, FileNoticeOtherKinds) {\n"
counted = "\t\tif IsKindNoticeFile(e.Name()) {\n"
countedbroken = "\t\tif IsKindNoticeFile(e.Name()) || e.Name() == FileNoticeOtherKinds {\n"
limit = "const kindNoticeLimit = 16\n"
nolimit = "const kindNoticeLimit = 1 << 20\n"
takekind = "\t\tmarker = p.st.TakeKindNotice(p.sessionID, key)\n"
unindexedmark = "\tmarker = store.UnindexedNoticeName(g.unindexed)\n"
unindexedbyfile = "\tmarker = store.UnindexedNoticeName(g.rel)\n"
stops = "\t\t\tc.gate.ok, c.gate.unindexed = false, l\n\t\t}\n\t\treturn graph.OverlayGroup{}, \"\", \"\", false\n\t}\n"
goeson = "\t\t\tc.gate.ok, c.gate.unindexed = false, l\n\t\t}\n\t}\n"
unreadwords = "\treturn unreadLine(g.unread.File, reasonWords(g.unread.Reason), kindWhat(g.rel, g.unreadLang)), marker\n"
unreaderror = "\treturn unreadLine(g.unread.File, g.unread.Error(), kindWhat(g.rel, g.unreadLang)), marker\n"
lastrev = "\t\t\tc.gate.lastIndex, c.gate.lastRev = r.Lang(), hdr.Rev.Commit\n"
lastmark = "\tmarker = store.LastIndexNoticeName(g.lastIndex)\n"

# TestLANGREG1_APythonEditWritesAnOverlayGroup. Break: carry the Go
# resolver's set for every language, and the group's set is Go's.
goset = "LANG-REG1 every group carries the Go reader's set"
IMPORTS[goset] = (overlay,
                  "\t\"github.com/altrace-dev-role/rashomon/internal/graph/lang\"\n",
                  "\t\"github.com/altrace-dev-role/rashomon/internal/graph/lang\"\n"
                  "\t\"github.com/altrace-dev-role/rashomon/internal/graph/lang/gopg\"\n")
m(goset, overlay,
  "\t\tCarried: r.OverlayCarried(), Recs: recs}\n",
  "\t\tCarried: gopg.Resolver{}.OverlayCarried(), Recs: recs}\n",
  "^TestLANGREG1_APythonEditWritesAnOverlayGroup$")

# TestLANGREG1_TheLineNamingNoKindIsNotOneOfTheSixteen. Break: count
# notice-other-kinds among the kind markers (the sixteenth named kind is
# silenced).
m("LANG-REG1 the other-kinds marker counts among the sixteen, the post", kindnotice,
  counted, countedbroken,
  "^TestLANGREG1_TheLineNamingNoKindIsNotOneOfTheSixteen$")

# TestLANGREG1_AnUnprintableKindTakesTheLineNamingNoKind. Break: print the
# key raw, or let the line naming no kind silence the named kinds after it.
m("LANG-REG1 an unprintable kind is said raw", kindline,
  "\tif printableKind(key) {\n", "\tif true {\n",
  "^TestLANGREG1_AnUnprintableKindTakesTheLineNamingNoKind$")
m("LANG-REG1 the other-kinds marker silences the named kinds after it", kindnotice,
  silence, silenced,
  "^TestLANGREG1_AnUnprintableKindTakesTheLineNamingNoKind$")

# TestLANGREG1_TheKindLineFollowsTheOverlayGate. Break: print the line
# outside the writer's gate.
m("LANG-REG1 the kind line prints with no session-content marker", overlay,
  " || session == store.UnattributedSession || !store.SessionContentMarker(st.Root()) {\n",
  " || session == store.UnattributedSession {\n",
  "^TestLANGREG1_TheKindLineFollowsTheOverlayGate$")

# TestLANGREG1_EachSessionHearsTheKindLine. Break: keep the markers
# store-wide.
m("LANG-REG1 the notice markers are store-wide, the kind line", notice,
  openmarker, storewide,
  "^TestLANGREG1_EachSessionHearsTheKindLine$")

# TestLANGREG1_AKindLineThatDoesNotPrintIsGivenBack. Break: keep the marker
# on a failed marshal.
m("LANG-REG1 a kind line that does not marshal keeps its marker", kindline,
  "\tif err != nil || b == nil {\n\t\tp.returnKindLine(marker)\n",
  "\tif err != nil || b == nil {\n",
  "^TestLANGREG1_AKindLineThatDoesNotPrintIsGivenBack$")

# TestLANGREG1_TheKindLineNamesTheRegistrysReaders. Break: name "go, py" as
# a literal (the test's registry holds a third reader).
m("LANG-REG1 the kind line names go, py as a literal", kindline,
  "\t\tstrings.Join(reg.Langs(), \", \") + \").\"\n",
  "\t\tstrings.Join([]string{\"go\", \"py\"}, \", \") + \").\"\n",
  "^TestLANGREG1_TheKindLineNamesTheRegistrysReaders$")

# TestLANGREG1_AnUnclaimedKindIsSaidOncePerKindUpToSixteen. Break: take no
# marker (every edit repeats), or count no limit (the seventeenth names
# itself).
m("LANG-REG1 the kind line takes no marker", kindline,
  takekind, "\t\tmarker = store.KindNoticeName(key)\n",
  "^TestLANGREG1_AnUnclaimedKindIsSaidOncePerKindUpToSixteen$")
m("LANG-REG1 the kind markers have no limit, the post", kindnotice,
  limit, nolimit,
  "^TestLANGREG1_AnUnclaimedKindIsSaidOncePerKindUpToSixteen$")

# TestLANGREG1_RacingKindsNeverPassTheLimit. Break: drop the count after
# the create, or the removal of a marker past the limit.
m("LANG-REG1 a kind take does not count again after its create", kindnotice,
  "\tif n, ok = s.kindNotices(sessionID); !ok || n > kindNoticeLimit {\n",
  "\tif false {\n",
  "^TestLANGREG1_RacingKindsNeverPassTheLimit$")
m("LANG-REG1 a kind take past the limit keeps its marker", kindnotice,
  "\t\ts.ReturnNotice(sessionID, name)\n\t\tif !ok {\n",
  "\t\tif !ok {\n",
  "^TestLANGREG1_RacingKindsNeverPassTheLimit$")

# TestLANGREG1_AKindNoticeIsOncePerKindUpToSixteen. Break: take without
# O_EXCL, drop the limit, or count the other-kinds marker among the kinds.
m("LANG-REG1 a notice marker is taken without O_EXCL", notice,
  openmarker,
  "\tf, err := os.OpenFile(filepath.Join(s.RunDir(sessionID), name), os.O_CREATE|os.O_WRONLY, fileMode)\n",
  "^TestLANGREG1_AKindNoticeIsOncePerKindUpToSixteen$")
m("LANG-REG1 the kind markers have no limit, the store", kindnotice,
  limit, nolimit,
  "^TestLANGREG1_AKindNoticeIsOncePerKindUpToSixteen$")
m("LANG-REG1 the other-kinds marker counts among the sixteen, the store", kindnotice,
  counted, countedbroken,
  "^TestLANGREG1_AKindNoticeIsOncePerKindUpToSixteen$")

# TestLANGREG1_AReturnedKindNoticeIsTakenAgain. Break: leave either name
# out of isNotice, so ReturnNotice removes nothing, or let the other-kinds
# marker silence every later kind.
m("LANG-REG1 isNotice leaves out the kind markers", notice,
  isnotice,
  "\treturn IsUnindexedNoticeFile(name) || IsUnreadNoticeFile(name) || IsLastIndexNoticeFile(name)\n",
  "^TestLANGREG1_AReturnedKindNoticeIsTakenAgain$")
m("LANG-REG1 isNotice leaves out the other-kinds marker", notice,
  "\tcase FileNoticeChannel, FileNoticeHoldout, FileNoticeCoverage, FileNoticeCoverageClear, FileNoticeIndexCap,\n"
  "\t\tFileNoticeOtherKinds:\n",
  "\tcase FileNoticeChannel, FileNoticeHoldout, FileNoticeCoverage, FileNoticeCoverageClear, FileNoticeIndexCap:\n",
  "^TestLANGREG1_AReturnedKindNoticeIsTakenAgain$")
m("LANG-REG1 the other-kinds marker silences every later kind", kindnotice,
  silence, silenced,
  "^TestLANGREG1_AReturnedKindNoticeIsTakenAgain$")

# TestLANGREG1_AKindMarkerNameIsADigestOfTheKey. Break: put the key in the
# name, change the digest, or let the other-kinds name match the family.
m("LANG-REG1 a kind marker's name holds the key", kindnotice,
  "func KindNoticeName(key string) string { return digestName(FileNoticeKindPrefix, key) }\n",
  "func KindNoticeName(key string) string { return FileNoticeKindPrefix + key }\n",
  "^TestLANGREG1_AKindMarkerNameIsADigestOfTheKey$")
m("LANG-REG1 the marker digest takes the sum's last bytes", kindnotice,
  "\treturn prefix + hex.EncodeToString(sum[:kindDigestLen/2])\n",
  "\treturn prefix + hex.EncodeToString(sum[len(sum)-kindDigestLen/2:])\n",
  "^TestLANGREG1_AKindMarkerNameIsADigestOfTheKey$")
m("LANG-REG1 the other-kinds name matches the kind family", record,
  "\tFileNoticeOtherKinds = \"notice-other-kinds\"\n",
  "\tFileNoticeOtherKinds = \"notice-kind-0123456789abcdef\"\n",
  "^TestLANGREG1_AKindMarkerNameIsADigestOfTheKey$")

# TestLANGREG1_AnUnclaimedKindSaysSoOnce. Break: key the line by the file
# (the second .ts file prints it again), or print it once per session (the
# .sh file prints none).
m("LANG-REG1 the kind line is keyed by the file", kindline,
  takekind, "\t\tmarker = p.st.TakeKindNotice(p.sessionID, g.rel)\n",
  "^TestLANGREG1_AnUnclaimedKindSaysSoOnce$")
m("LANG-REG1 the kind line is said once per session", kindline,
  takekind, "\t\tmarker = p.st.TakeKindNotice(p.sessionID, \"\")\n",
  "^TestLANGREG1_AnUnclaimedKindSaysSoOnce$")

# TestLANGREG1_TheKindLineIsForThePersonAlone. Break: say the line as
# additionalContext.
m("LANG-REG1 the kind line is said as additionalContext", kindline,
  "\tb, err := steer.Output{SystemMessage: line}.MarshalFor(p.event, c)\n",
  "\tb, err := steer.Output{AdditionalContext: line}.MarshalFor(p.event, c)\n",
  "^TestLANGREG1_TheKindLineIsForThePersonAlone$")

# TestUnindexedLanguage_IsSaidOncePerSessionAndLanguage. Break: key the
# marker by the file (the second file repeats), keep it store-wide (the
# second session is silent), or go on to the reader's configuration and
# overlay (a group is kept: the reader's record of the file alone makes one).
m("LANG-REG1 the unindexed marker is keyed by the file", kindline,
  unindexedmark, unindexedbyfile,
  "^TestUnindexedLanguage_IsSaidOncePerSessionAndLanguage$")
m("LANG-REG1 the notice markers are store-wide, the unindexed line", notice,
  openmarker, storewide,
  "^TestUnindexedLanguage_IsSaidOncePerSessionAndLanguage$")
m("LANG-REG1 an unindexed edit goes on to its reader's overlay", overlay,
  stops, goeson,
  "^TestUnindexedLanguage_IsSaidOncePerSessionAndLanguage$")

# TestUnindexedLanguage_AnUnprintableKindNamesTheLanguage. Break: print the
# key raw.
m("LANG-REG1 the unindexed line prints an unprintable key raw", kindline,
  rawkey, rawkeybroken,
  "^TestUnindexedLanguage_AnUnprintableKindNamesTheLanguage$")

# TestUnindexedNotice_IsADigestOfTheLanguageApartFromTheKinds. Break: share
# the kind prefix (counted among the kinds), leave the family out of the
# notice markers (never taken), or put the language in the name.
m("LANG-REG1 the unindexed markers share the kind prefix", kindnotice,
  "const FileNoticeUnindexedPrefix = \"notice-unindexed-\"\n",
  "const FileNoticeUnindexedPrefix = \"notice-kind-\"\n",
  "^TestUnindexedNotice_IsADigestOfTheLanguageApartFromTheKinds$")
m("LANG-REG1 isNotice leaves out the unindexed markers", notice,
  isnotice,
  "\treturn IsKindNoticeFile(name) || IsUnreadNoticeFile(name) || IsLastIndexNoticeFile(name)\n",
  "^TestUnindexedNotice_IsADigestOfTheLanguageApartFromTheKinds$")
m("LANG-REG1 an unindexed marker's name holds the language", kindnotice,
  "func UnindexedNoticeName(lang string) string { return digestName(FileNoticeUnindexedPrefix, lang) }\n",
  "func UnindexedNoticeName(lang string) string { return FileNoticeUnindexedPrefix + lang }\n",
  "^TestUnindexedNotice_IsADigestOfTheLanguageApartFromTheKinds$")

# TestLANGREG1_AnUnindexedLanguageSaysSoOnce, three Breaks of four (the
# registry rows file holds the missing configuration's): key the line by
# the file (the second .py file prints it again), say the line and go on to
# build the overlay (the log holds the file's group), or leave the edit's
# gate open (the post counts a unit-brief miss).
m("LANG-REG1 the unindexed marker is keyed by the file, end to end", kindline,
  unindexedmark, unindexedbyfile,
  "^TestLANGREG1_AnUnindexedLanguageSaysSoOnce$")
m("LANG-REG1 an unindexed edit goes on to its reader's overlay, end to end", overlay,
  stops, goeson,
  "^TestLANGREG1_AnUnindexedLanguageSaysSoOnce$")
m("LANG-REG1 an unindexed edit leaves the gate open", overlay,
  "\t\t\tc.gate.ok, c.gate.unindexed = false, l\n",
  "\t\t\tc.gate.unindexed = l\n",
  "^TestLANGREG1_AnUnindexedLanguageSaysSoOnce$")

# TestUnreadConfig_IsSaidOncePerSessionAndFile. Break: key the marker by
# the edited file (the second file repeats), keep it store-wide (the second
# session is silent), or say the step's error (the line names the path).
m("LANG-REG1 the unread marker is keyed by the edited file", kindline,
  "\tmarker = store.UnreadNoticeName(g.unread.File)\n",
  "\tmarker = store.UnreadNoticeName(g.rel)\n",
  "^TestUnreadConfig_IsSaidOncePerSessionAndFile$")
m("LANG-REG1 the notice markers are store-wide, the unread line", notice,
  openmarker, storewide,
  "^TestUnreadConfig_IsSaidOncePerSessionAndFile$")
m("LANG-REG1 the unread line says the step's error", kindline,
  unreadwords, unreaderror,
  "^TestUnreadConfig_IsSaidOncePerSessionAndFile$")

# TestUnreadConfig_EachReasonSaysItsOwnWords. Break: one phrase for two
# reasons, say the error's text, or print the key raw.
m("LANG-REG1 not JSON reads as JSON of the wrong shape", kindline,
  "\tcase lang.NotJSON:\n\t\treturn \"not JSON\"\n",
  "\tcase lang.NotJSON:\n\t\treturn \"JSON of the wrong shape\"\n",
  "^TestUnreadConfig_EachReasonSaysItsOwnWords$")
m("LANG-REG1 the unread line says the error's text", kindline,
  unreadwords, unreaderror,
  "^TestUnreadConfig_EachReasonSaysItsOwnWords$")
m("LANG-REG1 the unread line prints an unprintable key raw", kindline,
  rawkey, rawkeybroken,
  "^TestUnreadConfig_EachReasonSaysItsOwnWords$")

# TestUnreadNotice_IsADigestOfTheFileApartFromTheOthers. Break: share
# another family's prefix, leave the family out of the notice markers
# (never taken), or put the file in the name.
m("LANG-REG1 the unread markers share the unindexed prefix", kindnotice,
  "const FileNoticeUnreadPrefix = \"notice-unread-\"\n",
  "const FileNoticeUnreadPrefix = \"notice-unindexed-\"\n",
  "^TestUnreadNotice_IsADigestOfTheFileApartFromTheOthers$")
m("LANG-REG1 isNotice leaves out the unread markers", notice,
  isnotice,
  "\treturn IsKindNoticeFile(name) || IsUnindexedNoticeFile(name) || IsLastIndexNoticeFile(name)\n",
  "^TestUnreadNotice_IsADigestOfTheFileApartFromTheOthers$")
m("LANG-REG1 an unread marker's name holds the file", kindnotice,
  "func UnreadNoticeName(file string) string { return digestName(FileNoticeUnreadPrefix, file) }\n",
  "func UnreadNoticeName(file string) string { return FileNoticeUnreadPrefix + file }\n",
  "^TestUnreadNotice_IsADigestOfTheFileApartFromTheOthers$")

# TestLANGREG1_EachRowOfTheKindsTableIsSilent, one Break of two (the
# registry rows file holds the other): say the unclaimed line before asking
# the table.
m("LANG-REG1 a kinds-table row says the unclaimed line", overlay,
  "\t\tif lang.NonSource(rel) {\n\t\t\treturn \"\", keptGate{}\n\t\t}\n", "",
  "^TestLANGREG1_EachRowOfTheKindsTableIsSilent$")

# The seam: with no test registry set the registry is the wired one. No
# test is written for the seam alone; this one reaches it, since its edits
# go through the overlay writer with no test registry set, and its web/a.ts
# line must name the wired readers (go, py). An empty registry claims no
# file and names no reader, so the line differs.
seam = "LANG-REG1 the overlay writer's registry is empty when no test sets one"
IMPORTS[seam] = (overlay, "\t\"github.com/altrace-dev-role/rashomon/internal/graph/index\"\n", "")
m(seam, overlay,
  "\treturn index.Wire()\n", "\treturn &lang.Registry{}\n",
  "^TestLANGREG1_EachRowOfTheKindsTableIsSilent$")

# TestUnreadConfig_AGoEditIsByteEqualWhilePythonsConfigIsUnread, one Break
# of two (the registry rows file holds the other): keep another reader's
# refusal on the gate (the Go edit says the Python file was not read).
m("LANG-REG1 a Go edit keeps the Python reader's refusal on its gate", overlay,
  "\tr, _, cerr := g.reg.Claim(at, under)\n",
  "\tif _, _, e := g.reg.Claim(at, \"x.py\"); e != nil {\n\t\tc.keepUnread(g, at, \"x.py\", e)\n\t}\n"
  "\tr, _, cerr := g.reg.Claim(at, under)\n",
  "^TestUnreadConfig_AGoEditIsByteEqualWhilePythonsConfigIsUnread$")

# TestOverlayNilSaysLastIndex. Break: stay silent where Overlay returns
# nothing (the first edit says nothing), key the marker by the file (the
# second file repeats), keep it store-wide (the second session is silent),
# or name a commit other than the header's.
m("LANG-REG1 a reader with no overlay is silent", kindline,
  "\tcase g.lastIndex != \"\":\n\t\treturn p.takeLastIndexLine(g)\n", "",
  "^TestOverlayNilSaysLastIndex$")
m("LANG-REG1 the last-index marker is keyed by the file", kindline,
  lastmark, "\tmarker = store.LastIndexNoticeName(g.rel)\n",
  "^TestOverlayNilSaysLastIndex$")
m("LANG-REG1 the notice markers are store-wide, the last-index line", notice,
  openmarker, storewide,
  "^TestOverlayNilSaysLastIndex$")
m("LANG-REG1 the last-index line names HEAD for the header's commit", overlay,
  lastrev, "\t\t\tc.gate.lastIndex, c.gate.lastRev = r.Lang(), \"HEAD\"\n",
  "^TestOverlayNilSaysLastIndex$")

# TestOverlayNilKeepsTheIndexFacts. Break: clear the gate where the reader
# has no overlay (no landed edit), keep a group, or call the reader's
# Overlay first.
m("LANG-REG1 a reader with no overlay clears the gate", overlay,
  lastrev, "\t\t\tc.gate.ok, c.gate.lastIndex, c.gate.lastRev = false, r.Lang(), hdr.Rev.Commit\n",
  "^TestOverlayNilKeepsTheIndexFacts$")
m("LANG-REG1 a reader with no overlay keeps a group", overlay,
  "\t\treturn graph.OverlayGroup{}, \"\", \"\", false\n\t}\n"
  "\trecs, err := resolveOverlay(r, under, content, graph.RevCommit(hdr.Rev.Commit), c.seq)\n"
  "\tif err != nil || len(recs) == 0 {\n",
  "\t}\n\tvar recs []graph.Record\n\tif len(r.OverlayCarried()) != 0 {\n"
  "\t\trecs, err = resolveOverlay(r, under, content, graph.RevCommit(hdr.Rev.Commit), c.seq)\n"
  "\t}\n\tif err != nil {\n",
  "^TestOverlayNilKeepsTheIndexFacts$")
m("LANG-REG1 a reader with no overlay has its Overlay called first", overlay,
  "\tif len(r.OverlayCarried()) == 0 {\n",
  "\tif _, _ = resolveOverlay(r, under, content, graph.RevCommit(hdr.Rev.Commit), c.seq); len(r.OverlayCarried()) == 0 {\n",
  "^TestOverlayNilKeepsTheIndexFacts$")

# TestOverlayNil_AnUnprintableKindNamesTheLanguage. Break: print the key
# raw, or take the unindexed family's marker.
m("LANG-REG1 the last-index line prints an unprintable key raw", kindline,
  rawkey, rawkeybroken,
  "^TestOverlayNil_AnUnprintableKindNamesTheLanguage$")
m("LANG-REG1 the last-index line takes the unindexed family's marker", kindline,
  lastmark, "\tmarker = store.UnindexedNoticeName(g.lastIndex)\n",
  "^TestOverlayNil_AnUnprintableKindNamesTheLanguage$")

# TestLastIndexNotice_IsADigestOfTheLanguageApartFromTheOthers. Break:
# share the unindexed prefix (one line silences the other), leave the
# family out of the notice markers (never taken), or put the language in
# the name.
m("LANG-REG1 the last-index markers share the unindexed prefix", kindnotice,
  "const FileNoticeLastIndexPrefix = \"notice-lastindex-\"\n",
  "const FileNoticeLastIndexPrefix = \"notice-unindexed-\"\n",
  "^TestLastIndexNotice_IsADigestOfTheLanguageApartFromTheOthers$")
m("LANG-REG1 isNotice leaves out the last-index markers", notice,
  isnotice,
  "\treturn IsKindNoticeFile(name) || IsUnindexedNoticeFile(name) || IsUnreadNoticeFile(name)\n",
  "^TestLastIndexNotice_IsADigestOfTheLanguageApartFromTheOthers$")
m("LANG-REG1 a last-index marker's name holds the language", kindnotice,
  "func LastIndexNoticeName(lang string) string { return digestName(FileNoticeLastIndexPrefix, lang) }\n",
  "func LastIndexNoticeName(lang string) string { return FileNoticeLastIndexPrefix + lang }\n",
  "^TestLastIndexNotice_IsADigestOfTheLanguageApartFromTheOthers$")
