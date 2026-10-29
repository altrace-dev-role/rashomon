# Rubric hld-1, test-3, hld-3: the plugin origin is held to the coverage
# check a settings entry is held to. Every group of the plugin's hooks.json
# goes through coverageIntact before its command is believed, so a
# hooks.json narrowed to "Bash" reads hook_entry absent, not present_plugin;
# and the hooks.json this repository ships is tested against that same
# check, not against its event names alone. "matcher": "*" occurs three
# times in hooks.json, so each anchor carries its event's name. A shipped
# timeout different from Timeout/RecapTimeout is not a coverage defect --
# coverageIntact does not ask about it -- so there is no mutant for one here;
# intact, unchanged, still asks and still backs Apply and RemoveIf's refusal
# to overwrite a changed timeout.
m("hld-1 the plugin check believes a group without the coverage check", "internal/install/plugin.go",
  "\t\tif coverageIntact(raw, event) == \"\" {\n\t\t\treturn true, true, nil\n",
  "\t\tif true || coverageIntact(raw, event) == \"\" {\n\t\t\treturn true, true, nil\n",
  "TestRubricHLD1_")
m("hld-1 the shipped plugin narrows PreToolUse to Bash", "plugin/hooks/hooks.json",
  "\"PreToolUse\": [\n      {\n        \"matcher\": \"*\",",
  "\"PreToolUse\": [\n      {\n        \"matcher\": \"Bash\",",
  "TestRubricHLD1_ShippedHooksDeclareEveryEvent")
m("hld-1 the shipped plugin narrows PostToolUseFailure to Bash", "plugin/hooks/hooks.json",
  "\"PostToolUseFailure\": [\n      {\n        \"matcher\": \"*\",",
  "\"PostToolUseFailure\": [\n      {\n        \"matcher\": \"Bash\",",
  "TestRubricHLD1_ShippedHooksDeclareEveryEvent")

# The refusal, the overlap line and the recording line ask whether the plugin
# fires at all, which a hand-edited group still does. Each reverting to the
# intact-only list lets watch install beside a plugin that records every call
# it fires on, and status call that arrangement clean.
m("hld-1 Live reads the intact list", "internal/install/plugin.go",
  "\treturn p.Enabled && len(p.Declared) > 0\n",
  "\treturn p.Enabled && len(p.Events) > 0\n",
  "TestRubricHLD1_")
m("hld-1 an edited group does not count as declared", "internal/install/plugin.go",
  "\t\tdeclared = true\n",
  "\t\tdeclared = intact(raw, event) == \"\"\n",
  "TestRubricHLD1_")
m("hld-1 FindPlugin lists only intact events as declared", "internal/install/plugin.go",
  "\t\t\t\tif declared {\n",
  "\t\t\t\tif declared && whole {\n",
  "TestRubricHLD1_")
m("hld-1 watch refuses only under an intact plugin", "cmd/rashomon/watch.go",
  "\t} else if p != nil && p.Live() {\n",
  "\t} else if p != nil && p.Enabled && len(p.Events) > 0 {\n",
  "TestRubricHLD1_WatchRefusesUnderAHandEditedPlugin")
m("hld-1 status sees no overlap with an edited plugin", "cmd/rashomon/main.go",
  "\t\tfmt.Fprintf(stdout, \"  plugin %s: %s\\n\", event, state)\n\t}\n\treturn p.Live(), nil\n",
  "\t\tfmt.Fprintf(stdout, \"  plugin %s: %s\\n\", event, state)\n\t}\n\treturn p.Enabled && len(p.Events) > 0, nil\n",
  "TestRubricHLD1_StatusNamesTheOverlapUnderAHandEditedPlugin")
m("hld-1 an edited plugin reads as not installed", "cmd/rashomon/main.go",
  "\tif err != nil || p == nil {\n\t\treturn false, err\n\t}\n\treturn p.Live(), nil\n",
  "\tif err != nil || p == nil {\n\t\treturn false, err\n\t}\n\treturn p.Enabled && len(p.Events) > 0, nil\n",
  "TestRubricHLD1_AHandEditedPluginStillRecords")
# The recap pointer asks whether the plugin running this binary fires on Stop
# at all: a hand-edited Stop entry is loaded, so /rashomon:report exists. Each
# break sends that entry's line to the CLI command instead.
m("hld-1 the recap pointer asks the intact-only answer", "cmd/rashomon/recap_turn.go",
  "\tpresent, err := install.PluginFires(install.EventStop)\n",
  "\tpresent, err := install.PluginPresent(install.EventStop)\n",
  "TestRubricHLD1_AHandEditedPluginsRecapPointsAtItsOwnCommand")
m("hld-1 PluginFires reads only an intact group", "internal/install/plugin.go",
  "\tdeclared, _, err := pluginDeclaration(root, event)\n\treturn declared, err\n",
  "\t_, declared, err := pluginDeclaration(root, event)\n\treturn declared, err\n",
  "TestRubricHLD1_AHandEditedPluginsRecapPointsAtItsOwnCommand")
m("hld-1 FindPlugin swallows a per-event read or parse error", "internal/install/plugin.go",
  "\t\t\t\t\treturn nil, err\n",
  "\t\t\t\t\tcontinue\n",
  "TestRubricHLD1_UnreadablePluginHooksReadAsUnreadable")
