# naming-10: every gap is built by NewGap, and AppendGap stamps the type and
# version itself, so no site can write a gap a reader skips.
m("naming-10 AppendGap writes a gap as its caller built it", "internal/store/store.go",
  "\treturn s.appendLine(filepath.Join(s.root, FileGaps), stampGap(rec), lockBudget, false)\n",
  "\treturn s.appendLine(filepath.Join(s.root, FileGaps), rec, lockBudget, false)\n",
  "TestRubricNAMING10_AppendGapStampsTypeAndVersion")
m("naming-10 NewGap leaves the type and version unset", "internal/store/store.go",
  "\treturn stampGap(Gap{RecordedAtMS: now.UnixMilli(), SessionID: sessionID, Reason: reason})\n",
  "\treturn Gap{RecordedAtMS: now.UnixMilli(), SessionID: sessionID, Reason: reason}\n",
  "TestRubricNAMING10_NewGapIsAWholeGap")
