# H-149: a Codex execution takes its outcome from the rollout's item_completed
# entry with exactly its id, by the rule decided for Karthik's review, and from
# nothing else; the rollout is accounted against the store. The phase 4 replay
# catches a rule read wrongly; the second test, every way an item must fail to
# judge a call.
m("H-149 exit 0 reads as failed", "internal/report/codex.go",
  "\t\tif code == 0 {\n\t\t\treturn store.ExecOK", "\t\tif code != 0 {\n\t\t\treturn store.ExecOK", "TestH149_")
m("H-149 a nonzero exit is not judged", "internal/report/codex.go",
  "\t\treturn store.ExecFailed, code, true", "\t\treturn \"\", 0, false", "TestH149_")
m("H-149 an exit code that is not an integer is read anyway", "internal/report/codex.go",
  "\t\tcode, err := strconv.Atoi(string(bytes.TrimSpace(it.ExitCode)))",
  "\t\tcode, err := strconv.Atoi(string(bytes.Trim(it.ExitCode, \"\\\" \")))", "TestH149_")
m("H-149 a file change that completed reads as failed", "internal/report/codex.go",
  "\t\tif json.Unmarshal(it.Status, &status) == nil && status == \"failed\" {",
  "\t\tif json.Unmarshal(it.Status, &status) == nil && status != \"\" {", "TestH149_")
m("H-149 every item is judged as a command", "internal/report/codex.go",
  "\tswitch it.Type {\n\tcase codexCommand:", "\tswitch codexCommand {\n\tcase codexCommand:", "TestH149_")
m("H-149 a conflicting second entry wins", "internal/report/codex.go",
  "\t\tif prev, seen := r.outcomes[it.ID]; seen && (prev != outcome || r.codes[it.ID] != code) {", "\t\tif false {",
  "TestH149_")
m("H-149 an execution joins an item in a rollout its declaration does not name", "internal/report/codex.go",
  "\t\tr := rollouts[pathOf[x.ToolUseID]]",
  "\t\t_ = pathOf\n\t\tvar r *codexRollout\n\t\tfor _, c := range rollouts {\n\t\t\tif _, ok := c.outcomes[x.ToolUseID]; ok {\n\t\t\t\tr = c\n\t\t\t}\n\t\t}",
  "TestH149_")
m("H-149 the outcomes are never set", "internal/report/codex.go",
  "\t\tx.Outcome = outcome", "\t\t_ = outcome", "TestH149_")
m("H-149 the exit code is not carried", "internal/report/codex.go",
  "\t\t\tx.ExitCode = &c", "\t\t\t_ = c", "TestH149_")
m("H-149 every item is held to the store, not only calls", "internal/report/codex.go",
  "\tfor id := range r.calls {", "\tfor id := range r.ids {", "TestH149_")
m("H-149 a recorded call the rollout lacks is not reported", "internal/report/codex.go",
  "\t\tif !r.ids[id] {", "\t\tif false {", "TestH149_")
m("H-149 a call the store lacks is not reported", "internal/report/codex.go",
  "\t\tif !hooked[id] {", "\t\tif false {", "TestH149_")
