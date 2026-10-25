m("MS the reader stops accepting schema 4", "internal/store/kinds.go",
  "\t\treturn version >= 1 && version <= CallSchemaVersion",
  "\t\treturn version >= 1 && version <= CallSchemaVersion && version != 4", "TestAcceptsAdmits")

# Masked exit status (shape.status_masked, runner_digest, the report's masked
# outcome, test runs and masked runs, the turn digest and the end-of-turn
# sentence). Each anchor appears once in its file.
# Per-kind schema versions: the call records move to 4, coverage and gaps stay
# at SessionSchemaVersion, and a terminal goes with its declaration.
SV = "TestSchema4_|TestH15_SchemaVersion"
m("MS coverage is written at the call records' version", "internal/hook/coverage.go",
  "\t\tSchemaVersion: store.SessionSchemaVersion,\n", "\t\tSchemaVersion: store.SchemaVersion,\n", SV)
m("MS a gap is written at the call records' version", "internal/store/store.go",
  "\tg.Type, g.SchemaVersion = TypeGap, SessionSchemaVersion\n",
  "\tg.Type, g.SchemaVersion = TypeGap, SchemaVersion\n",
  SV + "|TestH15_SizeCapEvictionLeavesAGap|TestH25_ForgetHostRemovesTheRecordsAndLeavesAGap")
m("MS a terminal is written at another version than its declaration", "internal/hook/handle.go",
  "\t\t\tSchemaVersion: store.SchemaVersion,\n\t\t\tRecordedAtMS:  h.now().UnixMilli(),\n\t\t\tToolUseID:     h.toolUseID,",
  "\t\t\tSchemaVersion: store.SessionSchemaVersion,\n\t\t\tRecordedAtMS:  h.now().UnixMilli(),\n\t\t\tToolUseID:     h.toolUseID,", SV)

# rules_version: which lists decided a call record's shape, and the pins that
# make a list change bump it.
RV = "TestRulesVersion_|TestSchema4_"
m("MS the declaration carries no rules version", "internal/hook/handle.go",
  "\t\tRulesVersion:   shape.RulesVersion,\n", "", RV)
m("MS the execution carries no rules version", "internal/hook/post.go",
  "\t\tRulesVersion:  shape.RulesVersion,\n", "", RV)
m("MS a runner list changes without a rules version bump", "internal/shape/masked.go",
  "\t{\"make\"}, {\"gmake\"},\n", "\t{\"make\"}, {\"gmake\"}, {\"just\"},\n", RV)
m("MS a pass word is added without a rules version bump", "internal/report/account.go",
  "\t\"builds\",\n}", "\t\"builds\", \"works\",\n}", RV)

SH = "TestStatusMasked|TestRunnerDigest|TestMasked_"
# The parser runs the line twice in the abstract, the runner failing and
# passing (statusMasked); each mutant breaks one rule of that run.
m("MS a pipe after the runner no longer masks it", "internal/shape/masked_eval.go",
  "\t\ts := statuses[len(statuses)-1]\n", "\t\ts := statuses[0]\n", SH)
m("MS set -o pipefail is not read", "internal/shape/masked_eval.go",
  "\t\t\t\tst.pipefail = o.on\n", "\t\t\t\tst.pipefail = false\n", SH)
m("MS set -e is not read", "internal/shape/masked_eval.go",
  "\t\t\t\tst.errexit = o.on\n", "\t\t\t\tst.errexit = false\n", SH)
m("MS errexit is not ignored before && or ||", "internal/shape/masked_eval.go",
  "\t\tignored := noErr || k < len(ao.pipes)-1 || pl.neg\n", "\t\tignored := noErr || pl.neg\n", SH)
m("MS && masks the runner before it", "internal/shape/masked_eval.go",
  "if ao.ops[k-1] == \"&&\" && st.status != 0 ||", "if ao.ops[k-1] == \"&&\" && false ||", SH)
m("MS ; and a newline never mask", "internal/shape/masked_eval.go",
  "\t\tev.andOr(st, it.ao, noErr)\n", "\t\tev.andOr(st, it.ao, noErr)\n\t\tif st.status > 0 {\n\t\t\treturn\n\t\t}\n", SH)
