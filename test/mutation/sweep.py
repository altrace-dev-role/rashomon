#!/usr/bin/env python3
"""Show that every headless acceptance item fails when its implementation is broken.

An H-item that cannot be made to fail by breaking the implementation is not a
test; it is a line that happens to be green. This script applies one deliberate
break per item, runs the tests that item names, and reports whether they went
red. Every file is restored afterwards, whatever happens.

Run from the module root:  python3 test/mutation/sweep.py
"""
import os
import pathlib
import signal
import subprocess
import sys


M = []  # (name, file, old, new, test regex)
def m(name, f, old, new, test): M.append((name, f, old, new, test))


m("H-1  Guard no longer recovers", "internal/safe/safe.go",
  "\tdefer func() {\n\t\tif v := recover(); v != nil {\n\t\t\terr = newPanicError(v)\n\t\t}\n\t}()\n\treturn fn()", "\treturn fn()", "TestH1_NoFaultBlocksTheToolCall")
m("H-1  Go no longer recovers inside the goroutine", "internal/safe/safe.go",
  "\t\tdefer func() {\n\t\t\tif v := recover(); v != nil && onPanic != nil {\n\t\t\t\tdefer func() { _ = recover() }()\n\t\t\t\tonPanic(newPanicError(v))\n\t\t\t}\n\t\t}()\n\t\tfn()", "\t\tfn()", "TestH1_GoroutinePanicIsContained")
m("H-2  terminal record never written", "internal/hook/handle.go",
  "\tif h.opened || (h.toolUseID != \"\" && reason == store.ReasonLockTimeout) {", "\tif false {", "TestH2_HealthyPath")
m("H-2  transient settings read is not retried", "internal/hook/coverage.go",
  "\tdoc, err := settings.Load(path)\n\tfor attempt := 1; err != nil && attempt < settingsAttempts; attempt++ {\n\t\ttime.Sleep(settingsRetryPause)\n\t\tdoc, err = settings.Load(path)\n\t}\n\treturn doc, err",
  "\treturn settings.Load(path)", "TestH2_TransientSettingsReadIsRetried")
m("H-2  settings read is retried without a bound", "internal/hook/coverage.go",
  "\tsettingsAttempts   = 3\n", "\tsettingsAttempts   = 20\n", "TestH2_PersistentSettingsReadIsUnresolved")

m("H-3  matcher installed as Bash", "internal/install/install.go", 'Matcher = "*"', 'Matcher = "Bash"', "TestH3_")
m("H-3  timeout installed as 600", "internal/install/install.go", "Timeout = 5\n", "Timeout = 600\n", "TestH3_")
m("H-4  installer assigns instead of appends (foreign entries dropped)", "internal/install/install.go",
  "\t\t\tif Owner(e, event) != spec.InstallID {\n\t\t\t\tout = append(out, e)\n\t\t\t\tcontinue\n\t\t\t}\n\t\t\tif detail := intact(e, event); detail != \"\" {\n\t\t\t\t// Someone",
  "\t\t\tif Owner(e, event) != spec.InstallID {\n\t\t\t\tcontinue\n\t\t\t}\n\t\t\tif detail := intact(e, event); detail != \"\" {\n\t\t\t\t// Someone", "TestH4_")
m("H-5  absent settings file is an error", "internal/settings/document.go",
  "\tif errors.Is(err, fs.ErrNotExist) {\n\t\treturn &Document{}, nil\n\t}\n\tif err != nil {\n\t\treturn nil, err\n\t}\n\treturn Parse(data)",
  "\tif err != nil {\n\t\treturn nil, err\n\t}\n\t_ = errors.Is\n\t_ = fs.ErrNotExist\n\treturn Parse(data)", "TestH5_")
m("H-6  watch appends a second entry of ours", "internal/install/install.go", "\t\tif !found {\n\t\t\tout = append(out, want)", "\t\tif true {\n\t\t\tout = append(out, want)", "TestH6_")
m("H-6  detach removes every install's entries", "internal/install/install.go",
  "\treturn RemoveIf(doc, func(id string) bool { return id == spec.InstallID })",
  "\treturn RemoveIf(doc, func(string) bool { return true })", "TestH6_")
m("H-6  another install's entry records into this store", "cmd/rashomon/main.go",
  "\tif id == \"\" || id == st.InstallID() {", "\tif id == \"\" || true {", "TestH6_ForeignInstallEntryStandsDown")
m("H-6  watch does not say another install's entries are present", "cmd/rashomon/main.go",
  "\tif len(foreign) > 0 {", "\tif false && len(foreign) > 0 {", "TestH6_ForeignInstallEntryStandsDown")
m("H-6  detach --install ignores the id it was given", "internal/install/install.go",
  "\t\t\tif id == \"\" || !match(id) {", "\t\t\tif id == \"\" {", "TestH6_DetachByInstallIDNeedsNoStore")
m("H-6  detach --all removes only this machine's install", "cmd/rashomon/main.go",
  "\t\t\treturn install.RemoveIf(doc, func(string) bool { return true })",
  "\t\t\treturn install.Remove(doc, install.Spec{InstallID: installID})", "TestH6_DetachAllRemovesEveryInstall")
m("H-6  removing by predicate skips the intact check", "internal/install/install.go",
  "\t\t\tif detail := intact(e, event); detail != \"\" {\n\t\t\t\treturn 0, &ErrModified{Event: event, Detail: detail}\n\t\t\t}\n\t\t\tremoved++",
  "\t\t\tremoved++", "TestH6_DetachAllRefusesAnEditedEntry")
m("H-6  detach drops the entries it is not removing", "internal/install/install.go",
  "\t\t\tif id == \"\" || !match(id) {\n\t\t\t\tout = append(out, e)\n\t\t\t\tcontinue\n\t\t\t}",
  "\t\t\tif id == \"\" || !match(id) {\n\t\t\t\tcontinue\n\t\t\t}", "TestH6_Detach")
