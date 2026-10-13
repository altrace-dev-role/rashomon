# test-1 / errors-7: fuzz properties over the shell lexer and the tool_input
# readers -- Derive, Hosts, Label and LeadingDirectory (rubric fix list
# entry 23). The seed corpus runs as TestRubricTest1_ShapeSeeds.
m("test-1  wire hosts come back in reverse order", "internal/shape/hosts.go",
  "\tfor h := range seen {\n\t\tout = append(out, h)\n\t}\n\tsort.Strings(out)\n\treturn out\n}\n\n// authority",
  "\tfor h := range seen {\n\t\tout = append(out, h)\n\t}\n\tsort.Sort(sort.Reverse(sort.StringSlice(out)))\n\treturn out\n}\n\n// authority",
  "TestRubricTest1_")
m("test-1  a wire host is kept as written, not canonical", "internal/shape/hosts.go",
  "\t\t\tif h, ok := host.Canonical(authority(lower[at:])); ok {\n\t\t\t\tseen[h] = struct{}{}",
  "\t\t\tif _, ok := host.Canonical(authority(lower[at:])); ok {\n\t\t\t\tseen[authority(lower[at:])] = struct{}{}",
  "TestRubricTest1_")
m("test-1  an unreadable path gets a word outside Labels()", "internal/shape/label.go",
  "\tif !ok || p == \"\" {\n\t\treturn LabelUnknown\n\t}",
  "\tif !ok || p == \"\" {\n\t\treturn \"unreadable\"\n\t}", "TestRubricTest1_")
m("test-1  a leading cd into ~ is folded as written", "internal/shape/shape.go",
  "strings.HasPrefix(dir.text, \"-\") || strings.HasPrefix(dir.text, \"~\") ||",
  "strings.HasPrefix(dir.text, \"-\") ||", "TestRubricTest1_")
m("test-1  argc counts one fewer word than the line has", "internal/shape/shape.go",
  "\t\tn := len(dropLeadingAssignments(toks))\n",
  "\t\tn := len(dropLeadingAssignments(toks)) - 1\n", "TestRubricTest1_")
m("test-1  the shape digest is written in upper case", "internal/shape/shape.go",
  "\tm.Write(body)\n\treturn hex.EncodeToString(m.Sum(nil))\n",
  "\tm.Write(body)\n\treturn strings.ToUpper(hex.EncodeToString(m.Sum(nil)))\n", "TestRubricTest1_")
