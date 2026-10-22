# test-1 / errors-7: fuzz properties over the transcript readers, seeded with
# a live Claude Code 2.1.280 transcript (rubric fix list entry 23). The seeds
# run as TestRubricTEST1_TranscriptSeeds.
m("test-1  a line that does not parse ends the id scan", "internal/report/transcript.go",
  "\t\tif json.Unmarshal(sc.Bytes(), &line) != nil || len(line.Message.Content) == 0 {\n\t\t\tcontinue\n\t\t}",
  "\t\tif json.Unmarshal(sc.Bytes(), &line) != nil || len(line.Message.Content) == 0 {\n\t\t\tbreak\n\t\t}",
  "TestRubricTEST1_")
m("test-1  the first assistant text is taken for the last", "internal/report/transcript.go",
  "\t\tif text, ok := assistantText(line.Message.Content); ok {\n\t\t\tlast = text\n\t\t}",
  "\t\tif text, ok := assistantText(line.Message.Content); ok && last == \"\" {\n\t\t\tlast = text\n\t\t}",
  "TestRubricTEST1_")
m("test-1  the sidechain walk stops at the first key", "internal/report/transcript.go",
  "\t\tif dec.Decode(&skipValue{}) != nil {\n\t\t\treturn side\n\t\t}\n\t}\n\treturn side",
  "\t\tif dec.Decode(&skipValue{}) != nil {\n\t\t\treturn side\n\t\t}\n\t\treturn side\n\t}\n\treturn side",
  "TestRubricTEST1_")
