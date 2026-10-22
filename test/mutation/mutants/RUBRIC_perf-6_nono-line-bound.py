# perf-6: the nono trail's line bound is a named constant with its arithmetic,
# pinned at the bound and one byte over it (rubric fix list entry 68). A line
# over it is skipped and counted, and the trail after it is read.
m("perf-6  the trail line bound is halved", "internal/nono/nono.go",
  "\tnonoMaxLine       = 8 << 20\n",
  "\tnonoMaxLine       = 4 << 20\n", "TestRubricPERF6_")
m("perf-6  the trail scanner reads one byte past nonoMaxLine", "internal/nono/nono.go",
  "\tsc := linescan.New(f, initialScanBuffer, nonoMaxLine)\n",
  "\tsc := linescan.New(f, initialScanBuffer, nonoMaxLine+1)\n", "TestRubricPERF6_")
m("perf-6  a trail line over the bound is not counted", "internal/nono/nono.go",
  "\t\t\t// one; the trail after it is still the trail.\n\t\t\tobs.Skipped++\n",
  "\t\t\t// one; the trail after it is still the trail.\n", "TestRubricPERF6_")
m("perf-6  a trail line over the bound ends the read", "internal/nono/nono.go",
  "\t\t\tobs.Skipped++\n\t\t\tcontinue\n\t\t}\n\t\tline := strings.TrimSpace(string(sc.Bytes()))",
  "\t\t\tobs.Skipped++\n\t\t\tbreak\n\t\t}\n\t\tline := strings.TrimSpace(string(sc.Bytes()))", "TestRubricPERF6_")
