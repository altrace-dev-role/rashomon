# A transcript group's Status field is the only thing in the JSON that
# tells a missing transcript apart from one that exists and could not be
# read; Readable is false for both. The text render carries the same word.
m("transcript status line is dropped from the text render", "internal/report/text.go",
  "\t\tfmt.Fprintf(b, \"    status: %s\\n\", t.Status)\n",
  "",
  "TestAccounting_AReadableTranscriptRendersEveryEmptyListAsNone|TestRubricERRORS3_")