m("H-6  detach opens the store although it was given the id", "cmd/rashomon/main.go",
  "\tpath, err := settings.UserPath()\n\tif err != nil {\n\t\treturn err\n\t}\n\tremoved := 0",
  "\tpath, err := settings.UserPath()\n\tif err != nil {\n\t\treturn err\n\t}\n\tif _, err := openStore(); err != nil {\n\t\treturn err\n\t}\n\tremoved := 0", "TestH6_Detach")
m("H-6  plain detach creates a store to learn the id", "cmd/rashomon/main.go",
  "\tif _, err := os.Stat(filepath.Join(root, installMetaFile)); err != nil {",
  "\tif _, err := os.Stat(filepath.Join(root, installMetaFile)); err == nil && false {", "TestH6_PlainDetachLeavesNoStoreBehind")
m("H-6  watch does not print the undo line", "cmd/rashomon/main.go",
  "\tfmt.Fprintf(stdout, \"rashomon detach --install %s\\n\", st.InstallID())\n", "", "TestH6_WatchPrintsTheUndoLine")
# Two edit paths, two mutations. SetHookEntries is what watch uses and what
# detach uses while entries remain; RemoveHookEvent is what detach uses once an
# event empties, and it did not exist when this mutation was written -- so the
# H-7 mutation went NOT DETECTED, because the detach test that judged it no
# longer executes the line being mutated. The test filter is widened to the
# install paths that still reach it.
m("H-7  edit drops every other top-level key", "internal/settings/document.go",
  "\tif h := d.find(hooksKey); h != nil {\n\t\th.raw = obj\n\t} else {\n\t\td.members = append(d.members, member{key: hooksKey, raw: obj})\n\t}\n\treturn nil",
  "\td.members = []member{{key: hooksKey, raw: obj}}\n\treturn nil", "TestH3_|TestH7_|TestH06_")
m("H-7  removing a hook event drops every other top-level key", "internal/settings/document.go",
  "\tif len(kept) == 0 {\n\t\td.removeMember(hooksKey)\n\t\treturn nil\n\t}",
  "\tif len(kept) == 0 {\n\t\td.members = nil\n\t\treturn nil\n\t}", "TestH06_")
m("H-7  detach never notices an edited entry", "internal/install/install.go",
  "func intact(raw json.RawMessage, event string) string {\n", "func intact(raw json.RawMessage, event string) string {\n\tif true {\n\t\treturn \"\"\n\t}\n", "TestH7_DetachSaysSoWhenItCannot")
m("H-8  settings written in place instead of temp+rename", "internal/settings/write.go",
  "\ttmp, err := os.CreateTemp(dir, \".settings.json.rashomon-*\")", "\ttmp, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)", "TestH8_")
m("H-9  managed layer not consulted", "internal/settings/locate.go", "\t\t{LayerManaged, loc.Managed},\n", "", "TestH9_")
m("H-9  only the user layer consulted", "internal/settings/locate.go",
  "\t\t{LayerManaged, loc.Managed},\n\t\t{LayerLocal, loc.Local},\n\t\t{LayerProject, loc.Project},\n", "", "TestH9_")
m("H-20 PostToolUse is not installed", "internal/install/install.go",
  "\tEventPostToolUse,\n\tEventPostToolUseFailure,\n", "", "TestH3_")
m("H-20 the post entry is installed without a matcher", "internal/install/install.go",
  "\treturn event == EventPreToolUse ||\n\t\tevent == EventPostToolUse ||\n\t\tevent == EventPostToolUseFailure",
  "\treturn event == EventPreToolUse", "TestH3_")
m("H-20 the execution record is never written", "internal/hook/post.go",
  "\treturn p.st.AppendExecution(rec)", "\treturn nil",
  "TestH20_PostRecordsTheExecution")
m("H-20 an execution the lock refuses is dropped", "internal/store/store.go",
  "\treturn s.SpillExecution(rec)", "\treturn err", "TestAppendExecutionSpillsWhenTheLockIsHeld")
m("H-20 the post path ignores the install it was told it belongs to", "cmd/rashomon/main.go",
  "\t\tif standsDown(args, st, stderr) {\n\t\t\treturn nil\n\t\t}\n\n\t\tp := hook.NewPost(st, time.Now)",
  "\t\tp := hook.NewPost(st, time.Now)",
  "TestH20_PostFromAnotherInstallStandsDown")
m("H-20 post coverage resolves the recorder's entry instead of its own", "internal/hook/coverage.go",
  "\tif phase == store.PhasePost {", "\tif false {", "TestH20_PostCoverageReadsItsOwnEntry")
m("H-20 the report ignores tool_result blocks", "internal/report/transcript.go",
  "\t\t\tcase b.Type == \"tool_result\" && b.ToolUseID != \"\":", "\t\t\tcase false:", "TestH20_ExecutionAccounting")
# B3 -- keyed redaction. Both halves: the key itself, and the domain separator
# that keeps this digest from colliding with the one forget --host stores.
m("B3 redaction falls back to an unkeyed hash", "internal/report/redact.go",
  "\tmac := hmac.New(sha256.New, key)", "\tmac := hmac.New(sha256.New, nil)",
  "TestRedact_")
m("B3 redaction reuses the forget-host domain separator", "internal/report/redact.go",
  '\tmac.Write([]byte(redactDomain))', '\tmac.Write([]byte("forget-host\\x00"))',
  "TestRedact_")

# H-30 -- denials. Both halves of the conjunction get a mutation, because each
# is wrong in its own direction: dropping is_error turns any output that quotes
# the sentence into a denial (hiding a real execution), and dropping the prefix
# turns every failed command into one.
m("H-30 a denial is recognised on the prefix alone, without is_error", "internal/report/transcript.go",
  "\treturn isError && strings.HasPrefix(text, deniedPrefix)",
  "\treturn strings.HasPrefix(text, deniedPrefix)", "TestTranscript_|TestH30")
