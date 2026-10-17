# Rubric hld-4: report's transcript readers take an io.Reader and open
# nothing; no file of report reaches the filesystem, and digest imports
# neither report's read step nor the baseline, directly or through anything
# it links.
m("RUBRIC hld-4 a transcript reader reaches for the filesystem again", "internal/report/transcript.go",
  "func FinalAssistantText(r io.Reader) (string, bool) {\n\tvar last string",
  "func FinalAssistantText(r io.Reader) (string, bool) {\n\tif f, ok := r.(*os.File); ok && f == nil {\n\t\treturn \"\", false\n\t}\n\tvar last string",
  "TestRubricHLD4_NoReportFileReachesTheFilesystem")
IMPORTS["RUBRIC hld-4 a transcript reader reaches for the filesystem again"] = (
  "internal/report/transcript.go", "\t\"io\"\n", "\t\"io\"\n\t\"os\"\n")
m("RUBRIC hld-4 the digest links the report's read step", "internal/digest/digest.go",
  "\td := build(run, gaps, promptID, lastAssistantMessage, now)\n",
  "\t_ = gather.WithNonoTrail\n\td := build(run, gaps, promptID, lastAssistantMessage, now)\n",
  "TestRubricHLD4_TheDigestOpensNoFileOfItsOwn|TestRubricHLD4_TheVerdictPackagesLinkNoReadStep")
IMPORTS["RUBRIC hld-4 the digest links the report's read step"] = (
  "internal/digest/digest.go", "\t\"github.com/altrace-dev-role/rashomon/internal/report\"\n",
  "\t\"github.com/altrace-dev-role/rashomon/internal/report\"\n\t\"github.com/altrace-dev-role/rashomon/internal/report/gather\"\n")
m("RUBRIC hld-4 report reads the sandbox trail itself", "internal/report/assemble.go",
  "\tsess.Nono = buildNono(in.Nono, nonoConfigured, madeMCPCall(run), declaredHosts(run), forgotten)\n",
  "\tf, _ := os.Open(\"\")\n\tsess.Nono = buildNono(nono.Observe(f, \"\", SessionWindow(run)), nonoConfigured, madeMCPCall(run), declaredHosts(run), forgotten)\n",
  "TestRubricHLD4_NoReportFileReachesTheFilesystem")
IMPORTS["RUBRIC hld-4 report reads the sandbox trail itself"] = (
  "internal/report/assemble.go", "\t\"time\"\n", "\t\"os\"\n\t\"time\"\n")
m("RUBRIC hld-4 report writes the project baseline itself", "internal/report/destinations.go",
  "func AnsweredNovelty(established bool, known int, novel []string, err error) Novelty {\n",
  "func AnsweredNovelty(established bool, known int, novel []string, err error) Novelty {\n\t_ = baseline.Key\n",
  "TestRubricHLD4_NoReportFileReachesTheFilesystem|TestRubricHLD4_TheVerdictPackagesLinkNoReadStep")
IMPORTS["RUBRIC hld-4 report writes the project baseline itself"] = (
  "internal/report/destinations.go", "\t\"github.com/altrace-dev-role/rashomon/internal/store\"\n",
  "\t\"github.com/altrace-dev-role/rashomon/internal/baseline\"\n\t\"github.com/altrace-dev-role/rashomon/internal/store\"\n")
