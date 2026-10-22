# STR-14: banned words are ASCII case-insensitive whole words, refused
# outside a quote (tech 4.2.7's whole-string rule) and inside Approved
# (archive 40.3.8). A skipped check, a case-sensitive match, a quote that
# opens nowhere or never has to close, or Approved without its list is the
# whole regression.
m("STR-14 the whole-string check skips banned words", "internal/steer/render/validate.go",
  "\treturn checkWords(line)\n", "\treturn nil\n", "TestSTR14b_")
m("STR-14 banned words match in lower case only", "internal/steer/render/validate.go",
  "\t\tif strings.EqualFold(w, b) {\n", "\t\tif w == b {\n", "TestSTR14b_")
m("STR-14 a quote never opens, so quoted words are checked", "internal/steer/render/validate.go",
  "\t\t\tif !inQuote && !prev {\n", "\t\t\tif !inQuote && !prev && i < 0 {\n", "TestSTR14b_")
m("STR-14 an unclosed quote hides the rest of the line", "internal/steer/render/validate.go",
  "\tif inQuote {\n", "\tif false {\n", "TestSTR14b_")
m("STR-14 Approved drops its banned words", "internal/steer/render/slot/slot.go",
  "firstWordOf(s, approvedBanned)", "firstWordOf(s, nil)", "TestSTR14b_")
m("STR-14 Approved matches banned words in lower case only", "internal/steer/render/slot/slot.go",
  "\t\t\t\tif strings.EqualFold(s[start:i], w) {\n", "\t\t\t\tif s[start:i] == w {\n", "TestSTR14b_")
m("STR-14 the imperative phrase list loses make sure", "internal/steer/render/templates.go",
  'var imperativePhrases = []string{"please", "make sure", "do not", "don\'t", "you must", "you should"}',
  'var imperativePhrases = []string{"please", "do not", "don\'t", "you must", "you should"}', "TestSTR14_")
m("STR-14 a sentence may open with a bare imperative", "internal/steer/render/templates.go",
  '\t\tfirst := strings.Trim(words[0], "()\\"")\n', '\t\tfirst := strings.Trim(words[0], "()\\"") + "x"\n', "TestSTR14_")
m("STR-14 a template's quote may hold more than one placeholder", "internal/steer/render/templates.go",
  "\tif err := templateQuotesHoldOnlyPlaceholders(format); err != nil {\n\t\treturn err\n\t}\n", "", "TestSTR14_")
m("STR-14 the imperative opening list loses the edit and git verbs", "internal/steer/render/templates.go",
  '\t"restore", "undo", "skip", "commit", "push", "write", "edit", "open", "avoid",\n',
  "", "TestSTR14c_")
