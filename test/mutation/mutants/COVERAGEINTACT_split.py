# coverageIntact and matcherFires split the coverage question (does our
# command fire on every call) from intact's conformance question (is this
# the exact entry watch installs). Present reads coverageIntact for the
# settings origin, as pluginDeclaration does for the plugin one; intact
# itself is unchanged and still backs Apply and RemoveIf's refusal to
# overwrite or remove a changed entry.
m("coverageintact Present reads intact instead of coverageIntact", "internal/install/install.go",
  "\t\tif Owner(e, event) == installID && coverageIntact(e, event) == \"\" {\n",
  "\t\tif Owner(e, event) == installID && intact(e, event) == \"\" {\n",
  "TestPresent_|TestCoverageIntact_")
m("coverageintact an absent or empty matcher narrows", "internal/install/install.go",
  "\tif m == nil || *m == \"\" {\n\t\treturn true\n\t}\n",
  "",
  "TestCoverageIntact_")
m("coverageintact a narrow matcher reads as match-all on any event", "internal/install/install.go",
  "\treturn hasMatcher(event) && *m == Matcher\n",
  "\treturn true\n",
  "TestCoverageIntact_")
m("coverageintact believes a group with no command hook", "internal/install/install.go",
  "\tfor _, h := range g.Hooks {\n\t\tif h.Type == \"command\" {\n\t\t\treturn \"\"\n\t\t}\n\t}\n\treturn \"no hook in the group is type \\\"command\\\"\"\n",
  "\treturn \"\"\n",
  "TestCoverageIntact_")