m("MS a trailing ; masks", "internal/shape/masked_parse.go",
  "\t\tcase p.op(p.pos, \";\"):\n\t\t\tp.pos++\n", "\t\tcase p.op(p.pos, \";\"):\n\t\t\tp.pos++\n\t\t\tit.bg = p.atTerminator()\n", SH)
m("MS || never masks", "internal/shape/masked_eval.go",
  "|| ao.ops[k-1] == \"||\" && st.status == 0 {", "|| ao.ops[k-1] == \"||\" {", SH)
m("MS an exit after || is not read", "internal/shape/masked_eval.go",
  "\tcase opExit:\n\t\tswitch len(c.args) {", "\tcase -1:\n\t\tswitch len(c.args) {", SH)
m("MS an exit does not end the line", "internal/shape/masked_eval.go",
  "\t\tst.exited = true\n\tcase opRunner:", "\tcase opRunner:", SH)
m("MS false after || is not read", "internal/shape/masked_eval.go",
  "\tcase opFalse:\n\t\tst.status = 1\n", "\tcase opFalse:\n\t\tst.status = 0\n", SH)
m("MS a group is run apart from the line", "internal/shape/masked_eval.go",
  "\tcase *group:\n\t\tev.list(st, c.body, noErr)\n", "\tcase *group:\n\t\tev.list(st.copy(), c.body, noErr)\n", SH)
m("MS a subshell's exit ends the line", "internal/shape/masked_eval.go",
  "\t\tst.status = sub.status\n\tcase *group:", "\t\tst.status, st.exited = sub.status, sub.exited\n\tcase *group:", SH)
m("MS a failed condition with no else keeps its status", "internal/shape/masked_eval.go",
  "\t\t\tev.list(st, *c.els, noErr)\n\t\t\treturn\n\t\t}\n\t\tst.status = 0\n", "\t\t\tev.list(st, *c.els, noErr)\n\t\t\treturn\n\t\t}\n", SH)
m("MS ! is not read", "internal/shape/masked_eval.go",
  "\tif pl.neg && st.status >= 0 {", "\tif false {", SH)
m("MS a runner sent to the background with & is not masked", "internal/shape/masked_eval.go",
  "\t\t\tev.andOr(st.copy(), it.ao, true)\n\t\t\tst.status = 0\n", "\t\t\tev.andOr(st, it.ao, noErr)\n", SH)
m("MS the same runner twice is two runners", "internal/shape/masked_simple.go",
  "\tg, seen := p.groups[key]\n", "\tg, seen := p.groups[key]\n\tseen = false\n", SH)
m("MS a status kept in a variable is not read", "internal/shape/masked_eval.go",
  "\t\t\tst.vars[a.name] = ev.value(st, a.val)\n", "\t\t\tst.vars[a.name] = -1\n", SH)
m("MS a loop that holds a runner is read as succeeding", "internal/shape/masked_parse.go",
  "\t\tcase opRunner, opExit, opStatusRead, opSet, opAssign, opCd:\n\t\t\tl.unsure = true\n", "", SH)
m("MS a shell's -c line is read as a word", "internal/shape/masked_simple.go",
  "w[1] != '-' && strings.ContainsRune(w, 'c') {", "w[1] != '-' && strings.ContainsRune(w, 'c') && false {", SH)
m("MS a line the run is not sure of is none", "internal/shape/masked.go",
  "\t\t\t// Not sure: null.\n\t\t\treturn nil, nil, nil\n", "\t\t\tcontinue\n", SH)
m("MS a runner the run never reaches is judged", "internal/shape/masked.go",
  "\t\tif f < 0 || pass < 0 || !ran {", "\t\tif f < 0 || pass < 0 || !ran && false {", SH)
m("MS an exit 0 after || keeps the failure", "internal/shape/masked_eval.go",
  "\t\t\tst.status = ev.value(st, c.args[0])\n", "\t\t\tif v := ev.value(st, c.args[0]); v != 0 {\n\t\t\t\tst.status = v\n\t\t\t}\n", SH)
