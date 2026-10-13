# Rubric hld-1, test-3, hld-3: the plugin origin is held to the check a
# settings entry is held to. Every group of the plugin's hooks.json goes
# through intact() before its command is believed, so a hooks.json narrowed to
# "Bash" or slowed to a timeout of 600 reads hook_entry absent, not
# present_plugin; and the hooks.json this repository ships is tested against
# that same check, not against its event names alone. "matcher": "*" occurs
# three times in hooks.json, so each anchor carries its event's name.
m("hld-1 the plugin check believes a group without the intact check", "internal/install/plugin.go",
  "\t\tif intact(raw, event) != \"\" {\n\t\t\t// Narrowed, slowed",
  "\t\tif false && intact(raw, event) != \"\" {\n\t\t\t// Narrowed, slowed",
  "TestRubricHLD1_")
m("hld-1 the shipped plugin narrows PreToolUse to Bash", "plugin/hooks/hooks.json",
  "\"PreToolUse\": [\n      {\n        \"matcher\": \"*\",",
  "\"PreToolUse\": [\n      {\n        \"matcher\": \"Bash\",",
  "TestRubricHLD1_ShippedHooksDeclareEveryEvent")
m("hld-1 the shipped plugin narrows PostToolUseFailure to Bash", "plugin/hooks/hooks.json",
  "\"PostToolUseFailure\": [\n      {\n        \"matcher\": \"*\",",
  "\"PostToolUseFailure\": [\n      {\n        \"matcher\": \"Bash\",",
  "TestRubricHLD1_ShippedHooksDeclareEveryEvent")
m("hld-1 the shipped plugin gives PreToolUse a timeout of 600", "plugin/hooks/hooks.json",
  "\"args\": [\"hook\"],\n            \"timeout\": 5\n",
  "\"args\": [\"hook\"],\n            \"timeout\": 600\n",
  "TestRubricHLD1_ShippedHooksDeclareEveryEvent")
m("hld-1 the shipped plugin gives Stop a timeout of 600", "plugin/hooks/hooks.json",
  "\"args\": [\"recap\"],\n            \"timeout\": 10\n          }\n        ]\n      }\n    ],\n    \"StopFailure\"",
  "\"args\": [\"recap\"],\n            \"timeout\": 600\n          }\n        ]\n      }\n    ],\n    \"StopFailure\"",
  "TestRubricHLD1_ShippedHooksDeclareEveryEvent")
