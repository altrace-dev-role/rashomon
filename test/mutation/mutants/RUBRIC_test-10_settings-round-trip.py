# test-10: the settings.json parser and encoder round-trip, checked over
# arbitrary input against encoding/json (rubric fix list entry 52).
m("test-10  keys are written with Go quoting, not JSON's", "internal/settings/document.go",
  "\t\tkey, _ := json.Marshal(m.key)\n",
  "\t\tkey := []byte(strconv.Quote(m.key))\n", "TestRubricTest10_")
IMPORTS["test-10  keys are written with Go quoting, not JSON's"] = ("internal/settings/document.go",
  "\t\"os\"\n\t\"strings\"\n", "\t\"os\"\n\t\"strconv\"\n\t\"strings\"\n")
m("test-10  replacing an event appends a second one", "internal/settings/document.go",
  "\t\tif events[i].key == event {\n\t\t\tevents[i].raw = arr",
  "\t\tif false && events[i].key == event {\n\t\t\tevents[i].raw = arr", "TestRubricTest10_")
m("test-10  a carried value is rendered compacted", "internal/settings/document.go",
  "\t\tb.WriteString(\": \")\n\t\tb.Write(m.raw)\n",
  "\t\tb.WriteString(\": \")\n\t\tif json.Compact(&b, m.raw) != nil {\n\t\t\tb.Write(m.raw)\n\t\t}\n", "TestRubricTest10_")
