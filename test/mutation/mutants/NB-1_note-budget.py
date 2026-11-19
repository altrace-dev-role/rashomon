# NB-1 (card NOTE-BUDGET): three notes in any window
# of windowSeqs record seqs of a prompt, with the repeat reserve; the router's
# scan and charge, and the checker that holds a log to them. Rows 1 to 4 are
# the card's; 5 and 6 are the seq order of parallel calls. Every mutation keeps each
# variable in use, so none can fail to build and read as undetected.
BU = "internal/steer/budget.go"
DC = "internal/steer/decide.go"
m("NB-1 the window's edge counted inside", BU,
  "\t\tif r.seq > s-windowSeqs {\n",
  "\t\tif r.seq >= s-windowSeqs {\n", "TestNoteBudget_Window")
m("NB-1 the repeat reserve dropped", DC,
  "\t\tcase repeat && n+reserve+1 >= maxNotes:\n",
  "\t\tcase repeat && false:\n", "TestNoteBudget_RepeatReserve")
m("NB-1 the scan keeps two of three", BU,
  "\tif i == len(c.window) {\n",
  "\tif i == len(c.window)-1 {\n", "TestNoteBudget_ScanKeepsTheLargestThree")
m("NB-1 the checker counts per prompt", "internal/measure/record.go",
  "\t\t\t\tif a > l.AfterSeq-steer.NoteWindowSeqs {\n",
  "\t\t\t\tif a > l.AfterSeq-steer.NoteWindowSeqs || true {\n", "TestCheck_Window")
m("NB-1 the window's upper bound put back", BU,
  "\t\tif r.seq > s-windowSeqs {\n",
  "\t\tif r.seq > s-windowSeqs && r.seq <= s {\n", "TestNoteBudget_ALaterSeqWrittenFirstCounts")
m("NB-1 the state kept as the last three written", BU,
  "\t\tif seq <= c.window[i-1].seq {\n",
  "\t\tif true {\n", "TestNoteBudget_ScanKeepsTheLargestThree")
