# RES-28: a run counts only when it did its entry's work. Each check, removed
# alone, lets an early return be measured as the entry's cost.
m("RES-28 a hook run that wrote no record counts", "test/perf/runs.go",
  "\t\tif a.Records < 1 {\n", "\t\tif a.Records < 0 {\n", "TestRES28l_")
m("RES-28 a probe on the entry-absent path counts", "test/perf/runs.go",
  "\tif c.Reason != nil {\n", "\tif c.Reason != nil && false {\n", "TestRES28l_")
m("RES-28 a silent recap counts", "test/perf/runs.go",
  '\t\tif err := json.Unmarshal(a.Stdout, &out); err != nil {\n\t\t\treturn fmt.Errorf("recap printed no line (%q): %v", a.Stdout, err)\n\t\t}\n',
  '\t\tif err := json.Unmarshal(a.Stdout, &out); err != nil {\n\t\t\treturn nil\n\t\t}\n', "TestRES28l_")
m("RES-28 an entry with no check counts", "test/perf/runs.go",
  '\treturn fmt.Errorf("no work-done check for entry %q", strings.Join(command, " "))\n', "\treturn nil\n", "TestRES28l_")
