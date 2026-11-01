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
  "\tcase \"Write\":\n\t\treturn triOf(create && modify, create || modify)\n",
  "\tcase \"Write\":\n\t\treturn triOf(modify, modify)\n",
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
  "\treturn len(snap.Integrity.Unresolved) + bindingViolations(snap.SpecVerdicts)\n",
  "\treturn len(snap.Integrity.Unresolved)\n",
  "TestSR1h_|TestSR1d_|TestSRS1_|TestSRS1e2_")
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
  "\tvar run *store.Run\n\tif readRun {\n",
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
  "\t\t\tif event != string(steer.EventPostToolUse) {\n\t\t\t\tcontinue\n\t\t\t}\n",
  "",
  "TestSR1_|TestSRS1h_")
IMPORTS["sr1 the post raises a forbidden command that failed"] = ("internal/verify/rules_raise.go", '\t"github.com/altrace-dev-role/rashomon/internal/steer"\n', "")
m("sr1 protectOver cleans each path again inside the rule", "internal/verify/rules.go",
  "\t\trel, inside := rels[i].rel, rels[i].inside\n",
  "\t\trel, inside := projectPath(*w.Path)\n",
  "TestSR1n_")
m("sr1 a no-spec discovery resolves symlinks before asking for a root", "internal/hook/specdiscover.go",
  "\tif git == \"\" && projectDir == \"\" {\n\t\treturn nil\n\t}\n", "",
  "TestSR1ab_")
m("sr1 a rules-only call trusts a bound spec without reading the run", "internal/hook/steerstep.go",
  "\t\tif t := TrustRoot(r, root, p.landedSeq); t != (TrustedRoot{}) {\n",
  "\t\tif t := (TrustedRoot{root: root}); t != (TrustedRoot{}) && (r != nil || r == nil) {\n",
  "TestSR1aa_")

# D210's no-spec note: one row per way the note could be misread.
m("sr1 the no-spec note is used across turns", "internal/store/specnone.go",
  "\tif !ok || string(turn) != promptID {\n", "\tif !ok || len(turn) < 0 {\n", "TestSR1ae_")
m("sr1 the no-spec note ignores the directory", "internal/store/specnone.go",
  "\treturn ok && string(dig) == digest && len(b) == 0\n", "\treturn ok && len(dig) >= 0 && len(b) == 0\n", "TestSR1ae_")
m("sr1 a call naming .claude/rashomon leaves the no-spec note", "internal/hook/steerstep.go",
  "\tif p.landedStore {\n\t\tp.st.ClearNoSpec(p.sessionID)\n\t}\n", "", "TestSR1ae_")
m("sr1 a found spec is noted too", "internal/hook/specnone.go",
  "\tif sp != nil {\n\t\tst.ClearNoSpec(sessionID)\n\t\treturn\n\t}\n", "", "TestSR1ae_")
m("sr1 a missing no-spec note reads as no spec", "internal/store/specnone.go",
  "\tif err != nil {\n\t\treturn false\n\t}\n\t// One byte over",
  "\tif err != nil {\n\t\treturn true\n\t}\n\t// One byte over", "TestSR1ae_")
m("sr1 the Stop takes the no-spec note for no spec", "cmd/rashomon/recap_stop.go",
  "\tsp := hook.DiscoverSpec(p.CWD, projectDir, managedDir, hook.TrustRoot(run, st.Root(), seq))\n",
  "\tsp := hook.DiscoverSpec(p.CWD, projectDir, managedDir, hook.TrustRoot(run, st.Root(), seq))\n\tif hook.SpecNoted(st, p.SessionID, d.PromptID, p.CWD, projectDir, managedDir) {\n\t\tsp = nil\n\t}\n",
  "TestSR1ad_")

# D210's per-prompt discovery.
m("sr1 a prompt naming no path writes no note", "cmd/rashomon/recap_turn.go",
  "\t\tnoteTurnSpec(st, run, &pp)\n", "", "TestSR1af_")
m("sr1 the preflight's discovery runs twice", "cmd/rashomon/preflight.go",
  "p.CWD == \"\" || p.specNoted {\n", "p.CWD == \"\" {\n", "TestSR1ag_")

O = "internal/verify/rules_order.go"
# SPEC-RULES1 1b: order (verify/rules.go orderOver, its post raise in
# rules_raise.go and the hook's carrying of the run) and forbid_code (codeOver,
# undecided "content not read" at every applicable write).
m("sr1b order reads a failed after as qualifying", "internal/verify/rules_order.go",
  "\tcase h.exit == nil || *h.exit != 0 || !h.fg:\n",
  "\tcase h.exit == nil || !h.fg:\n",
  "TestSR1q_")
