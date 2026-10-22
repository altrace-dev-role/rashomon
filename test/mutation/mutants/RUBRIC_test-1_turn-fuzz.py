# test-1: FinalAssistantTexts agrees, turn by turn, with FinalAssistantText
# over that turn's own lines. Its seeds run as TestRubricTEST1_TurnSeeds.
m("test-1  a reply whose content is a plain string has no words", "internal/report/transcript.go",
  "\treturn content.plain || bytes.Contains(raw, textMarker) || bytes.Contains(raw, escapeMarker)\n",
  "\treturn bytes.Contains(raw, textMarker) || bytes.Contains(raw, escapeMarker)\n",
  "TestRubricTEST1_")
m("test-1  a turn's words are its first reply's, not its last", "internal/report/transcript.go",
  "\t\t\ttf := out[current]\n\t\t\ttf.Said, tf.Text, tf.AtMS = true, text, at.UnixMilli()",
  "\t\t\ttf := out[current]\n\t\t\tif tf.Said {\n\t\t\t\tcontinue\n\t\t\t}\n\t\t\ttf.Said, tf.Text, tf.AtMS = true, text, at.UnixMilli()", "TestRubricTEST1_")