m("H-30 every failed call is treated as a denial", "internal/report/transcript.go",
  "\treturn isError && strings.HasPrefix(text, deniedPrefix)",
  "\treturn isError", "TestTranscript_|TestH30")
m("H-30 denials are counted as results again", "internal/report/transcript.go",
  "\t\t\t\tif isDenial(b.IsError, resultText(b.Content)) {\n\t\t\t\t\tdenied[b.ToolUseID] = true\n\t\t\t\t\tcontinue\n\t\t\t\t}\n",
  "\t\t\t\tif isDenial(b.IsError, resultText(b.Content)) {\n\t\t\t\t\tdenied[b.ToolUseID] = true\n\t\t\t\t}\n",
  "TestH30")
m("H-20 a result with no execution record is not a coverage failure", "internal/report/report.go",
  "\t\tif t.Readable && len(t.ExecutedButUnrecorded) > 0 {\n\t\t\tsess.Coverage.add(ReasonExecutionMismatch)\n\t\t}\n", "",
  "TestH20_ExecutionAccounting")
m("H-20 a declaration with no result is treated as a coverage failure", "internal/report/report.go",
  "\t\tif t.Readable && len(t.ExecutedButUnrecorded) > 0 {", "\t\tif t.Readable && len(t.DeclaredWithoutResult) > 0 {",
  "TestH20_ExecutionAccounting")
m("H-20 declarations with no execution are not named", "internal/store/read.go",
  "\t\tif !executed[d.ToolUseID] {", "\t\tif false {", "TestH20_DeclarationWithoutAnExecutionIsNamed")
m("H-20 the unexecuted list drops the permission mode", "internal/report/report.go",
  "Unexecuted{ToolUseID: id, PermissionMode: mode[id]})", "Unexecuted{ToolUseID: id})",
  "TestH20_DeclarationWithoutAnExecutionIsNamed")
m("H-20 the post path runs outside the panic barrier", "cmd/rashomon/main.go",
  "\tcaptureErr := safe.Guard(func() error { return p.Capture(stdin) })", "\tcaptureErr := p.Capture(stdin)",
  "TestH20_NoFaultOnThePostPathReachesTheAgent")
m("H-18 the post path installs on every call", "cmd/rashomon/main.go",
  "\tp := hook.NewPost(st, time.Now)\n", "\t_ = cmdWatch(io.Discard)\n\tp := hook.NewPost(st, time.Now)\n", "TestH18_")
m("H-20 the post payload declares tool_response, and it reaches the debug log", "internal/hook/post.go",
  "\tvar pl PostPayload\n", "\tvar pl struct {\n\t\tPostPayload\n\t\tToolResponse json.RawMessage `json:\"tool_response\"`\n\t}\n\tdefer func() { fmt.Fprintln(os.Stderr, string(pl.ToolResponse)) }()\n",
  "TestH20_ToolResponseNeverReachesDisk")
m("H-20 the response is measured, and the measurement moves the record's width", "internal/hook/post.go",
  "\t\tToolName:      pl.ToolName,\n",
  "\t\tToolName:      pl.ToolName + strconv.Itoa(len(raw)),\n",
  "TestH20_ExecutionWidthIsIndependentOfTheResponse")

m("H-10 accounting compares counts, not sets", "internal/report/report.go",
  "\tt.MissingFromStore = []string{}\n\tfor id := range ids {", "\tt.MissingFromStore = []string{}\n\tfor id := range ids {\n\t\tif len(ids) == len(recorded) {\n\t\t\tbreak\n\t\t}", "TestH10_SetsNotCounts")
m("H-10 every transcript is checked against the union of the run's ids", "internal/report/report.go",
  "\tfor _, path := range paths {\n\t\tt := accounting(path, byPath[path], executed)",
  "\tunion := map[string]bool{}\n\tfor _, ids := range byPath {\n\t\tfor id := range ids {\n\t\t\tunion[id] = true\n\t\t}\n\t}\n\tfor _, path := range paths {\n\t\tt := accounting(path, union, executed)", "TestH10_PerTranscript")
m("H-10 declarations naming no transcript are not counted", "internal/report/report.go",
  "\t\tif d.TranscriptPath == \"\" {\n\t\t\tsess.Declarations.WithoutTranscript++\n\t\t\tcontinue\n\t\t}",
  "\t\tif d.TranscriptPath == \"\" {\n\t\t\tcontinue\n\t\t}", "TestH10_DeclarationsWithoutATranscript")
m("H-11 end probe does not scan for unterminated entries", "internal/hook/probe.go",
  "\t\t\tif len(run.Unterminated()) > 0 {\n\t\t\t\treason = store.ReasonUnterminatedEntry\n\t\t\t}", "\t\t\t_ = run", "TestH11_")
m("H-12 SIGTERM ignored on the close path", "internal/hook/handle.go", "\tcase sig.Delivered():\n\t\toutcome, reason = store.OutcomeSignal", "\tcase false && sig.Delivered():\n\t\toutcome, reason = store.OutcomeSignal", "TestH12_SIGTERM")
m("H-13 program carries the whole command line", "internal/shape/shape.go", "prog := path.Base(toks[0])", "prog := cmd + path.Base(\"\")", "TestH13_")
m("H-14 untokenizable command records argc 0", "internal/shape/shape.go",
  "\tif err == nil {\n\t\tn := len(toks)\n\t\ts.Argc = &n\n\t}", "\tn := len(toks)\n\tif err != nil {\n\t\tn = 0\n\t}\n\ts.Argc = &n", "TestH14_Untokenizable")
