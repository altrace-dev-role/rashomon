# Rubric hld-4: report's transcript readers take an io.Reader and open
# nothing; every open is in transcript_files.go, and digest calls none of
# report's filesystem adapters.
m("RUBRIC hld-4 a transcript reader reaches for the filesystem again", "internal/report/transcript.go",
  "func FinalAssistantText(r io.Reader) (string, bool) {\n\tvar last string",
  "func FinalAssistantText(r io.Reader) (string, bool) {\n\tif f, ok := r.(*os.File); ok && f == nil {\n\t\treturn \"\", false\n\t}\n\tvar last string",
  "TestRubricHLD4_OnlyTheAdapterFilesReachTheFilesystem")
IMPORTS["RUBRIC hld-4 a transcript reader reaches for the filesystem again"] = (
  "internal/report/transcript.go", "\t\"io\"\n", "\t\"io\"\n\t\"os\"\n")
m("RUBRIC hld-4 the digest reads a transcript through report", "internal/digest/digest.go",
  "\td := build(run, gaps, promptID, lastAssistantMessage, now)\n",
  "\tif _, _, _, _, _, err := report.TranscriptIDs(\"\"); err == nil {\n\t\treturn nil, err\n\t}\n\td := build(run, gaps, promptID, lastAssistantMessage, now)\n",
  "TestRubricHLD4_TheDigestOpensNoFileOfItsOwn")