m("MS an exit 256 after || keeps the failure", "internal/shape/masked_simple.go",
  "\treturn value{kind: valLiteral, n: int(((n % 256) + 256) % 256)}", "\treturn value{kind: valLiteral, n: int(n)}", SH)
m("MS an exit in a pipe after || is the line's exit", "internal/shape/masked_eval.go",
  "\t\t\tstatuses[n] = sub.status\n", "\t\t\tif sub.exited {\n\t\t\t\tst.status, st.exited = sub.status, true\n\t\t\t\treturn\n\t\t\t}\n\t\t\tstatuses[n] = sub.status\n", SH)
m("MS false | x after || keeps the failure", "internal/shape/masked_eval.go",
  "\t\tif st.pipefail {\n\t\t\ts = 0\n", "\t\tif true {\n\t\t\ts = 0\n", SH)
m("MS errexit fires in the zsh run", "internal/shape/masked_eval.go",
  "if st.errexit && !ignored && !ev.zsh && st.status != 0 {", "if st.errexit && !ignored && st.status != 0 {", SH)
m("MS bash and zsh disagreeing is not null", "internal/shape/masked_eval.go",
  "\tif zf < 0 || zpass < 0 || (f == 0 || f == pass) != (zf == 0 || zf == zpass) {", "\tif zf < 0 || zpass < 0 {", SH)
m("MS a line that sets errexit skips the zsh run", "internal/shape/masked_eval.go",
  "\tif !p.errexit {\n\t\t// Without errexit the two shells run it alike.\n", "\tif true {\n\t\t// Without errexit the two shells run it alike.\n", SH)
m("MS the line runs only as bash runs it", "internal/shape/masked_eval.go",
  "\tev := &evaluator{p: p, target: g, fail: fail, zsh: zsh}\n", "\tev := &evaluator{p: p, target: g, fail: fail}\n", SH)
m("MS set -opipefail is read as short options", "internal/shape/masked_simple.go",
  "\t\t\t\tif k < len(w)-2 {\n", "\t\t\t\tif k < 0 {\n", SH)
m("MS the pipeline after && ends at its first stage", "internal/shape/masked_parse.go",
  "\t\tao.pipes = append(ao.pipes, pl)\n", "\t\tif len(ao.pipes) > 0 {\n\t\t\tpl.stages = pl.stages[:1]\n\t\t}\n\t\tao.pipes = append(ao.pipes, pl)\n", SH)
m("MS a here-string is read as a here-document", "internal/shape/tokenize.go",
  "\tif l.docs && c == '<' && j-i == 2 && (j >= len(s) || s[j] != '<') {", "\tif l.docs && c == '<' && j-i == 2 {", SH)
m("MS a here-document after an expansion is read", "internal/shape/tokenize.go",
  "\t\tif l.grouped || l.expanded {", "\t\tif l.grouped {", SH)
m("MS only the first here-document on a line is skipped", "internal/shape/tokenize.go",
  "\t\tl.pending = append(l.pending, j)\n", "\t\tif len(l.pending) == 0 {\n\t\t\tl.pending = append(l.pending, j)\n\t\t}\n", SH)
m("MS a group's close is a word of the runner digest", "internal/shape/masked_parse.go",
  "\t\tcase t.text == \")\":\n\t\t\t// A subshell's close: the command ends before it.\n\t\t\treturn j, true\n", "", SH)
m("MS make is no build runner", "internal/shape/masked.go",
  "\t{\"make\"}, {\"gmake\"},\n", "\t{\"gmake\"},\n", SH)
m("MS a build masked before a test wins", "internal/shape/masked.go",
  "\t\t\tif p.runners[g].kind == MaskedTest {\n\t\t\t\tk = MaskedTest\n", "\t\t\tif p.runners[g].kind == MaskedTest && false {\n\t\t\t\tk = MaskedTest\n", SH)