m("H-15 forget deletes without a gap record", "internal/store/gaps.go", "\tif err := s.AppendGap(g); err != nil {\n\t\treturn nil, err\n\t}\n\n\tif records != nil && removedRec > 0 {", "\tif records != nil && removedRec > 0 {", "TestH15_ForgetLeavesAGap")
m("H-15 eviction deletes without a gap record", "internal/store/gaps.go", "\t\tif _, err := gf.Write(line); err != nil {", "\t\tif _, err := gf.Write(line[:0]); err != nil {", "TestH15_SizeCap")
m("H-15 eviction does not count the executions it removed", "internal/store/gaps.go",
  "RemovedRecords: len(run.Declarations) + len(run.Executions) + len(run.Terminals),",
  "RemovedRecords: len(run.Declarations) + len(run.Terminals),", "TestH15_SizeCap")
m("H-15 forget --before behaves like --since", "cmd/rashomon/main.go",
  "\t\t\tif flag == \"--since\" {\n\t\t\t\tfrom = &t\n\t\t\t} else {\n\t\t\t\tto = &t\n\t\t\t}", "\t\t\tfrom = &t",
  "TestH15_Forget")
m("H-15 forget takes both windows and resolves them itself", "cmd/rashomon/main.go",
  "\tcase from != nil && to != nil:\n\t\treturn errors.New(\"--since and --before name opposite ends of the store's timeline; pass one or the other\")\n",
  "", "TestH15_ForgetRefusesBothWindows")
m("H-15 executions are not part of forget's doomed plan", "internal/store/gaps.go",
  "\t\tif json.Unmarshal(line, &h) != nil {\n\t\t\tcontinue\n\t\t}",
  "\t\tif json.Unmarshal(line, &h) != nil || bytes.Contains(line, []byte(`\"type\":\"execution\"`)) {\n\t\t\tcontinue\n\t\t}",
  "TestH15_ForgetTakesTheExecutionWithThePair")
m("H-15 forget --before writes no gap record", "internal/store/gaps.go",
  "\tif err := s.AppendGap(g); err != nil {\n\t\treturn nil, err\n\t}",
  "\tif !w.openBelow() {\n\t\tif err := s.AppendGap(g); err != nil {\n\t\t\treturn nil, err\n\t\t}\n\t}",
  "TestH15_Forget")
m("H-15 the gap's open end is closed by the bound nobody named", "internal/store/gaps.go",
  "\tif w.openBelow() {\n\t\tfrom = earliest\n\t}", "", "TestH15_ForgetBeforeLeavesAGap")
m("H-16 append lock removed", "internal/store/store.go",
  "\tunlock, err := lockFile(f, lockBudget)\n\tif err != nil {\n\t\treturn err\n\t}\n\tdefer unlock()\n\n\tseq, err := nextSeq(dir)",
  "\tseq, err := nextSeq(dir)", "TestH16_")
m("H-16 the lock is shared, not exclusive", "internal/store/lock_unix.go",
  "syscall.LOCK_EX|syscall.LOCK_NB", "syscall.LOCK_SH|syscall.LOCK_NB", "TestLockFileIsExclusiveAndBounded")
m("H-16 the lock ignores the caller's budget", "internal/store/lock_unix.go",
  "deadline := time.Now().Add(budget)", "deadline := time.Now().Add(lockBudget)", "TestLockFileIsExclusiveAndBounded")
m("H-16 the lock gives up without waiting out the budget", "internal/store/lock_unix.go",
  "\t\tif time.Now().After(deadline) {", "\t\tif !time.Now().After(deadline) {", "TestLockFileIsExclusiveAndBounded")
m("H-17 handler opens a socket (no net import, so only the trace sees it)", "internal/hook/handle.go",
  "\tfault.Inject(fault.PointHookStart)\n", "\tfault.Inject(fault.PointHookStart)\n\tif fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_DGRAM, 0); err == nil {\n\t\tsyscall.Close(fd)\n\t}\n", "TestH17_HandlerOpensNoSockets")
m("H-18 the hook installs on every call", "cmd/rashomon/main.go",
  "\th := hook.New(st, time.Now)\n", "\t_ = cmdWatch(io.Discard)\n\th := hook.New(st, time.Now)\n", "TestH18_")
m("H-18 detach writes the settings file although it changed nothing", "cmd/rashomon/main.go",
  "\t\tn, err := remove(doc)\n\t\tremoved = n\n\t\treturn n > 0, err",
  "\t\tn, err := remove(doc)\n\t\tremoved = n\n\t\treturn true, err", "TestH18_")
m("H-19 report re-reads today's config to judge a past run", "internal/report/report.go",
  "\t\tsess := build(run)\n", "\t\tsess := build(run)\n\t\tif p, err := settings.UserPath(); err == nil {\n\t\t\tif doc, err := settings.Load(p); err == nil {\n\t\t\t\tif ok, _ := install.Present(doc, st.InstallID(), install.EventPreToolUse); !ok {\n\t\t\t\t\tsess.Coverage.add(store.ReasonHookEntryAbsent)\n\t\t\t\t}\n\t\t\t}\n\t\t}\n", "TestH19_")

m("report text renders a count it does not have as 0", "internal/report/text.go",
  "\tif p == nil {\n\t\treturn notRead\n\t}", "\tif p == nil {\n\t\treturn \"0\"\n\t}", "TestReport_")
m("report text renders a comparison it could not make as an empty list", "internal/report/text.go",
  "\tif ids == nil {\n\t\treturn unknown\n\t}", "\tif ids == nil {\n\t\treturn none\n\t}", "TestReport_")
m("report --json is accepted and ignored", "cmd/rashomon/main.go",
  "\t\tcase \"--json\":\n\t\t\tasJSON = true", "\t\tcase \"--json\":\n\t\t\tasJSON = false", "TestReport_")
