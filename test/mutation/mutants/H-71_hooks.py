m("H-71 the plugin runs a model beside the recorder", "plugin/hooks/hooks.json",
  "\"args\": [\"hook\"],\n            \"timeout\": 5\n          }",
  "\"args\": [\"hook\"],\n            \"timeout\": 5\n          },\n          {\"type\": \"prompt\", \"prompt\": \"Is this call safe?\"}",
  "TestH71_PluginHooksRunOnlyTheRecorder")
# The same model, declared where the hooks.json walk does not look: the
# manifest's own hooks field, which Claude Code loads beside hooks.json. This
# passed every H-71 test until the manifest's key set was closed.
m("H-71 the manifest declares a model-run hook", "plugin/.claude-plugin/plugin.json",
  '"license": "Apache-2.0",',
  '"license": "Apache-2.0",\n  "hooks": {"PreToolUse": [{"matcher": "*", "hooks": [{"type": "prompt", "prompt": "x"}]}]},',
  "TestH71_ManifestDeclaresNoHooks")
m("H-71 a skill's frontmatter declares a model-run hook", "plugin/skills/report/SKILL.md",
  "name: report\n",
  "name: report\nhooks:\n  PreToolUse:\n    - matcher: \"*\"\n      hooks:\n        - type: prompt\n          prompt: x\n",
  "TestH71_MarkdownDeclaresNoHooks")

# Part 1 -- the plugin, and origin-aware ownership. H-71 through H-77 are
# provisional from H-71 (H-70 is the highest item on the merge target) and are
# grepped against it before they become the contract.
m("H-71 coverage reverts to the settings-only Present check", "internal/hook/coverage.go",
  "\tcase pluginPresent:\n\t\tres.HookEntry = store.EntryPresentPlugin",
  "\tcase false && pluginPresent:\n\t\tres.HookEntry = store.EntryPresentPlugin",
  "TestH71_|TestH72_")
