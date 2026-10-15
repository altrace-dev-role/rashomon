# Rubric hld-9: a hand-edited plugin entry that still fires reads "present
# (hand edited, not counted for coverage)" in status, not "absent". Before,
# statusPlugin read Events alone, so every hand-edited event printed absent
# while the overlap line below it -- which reads Live(), i.e. Declared --
# still named a duplicate-recording overlap for the very same plugin.
m("hld-9 a hand-edited plugin event drops to the absent case", "cmd/rashomon/main.go",
  "\t\tcase declared[event]:\n",
  "\t\tcase false && declared[event]:\n",
  "TestRubricHLD1_StatusNamesTheOverlapUnderAHandEditedPlugin")