m("MS the runner's arguments run to the end of the line", "internal/shape/runner.go",
  "\t\treturn !refusesRun(refusalsOf(c), toks[:i], toks[from+len(c)-1:end])", "\t\treturn !refusesRun(refusalsOf(c), toks[:i], toks[from+len(c)-1:])", SH)
m("MS here-document bodies are read as commands", "internal/shape/tokenize.go",
  "\t\tl.pending = append(l.pending, j)\n", "\t\t_ = j\n", SH)
m("MS a comment is read as words in the list walk", "internal/shape/tokenize.go",
  "\t\tcase docs && c == '#' && !l.w.started:", "\t\tcase false && docs && c == '#' && !l.w.started:", SH)
m("MS the runner digest keeps a redirection's target", "internal/shape/masked_parse.go",
  "\t\t\tj = operatorEnd(toks, j) + 1\n", "\t\t\tj = operatorEnd(toks, j)\n", SH)
m("MS the runner digest runs to the end of the line", "internal/shape/masked_simple.go",
  "\tkey := p.runnerWords(ri, e)\n", "\tkey := p.runnerWords(ri, len(toks))\n", SH)
m("MS the runner digest is not keyed", "internal/shape/masked_eval.go",
  "\td := digest(key, \"\\x00runner\",", "\td := digest(nil, \"\\x00runner\",", SH)
m("MS a runner whose failure reaches the line carries no runner digest", "internal/shape/masked.go",
  "\t\td, rk := p.runnerOf(reach, key)\n\t\treturn &none, d, rk\n", "\t\treturn &none, nil, nil\n", SH)
m("MS two hidden runners carry one's digest", "internal/shape/masked.go",
  "\t\tif len(hidden) > 1 {\n\t\t\t// No one later run follows up two runners.\n\t\t\treturn &k, nil, nil\n\t\t}\n", "", SH)
m("MS a runner after a cd keeps its digest", "internal/shape/masked_eval.go",
  "\tif p.runners[g].displaced {\n\t\treturn nil\n\t}\n", "", SH + "|TestFoldKeepsRunnerIdentity")
m("MS a folded leading cd displaces the runner", "internal/shape/masked_eval.go",
  "\t\tif s.op != opCd || s.start < after {", "\t\tif s.op != opCd {", SH)
m("MS a fold the two readings step over apart is passed over", "internal/shape/program.go",
  "\tif !slices.Equal(toks[:after], fold) {\n", "\tif false && !slices.Equal(toks[:after], fold) {\n",
  "TestFoldKeepsRunnerIdentity|TestRubricTEST1_LeadingFoldSeeds")
m("MS an expansion in the runner's words loses its digest", "internal/shape/masked_eval.go",
  "\t\tif t.opaque {\n\t\t\tw = \"\\x02\" + w\n\t\t}\n", "\t\tif t.opaque {\n\t\t\treturn \"\"\n\t\t}\n", SH)
m("MS a backslash at a comment's end joins the next line", "internal/shape/join.go",
  "\t\tcase comments && c == '#' && !inSingle && !inDouble && wordStart(b.String()):", "\t\tcase false:", SH)
m("MS mvn's build row refuses -DskipTests", "internal/shape/verb.go",
  "\t\tif len(c) == 1 {\n\t\t\t// The build row: a flag that skips the tests still builds.\n\t\t\treturn \"mvn build\"\n\t\t}\n", "", SH)
m("MS python -m unittest is not a test runner", "internal/shape/runner.go",
  "\t{\"python\", \"-m\", \"unittest\"}, {\"python3\", \"-m\", \"unittest\"},\n", "", "TestUnittestIsATestRunner")

RP = "TestMasked_"
m("MS a masked call's ok is an ok row", "internal/report/timeline.go",
  "\tif outcome == store.ExecOK && statusMasked(d) {", "\tif false {", RP)
m("MS a masked call's failure is not a failed row", "internal/report/timeline.go",
  "\tif outcome == store.ExecOK && statusMasked(d) {", "\tif statusMasked(d) {", RP)
m("MS a masked build is not masked", "internal/report/timeline.go",
  "\treturn m != nil && (*m == shape.MaskedTest || *m == shape.MaskedBuild)", "\treturn m != nil && *m == shape.MaskedTest", RP)