m("status opens the store, which creates one", "cmd/rashomon/main.go",
  "\tif _, err := os.Stat(filepath.Join(root, installMetaFile)); err == nil {\n\t\tst, err := openStore()",
  "\tif true {\n\t\tst, err := openStore()", "TestH18_")
m("status reports an entry present whatever it finds", "cmd/rashomon/main.go",
  "\tcase present:\n\t\treturn \"present\"\n\tdefault:\n\t\treturn \"absent\"\n\t}",
  "\tcase present:\n\t\treturn \"present\"\n\tdefault:\n\t\treturn \"present\"\n\t}", "TestStatus_")
m("status does not name the other installs sharing the file", "cmd/rashomon/main.go",
  "\t\tfmt.Fprintf(stdout, \"  %s: %s\\n\", label, strings.Join(others, \", \"))",
  "\t\tfmt.Fprintf(stdout, \"  %s: none\\n\", label)", "TestStatus_")
m("status does not resolve the layer that disabled hooks", "cmd/rashomon/main.go",
  "\tcase decision.Disabled:\n\t\tfmt.Fprintf(stdout, \"hooks: disabled by the %s settings layer\\n\", decision.Layer)",
  "\tcase false:\n\t\tfmt.Fprintf(stdout, \"hooks: disabled by the %s settings layer\\n\", decision.Layer)", "TestStatus_")
# B9 -- naming the rewritten calls. The wording split is the correctness half:
# shape.Derive digests the whole input for every tool but Bash, so calling a
# non-Bash difference a changed COMMAND is false.
m("B9 every rewritten call is described as a command", "internal/report/text.go",
  '\t\twhat := "input changed"\n\t\tif r.ToolName == "Bash" {\n\t\t\twhat = "command changed"\n\t\t}',
  '\t\twhat := "command changed"', "TestRewritten")
m("B9 the count and the rows are computed separately", "internal/report/destinations.go",
  "\treturn len(rewrittenCalls(run))", "\treturn len(rewrittenCalls(run)) + 1", "TestRewritten|TestWireOnly")

# B2 -- the version gate itself. Putting a schema-3 field in the TOP-LEVEL
# required is the tempting edit and it invalidates every record already on
# disk, so it gets its own mutation.
m("B2 a schema-3 field is required at every version", "docs/store-schema.json",
  '        "tool_name",\n        "shape",\n', '        "tool_name",\n        "shape",\n        "host_source",\n',
  "TestSchema3|TestStoreSchema")
m("B2 the reader stops accepting schema 3", "internal/store/record.go",
  "\treturn version == 1 || version == 2 || version == 3",
  "\treturn version == 1 || version == 2", "TestAcceptsAdmits")

# Part 1 -- the in-window counters. Both mutations are the two ways the
# distinction they exist to make can be lost silently: counting traffic that is
# not this session's, and counting rows rather than folded requests. Neither
# changes any existing number, so only a test written for them can catch either.
m("P1 inherited rows count toward this session's reached/failed",
  "internal/wire/wire.go",
  "\t\tif !inherited {\n\t\t\tif reached {", "\t\tif true {\n\t\t\tif reached {",
  "TestInWindow")
m("P1 the counters are derived separately from Unreached", "internal/wire/wire.go",
  "\t\t\td.Unreached = false\n\t\t\treached = true", "\t\t\td.Unreached = false",
  "TestInWindow")

# Part 1 -- the chain view. Each of these is a way the view keeps rendering
# while asserting something the store does not support.
m("P1 chains are keyed on the prompt id alone", "internal/report/chains.go",
  "\t\tk := key{transcript: d.TranscriptPath, prompt: *d.PromptID}",
  "\t\tk := key{prompt: *d.PromptID}", "TestChains")
m("P1 links are left in store order", "internal/report/chains.go",
  "\t\tsort.SliceStable(c.Links, func(i, j int) bool { return c.Links[i].Seq < c.Links[j].Seq })",
  "", "TestChains")
m("P1 an unattributable window still yields host verdicts", "internal/report/chains.go",
  "\tif !dests.WindowApplied {\n\t\treturn LinkUnknown\n\t}\n", "", "TestChains|TestH31")
m("P1 a denial reads as a missing execution record", "internal/report/chains.go",
  "\t\tif denied[id] {\n\t\t\treturn LinkOutcomeDenied, []string{}, 0\n\t\t}\n", "",
  "TestChains|TestH31")
# The aliasing one. It is the reason redactChains rebuilds three slices rather
# than copying the struct, and a shallow copy compiles and renders correctly --
# the damage is entirely to the ORIGINAL report the caller still holds.
# The notices union. CI found this one: the linked set is platform-dependent,
# so a host-only check is wrong in the stale direction on every platform.
# Pinned to windows rather than "drop the env", because dropping it is a
# MACHINE-DEPENDENT break: a darwin host links a superset of every platform, so
# removing the override changes nothing there and the mutation reads as
# undetected on the developer's machine while being detected in Linux CI. A
# mutation whose verdict depends on who runs it is not evidence. Windows is the
# one platform that lacks a module another links (google/uuid), so pinning to it
# collapses the union everywhere.
m("the notices union collapses to one platform", "test/acceptance/notices_test.go",
  "\t\tcmd.Env = append(os.Environ(), \"GOOS=\"+p.goos, \"GOARCH=\"+p.goarch)",
  "\t\tcmd.Env = append(os.Environ(), \"GOOS=windows\", \"GOARCH=\"+p.goarch)",
  "TestThirdPartyNotices")
m("the release matrix drops a shipped platform", "test/acceptance/notices_test.go",
  "\t{\"windows\", \"amd64\"}, {\"windows\", \"arm64\"},", "", "TestThirdPartyNotices")
m("P1 a forgotten host is dropped instead of named", "internal/report/chains.go",
  "\tif forgotten != nil && forgotten(h) {\n\t\treturn LinkForgotten\n\t}\n", "", "TestChains|TestH31")