m("sr1b order ignores an invalidating write", "internal/verify/rules_order.go",
  "\t\tcase anyPath(r.InvalidatedBy, rel):\n\t\t\treturn triNo\n",
  "\t\tcase anyPath(r.InvalidatedBy, rel):\n\t\t\treturn triYes\n",
  "TestSR1q_")
m("sr1b order fires on a then the line does not show", "internal/verify/rules_order.go",
  "\t\tcase best == triMaybe || then == triMaybe:\n",
  "\t\tcase best == triMaybe:\n",
  "TestSR1r_")
m("sr1b order reads an after and a then in one line as ordered", "internal/verify/rules_order.go",
  "\t\tif lineMatch(r.After, l) != triNo {\n\t\t\tbest = triMaybe\n\t\t}\n",
  "\t\tif lineMatch(r.After, l) != triNo {\n\t\t\tbest = triYes\n\t\t}\n",
  "TestSR1r_")
m("sr1b forbid_code reads an applicable write as violated", "internal/verify/rules_order.go",
  "\treturn ruleRead{undecided: true, reason: ReasonContentNotRead}\n",
  "\treturn ruleRead{seqs: append(w.seqs, 1), reason: ReasonContentNotRead}\n",
  "TestSR1t_")
m("sr1b forbid_code is undecided at a write elsewhere", "internal/verify/rules_order.go",
  "\tif len(w.seqs) == 0 && !w.undecided {\n\t\treturn ruleRead{}\n\t}\n",
  "\tif false && len(w.seqs) == 0 && !w.undecided {\n\t\treturn ruleRead{}\n\t}\n",
  "TestSR1t_")
m("sr1b the post raises an order at a call that is not the then", "internal/verify/rules_raise.go",
  "\t\t\treturn slices.Contains(orderOver(r, v.lines, hits, &v.c, v.rels, v.at).seqs, d.Seq)\n",
  "\t\t\treturn len(orderOver(r, v.lines, hits, &v.c, v.rels, v.at).seqs) > 0\n",
  "TestSR1p_")
IMPORTS["sr1b the post raises an order at a call that is not the then"] = ("internal/verify/rules_raise.go", '\t"slices"\n', "")
m("sr1b the post raises an order with no run read", "internal/verify/rules_raise.go",
  "\t\t\tif call.Run == nil || !read {\n",
  "\t\t\tif !read {\n",
  "TestSR1p_")
m("sr1b the rules step does not carry the run to an order rule", "internal/hook/steerstep.go",
  "\t\t\tc.Run = run\n",
  "\t\t\tc.Run = nil\n",
  "TestSR1ab_")

# SPEC-RULES1 1b, #216 ruling 1: the order judge reads after_hit records
# (afterHits, afterRan, orderOver) and never an After call's line; the post
# writes one for the After call (RulesAtCall's AfterHits, the hook's keepHits)
# as it writes a rule_hit; the kind itself (store/afterhit.go, kinds.go).
m("sr1b order reads a background after as qualifying", O,
  "\tcase h.exit == nil || *h.exit != 0 || !h.fg:\n",
  "\tcase h.exit == nil || *h.exit != 0:\n",
  "TestSR1q_")
m("sr1b order reads an after in another directory as qualifying", O,
  "\tcase h.cwd != then.cwd:\n\t\treturn triNo\n",
  "\tcase h.cwd != then.cwd:\n\t\treturn triYes\n",
  "TestSR1q_")
m("sr1b order reads an after with no known directory as qualifying", O,
  "\tcase h.cwd == \"\" || then.cwd == \"\":\n\t\treturn triMaybe\n",
  "\tcase h.cwd == \"\" || then.cwd == \"\":\n\t\treturn triYes\n",
  "TestSR1v_")
m("sr1b order reads a maybe hit as qualifying", O,
  "\tcase h.maybe:\n\t\treturn triMaybe\n",
  "\tcase h.maybe:\n\t\treturn triYes\n",
  "TestSR1v_")
m("sr1b order counts a hit under any digest of its file", O,
  "h.RuleID == id && h.SourceDigest == digest && h.LandedSeq > 0",
  "h.RuleID == id && h.LandedSeq > 0",
  "TestSR1v_")
m("sr1b order counts another rule's hit", O,
  "h.RuleID == id && h.SourceDigest == digest && h.LandedSeq > 0",
  "h.SourceDigest == digest && h.LandedSeq > 0",
  "TestSR1v_")
m("sr1b order counts a hit that is not before the then", O,
  "\t\t\tif h.seq >= l.seq {\n\t\t\t\tcontinue\n",
  "\t\t\tif h.seq > l.seq {\n\t\t\t\tcontinue\n",
  "TestSR1r_")
