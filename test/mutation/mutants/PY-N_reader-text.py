# PY-N, the Python reader keeps no source text: a constant is its name and
# its line, a signature names its parameters only, a call keeps its argument
# count and its line and no keyword text, no record comes from a string
# literal, the reader is pylite/4 so an edit under an index of pylite/2 says
# the unindexed line, rashomon graph index writes the index whole, and the
# inside of each string and comment is blanked before any line is read, a
# string in one quote that never closes ending at its line end. One row per
# pin, each killed by the test it names; each was run once with its killer
# alone, by go test -overlay.
pylite = "internal/graph/lang/pylite/pylite.go"
scan = "internal/graph/lang/pylite/scan.go"
blank = "internal/graph/lang/pylite/blank.go"
persist = "internal/graph/persist.go"

m("PY-N a constant keeps a value", pylite,
  '"symkind": "const", "line"', '"symkind": "const", "value": c.name, "line"',
  "TestPYN_TheReaderWritesOnlyTheseAttributeKeys")
m("PY-N a call keeps a keyword's text", pylite,
  '"argc": strconv.Itoa(c.argc), "line"', '"argc": strconv.Itoa(c.argc), "kw." + c.name: c.name, "line"',
  "TestPYN_TheReaderWritesOnlyTheseAttributeKeys")
m("PY-N a signature keeps defaults and annotations", scan,
  "shown = append(shown, paramName(p))", "shown = append(shown, p)",
  "TestPYN_ASignatureNamesItsParametersOnly")
m("PY-N the reader stays pylite/2", pylite,
  '\tprovVersion   = "4"\n', '\tprovVersion   = "2"\n',
  "TestPYN_AnIndexOfTheSecondPyliteSaysTheUnindexedLine")
m("PY-N the SQL loop is put back, a table read from a string", pylite,
  "\trecs = append(recs, r.extras(f, file, rev, defConf, algo)...)\n",
  "\trecs = append(recs, r.extras(f, file, rev, defConf, algo)...)\n"
  '\tfor i, ln := range strings.Split(string(content), "\\n") {\n'
  '\t\tif _, rest, ok := strings.Cut(ln, " FROM "); ok {\n'
  '\t\t\tname, _, _ := strings.Cut(rest, " ")\n'
  "\t\t\ttbl := graph.TableID(name)\n"
  "\t\t\trecs = append(recs,\n"
  '\t\t\t\tgraph.Record{Node: &graph.Node{ID: tbl, Kind: "table", Tier: graph.Algorithmic, Conf: 0.9, Rev: rev, Prov: algo,\n'
  '\t\t\t\t\tAttrs: map[string]string{"name": name}}},\n'
  "\t\t\t\tgraph.Record{Edge: &graph.Edge{Src: file, Pred: graph.Reads, Dst: tbl, Tier: graph.Algorithmic, Conf: 0.9, Rev: rev, Prov: algo,\n"
  '\t\t\t\t\tAttrs: map[string]string{"line": strconv.Itoa(i + 1), "cols": rest}}})\n'
  "\t\t}\n"
  "\t}\n",
  "TestPYN_TheIndexAndTheOverlayKeepNoSourceText")
m("PY-N graph index carries the older index", persist,
  "\treturn rootio.ReplaceFile(r, filepath.FromSlash(GraphFile), 0o600, MaxGraphFileBytes, func(w io.Writer) error { return Save(w, s, h) })\n",
  "\told, _ := r.ReadFile(filepath.FromSlash(GraphFile))\n"
  "\treturn rootio.ReplaceFile(r, filepath.FromSlash(GraphFile), 0o600, MaxGraphFileBytes, func(w io.Writer) error {\n"
  "\t\tif _, err := w.Write(old); err != nil {\n"
  "\t\t\treturn err\n"
  "\t\t}\n"
  "\t\treturn Save(w, s, h)\n"
  "\t})\n",
  "TestPYN_GraphIndexWritesTheIndexWhole")
m("PY-N lines are read with strings and comments as written", pylite,
  "\tsrc := blank(string(content))\n", "\tsrc := string(content)\n",
  "TestPYN_StringsAndCommentsChangeNothing")
m("PY-N a string in one quote that never closes runs on", blank,
  '\t\t\tif len(cl) == 1 {\n\t\t\t\treturn j\n\t\t\t}\n\t\t\tj++\n\t\tcase strings.HasPrefix(s[j:], cl):\n\t\t\treturn j\n\t\tcase f && strings.HasPrefix(s[j:], "{{"):\n',
  '\t\t\tif false {\n\t\t\t\treturn j\n\t\t\t}\n\t\t\tj++\n\t\tcase strings.HasPrefix(s[j:], cl):\n\t\t\treturn j\n\t\tcase f && strings.HasPrefix(s[j:], "{{"):\n',
  "TestPYN_AStringNeverClosedEndsAtItsLineOrAtTheEnd")