m("P1 loopback reads as a finding", "internal/report/chains.go",
  "\tif loopbackHosts[h] {\n\t\treturn LinkLoopback\n\t}\n", "", "TestChains|TestH31")
m("P1 client plane is re-derived instead of read from the view",
  "internal/report/chains.go",
  "\tfor _, c := range dests.ClientPlane {\n\t\tif c == h {\n\t\t\treturn LinkClientPlane\n\t\t}\n\t}\n",
  "\tif clientPlaneHosts[h] {\n\t\treturn LinkClientPlane\n\t}\n", "TestChains")
m("P1 the structural states collapse under the window gate",
  "internal/report/chains.go",
  "\tif forgotten != nil && forgotten(h) {", "\tif !dests.WindowApplied {\n\t\treturn LinkUnknown\n\t}\n\tif forgotten != nil && forgotten(h) {",
  "TestChains")
m("P1 the later slice index wins over the higher seq", "internal/report/chains.go",
  "\t\t\treturn execSeq(recs[i]) < execSeq(recs[j])", "\t\t\treturn false", "TestChains")
m("P1 a record with no seq outranks one that has a position",
  "internal/report/chains.go", "\t\treturn -1", "\t\treturn 1<<62", "TestChains")
m("P1 the second execution record is not reported", "internal/report/chains.go",
  "\treturn all[len(all)-1], all, len(recs)", "\treturn all[len(all)-1], all[:1], 1", "TestChains")
m("P1 unattributed calls lose their ids", "internal/report/chains.go",
  "\t\t\tout.Unattributed = append(out.Unattributed,\n\t\t\t\tbuildLink(d, executed, denied, state, dests, forgotten))\n\t\t\tcontinue",
  "\t\t\tcontinue", "TestChains|TestH31")
m("P1 dropped declarations vanish", "internal/report/chains.go",
  "\tfor _, id := range run.Dropped() {", "\tfor _, id := range []string{} {", "TestChains")
m("P1 ssh hosts are joined into the observable list", "internal/report/chains.go",
  "\tl.SSHHosts = append(l.SSHHosts, d.SSHHosts...)", "", "TestChains|TestH31")
m("P1 redaction leaves ssh hosts in clear", "internal/report/redact.go",
  "\t\tfor _, h := range link.SSHHosts {\n\t\t\tl.SSHHosts = append(l.SSHHosts, redactHost(h, key))\n\t\t}",
  "\t\tl.SSHHosts = append(l.SSHHosts, link.SSHHosts...)", "TestRedact|TestH31")
m("P1 the chain count hides behind the flag too", "internal/report/text.go",
  '\tfmt.Fprintf(b, "  chains: %d\\n", len(c.Prompts))', "", "TestH31|TestChains")
m("P1 the chain flag is inverted", "cmd/rashomon/main.go",
  "\t\tif chain {\n\t\t\topts = append(opts, report.WithChain())",
  "\t\tif !chain {\n\t\t\topts = append(opts, report.WithChain())", "TestH31")
m("P1 redaction writes through to the unredacted report", "internal/report/redact.go",
  "\t\tl.Hosts = make([]LinkHost, 0, len(link.Hosts))\n\t\tfor _, h := range link.Hosts {\n\t\t\t// The state is carried through untouched: it is a verdict, not a\n\t\t\t// name, and it is the only thing left worth reading.\n\t\t\tl.Hosts = append(l.Hosts, LinkHost{Host: redactHost(h.Host, key), State: h.State})\n\t\t}",
  "\t\tfor k := range l.Hosts {\n\t\t\tl.Hosts[k].Host = redactHost(l.Hosts[k].Host, key)\n\t\t}",
  "TestRedactChains")

# Re-anchored after the schema-3 bump reformatted the file. The mutation is
# unchanged: remove a declared key and confirm the allowlist test notices.
m("the store schema drops a record's key", "docs/store-schema.json",
  '        "agent_type": {\n', '        "agent_type_REMOVED": {\n', "TestStoreSchema")
# Anchored on schema 2, where tool_name is no longer the last property of the
# declaration block. The previous anchor assumed it was and silently stopped
# matching when hosts/ssh_hosts were added -- reported as ANCHOR MISSING, which
# lands in the same bucket as a real gap.
m("the store schema declares a key no record carries", "docs/store-schema.json",
  '        "shape": {\n', '        "tool_response": {\n          "type": "string"\n        },\n        "shape": {\n',
  "TestStoreSchema")
m("the store schema's coverage reasons are a subset of the code's", "docs/store-schema.json",
  "            \"probe_unresolved\"\n", "", "TestStoreSchema")

m("H-14 shell digest covers the whole tool_input again", "internal/shape/shape.go",
  "\ts.Digest = digest(key, toolName, []byte(cmd))", "\ts.Digest = digest(key, toolName, canonical(toolInput))",
  "TestH14_ShellDigestCoversTheCommandAlone")
m("H-14 non-shell digest covers only the tool name", "internal/shape/shape.go",
  "\t\ts.Digest = digest(key, toolName, canonical(toolInput))", "\t\ts.Digest = digest(key, toolName, nil)",
  "TestH14_NonShellDigestCoversTheWholeInput")
m("H-3  executable path installed unquoted", "internal/install/install.go",
  "\tif safe {\n\t\treturn s\n\t}\n", "\tif safe || true {\n\t\treturn s\n\t}\n",
  "TestShellQuote|TestH3_SpacedExecutablePathRunsAsInstalled")

m("tokenizer does not split on metacharacters", "internal/shape/tokenize.go",
  "\t\tcase isMeta(c):\n", "\t\tcase isMeta(c) && false:\n", "TestTokenize")
