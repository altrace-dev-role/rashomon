# Rubric errors-3: a transcript that exists and cannot be read used to skip
# the H-10 accounting check in silence, and the run rendered verified. It now
# adds transcript_unreadable. A transcript that does not exist adds no
# reason, by a written decision (README, accounting's comment).
m("errors-3 an unreadable transcript is not marked", "internal/report/report.go",
  "\t\tt.unreadable = !errors.Is(err, fs.ErrNotExist)\n",
  "\t\tt.unreadable = false && !errors.Is(err, fs.ErrNotExist)\n",
  "TestRubricERRORS3_")
m("errors-3 a missing transcript is marked unreadable too", "internal/report/report.go",
  "\t\tt.unreadable = !errors.Is(err, fs.ErrNotExist)\n",
  "\t\tt.unreadable = err != nil || errors.Is(err, fs.ErrNotExist)\n",
  "TestRubricERRORS3_")
m("errors-3 the mark never becomes a coverage reason", "internal/report/report.go",
  "\t\tif t.unreadable {\n\t\t\tsess.Coverage.add(ReasonTranscriptUnreadable)\n\t\t}\n",
  "",
  "TestRubricERRORS3_")
