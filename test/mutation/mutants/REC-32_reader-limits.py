# REC-32: over-long lines are skipped and counted, never fatal; Evict and
# ForgetHost (and the window forget beside it) continue past a run they cannot
# read, and the probe says when eviction could not finish instead of throwing
# the error away.
m("REC-32 the reader returns the line-too-long error", "internal/store/lines.go",
  "\t\tcase over:\n\t\t\tfn(nil)\n", "\t\tcase over:\n\t\t\treturn errLineTooLong\n", "TestREC32_")
m("REC-32 an over-long line is dropped without a count", "internal/store/lines.go",
  "\t\tcase over:\n\t\t\tfn(nil)\n", "\t\tcase over:\n", "TestREC32_OverlongLineSkippedAndCounted")
m("REC-32 a line of exactly maxLine bytes is skipped", "internal/store/lines.go",
  "\t\t\tif len(line)+len(chunk) > maxLine {", "\t\t\tif len(line)+len(chunk) >= maxLine {", "TestREC32_OverlongLineSkippedAndCounted")
m("REC-32 forget rewrites a file it could not read whole", "internal/store/gaps.go",
  "\tif err == nil && tooLong {\n", "\tif err == nil && tooLong && false {\n", "TestREC32b_")
m("REC-32 forget stops at the first run it cannot read", "internal/store/gaps.go",
  "\t\tif err != nil {\n\t\t\terrs = append(errs, fmt.Errorf(\"run %s: %w\", name, err))\n\t\t}\n",
  "\t\tif err != nil {\n\t\t\treturn gaps, fmt.Errorf(\"run %s: %w\", name, err)\n\t\t}\n", "TestREC32b_")
m("REC-32 eviction stops at the run it cannot read", "internal/store/evict.go",
  "\t\tif err != nil {\n\t\t\terrs = append(errs, fmt.Errorf(\"run %s: %w\", c.name, err))\n\t\t}\n",
  "\t\tif err != nil {\n\t\t\treturn gaps, fmt.Errorf(\"run %s: %w\", c.name, err)\n\t\t}\n", "TestREC32_EvictContinuesPastUnreadableRun")
m("REC-32 eviction counts a run with a skipped line", "internal/store/evict.go",
  "\tif err != nil || run.Skipped > 0 {\n\t\tg.RemovedRecords = RemovedUnknown\n", "\tif err != nil {\n\t\tg.RemovedRecords = RemovedUnknown\n",
  "TestREC32_EvictContinuesPastUnreadableRun")
m("REC-32 eviction claims a count for a run it could not read", "internal/store/evict.go",
  "\tif err != nil || run.Skipped > 0 {\n\t\tg.RemovedRecords = RemovedUnknown\n", "\tif run.Skipped > 0 {\n\t\tg.RemovedRecords = RemovedUnknown\n",
  "TestREC32_EvictContinuesPastUnreadableRun")
m("REC-32 the probe swallows eviction's error", "internal/hook/probe.go",
  "\t\t_, err := st.Evict(capSetting.Bytes, sessionID, now())\n\t\treturn err\n",
  "\t\t_, err := st.Evict(capSetting.Bytes, sessionID, now())\n\t\t_ = err\n\t\treturn nil\n", "TestREC32d_")
m("REC-32 probe is silent when eviction fails", "cmd/rashomon/main.go",
  "\t\t\tfmt.Fprintln(stderr, evictionLine)\n", "", "TestREC32d_")
m("REC-32 the report prints an unknown count as -1", "internal/report/text.go",
  "\t\t\tif g.RemovedRecords < 0 {\n", "\t\t\tif false {\n", "TestREC32c_")
m("REC-32 forget --host prints nothing when one run fails", "cmd/rashomon/main.go",
  "\tgaps, forgetErr := st.ForgetHost(host, time.Now())\n",
  "\tgaps, forgetErr := st.ForgetHost(host, time.Now())\n\tif forgetErr != nil {\n\t\treturn forgetErr\n\t}\n", "TestREC32e_")
m("REC-32 forget --host succeeds while a run still holds the host", "cmd/rashomon/main.go",
  "\tfmt.Fprintln(stdout)\n\treturn errors.Join(forgetErr, clearErr)\n", "\tfmt.Fprintln(stdout)\n\treturn clearErr\n", "TestREC32e_")
m("REC-32 the ordered append goes on after a torn tail", "internal/store/store.go",
  "\tif err := appendHealed(f, line, true); err != nil {\n", "\tif _, err := f.Write(line); err != nil {\n",
  "TestREC32_TornTailNeverSwallowsTheNextRecord")
# A forget --host that failed before touching a run, its store-level record
# included, prints no line: "later reports still leave it out" would be false.
m("REC-32 forget --host reports a forget that failed before any run", "cmd/rashomon/main.go",
  "\tif forgetErr != nil && !errors.Is(forgetErr, store.ErrForgetIncomplete) {",
  "\tif false && forgetErr != nil && !errors.Is(forgetErr, store.ErrForgetIncomplete) {",
  "TestREC32f_")
# Nor does it clear a baseline: the error would be the only thing said, and the
# command would have changed what the next report calls new.
m("REC-32 forget --host clears baselines after a forget that failed before any run", "cmd/rashomon/main.go",
  "\t\t// command changed nothing it does not say.\n\t\treturn forgetErr\n",
  "\t\t// command changed nothing it does not say.\n\t\t_, _, _ = clearBaselines(st.Root(), host)\n\t\treturn forgetErr\n",
  "TestForgetHost_AnEarlyFailureClearsNoBaselineAndPrintsNothing")
# A partial forget prints "later reports still leave it out", which the
# store-level record makes true: it must land before the pass over the runs
# that can end in ErrForgetIncomplete, not only after a pass that finished.
m("REC-32 forget --host records the host only after every run was rewritten", "internal/store/gaps.go",
  "\tif err := s.recordForgottenHost(s.HostDigest(host), now); err != nil {\n\t\treturn nil, err\n\t}\n"
  "\treturn s.forgetEach(func(dir string) (*Gap, error) { return s.forgetHostInRun(dir, host, now) })\n",
  "\tgaps, err := s.forgetEach(func(dir string) (*Gap, error) { return s.forgetHostInRun(dir, host, now) })\n"
  "\tif err != nil {\n\t\treturn gaps, err\n\t}\n\treturn gaps, s.recordForgottenHost(s.HostDigest(host), now)\n",
  "TestForgetHost_APartialForgetSaysLaterReportsLeaveItOutOnlyWithItsRecord")
m("REC-32 eviction stops at a run it cannot list", "internal/store/evict.go",
  "\t\t\tplan.unlisted = append(plan.unlisted, fmt.Errorf(\"run %s: %w\", e.Name(), err))\n\t\t\tcontinue\n",
  "\t\t\treturn plan, err\n", "TestREC32_EvictContinuesPastUnreadableRun")
m("REC-32 eviction drops the error of a run it cannot list", "internal/store/evict.go",
  "\ttotal, errs := plan.total, plan.unlisted\n", "\ttotal, errs := plan.total, []error(nil)\n",
  "TestREC32_EvictContinuesPastUnreadableRun")