m("tokenizer emits the token an unterminated quote interrupted", "internal/shape/tokenize.go",
  "\t\t\tif !closed {\n\t\t\t\treturn toks, errUnterminated",
  "\t\t\tif !closed {\n\t\t\t\tstarted = true\n\t\t\t\tflush()\n\t\t\t\treturn toks, errUnterminated", "TestTokenize")
m("settings accepts a duplicate key", "internal/settings/document.go",
  "\t\tif seen[key] {\n\t\t\treturn nil, fmt.Errorf(\"duplicate key %q\", key)\n\t\t}\n\t\tseen[key] = true", "\t\tseen[key] = true",
  "TestParseRefuses|TestHookEntriesRefusesDuplicateEventKeys")
m("settings accepts trailing data after the object", "internal/settings/document.go",
  "\t\tif _, err := dec.Token(); err != io.EOF {\n\t\t\treturn nil, errors.New(\"trailing data after the top-level object\")\n\t\t}", "\t\t_ = io.EOF",
  "TestParseRefuses")
m("settings reads a non-object top level as an empty document", "internal/settings/document.go",
  "\t\treturn nil, errors.New(\"not a JSON object\")", "\t\treturn nil, nil", "TestParseRefuses")

# Part 2, sensitive-file labels. Each filter names the acceptance item AND the
# shape unit tests, deliberately: until schema 3 carries file_label the record
# does not show the label, so the acceptance halves that read it skip and the
# unit tests are what observe the break. This is the same widening the H-7
# mutation above needed, for the same reason -- a mutation judged only by a
# test that no longer executes the mutated line goes NOT DETECTED and says
# nothing.
m("H-44 the basename is stored as the label", "internal/shape/label.go",
  "\t}\n\treturn LabelNone\n}", "\t}\n\treturn base\n}", "TestH44_|TestLabel")
m("H-45 an unmatched path falls back to a substring of itself", "internal/shape/label.go",
  "\t}\n\treturn LabelNone\n}",
  "\t}\n\tif i := strings.LastIndexByte(base, '.'); i >= 0 {\n\t\treturn base[i+1:]\n\t}\n\treturn LabelNone\n}",
  "TestH45_|TestLabel")
m("H-46 Bash is labelled from its first path-like token", "internal/shape/label.go",
  "\tfield, ok := pathFields[toolName]\n\tif !ok {\n\t\treturn \"\"\n\t}",
  "\tfield, ok := pathFields[toolName]\n\tif !ok {\n\t\tif toolName == \"Bash\" {\n\t\t\tcmd, _ := stringField(toolInput, \"command\")\n\t\t\tfor _, tok := range strings.Fields(cmd) {\n\t\t\t\tif strings.Contains(tok, \"/\") {\n\t\t\t\t\treturn labelForBase(basename(tok))\n\t\t\t\t}\n\t\t\t}\n\t\t}\n\t\treturn \"\"\n\t}",
  "TestH46_|TestLabel")
m("H-44 the label is computed and thrown away", "internal/hook/handle.go",
  "\tif label := fileLabel(p.ToolName, p.ToolInput); label != \"\" {\n\t\tdecl.FileLabel = &label\n\t}",
  "\t_ = fileLabel(p.ToolName, p.ToolInput)", "TestH44_|TestH45_|TestH46_")
m("H-44 a stored label reaches the report unclamped", "internal/report/report.go",
  "\t\t\tsess.Declarations.ByLabel[knownLabel(*d.FileLabel)]++",
  "\t\t\tsess.Declarations.ByLabel[*d.FileLabel]++", "TestByLabel_")
m("H-46 labelling recovers after the append instead of before it", "internal/hook/handle.go",
  "\tdefer func() {\n\t\tif v := recover(); v != nil {\n\t\t\tlabel = shape.LabelUnknown\n\t\t}\n\t}()\n\tfault.Inject(fault.PointLabel)",
  "\tfault.Inject(fault.PointLabel)", "TestH46_NoFault")

# Import additions some mutants need.
IMPORTS = {
  "H-20 the post payload declares tool_response, and it reaches the debug log": ("internal/hook/post.go", '\t"io"\n', '\t"fmt"\n\t"io"\n\t"os"\n'),
  "H-17 handler opens a socket (no net import, so only the trace sees it)": ("internal/hook/handle.go", '\t"io"\n', '\t"io"\n\t"syscall"\n'),
  "H-19 report re-reads today's config to judge a past run": ("internal/report/report.go", '\t"github.com/altrace-dev-role/rashomon/internal/store"\n', '\t"github.com/altrace-dev-role/rashomon/internal/install"\n\t"github.com/altrace-dev-role/rashomon/internal/settings"\n\t"github.com/altrace-dev-role/rashomon/internal/store"\n'),
  "H-15 executions are not part of forget's doomed plan": ("internal/store/gaps.go", '\t"bufio"\n', '\t"bufio"\n\t"bytes"\n'),
}

backups = {}
def backup(f):
    if f not in backups:
        backups[f] = pathlib.Path(f).read_bytes()
def restore():
    for f, b in backups.items():
        pathlib.Path(f).write_bytes(b)

# RESTORE ON SIGNAL, not only on the way out of the try block.
#
# This script rewrites TRACKED SOURCE FILES IN PLACE and puts them back from an
# in-memory copy. `finally` covers a normal exit and an exception; it does not
# run when the process is killed. The realistic case is not exotic: piping the
# sweep into `head` or `grep` that exits early delivers SIGPIPE, and Python's
# default disposition terminates the process. What is left behind is a source
# file that has been broken ON PURPOSE, in a tree somebody is about to commit
# from -- and `git status` showing it is how a peer found one mid-run and had
# to ask whether it was a real edit.
#
# Each handler restores and then re-raises with the default disposition, so the
# exit status still says the process was signalled. Swallowing the signal would
# trade one lie for another.
def _restore_and_reraise(signum, _frame):
    restore()
    signal.signal(signum, signal.SIG_DFL)
    os.kill(os.getpid(), signum)

