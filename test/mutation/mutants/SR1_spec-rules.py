# SPEC-RULES1 1a: forbid_command, protect_path and require_evidence judged
# over the record (verify/rules.go), raised at the post from the call alone
# (verify/rules_raise.go) by the hook's rules step (hook/steerstep.go), and
# the command walker and line reader they read with (shape/commands.go). Two
# rows per kind, then trust, record-green, the one reading per line, the
# post's certainty, the walker, the rules step, the line's cap and a store
# fault that must not silence the rules.
m("sr1 forbid_command compares the program as written, not by base name", "internal/verify/rules.go",
  "\tprog, want := path.Base(c.Words[0]), path.Base(m.Argv[0])\n",
  "\tprog, want := c.Words[0], m.Argv[0]\n",
  "TestSR1l_")
m("sr1 forbid_command reads an expanding word as literal", "internal/verify/rules.go",
  "\topaque := slices.Contains(exp, true)\n",
  "\topaque := false\n",
  "TestSR1l_|TestSR1e_")
m("sr1 protect_path reads a Write as a modify", "internal/verify/rules.go",
  "\tcase \"Edit\", \"MultiEdit\", \"NotebookEdit\":\n\t\treturn triOf(modify, modify)\n",
  "\tcase \"Edit\", \"MultiEdit\", \"NotebookEdit\", \"Write\":\n\t\treturn triOf(modify, modify)\n",
  "TestSR1b_")
m("sr1 protect_path ignores exclude_paths", "internal/verify/rules.go",
  "\treturn anyPath(r.Paths, rel) && !anyPath(r.ExcludePaths, rel)\n",
  "\treturn anyPath(r.Paths, rel)\n",
  "TestSR1b_")
m("sr1 require_evidence reads a gate the snapshot lacks as not green", "internal/verify/rules.go",
  "\t\t\treturn gateTri(gates[i].State)\n\t\t}\n\t}\n\treturn triMaybe\n",
  "\t\t\treturn gateTri(gates[i].State)\n\t\t}\n\t}\n\treturn triNo\n",
  "TestSR1i_|TestSR1c_")
m("sr1 require_evidence reads done with no write in the window", "internal/verify/rules.go",
  "\tcase !ok || !slices.Contains(r.At, word) || word == \"done\" && !wrote:\n",
  "\tcase !ok || !slices.Contains(r.At, word):\n",
  "TestSR1c_")
m("sr1 an untrusted rule binds", "internal/verify/rules.go",
  "func binding(t spec.Trust) bool { return t == spec.Trusted || t == spec.Managed }\n",
  "func binding(t spec.Trust) bool { return t != spec.Builtin }\n",
  "TestSR1d_")
m("sr1 record-green ignores a binding rule's violation", "internal/verify/evaluate.go",
  "recordGreen(snap.Gates, len(snap.Integrity.Unresolved)+bindingViolations(snap.SpecVerdicts))",
  "recordGreen(snap.Gates, len(snap.Integrity.Unresolved))",
  "TestSR1h_|TestSR1d_")
m("sr1 each rule reads every line again", "internal/verify/rules.go",
  "\t\t\t\tlines, read = ruleLines(run, picked, at), true\n",
  "\t\t\t\tlines, read = ruleLines(run, picked, at), false\n",
  "TestSR1m_")
m("sr1 the post raises a rule it cannot decide", "internal/verify/rules_raise.go",
  "\t\tif commandMatch(m, c) == triYes {\n",
  "\t\tif commandMatch(m, c) != triNo {\n",
  "TestSR1e_|TestSR1l_")
m("sr1 commands reads an unquoted glob as literal", "internal/shape/commands.go",
  "\treturn strings.ContainsAny(bare, \"*?[{\")\n",
  "\treturn false\n",
  "TestSR1t_")
m("sr1 commands reads a reserved word as the program", "internal/shape/commands.go",
  "\t\tfor b < e && !toks[b].meta && toks[b].quotedAt < 0 && reservedWord(toks[b].text) {\n",
  "\t\tfor b < e && false {\n",
  "TestSR1s_")
m("sr1 the rules step passes a Bash call over", "internal/hook/steerstep.go",
  "\tcase \"Bash\", \"Edit\", \"MultiEdit\", \"NotebookEdit\", \"Write\":\n\t\treturn true\n",
  "\tcase \"Edit\", \"MultiEdit\", \"NotebookEdit\", \"Write\":\n\t\treturn true\n",
  "TestSR1w_|TestSR1x_")
m("sr1 the rules step reads the run", "internal/hook/steerstep.go",
  "\tvar run *store.Run\n\tif stepped {\n",
  "\tvar run *store.Run\n\tif true {\n",
  "TestSR1x_")
m("sr1 the step drops the rules' findings", "internal/hook/steerstep.go",
  "\tview.Findings = append(view.Findings, rules.Findings...)\n",
  "",
  "TestSR1y_")
m("sr1 the rules step reads what no rule reads", "internal/hook/steerstep.go",
  "\treturn command, path\n",
  "\treturn true, true\n",
  "TestSR1z_")
m("sr1 the command line has no cap", "internal/shape/commands.go",
  "\tif !ok || s.Kind != jsonscan.String || int(s.Len)-2 > max {\n",
  "\tif !ok || s.Kind != jsonscan.String {\n",
  "TestSR1v_")
m("sr1 a store fault silences the rules", "internal/hook/steerstep.go",
  "\t\tif r, err := p.st.ReadRunConsistent(p.sessionID, SteerReadBudget); err == nil {\n\t\t\trun = r\n\t\t}\n",
  "\t\tr, err := p.st.ReadRunConsistent(p.sessionID, SteerReadBudget)\n\t\tif err != nil {\n\t\t\treturn steer.Context{}, verify.SteerView{}, false\n\t\t}\n\t\trun = r\n",
  "TestSR1y_")
m("sr1 the post raises a forbidden command that failed", "internal/verify/rules_raise.go",
  "\t\t\tif event != string(steer.EventPostToolUse) || call.ToolName != \"Bash\" || call.Command == \"\" ||\n",
  "\t\t\tif call.ToolName != \"Bash\" || call.Command == \"\" ||\n",
  "TestSR1_")
m("sr1 protectOver cleans each path again inside the rule", "internal/verify/rules.go",
  "\t\trel, inside := rels[i].rel, rels[i].inside\n",
  "\t\trel, inside := projectPath(*w.Path)\n",
  "TestSR1n_")
m("sr1 a no-spec discovery resolves symlinks before asking for a root", "internal/hook/specdiscover.go",
  "\tif git == \"\" && projectDir == \"\" {\n\t\treturn nil\n\t}\n", "",
  "TestSR1ab_")
