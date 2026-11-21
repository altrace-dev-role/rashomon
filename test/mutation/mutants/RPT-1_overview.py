# RPT-1 (the overview a bare `rashomon report` prints): which runs print the
# overview of one session, how it is painted, and what each of its sentences
# rests on. The dispatch rows break one branch of overviewTarget each (rows a
# to e); the paint rows break one clause of useColor. The internal/report rows
# break one branch of the Known now row, the order and branches of the Outcome
# verified row, one clause of the recording's state and the lessons-sent
# count. Every row keeps the file's imports in use, so each break builds.
#
# Not here: overviewTarget's `case err != nil` for a bare run. Its only cause a
# test can raise is a store that will not open, and today's text then fails on
# the same open with the same error, so the break is equivalent there; a
# checkpoint scan that fails on a store that opens needs a fault the store
# offers no way to inject. useColor's `!ok` is equivalent too: Stat on a nil
# *os.File returns os.ErrInvalid, which already reads as no paint. Its Windows
# clause cannot be judged on darwin or linux.
O = "cmd/rashomon/report_overview.go"
R = "cmd/rashomon/report.go"

m("RPT-1 --overview with another view is not refused", O,
  "\t\tcase f.on && overview:\n",
  "\t\tcase false && f.on && overview:\n",
  "TestRPT1r_|TestRPT1_RowE_")
m("RPT-1 a view flag without --overview still prints the overview", O,
  "\t\tcase f.on:\n\t\t\treturn \"\", nil\n",
  "\t\tcase false && f.on:\n\t\t\treturn \"\", nil\n",
  "TestRPT1_RowB_|TestRPT1_RowD_")
m("RPT-1 --overview --session shows the last session instead", O,
  "\tcase overview && sessionID != \"\":\n\t\treturn sessionID, nil\n",
  "\tcase overview && sessionID != \"\":\n\t\treturn lastSession()\n",
  "TestRPT1_RowC_|TestRPT1s_")
m("RPT-1 --overview alone prints today's text", O,
  "\tcase overview:\n\t\treturn lastSession()\n",
  "\tcase overview:\n\t\treturn \"\", nil\n",
  "TestRPT1_RowC_|TestRPT1r_")
m("RPT-1 a named session prints the overview", O,
  "\tcase sessionID != \"\":\n\t\treturn \"\", nil\n",
  "\tcase false && sessionID != \"\":\n\t\treturn \"\", nil\n",
  "TestRPT1_RowB_|TestRPT1s_")
m("RPT-1 a bare report with no store is an error", O,
  "\tcase errors.Is(err, store.ErrNoStore):\n\t\treturn \"\", nil\n",
  "\tcase errors.Is(err, store.ErrNoStore):\n\t\treturn \"\", err\n",
  "TestRPT1_RowA_|TestH29_")
m("RPT-1 a bare report from a project with no checkpoint is an error", O,
  "\tcase !ok:\n\t\treturn \"\", nil\n",
  "\tcase !ok:\n\t\treturn \"\", errors.New(\"no checkpoint\")\n",
  "TestRPT1_RowA_")
m("RPT-1 a bare report with a checkpoint prints today's text", O,
  "\treturn id, nil\n}\n",
  "\t_ = id\n\treturn \"\", nil\n}\n",
  "TestRPT1_RowA_")
m("RPT-1 --redact does not reach the overview", O,
  "\tif redact {\n\t\trep = report.Redact(rep, key)\n\t}\n",
  "\t_ = key\n",
  "TestRPT1_RowA_")
m("RPT-1 the overview is painted into a pipe", O,
  "\tif useColor(stdout) {\n",
  "\tif true || useColor(stdout) {\n",
  "TestRPT1_RowA_")
m("RPT-1 NO_COLOR does not stop the paint", O,
  "!ok || os.Getenv(\"NO_COLOR\") != \"\" ||",
  "!ok || false && os.Getenv(\"NO_COLOR\") != \"\" ||",
  "TestRPT1q_")
m("RPT-1 TERM=dumb does not stop the paint", O,
  "|| os.Getenv(\"TERM\") == \"dumb\" ||",
  "|| false && os.Getenv(\"TERM\") == \"dumb\" ||",
  "TestRPT1q_")
m("RPT-1 the paint follows a file that is not a terminal", O,
  "\treturn err == nil && fi.Mode()&os.ModeCharDevice != 0\n",
  "\treturn err == nil && fi.Mode()&os.ModeCharDevice == 0\n",
  "TestRPT1q_")
m("RPT-1 the dispatch never prints the overview", R,
  "\tif view != \"\" {\n",
  "\tif false && view != \"\" {\n",
  "TestRPT1_RowA_|TestRPT1_RowC_")
m("RPT-1 --ledger does not refuse --overview", R,
  "{nonoTrail != \"\", \"--nono-audit\"}, {overview, \"--overview\"}, {full, \"--full\"}} {",
  "{nonoTrail != \"\", \"--nono-audit\"}, {full, \"--full\"}} {\n\t\t\t_ = overview",
  "TestRPT1r_|TestRPT1_RowE_")
m("RPT-1 --ledger does not refuse --full", R,
  "{nonoTrail != \"\", \"--nono-audit\"}, {overview, \"--overview\"}, {full, \"--full\"}} {",
  "{nonoTrail != \"\", \"--nono-audit\"}, {overview, \"--overview\"}} {",
  "TestRPT1r_")

