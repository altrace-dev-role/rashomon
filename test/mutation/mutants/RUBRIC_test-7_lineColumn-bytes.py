# lineColumn's column is documented to count bytes, as Go's own tools do, not
# characters, and a \r before \n is not a second line break. Counting runes
# instead of bytes, or counting \r as its own line ending, is the regression
# the multibyte and CRLF table rows exist to catch.
m("lineColumn counts runes instead of bytes", "internal/settings/errors.go",
  "\tcolumn = off - (bytes.LastIndexByte(before, '\\n') + 1) + 1\n",
  "\tcolumn = len([]rune(string(before[bytes.LastIndexByte(before, '\\n')+1:]))) + 1\n",
  "TestRubricErrors10_")
m("lineColumn counts a CRLF as two line breaks", "internal/settings/errors.go",
  "\tline = bytes.Count(before, []byte{'\\n'}) + 1\n",
  "\tline = bytes.Count(before, []byte{'\\n'}) + bytes.Count(before, []byte{'\\r'}) + 1\n",
  "TestRubricErrors10_")
