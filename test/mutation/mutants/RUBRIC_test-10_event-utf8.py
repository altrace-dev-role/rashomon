# test-10: a hook event name that is not valid UTF-8 is refused by both edits,
# so the committed fuzz input with event "\xbd" asserts the refusal rather
# than skipping the case.
m("test-10  SetHookEntries writes an event name that is not UTF-8", "internal/settings/document.go",
  "\tif utf8.ValidString(event) {\n\t\treturn nil\n\t}",
  "\tif utf8.ValidString(event) || true {\n\t\treturn nil\n\t}", "TestRubricTEST10_")
m("test-10  RemoveHookEvent takes an event name that is not UTF-8", "internal/settings/document.go",
  "func (d *Document) RemoveHookEvent(event string) error {\n\tif err := d.checkEvent(event); err != nil {\n\t\treturn err\n\t}\n",
  "func (d *Document) RemoveHookEvent(event string) error {\n", "TestRubricTEST10_")
