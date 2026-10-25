# test-1 / errors-7: fuzz properties over the shell lexer and the tool_input
# readers -- Derive, Hosts, Label and LeadingDirectory (rubric fix list
# entry 23). The seed corpus runs as TestRubricTEST1_ShapeSeeds.
m("test-1  wire hosts come back in reverse order", "internal/shape/hosts.go",
  "\tfor h := range seen {\n\t\tout = append(out, h)\n\t}\n\tsort.Strings(out)\n\treturn out\n}\n\n// authority",
  "\tfor h := range seen {\n\t\tout = append(out, h)\n\t}\n\tsort.Sort(sort.Reverse(sort.StringSlice(out)))\n\treturn out\n}\n\n// authority",
  "TestRubricTEST1_")
m("test-1  a wire host is kept as written, not canonical", "internal/shape/hosts.go",
  "\t\t\tif h, ok := host.Canonical(authority(lower[at:])); ok && host.Shaped(h) {\n\t\t\t\tseen[h] = struct{}{}",
  "\t\t\tif h, ok := host.Canonical(authority(lower[at:])); ok && host.Shaped(h) {\n\t\t\t\tseen[authority(lower[at:])] = struct{}{}",
  "TestRubricTEST1_")
m("test-1  an unreadable path gets a word outside Labels()", "internal/shape/label.go",
  "\tif !ok || p == \"\" {\n\t\treturn LabelUnknown\n\t}",
  "\tif !ok || p == \"\" {\n\t\treturn \"unreadable\"\n\t}", "TestRubricTEST1_")
m("test-1  a leading cd into a named home is folded as written", "internal/shape/leading.go",
  "\t\tstrings.HasPrefix(dir.text, \"~\") && dir.text != \"~\" && !strings.HasPrefix(dir.text, \"~/\") ||\n",
  "", "TestRubricTEST1_")
m("test-1  argc counts one fewer word than the line has", "internal/shape/shape.go",
  "\t\tn := len(dropLeadingAssignments(toks))\n",
  "\t\tn := len(dropLeadingAssignments(toks)) - 1\n", "TestRubricTEST1_")
m("test-1  the shape digest is written in upper case", "internal/shape/shape.go",
  "\t\tbody = body[n:]\n\t}\n\treturn hex.EncodeToString(m.Sum(nil))\n",
  "\t\tbody = body[n:]\n\t}\n\tout := []byte(hex.EncodeToString(m.Sum(nil)))\n\tfor i, c := range out {\n\t\tif c >= 'a' {\n\t\t\tout[i] = c - 'a' + 'A'\n\t\t}\n\t}\n\treturn string(out)\n", "TestRubricTEST1_")
# Round 3: the FuzzHosts property holds the hostname whitelist to the hosts a
# command argument names, which is where hosts.go applies it; the seed
# `ssh h.example.com!x` is what a reader without the whitelist records.
m("test-1  a command argument host skips the hostname whitelist", "internal/shape/hosts.go",
  "\tif !host.Shaped(hostPart) {\n\t\treturn \"\", false\n\t}\n",
  "\tif false {\n\t\treturn \"\", false\n\t}\n", "TestRubricTEST1_")
