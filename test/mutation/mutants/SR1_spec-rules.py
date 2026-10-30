# SPEC-RULES1 1a: forbid_command, protect_path and require_evidence judged
# over the record (verify/rules.go), and the command walker they read lines
# with (shape/commands.go). Two rows per kind, then trust, record-green, the
# one reading per line and the walker.
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
m("sr1 commands reads an unquoted glob as literal", "internal/shape/commands.go",
  "\treturn strings.ContainsAny(bare, \"*?[{\")\n",
  "\treturn false\n",
  "TestSR1t_")
m("sr1 commands reads a reserved word as the program", "internal/shape/commands.go",
  "\t\tfor b < e && !toks[b].meta && toks[b].quotedAt < 0 && reservedWord(toks[b].text) {\n",
  "\t\tfor b < e && false {\n",
  "TestSR1s_")
