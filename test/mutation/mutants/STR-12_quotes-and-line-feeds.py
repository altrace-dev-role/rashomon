# STR-12: no tag characters or system-reminder in any case; control runes
# only as a briefing's line separators; Path, Program and AgentType only
# inside a quote. A case-sensitive reminder check, a line feed in a note, an
# empty briefing line, a skipped whole-string check or a bare path is the
# whole regression.
m("STR-12 system-reminder is refused in lower case only", "internal/steer/render/validate.go",
  "\tif strings.Contains(strings.ToLower(line), forbiddenSubstring) {\n",
  "\tif strings.Contains(line, forbiddenSubstring) {\n", "TestSTR12b_")
m("STR-12 a note may hold a line feed", "internal/steer/render/validate.go",
  "\t\tcase c < 0x20 || c == 0x7f:\n",
  "\t\tcase (c < 0x20 && c != '\\n') || c == 0x7f:\n", "TestSTR12b_")
m("STR-12 a briefing may hold an empty line", "internal/steer/render/validate.go",
  "\t\tif line == \"\" {\n", "\t\tif false {\n", "TestSTR12b_")
m("STR-12 New skips the whole-string check", "internal/steer/render/render.go",
  "\tif err := check(text); err != nil {\n",
  "\tif err := check(text); err != nil && p == \"\" {\n", "TestSTR12b_")
m("STR-12 a path renders outside a quote", "internal/steer/render/slot/path.go",
  "func (p Path) Text() string         { return quoted(string(p)) }\n",
  "func (p Path) Text() string         { return string(p) }\n", "TestSTR12b_")
m("STR-12 an agent type renders outside a quote", "internal/steer/render/slot/slot.go",
  "func (a AgentType) Text() string         { return quoted(string(a)) }\n",
  "func (a AgentType) Text() string         { return string(a) }\n", "TestSTR12b_")
