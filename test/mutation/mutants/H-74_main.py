# H-74's own break is "remove the guard", but H-73's refusal is never reached
# by H-74's fixture -- it bypasses watch entirely, constructing both origins
# as files, exactly as the spec's own migration window does without running
# watch a second time. What H-74 actually guards is standsDown treating a
# plugin invocation (no --install marker) as belonging to a foreign install
# and silently dropping its declaration -- the same line H-6's mutations
# touch, mutated a different way: dropping the id == "" branch rather than
# forcing the whole condition true.
m("H-74 a plugin invocation's declaration is silently dropped", "cmd/rashomon/main.go",
  "\tif id == \"\" || id == st.InstallID() {",
  "\tif id == st.InstallID() {",
  "TestH74_")
