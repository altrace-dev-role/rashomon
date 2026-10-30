# MS-1 (card MEASURE1, PR A2): the measure pair reader.
m("MS-1 pair sums every transcript, not the session's", "internal/measure/transcript.go",
  "\t\tif f.Session == session {\n",
  "\t\tif f.Session != \"\" {\n", "TestMS1e_")
m("MS-1 pair resolves a finding on any green", "internal/measure/record.go",
  "g.LastGreenSeq != nil && *g.LastGreenSeq > f.LastAfterSeq {\n",
  "g.LastGreenSeq != nil {\n", "TestMS1e_")
m("MS-1 pair counts a withheld note as given", "internal/measure/record.go",
  "\t\tcase given:\n\t\t\ts.NotesGiven++\n",
  "\t\tcase given || withheld:\n\t\t\ts.NotesGiven++\n", "TestMS1e_")
m("MS-1 pair reads a session of two arms as its first", "internal/measure/record.go",
  "\t\tcase a != s.RecordedArm:\n",
  "\t\tcase false:\n", "TestMS1f_")
m("MS-1 pair passes a prompt over the router's note cap", "internal/measure/record.go",
  "perPrompt[prompt] > steer.NotesPerPrompt {\n",
  "perPrompt[prompt] > steer.NotesPerPrompt+1 {\n", "TestMS1f_")
m("MS-1 the n = 1 sentence dropped", "internal/measure/text.go",
  "\tout.WriteString(\"  \" + Hypothesis + \"\\n\")\n",
  "", "TestMS1g_")
m("MS-1 pair's line total counts only the given", "internal/measure/record.go",
  "\ts.Lines = len(lines)\n",
  "\ts.Lines = s.NotesGiven\n", "TestMS1e_")