m("sr1b order reads an after line with no hit as no after", O,
  "lineMatch(r.After, a) != triNo && !hasHit(hits, a.seq) {\n",
  "lineMatch(r.After, a) != triNo && false {\n",
  "TestSR1v_")
m("sr1b order reads a line the record withheld as an after with no hit", O,
  "a.seq < l.seq && len(a.cmds) > 0 && lineMatch",
  "a.seq < l.seq && lineMatch",
  "TestSR1s_|TestSR1v_")
m("sr1b the post reads the call's own line from the record, not the payload", "internal/verify/rules_raise.go",
  "\tif own.id != \"\" {\n\t\tv.lines = append(v.lines, own)\n\t}\n",
  "",
  "TestSR1v_")
m("sr1b the post leaves no hit for an After call", "internal/verify/rules_raise.go",
  "\t\t\tif t := lineMatch(dr.Rule.After, &ordLine); t != triNo && read {\n",
  "\t\t\tif t := lineMatch(dr.Rule.After, &ordLine); t != triNo && false {\n",
  "TestSR1u_")
m("sr1b the post leaves a hit for any Bash call", "internal/verify/rules_raise.go",
  "\t\t\tif t := lineMatch(dr.Rule.After, &ordLine); t != triNo && read {\n",
  "\t\t\tif t := lineMatch(dr.Rule.After, &ordLine); read {\n",
  "TestSR1u_")
m("sr1b the hit takes no directory from the declaration", "internal/verify/rules_raise.go",
  "CWDDigest: call.cwdDigest(), Exit: call.Exit,",
  "CWDDigest: \"\", Exit: call.Exit,",
  "TestSR1u_")
m("sr1b the hit does not say a line only may be the after", "internal/verify/rules_raise.go",
  "Foreground: call.Foreground, Maybe: t == triMaybe})",
  "Foreground: call.Foreground, Maybe: false})",
  "TestSR1u_")
m("sr1b the hook keeps no after_hit", "internal/hook/steerstep.go",
  "\tfor _, h := range after {\n",
  "\tfor _, h := range after[:0] {\n",
  "TestSPR1a_")
m("sr1b the hook records every after as foreground", "internal/hook/steerstep.go",
  "Exit: p.exitCode, Foreground: !p.backgrounded}",
  "Exit: p.exitCode, Foreground: true}",
  "TestSPR1g_")
m("sr1b the hook records no exit status at PostToolUse", "internal/hook/steerstep.go",
  "\t\tzero := 0\n\t\tc.Exit = &zero\n",
  "",
  "TestSPR1a_")
m("sr1b the hook records an exit 0 at a failure too", "internal/hook/steerstep.go",
  "\tif p.event == string(steer.EventPostToolUse) {\n\t\tzero := 0\n",
  "\tif p.event != \"\" {\n\t\tzero := 0\n",
  "TestSPR1c_")
m("sr1b the hook reads the run for every Bash call under an order spec", "internal/hook/steerstep.go",
  "\t\tif verify.OrderTouches(p.sp, c.Cmds, c.CmdsOK) {\n",
  "\t\tif true {\n",
  "TestSR1ab_")
m("sr1b the hook leaves a call that is not seen as order-bearing to a hit-less post", "internal/hook/steerstep.go",
  "\t\tif len(rules.Findings) == 0 && len(rules.Hits) == 0 && len(rules.AfterHits) == 0 {\n",
  "\t\tif len(rules.Findings) == 0 && len(rules.Hits) == 0 {\n",
  "TestSPR1a_")
m("sr1b the after_hit kind is not in the version table", "internal/store/kinds.go",
  "\tTypeAfterHit:      EventSchemaVersion,\n", "",
  "TestKindVersionsAreTheFrozenTable|TestSPR1w_")
m("sr1b the reader does not file an after_hit", "internal/store/read.go",
  "\tcase TypeAfterHit:\n\t\treturn readAfterHit(run, line, kind, version)\n", "",
  "TestSPR1w_")
m("sr1b the after_hit is stamped not critical", "internal/store/afterhit.go",
  "SchemaVersion: EventSchemaVersion, Critical: true,", "SchemaVersion: EventSchemaVersion, Critical: false,",
  "TestSPR1w_")
m("sr1b the records row omits the after_hit it holds", "internal/disclose/disclose.go",
  "; where a call matched the after pattern of an order rule of that spec, an after_hit with the same four, its exit status, whether it ran in the foreground, the keyed digest of its working directory and one bit for a line that may run the after program but does not show it, no command text;",
  ";", "TestTRU24_RecordsRowNamesTheAfterHit")
