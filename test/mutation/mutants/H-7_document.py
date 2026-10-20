# Two edit paths, two mutations. SetHookEntries is what watch uses and what
# detach uses while entries remain; RemoveHookEvent is what detach uses once an
# event empties, and it did not exist when this mutation was written -- so the
# H-7 mutation went NOT DETECTED, because the detach test that judged it no
# longer executes the line being mutated. The test filter is widened to the
# install paths that still reach it.
m("H-7  edit drops every other top-level key", "internal/settings/document.go",
  "\tif h := d.find(hooksKey); h != nil {\n\t\th.raw = obj\n\t} else {\n\t\td.members = append(d.members, member{key: hooksKey, raw: obj})\n\t}\n\treturn nil",
  "\td.members = []member{{key: hooksKey, raw: obj}}\n\treturn nil", "TestH3_|TestH7_|TestH06_")
m("H-7  removing a hook event drops every other top-level key", "internal/settings/document.go",
  "\tif len(kept) == 0 {\n\t\td.removeMember(hooksKey)\n\t\treturn nil\n\t}",
  "\tif len(kept) == 0 {\n\t\td.members = nil\n\t\treturn nil\n\t}", "TestH06_")
m("H-7  detach never notices an edited entry", "internal/install/install.go",
  "func intact(raw json.RawMessage, event string) string {\n", "func intact(raw json.RawMessage, event string) string {\n\tif true {\n\t\treturn \"\"\n\t}\n", "TestH7_DetachSaysSoWhenItCannot")
