# RECALL-15 (b): the Stop names each trusted or managed spec rule the turn's
# calls matched, as the post note named it to the agent at the call: the
# rule's id and the turn's calls, on its own line beside the value line's k,
# which names none (1.8 Gate B F1). Cut to the turn, since the verdicts are
# the session's (D40); carried by recap.Line's sentence, the done flag and
# the value line, else a line of its own, also with the summary switch off;
# never at StopFailure (tech 2.2.11 condition 4).
VR = "internal/verify/rules.go"
VS = "internal/verify/stop.go"
RR = "internal/recap/rules.go"
RS = "cmd/rashomon/recap_stop.go"
RT = "cmd/rashomon/recap_turn.go"

# Which rules: binding ones, this turn's.
m("RC-15 the Stop names every turn's seqs", VR,
  "slices.BinarySearch(v.Seqs, from)", "slices.BinarySearch(v.Seqs, 0)", "TestRC15f_|TestRC15g_")
m("RC-15 a turn with no declaration names the session's rules", VR,
  "\tif from <= 0 {\n\t\treturn nil\n\t}\n\tvar out []RuleVerdict", "\tvar out []RuleVerdict", "TestRC15f_")
m("RC-15 an untrusted rule is named", VR,
  "if v.Verdict != Violation || !binding(v.trust) {", "if v.Verdict != Violation {", "TestRC15f_|TestRC15g_")
m("RC-15 the Stop view names no rule", VS,
  "Index: idx,\n\t\tViolations: turnViolations(snap.SpecVerdicts, turn.FirstSeq)}", "Index: idx}", "TestRC15_|TestRC15g_")

# The lines.
m("RC-15 the value line names no rule", "internal/recap/value.go",
  "\tb.WriteString(ruleLines(v))\n", "", "TestRC15d_|TestRC15_")
m("RC-15 the done flag names no rule", "internal/recap/done.go",
  "expectedClause(unmet) + \"\\n\" + ruleLines(v)", "expectedClause(unmet) + \"\\n\"", "TestRC15e_")
m("RC-15 no cap", RR,
  "const maxRuleLines = 2", "const maxRuleLines = 64", "TestRC15d_")
m("RC-15 a refused id prints", RR,
  "; id.Validate() == nil {", "; true {", "TestRC15d_")
m("RC-15 the id keeps its spec: prefix", RR,
  "slot.RuleID(strings.TrimPrefix(string(r.Rule), specPrefix))", "slot.RuleID(string(r.Rule))", "TestRC15d_|TestRC15_")
m("RC-15 a protected path reads as a command", RR,
  "\tcase spec.ProtectPath:\n\t\treturn \"protects a path \" + calls(r.Seqs) + \" wrote\"\n", "", "TestRC15d_")
m("RC-15 no width rule", RR,
  "\tif columns(one) <= lineColumns {", "\tif true {", "TestRC15d_")
m("RC-15 the rule line speaks for none", RR,
  "\tif n == 0 && c == 0 {\n\t\treturn \"\"\n\t}\n\tvar parts", "\tvar parts", "TestRC15e_")

# Where the turn speaks.
m("RC-15 a turn that wrote nothing names no rule", RS,
  "l, clause = recap.RuleLine(s.view, sessionID, fromPlugin), \"\"", "clause = \"\"", "TestRC15_|TestRC15g_")
m("RC-15 the summary switch off silences the rule", RS,
  "\t\tl := recap.RuleLine(s.view, sessionID, fromPlugin)\n\t\treturn l, l != \"\", \"\"\n",
  "\t\treturn \"\", false, \"\"\n", "TestRC15g_")
m("RC-15 StopFailure names the rule", RS,
  "\tif !s.speaks {\n\t\treturn \"\"\n\t}\n\treturn recap.RuleLines(s.view)", "\treturn recap.RuleLines(s.view)", "TestRC15g_")
m("RC-15 recap.Line's sentence names no rule", "cmd/rashomon/recap_conflicts.go",
  "stop.rules() + recap.PlatformLines(", "recap.PlatformLines(", "TestRC15g_")
