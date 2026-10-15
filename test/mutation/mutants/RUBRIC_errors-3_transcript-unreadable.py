# Rubric errors-3: a transcript that exists and cannot be read used to skip
# the H-10 accounting check in silence, and the run rendered verified. It now
# adds transcript_unreadable. A transcript that does not exist adds no
# reason, by a written decision (README, accounting's comment).
m("errors-3 an unreadable transcript is not marked", "internal/report/report.go",
  "\t\tt.Status = TranscriptMissing\n\t\tif !missing {\n\t\t\tt.Status = TranscriptUnreadable\n\t\t}\n",
  "\t\tt.Status = TranscriptMissing\n\t\tif false {\n\t\t\tt.Status = TranscriptUnreadable\n\t\t}\n",
  "TestRubricERRORS3_")
m("errors-3 a missing transcript is marked unreadable too", "internal/report/report.go",
  "\t\tt.Status = TranscriptMissing\n\t\tif !missing {\n\t\t\tt.Status = TranscriptUnreadable\n\t\t}\n",
  "\t\tt.Status = TranscriptUnreadable\n",
  "TestRubricERRORS3_")
m("errors-3 the mark never becomes a coverage reason", "internal/report/report.go",
  "\t\tif t.Status == TranscriptUnreadable {\n\t\t\tsess.Coverage.add(ReasonTranscriptUnreadable)\n\t\t}\n",
  "",
  "TestRubricERRORS3_")
m("errors-3 a subagent's own absence outranks the main transcript's Stat", "internal/report/report.go",
  "\t\t\tif _, statErr := os.Stat(path); statErr == nil {\n",
  "\t\t\tif _, statErr := os.Stat(path); statErr != nil {\n",
  "TestRubricERRORS3_")
