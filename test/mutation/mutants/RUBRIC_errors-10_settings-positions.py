# errors-10: a settings.json error names the file, the line and column, and the
# hook event it is about (rubric fix list entry 48).
m("errors-10  every settings error is placed on line 1", "internal/settings/errors.go",
  "\tline = bytes.Count(before, []byte{'\\n'}) + 1",
  "\tline = 1", "TestRubricERRORS10_")
m("errors-10  a position inside hooks is not moved to the file", "internal/settings/errors.go",
  "\treturn p + base - 1\n",
  "\treturn p\n", "TestRubricERRORS10_")
m("errors-10  a rewritten value keeps the place it had in the file", "internal/settings/document.go",
  "func (m *member) set(raw json.RawMessage) { m.raw, m.at = raw, 0 }",
  "func (m *member) set(raw json.RawMessage) { m.raw = raw }", "TestRubricERRORS10_")
m("errors-10  syntax errors are placed by the decoder's last token", "internal/settings/errors.go",
  "\tif syntactic || closed {\n",
  "\tif false {\n", "TestRubricERRORS10_")
m("errors-10  the hook event error names no event", "internal/settings/document.go",
  "\t\treturn nil, fmt.Errorf(\"hook event %q is not an array\", event)",
  "\t\treturn nil, errors.New(\"hook event is not an array\")", "TestRubricERRORS10_")
m("errors-10  a named document forgets its path", "internal/settings/document.go",
  "\td := &Document{path: path, src: data}",
  "\td := &Document{src: data}", "TestRubricERRORS10_")
m("errors-10  watch edits a document that does not know its file", "cmd/rashomon/watch.go",
  "\tdoc, err := settings.ParseNamed(plan.loc.User, original)",
  "\tdoc, err := settings.Parse(original)", "TestRubricERRORS10_")
# watch refuses a broken file at its plan, before editSettings reads it again;
# detach's edit is the one a broken file reaches editSettings through.
m("errors-10  the settings edit parses a document that does not know its file", "cmd/rashomon/main.go",
  "\t\tdoc, err := settings.ParseNamed(path, original)",
  "\t\tdoc, err := settings.Parse(original)", "TestRubricERRORS10_")
m("errors-10  the layer resolver names the file twice", "internal/settings/locate.go",
  "\t\tif errors.As(err, &located) {",
  "\t\tif false && errors.As(err, &located) {", "TestRubricERRORS10_")