m("MS the unknown legend does not name masked rows", "internal/report/timeline_text.go",
  "; or ok as a line that did not return a build or test run's exit status)", ")", RP)
m("MS a masked test run counts as ok", "internal/report/testbending.go",
  "\t\t\t\tout.Runs++\n\t\t\t\t*out.Masked++", "\t\t\t\tout.Runs++\n\t\t\t\tout.OK++", RP)
m("MS a masked test run is not counted", "internal/report/testbending.go",
  "\t\tif m := d.Shape.StatusMasked; m != nil && *m == shape.MaskedTest {", "\t\tif m := d.Shape.StatusMasked; false && m != nil {", RP)
m("MS a masked line is a run with a result", "internal/report/testbending.go",
  "d.Shape.VerbClass == shape.VerbTest && !statusMasked(d) && (outcome", "d.Shape.VerbClass == shape.VerbTest && (outcome", RP)
m("MS a masked line is no edit", "internal/report/testbending.go",
  "\tif d.Shape.MayWrite || statusMasked(d) {", "\tif d.Shape.MayWrite {", RP)
m("MS the test runs line does not say how many were masked", "internal/report/text.go",
  "\tif t.Masked != nil && *t.Masked > 0 {", "\tif false {", RP)
m("MS the session report does not print masked runs", "internal/report/text.go",
  "\t\twriteMaskedRuns(b, *sess.MaskedRuns)\n", "", RP)
m("MS masking is measured before schema 4", "internal/report/testbending.go",
  "\t\tif d.SchemaVersion >= 4 || d.Shape.StatusMasked != nil {", "\t\tif d.SchemaVersion >= 3 || d.Shape.StatusMasked != nil {", RP)
m("MS the session's masked runs are built for a session that never measured them", "internal/report/account.go",
  "\tif !measuresMasking(run) {\n\t\treturn nil\n\t}\n\tm := BuildMaskedRuns", "\tm := BuildMaskedRuns", RP)
m("MS a plain follow-up in another directory counts", "internal/report/account.go",
  "\t\tk := runner{*r, d.CWDDigest}", "\t\tk := runner{*r, \"\"}", RP)
m("MS an earlier plain run counts as a follow-up", "internal/report/account.go",
  "ok && s > d.Seq {", "ok && s >= 0 {", RP)
m("MS a backgrounded call counts as ended", "internal/report/account.go",
  "\t\treturn rec != nil && !rec.Backgrounded && rec.Outcome == result", "\t\treturn rec != nil && rec.Outcome == result", RP)
m("MS a later masked run is a follow-up", "internal/report/account.go",
  "\t\tif m == nil || *m != shape.MaskedNone || r == nil {", "\t\tif m == nil || *m != shape.MaskedNone && *m == \"\" || r == nil {", RP)
m("MS a plain follow-up that failed is no follow-up", "internal/report/account.go",
  "\t\tif !ended(d, store.ExecOK) && !ended(d, store.ExecFailed) {", "\t\tif !ended(d, store.ExecOK) {", RP)
m("MS masked runs fire over a failure word", "internal/report/account.go",
  "\tout.Fires = out.PassClaimed && !failure", "\tout.Fires = out.PassClaimed && (failure || !failure)", RP)
m("MS masked runs fire with no pass claimed", "internal/report/account.go",
  "\tout.Fires = out.PassClaimed && !failure", "\tout.Fires = !failure", RP)

# Masking judged on the command that ran: the execution carries it, and the
# report and the digest take it over the declaration's (MaskingAsRan).
m("MS the post hook keeps no masking", "internal/hook/post.go",
  "\t\trec.StatusMasked, rec.RunnerDigest, rec.RunnerKind = s.StatusMasked, s.RunnerDigest, s.RunnerKind\n", "", RP)
m("MS the report judges the declared line", "internal/report/assemble.go",
  "\tMaskingAsRan(run)\n", "", RP)
m("MS the digest judges the declared line", "internal/digest/digest.go",
  "\treport.MaskingAsRan(run)\n", "", RP)
