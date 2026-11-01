# Release check A8: watch then detach leaves settings.json byte for byte as it
# was. watch and detach splice their entries into and out of the user's own
# bytes (internal/settings/splice.go); each mutant below breaks one half of that
# and must be seen by the byte-exact tests.
T = "TestDetachByteExact_"
m("A8  the separator is not the file's own", "internal/settings/splice.go",
  "\t\treturn raw[spans[n-2].end:spans[n-1].start]",
  "\t\treturn []byte(\", \")", T)
m("A8  a one-element multi-line array is read as inline", "internal/settings/splice.go",
  "\tif i := bytes.LastIndexByte(gap, '\\n'); i >= 0 {",
  "\tif i := bytes.LastIndexByte(gap, '\\n'); false && i >= 0 {", T)
m("A8  removing an element eats the one before it", "internal/settings/splice.go",
  "\t\tfrom, to = spans[i-1].end, spans[i].end",
  "\t\tfrom, to = spans[i-1].start, spans[i].end", T)
m("A8  an inline container takes a multi-line element", "internal/settings/splice.go",
  "\tif !bytes.ContainsRune(sep, '\\n') {",
  "\tif false {", T)
m("A8  the document renders from its members, not its own bytes", "internal/settings/document.go",
  "\tif d.buf != nil {\n\t\treturn bytes.Clone(d.buf)",
  "\tif false {\n\t\treturn bytes.Clone(d.buf)", T)
m("A8  an entry that stays is treated as removed and re-added", "internal/settings/document.go",
  "\t\tif j < len(entries) && bytes.Equal(o, entries[j]) {",
  "\t\tif false && j < len(entries) && bytes.Equal(o, entries[j]) {", T)
m("A8  removing a top-level member re-renders the document", "internal/settings/document.go",
  "\t\t\td.edit(func(buf []byte) ([]byte, bool) { return spliceRemove(buf, true, i) })",
  "\t\t\td.edit(func(buf []byte) ([]byte, bool) { return nil, false })", T)
