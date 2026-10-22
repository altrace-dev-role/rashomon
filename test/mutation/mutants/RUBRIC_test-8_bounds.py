# rubric test-8 (fix list 17): every input bound is tested at N and N+1.
# Moving a bound's value, or reading past it or short of it where it is
# applied, is the regression. (The payload cap is RES-4's; maxNest, the
# token bound and the leading-cd bound have rows of their own.)
m("RUBRIC test-8 the store reads a line one byte past maxLine", "internal/store/lines.go",
  "\t\t\tif len(line)+len(chunk) > maxLine {", "\t\t\tif len(line)+len(chunk) > maxLine+1 {", "TestRubricTEST8_")
m("RUBRIC test-8 the store's maxLine moves", "internal/store/lines.go",
  "const maxLine = 16 << 20\n", "const maxLine = 16<<20 - 1\n", "TestRubricTEST8_")
m("RUBRIC test-8 recap keeps one session fewer", "internal/recap/state.go",
  "\t\tif len(m) <= maxTrackedSessions {\n\t\t\treturn\n\t\t}\n\t\tif id", "\t\tif len(m) < maxTrackedSessions {\n\t\t\treturn\n\t\t}\n\t\tif id", "TestRubricTEST8_")
m("RUBRIC test-8 a count of exactly maxTokens is implausible", "internal/spend/scan.go",
  "\t\t\tif v < 0 || v > maxTokens {\n", "\t\t\tif v < 0 || v >= maxTokens {\n", "TestRubricTEST8_")
m("RUBRIC test-8 spend reads one byte short of TranscriptMaxLine", "internal/spend/scan.go",
  "\ts := linescan.New(fh, report.TranscriptInitialBuffer, report.TranscriptMaxLine)\n", "\ts := linescan.New(fh, report.TranscriptInitialBuffer, report.TranscriptMaxLine-1)\n", "TestRubricTEST8_")
m("RUBRIC test-8 a final message is cut one rune short", "internal/report/account.go",
  "\t\trunes = runes[:messageCap]\n", "\t\trunes = runes[:messageCap-1]\n", "TestRubricTEST8_")
m("RUBRIC test-8 the transcript's TranscriptMaxLine moves", "internal/report/transcript.go",
  "\tTranscriptMaxLine       = 64 << 20\n", "\tTranscriptMaxLine       = 64<<20 + 1\n", "TestRubricTEST8_")
m("RUBRIC test-8 the transcript reader takes a line past maxLine", "internal/report/transcript.go",
  "\tsc := linescan.New(f, TranscriptInitialBuffer, TranscriptMaxLine)\n\tfor sc.Scan() {\n\t\tvar line struct {\n\t\t\tMessage struct {\n\t\t\t\tContent json.RawMessage `json:\"content\"`\n",
  "\tsc := linescan.New(f, TranscriptInitialBuffer, TranscriptMaxLine+1)\n\tfor sc.Scan() {\n\t\tvar line struct {\n\t\t\tMessage struct {\n\t\t\t\tContent json.RawMessage `json:\"content\"`\n",
  "TestRubricTEST8_")