K = "internal/report/overview_sections.go"
F = "internal/report/overview_attention.go"
V = "internal/report/overview.go"

m("RPT-1 Known now hides the hosts of a baseline read when no family is established", K,
  "\tcase k.Retained.Prior && nv.Available:\n",
  "\tcase false && k.Retained.Prior && nv.Available:\n",
  "TestRPT1t_")
m("RPT-1 Known now counts zero families when none is established", K,
  "\tcase k.Retained.Prior:\n",
  "\tcase false && k.Retained.Prior:\n",
  "TestRPT1b_|TestRPT1h_")
m("RPT-1 Known now hides the hosts of a baseline read", K,
  "\tcase nv.Available:\n",
  "\tcase false && nv.Available:\n",
  "TestRPT1a_|TestRPT1p_")
m("RPT-1 a baseline not available reads as a host count", K,
  "\t\tfmt.Fprintf(&b, \"%d gate famil%s required (index). The host count of the network baseline is not known.\", f, yIes(f))\n",
  "\t\tfmt.Fprintf(&b, \"%d gate famil%s required (index) and %d host%s in this project's network baseline.\", f, yIes(f), n, plural(n))\n",
  "TestRPT1p_")
m("RPT-1 no finding reads as 0 of 0 resolved", F,
  "\tcase f.n == 0:\n\t\treturn \"–\", \"No finding was recorded, so there was nothing to settle.\"\n",
  "\tcase false && f.n == 0:\n\t\treturn \"–\", \"No finding was recorded, so there was nothing to settle.\"\n",
  "TestRPT1b_|TestRPT1h_")
m("RPT-1 one finding that names no gate reads in the plural", F,
  "\tcase f.unchecked == f.n && f.n == 1:\n",
  "\tcase false && f.unchecked == f.n && f.n == 1:\n",
  "TestRPT1t_")
m("RPT-1 findings that all name no gate read as a result row", F,
  "\tcase f.unchecked == f.n:\n",
  "\tcase false && f.unchecked == f.n:\n",
  "TestRPT1a_")
m("RPT-1 a finding in a state recap does not write goes unnamed", F,
  "\tif f.other > 0 {\n\t\tparts = append(parts, f.otherState())\n",
  "\tif false && f.other > 0 {\n\t\tparts = append(parts, f.otherState())\n",
  "TestRPT1t_")
m("RPT-1 a finding in a state recap does not write counts as resolved", F,
  "\t\tdefault:\n\t\t\tf.other++\n",
  "\t\tdefault:\n\t\t\tf.resolved++\n\t\t\tcontinue\n",
  "TestRPT1t_")
m("RPT-1 a call missing from the store leaves the recording complete", V,
  "mismatch = mismatch || len(t.MissingFromStore) > 0 ||",
  "mismatch = mismatch || false && len(t.MissingFromStore) > 0 ||",
  "TestRPT1g_")
m("RPT-1 a call missing from the transcript leaves the recording complete", V,
  "|| len(t.MissingFromTranscript) > 0\n",
  "|| false && len(t.MissingFromTranscript) > 0\n",
  "TestRPT1g_")
m("RPT-1 a result with no recorded execution leaves the recording complete", V,
  "unexecuted = unexecuted || len(t.ExecutedButUnrecorded) > 0\n",
  "unexecuted = unexecuted || false && len(t.ExecutedButUnrecorded) > 0\n",
  "TestRPT1g_")
m("RPT-1 a gap record leaves the recording complete", V,
  "add(ReasonGap, len(s.Gaps) > 0)",
  "add(ReasonGap, false && len(s.Gaps) > 0)",
  "TestRPT1g_")
m("RPT-1 a transcript not read counts as read", V,
  "\t\tif t.Status != TranscriptRead {\n\t\t\tcontinue\n",
  "\t\tif false && t.Status != TranscriptRead {\n\t\t\tcontinue\n",
  "TestRPT1g_|TestRPT1h_")
m("RPT-1 coverage not verified with no reason reads complete", V,
  "case s.Coverage.State != store.StateVerified || len(reasons) > 0:",
  "case false && s.Coverage.State != store.StateVerified || len(reasons) > 0:",
  "TestRPT1g_")
m("RPT-1 a reason under verified coverage reads complete", V,
  "case s.Coverage.State != store.StateVerified || len(reasons) > 0:",
  "case s.Coverage.State != store.StateVerified || false && len(reasons) > 0:",
  "TestRPT1g_")
m("RPT-1 no transcript read reads complete", V,
  "\tcase len(read) == 0:\n",
  "\tcase false && len(read) == 0:\n",
  "TestRPT1g_|TestRPT1h_")
m("RPT-1 a reason both coded and found prints twice", V,
  "if on && !slices.Contains(reasons, code) {",
  "if on {",
  "TestRPT1g_")
m("RPT-1 every note delivered counts as a lesson sent", K,
  "if d.Pattern == string(pattern.LearnedFact) {",
  "if d.Pattern != string(pattern.LearnedFact) {",
  "TestRPT1a_|TestRPT1t_")
m("RPT-1 a lesson sent is not counted", K,
  "\tcase sent > 0:\n",
  "\tcase false && sent > 0:\n",
  "TestRPT1t_")