m("MS the timeline judges the declared line", "internal/report/timeline.go",
  "\t\td.Shape.StatusMasked, d.Shape.RunnerDigest, d.Shape.RunnerKind = rec.StatusMasked, rec.RunnerDigest, rec.RunnerKind\n", "\t\td.Shape.RunnerDigest, d.Shape.RunnerKind = rec.RunnerDigest, rec.RunnerKind\n", RP)
m("MS masking is taken from an execution before schema 4", "internal/report/timeline.go",
  "\t\tif rec == nil || rec.SchemaVersion < 4 || rec.ExecutedDigest == \"\" {", "\t\tif rec == nil || rec.ExecutedDigest == \"\" {", RP)
m("MS masking is taken from an execution that saw no input", "internal/report/timeline.go",
  "\t\tif rec == nil || rec.SchemaVersion < 4 || rec.ExecutedDigest == \"\" {", "\t\tif rec == nil || rec.SchemaVersion < 4 {", RP)

m("MS the digest builds no masked runs", "internal/digest/digest.go",
  "\td.MaskedRuns = report.BuildMaskedRuns(turnRun, acct)\n", "", "TestBuild_MaskedRuns|TestMasked_")
m("MS the digest counts the session's masked runs", "internal/digest/digest.go",
  "\td.MaskedRuns = report.BuildMaskedRuns(turnRun, acct)\n", "\td.MaskedRuns = report.BuildMaskedRuns(run, acct)\n", "TestBuild_MaskedRuns|TestMasked_")
m("MS the end-of-turn line never says masked", "internal/recap/recap.go",
  "\tif m := d.MaskedRuns; m.Fires {", "\tif m := d.MaskedRuns; false && m.Fires {", "TestLineMaskedRuns|TestMasked_")
m("MS the end-of-turn line says masked on the count alone", "internal/recap/recap.go",
  "\tif m := d.MaskedRuns; m.Fires {", "\tif m := d.MaskedRuns; m.Runs > 0 {", "TestLineMaskedRuns|TestMasked_")
m("MS the masked sentence has no singular", "internal/recap/recap.go",
  "\tif n == 1 {\n\t\treturn \"pass claimed; exit status of 1 build", "\tif false {\n\t\treturn \"pass claimed; exit status of 1 build", "TestLineMaskedRuns")
m("MS a pass word is matched inside another word", "internal/report/account.go",
  "\t\t\tcase !negated && slices.Contains(passVocabulary, w):", "\t\t\tcase !negated && slices.ContainsFunc(passVocabulary, func(p string) bool { return strings.Contains(w, p) }):", RP)
m("MS a negated pass word is a claim", "internal/report/account.go",
  "\t\t\tcase negations[w] || strings.HasSuffix(w, \"n't\"):\n\t\t\t\tnegated = true\n", "\t\t\tcase negations[w] || strings.HasSuffix(w, \"n't\"):\n\t\t\t\tnegated = false\n", RP)
m("MS a negation reaches across a clause", "internal/report/account.go",
  "\tfor _, clause := range strings.FieldsFunc(msg, func(r rune) bool { return strings.ContainsRune(clauseBreaks, r) }) {\n\t\tnegated := false\n",
  "\tnegated := false\n\tfor _, clause := range strings.FieldsFunc(msg, func(r rune) bool { return strings.ContainsRune(clauseBreaks, r) }) {\n", RP)
m("MS and and but do not end a negation", "internal/report/account.go",
  "\t\t\tcase w == \"and\" || w == \"but\":\n\t\t\t\tnegated = false\n", "", RP)
m("MS none, nothing, neither, nor and without negate nothing", "internal/report/account.go",
  "\t\"none\": true, \"nothing\": true, \"neither\": true, \"nor\": true, \"without\": true,\n", "", RP)
m("MS a typographic apostrophe is not read", "internal/report/account.go",
  "\tmsg = strings.ReplaceAll(strings.ToLower(msg), \"\\u2019\", \"'\")", "\tmsg = strings.ToLower(msg)", RP)