for _sig in (signal.SIGINT, signal.SIGTERM, signal.SIGHUP, signal.SIGPIPE):
    signal.signal(_sig, _restore_and_reraise)

# SIGKILL CANNOT BE CAUGHT, and neither can a hard OOM kill. The backup is in
# memory, so it dies with the process and no handler can help. That residue is
# real and this script cannot prevent it -- what it can do is refuse to start
# on top of it, below, rather than mutating an already-mutated file and
# restoring it to the wrong bytes.
def _dirty_targets():
    """Files this run would mutate that git already reports as modified."""
    targets = sorted({f for _, f, _, _, _ in M} | {f for f, _, _ in IMPORTS.values()})
    r = subprocess.run(["git", "status", "--porcelain", "--"] + targets,
                       capture_output=True, text=True)
    if r.returncode != 0:
        return []  # not a git checkout, or git unavailable: not this script's problem
    return [ln[3:] for ln in r.stdout.splitlines() if ln.strip()]

_dirty = _dirty_targets()
if _dirty:
    # A WARNING, not a refusal. The first version of this refused to start, and
    # it was wrong: it conflated two states that need opposite treatment.
    #
    # A developer's own uncommitted work in a mutated file is SAFE. The backup
    # is taken at startup, so their version is what gets restored -- mutate,
    # test, put it back exactly as it was. Refusing there blocks the normal way
    # of working on this repository, which is how this was found: it fired on
    # the very next commit's work-in-progress.
    #
    # Residue from a killed run is the dangerous state, and the GREEN BASELINE
    # CHECK above already catches it: a file left holding a deliberate break
    # makes the suite red, and the sweep stops there and says so. That check is
    # the guard; this is a note, because a developer who does not know why a
    # tracked file is modified should be told which ones and how to undo it.
    print("NOTE: files this sweep mutates are already modified:\n")
    for f in _dirty:
        print("   ", f)
    print("\nIf that is your own work in progress, this is fine -- the sweep backs up what\n"
          "is there now and restores exactly that. If it is residue from a run killed with\n"
          "SIGKILL (which no signal handler can catch), undo it first:\n"
          "    git checkout -- " + " ".join(_dirty) + "\n"
          "Residue would also have failed the green-baseline check below, which is the\n"
          "real guard.\n")

# GREEN BASELINE, before the first mutation.
#
# The sweep decides "the test went red" from a non-zero exit. With a
# pre-existing failure anywhere the filter happens to match, every mutation
# reports as detected and the sweep certifies a suite it never exercised --
# the tool whose job is guarding premises, not guarding its own. Found by a
# review that noticed the tree was red while the sweep would have passed.
print("baseline: the suite must be green before anything is mutated")
_baseline = subprocess.run(["go", "test", "./...", "-count=1"], capture_output=True, text=True)
if _baseline.returncode != 0:
    print("BASELINE NOT GREEN. Every mutation below would report as detected, because a\n"
          "mutation is judged by a non-zero exit and the suite already gives one. Fix the\n"
          "failure first; the sweep proves nothing until then.\n")
    print(_baseline.stdout[-4000:])
    print(_baseline.stderr[-2000:])
    sys.exit(1)
print("baseline: green\n")

undetected = []
skipped = []
try:
    for name, f, old, new, test in M:
        backup(f)
        s = pathlib.Path(f).read_text()
        if old not in s:
            print(f"  ANCHOR MISSING <- {name}"); undetected.append(name); continue
        s = s.replace(old, new, 1)
        pathlib.Path(f).write_text(s)
        if name in IMPORTS:
            fi, io_, in_ = IMPORTS[name]; backup(fi)
            t = pathlib.Path(fi).read_text(); assert io_ in t; pathlib.Path(fi).write_text(t.replace(io_, in_, 1))
        r = subprocess.run(["go", "test", "./...", "-run", test, "-count=1", "-v"], capture_output=True, text=True)
        judged = r.stdout + r.stderr
        # "--- SKIP" ONLY, never "no tests to run". The suite is invoked as
        # `go test ./... -run <regex>`, so every package without a matching
        # test prints "no tests to run" -- which made any mutation whose tests
        # passed look unjudged rather than UNDETECTED. It masked a real one:
        # a count mutation that no test asserted against was reported as "not
        # judged here" instead of as the gap it was. A check that cannot tell
        # "nothing ran" from "nothing matched in this package" is worse than no
        # check, because it converts findings into reassurance.
        if r.returncode == 0 and "--- SKIP" in judged:
            # The judging test did not RUN, so this mutation was not judged.
            # Reporting it as "the test cannot be made to fail" would name a
            # spec defect that may not exist: the socket half of H-17 needs
            # strace, which ubuntu-latest installs in CI and macOS does not
            # have, so the same mutation is judged there and unjudgeable here.
            # A verdict that cannot tell "we checked and it passed" from "we
            # could not check" is the error this whole suite exists to prevent.
            print(f"  NOT JUDGED     <- {name}   (the judging test skipped here)")
            skipped.append(name)
        elif r.returncode == 0:
            print(f"  NOT DETECTED   <- {name}   (spec defect: the test cannot be made to fail)"); undetected.append(name)
        elif "build failed" in r.stdout + r.stderr or "cannot" in r.stderr and "FAIL" not in r.stdout:
            print(f"  BUILD BROKEN   <- {name}\n{r.stdout}{r.stderr}"); undetected.append(name)
        else:
            print(f"  went red       <- {name}")
        restore()
finally:
    restore()

print()
print("undetected:", undetected if undetected else "none")
if skipped:
    print("not judged here:", skipped)
    print("  These were not checked because the judging test skipped in this")
    print("  environment. They are not passes. Run the sweep where those tests")
    print("  run -- CI installs strace on ubuntu-latest for exactly this reason.")
sys.exit(1 if undetected else 0)
