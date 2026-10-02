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
  "\treturn RemoveIf(doc, func(id string) bool { return id == spec.InstallID }, force)",
  "\treturn RemoveIf(doc, func(string) bool { return true }, force)", "TestH6_")
m("H-6  another install's entry records into this store", "cmd/rashomon/main.go",
  "\tif id == \"\" || id == st.InstallID() {", "\tif id == \"\" || true {", "TestH6_ForeignInstallEntryStandsDown")
m("H-6  watch does not say another install's entries are present", "cmd/rashomon/main.go",
  "\tif len(foreign) > 0 {", "\tif false && len(foreign) > 0 {", "TestH6_ForeignInstallEntryStandsDown")
m("H-6  detach --install ignores the id it was given", "internal/install/install.go",
  "\t\t\tif id == \"\" || !match(id) {", "\t\t\tif id == \"\" {", "TestH6_DetachByInstallIDNeedsNoStore")
m("H-6  detach --all removes only this machine's install", "cmd/rashomon/main.go",
  "\t\t\treturn install.RemoveIf(doc, func(string) bool { return true }, force)",
  "\t\t\treturn install.Remove(doc, install.Spec{InstallID: installID}, force)", "TestH6_DetachAllRemovesEveryInstall")
m("H-6  removing by predicate skips the intact check", "internal/install/install.go",
  "\t\t\t\tif !force {\n\t\t\t\t\treturn 0, nil, &ErrModified{Event: event, Detail: detail}\n\t\t\t\t}",
  "\t\t\t\tif false {\n\t\t\t\t\treturn 0, nil, &ErrModified{Event: event, Detail: detail}\n\t\t\t\t}", "TestH6_DetachAllRefusesAnEditedEntry")
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
  "\tif !isError {\n\t\treturn false\n\t}",
  "", "TestTranscript_|TestH30")
m("H-30 every failed call is treated as a denial", "internal/report/transcript.go",
  "\tif !isError {\n\t\treturn false\n\t}",
  "\tif !isError {\n\t\treturn false\n\t}\n\treturn true", "TestTranscript_|TestH30")
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
m("H-13 program carries the whole command line", "internal/shape/shape.go", "prog := path.Base(pshaped[i].text)", "prog := cmd + path.Base(pshaped[i].text[:0])", "TestH13_")
m("the report counts an unknown program among the real ones", "internal/report/report.go",
  "\t\tcase d.ToolName == \"Bash\":", "\t\tcase false:", "TestProgramsUnknownIsCountedApart|TestNonShell")
m("the report stops counting programs at all", "internal/report/report.go",
  "\t\t\tsess.Declarations.ByProgram[*d.Shape.Program]++", "\t\t\t_ = d.Shape.Program",
  "TestByProgramAndVerb")
m("H-13 a word the tokenizer cannot vouch for is read as the program", "internal/shape/shape.go",
  "\t\tcase t.opaque, isComment(t):", "\t\tcase isComment(t):", "TestNoCorpusLineLeaksIntoProgram")
m("H-13 the tokenizer never marks an expansion opaque", "internal/shape/tokenize.go",
  "\t\tdefault:\n\t\t\tif expands(i, false) {\n\t\t\t\topaque = true\n\t\t\t}",
  "\t\tdefault:\n\t\t\tif expands(i, false) {\n\t\t\t\t_ = opaque\n\t\t\t}", "TestNoCorpusLineLeaksIntoProgram")
m("H-13 backslash-newlines are not joined before the program search", "internal/shape/shape.go",
  "\tif !strings.Contains(s, \"\\\\\\n\") {\n\t\treturn s, nil\n\t}", "\tif true {\n\t\treturn s, nil\n\t}",
  "TestNoCorpusLineLeaksIntoProgram|TestTokenizeProgram")
m("H-13 a paren where a redirect target belongs is read as a subshell", "internal/shape/shape.go",
  "\tif toks[i+1].meta {\n\t\t// A paren where the target belongs",
  "\tif false && toks[i+1].meta {\n\t\t// A paren where the target belongs",
  "TestNoCorpusLineLeaksIntoProgram")
m("H-13 a word quoted before its = is read as an assignment", "internal/shape/shape.go",
  "\treturn t.quotedAt < 0 || t.quotedAt > eq", "\treturn true", "TestProgramIsAProgram|TestNoCorpusLineLeaksIntoProgram")
m("H-13 a quoted command line with spaces is recorded as the program", "internal/shape/shape.go",
  "\t\tcase c < 0x21 || c > 0x7e:", "\t\tcase c < 0x20 || c > 0x7e:", "TestProgramIsAProgram")
m("H-13 program is whatever token came first, operator or not", "internal/shape/shape.go",
  "\t\tt := toks[i]\n\t\tif t.meta {",
  "\t\tt := toks[i]\n\t\tif false && t.meta {", "TestProgramIsAProgram")
m("H-13 a split redirect operator's second half is taken for its target", "internal/shape/shape.go",
  "toks[i+1].meta && toks[i+1].glued && continuesRedirect(op, toks[i+1].text); n++ {", "toks[i+1].meta && op == \"\"; n++ {",
  "TestH13_CanaryNeverReachesDisk|TestProgramIsAProgram")
m("H-13 a redirect skips every operator before its target", "internal/shape/shape.go",
  "\tfor n := 0; n < pieces && i+1 < len(toks) && toks[i+1].meta && toks[i+1].glued && continuesRedirect(op, toks[i+1].text); n++ {\n\t\ti++\n\t}",
  "\tfor i+1 < len(toks) && toks[i+1].meta {\n\t\ti++\n\t}\n\t_, _ = op, pieces",
  "TestH13_CanaryNeverReachesDisk")
# The program search: one break per rule, each judged by the rows and corpus
# lines that rule alone closes -- the matrix in shape_program_test.go carries a
# row per glob character, per separator, per glue check and per case of an
# unknowable group depth, so that no rule rides on another.
PROG = "TestProgramIsAProgram|TestNoCorpusLineLeaksIntoProgram"
m("H-13 a redirect's pieces are joined across a blank or a newline", "internal/shape/shape.go",
  "toks[i+1].meta && toks[i+1].glued && continuesRedirect(op, toks[i+1].text)", "toks[i+1].meta && continuesRedirect(op, toks[i+1].text)", PROG)
m("H-13 an operator where a redirect target belongs is read past to the next command", "internal/shape/shape.go",
  "\treturn !procSub(toks, i+1)\n}", "\treturn false\n}", PROG)
m("H-13 the tokenizer never marks an operator glued", "internal/shape/tokenize.go",
  "nlBefore: sawNL, glued: !gap && len(toks) > 0})", "nlBefore: sawNL, glued: false})", "TestTokenizeGlued|TestProgramIsAProgram")
m("H-13 the tokenizer never marks a word glued", "internal/shape/tokenize.go",
  "\t\t\tglued = !gap && len(toks) > 0\n", "\t\t\tglued = false\n", "TestTokenizeGlued|TestProgramIsAProgram")
m("H-13 a byte the shell does not split on is read as part of a program name", "internal/shape/shape.go",
  "\t\tcase c < 0x21 || c > 0x7e:", "\t\tcase c == ' ' || c == '\\t' || c == '\\n' || c == '\\r':", PROG)
m("H-13 a command word holding $ is named", "internal/shape/shape.go",
  "\t\tcase c == '$':", "\t\tcase false && c == '$':", PROG)
m("H-13 a glob command word is named", "internal/shape/shape.go",
  "\t\tcase c == '*' || c == '?':", "\t\tcase false && (c == '*' || c == '?'):", PROG)
m("H-13 zsh's ~ exclusion is read as a path", "internal/shape/shape.go",
  "\t\tcase c == '~' && j > 0:", "\t\tcase false && c == '~' && j > 0:", PROG)
m("H-13 an all-digit command word is named", "internal/shape/shape.go",
  "\treturn !digits\n}", "\treturn true\n}", PROG)
m("H-13 a word glued to <( or a numeric glob is named by its front", "internal/shape/shape.go",
  "\treturn next.meta && next.text == \"(\" || op == \"<\" && isNumericGlob(next)",
  "\treturn false && next.meta && op == \"\"", PROG)
m("H-13 a directory is named by its last component", "internal/shape/shape.go",
  "\tcase strings.HasSuffix(s, \"/\"):", "\tcase false && strings.HasSuffix(s, \"/\"):", PROG)
m("H-13 a control byte in the line is read past", "internal/shape/shape.go",
  "\t\tif c := cmd[i]; c < 0x20 && c != '\\t' && c != '\\n' || c == 0x7f {", "\t\tif c := cmd[i]; c == '\\r' {", PROG)
m("H-13 the function-definition scan stops at a redirection", "internal/shape/shape.go",
  "\t\tcase isRedirect(t.text):\n\t\t\tend := operatorEnd(toks, j)\n",
  "\t\tcase isRedirect(t.text):\n\t\t\tif true {\n\t\t\t\treturn -1, true\n\t\t\t}\n\t\t\tend := operatorEnd(toks, j)\n", PROG)
m("H-13 the function-definition scan reads &> as the background separator", "internal/shape/shape.go",
  "\t\tcase t.text == \"&\" && j+1 < len(toks)", "\t\tcase false && t.text == \"&\" && j+1 < len(toks)", PROG)
m("H-13 the function-definition scan stops at a paren that does not close at once", "internal/shape/shape.go",
  "\t\t\tif j+1 < len(toks) && toks[j+1].meta && toks[j+1].text == \")\" {\n\t\t\t\treturn 0, false\n\t\t\t}\n",
  "\t\t\tif j+1 < len(toks) && toks[j+1].meta && toks[j+1].text == \")\" {\n\t\t\t\treturn 0, false\n\t\t\t}\n\t\t\treturn -1, true\n", PROG)
m("H-13 the function-definition scan runs on past a newline", "internal/shape/shape.go",
  "\t\tcase t.nlBefore:\n\t\t\treturn -1, true\n", "\t\tcase false && t.nlBefore:\n\t\t\treturn -1, true\n", "TestProgramIsAProgram")
SEPS = ["\";\"", "\"&&\"", "\"||\"", "\"|\"", "\"&\""]
_seps = "\t\tcase " + " || ".join("t.text == " + x for x in SEPS) + ":\n\t\t\treturn j, true"
for _drop in SEPS:
    m("H-13 the function-definition scan reads on past " + _drop.strip('"'), "internal/shape/shape.go",
      _seps, "\t\tcase " + " || ".join("t.text == " + x for x in SEPS if x != _drop) + ":\n\t\t\treturn j, true", PROG)
m("H-13 the function-definition scan does not skip a paren group", "internal/shape/shape.go",
  "\t\t\t}\n\t\t\topen = append(open, '(')\n", "\t\t\t}\n", PROG)
m("H-13 the function-definition scan does not skip backticks", "internal/shape/shape.go",
  "\t\tcase t.ticks%2 == 1:\n\t\t\topen = append(open, '`')\n", "\t\tcase false:\n\t\t\topen = append(open, '`')\n", PROG)
m("H-13 a paren inside a group does not nest", "internal/shape/shape.go",
  "\tcase t.text == \"(\":\n\t\treturn append(open, '(')\n", "\tcase t.text == \"(\":\n", PROG)
m("H-13 a paren group never closes", "internal/shape/shape.go",
  "\tcase t.text == \")\":\n\t\treturn open[:len(open)-1]\n", "\tcase t.text == \")\":\n", PROG)
m("H-13 backticks never close", "internal/shape/shape.go",
  "\tcase t.ticks%2 == 1 && top == '`':\n\t\treturn open[:len(open)-1]\n", "\tcase false:\n\t\treturn open[:len(open)-1]\n", PROG)
m("H-13 a group that never closes is read as closed", "internal/shape/shape.go",
  "\tif len(open) > 0 || uncertain {\n\t\treturn 0, false\n\t}", "\tif uncertain {\n\t\treturn 0, false\n\t}", PROG)
m("H-13 the function-definition scan reads past where the lexer's reading stopped", "internal/shape/shape.go",
  "\tif len(open) > 0 || uncertain {\n\t\treturn 0, false\n\t}", "\tif len(open) > 0 {\n\t\treturn 0, false\n\t}", PROG)
m("H-13 a ${ the word does not close is read past", "internal/shape/shape.go",
  "\t\tif openBrace(t) {\n\t\t\treturn 0, false\n\t\t}", "\t\tif false && openBrace(t) {\n\t\t\treturn 0, false\n\t\t}", PROG)
m("H-13 a closed ${ } is taken for an open one", "internal/shape/shape.go",
  "strings.Count(t.text[k:], \"{\") > strings.Count(t.text[k:], \"}\")",
  "strings.Count(t.text[k:], \"{\") >= strings.Count(t.text[k:], \"}\")", "TestProgramIsAProgram")
m("H-13 a quoted ${ is taken for an expansion", "internal/shape/shape.go",
  "\treturn t.opaque && k >= 0 &&", "\treturn k >= 0 &&", "TestProgramIsAProgram")
m("H-13 a comment inside a group is read past", "internal/shape/shape.go",
  "\tcase isComment(t):\n\t\treturn true\n", "\tcase false && isComment(t):\n\t\treturn true\n", PROG)
m("H-13 a case statement inside a group is read past", "internal/shape/shape.go",
  "\tcase !t.meta && t.quotedAt < 0 && t.text == \"case\":\n", "\tcase false:\n", PROG)
m("H-13 a here-document inside a group is read past", "internal/shape/shape.go",
  "\tcase hereDoc(toks, j):\n\t\treturn true\n", "\tcase false && hereDoc(toks, j):\n\t\treturn true\n", PROG)
m("H-13 a newline in a group after a here-document is read past", "internal/shape/shape.go",
  "\tcase heredoc && t.nlBefore:\n", "\tcase false:\n", PROG)
m("H-13 a here-string is taken for a here-document", "internal/shape/shape.go",
  "\treturn next >= len(toks) || !(toks[next].meta && toks[next].glued && toks[next].text == \"<\")",
  "\treturn next >= 0", "TestProgramIsAProgram")
m("H-13 a trailing backslash after an expansion is read as a real one", "internal/shape/tokenize.go",
  "\t\t\t\tflush()\n\t\t\t\treturn toks, unterminated()", "\t\t\t\tflush()\n\t\t\t\treturn toks, errUnterminated",
  "TestTokenizeProgramStops|" + PROG)
m("H-13 the function-definition scan reads &> as & and then >", "internal/shape/shape.go",
  "\t\t\tj++\n\t\t\tfallthrough", "\t\t\tfallthrough", "TestProgramIsAProgram")
m("H-13 a word glued to any operator and a paren is named by its front", "internal/shape/shape.go",
  "\tif op != \"<\" && op != \">\" {", "\tif false && op != \"<\" && op != \">\" {", "TestProgramIsAProgram")
m("H-13 a word glued to > and digits is read as a numeric glob", "internal/shape/shape.go",
  "|| op == \"<\" && isNumericGlob(next)", "|| isNumericGlob(next)", "TestProgramIsAProgram")
m("H-13 a backtick inside a paren group opens nothing", "internal/shape/shape.go",
  "\tcase t.ticks%2 == 1:\n\t\treturn append(open, '`')\n", "\tcase false:\n\t\treturn append(open, '`')\n", PROG)
m("H-13 a paren inside backticks closes them", "internal/shape/shape.go",
  "\tcase top == '`' || !t.meta:", "\tcase !t.meta:", PROG)
m("H-13 & then > is read as &>", "internal/shape/shape.go",
  "toks[j+1].meta && toks[j+1].glued && strings.HasPrefix(toks[j+1].text, \">\")",
  "toks[j+1].meta && strings.HasPrefix(toks[j+1].text, \">\")", "TestProgramIsAProgram")
m("H-13 a numeric glob spaced from the command word is read as part of it", "internal/shape/shape.go",
  "!toks[i+1].meta || !toks[i+1].glued || !toks[i+2].glued", "!toks[i+1].meta || !toks[i+2].glued", "TestProgramIsAProgram")
m("H-13 digits spaced from a glued < are read as a numeric glob", "internal/shape/shape.go",
  "!toks[i+1].meta || !toks[i+1].glued || !toks[i+2].glued", "!toks[i+1].meta || !toks[i+1].glued", "TestProgramIsAProgram")
m("H-13 a path ending in . is named", "internal/shape/shape.go",
  "\tcase s != \".\" && (path.Base(s) == \".\" || path.Base(s) == \"..\"):",
  "\tcase s != \".\" && path.Base(s) == \"..\":", PROG)
m("H-13 a path ending in .. is named", "internal/shape/shape.go",
  "\tcase s != \".\" && (path.Base(s) == \".\" || path.Base(s) == \"..\"):",
  "\tcase s != \".\" && path.Base(s) == \".\":", PROG)
m("H-13 the source builtin is refused as a directory", "internal/shape/shape.go",
  "\tcase s != \".\" && (path.Base(s) == \".\" || path.Base(s) == \"..\"):",
  "\tcase path.Base(s) == \".\" || path.Base(s) == \"..\":", "TestProgramIsAProgram")
m("H-13 a jobspec is named", "internal/shape/shape.go",
  "\tcase strings.HasPrefix(s, \"%\"):", "\tcase false && strings.HasPrefix(s, \"%\"):", PROG)
m("H-13 a quoted command word split by a tab is named", "internal/shape/shape.go",
  "\t\tcase c < 0x21 || c > 0x7e:", "\t\tcase c < 0x21 && c != '\\t' || c > 0x7e:", PROG)
m("H-13 a quoted command word split by a newline is named", "internal/shape/shape.go",
  "\t\tcase c < 0x21 || c > 0x7e:", "\t\tcase c < 0x21 && c != '\\n' || c > 0x7e:", PROG)
m("H-13 a tab in the line is taken for a control byte", "internal/shape/shape.go",
  "\t\tif c := cmd[i]; c < 0x20 && c != '\\t' && c != '\\n' || c == 0x7f {",
  "\t\tif c := cmd[i]; c < 0x20 && c != '\\n' || c == 0x7f {", "TestProgramIsAProgram")
m("H-13 a newline in the line is taken for a control byte", "internal/shape/shape.go",
  "\t\tif c := cmd[i]; c < 0x20 && c != '\\t' && c != '\\n' || c == 0x7f {",
  "\t\tif c := cmd[i]; c < 0x20 && c != '\\t' || c == 0x7f {", "TestProgramIsAProgram")
m("H-13 the tokenizer never counts an unquoted backtick", "internal/shape/tokenize.go",
  "\t\t\tif c == '`' {\n\t\t\t\tticks++\n\t\t\t}\n", "", "TestTokenizeTicks|" + PROG)
m("H-13 the tokenizer never counts a backtick inside double quotes", "internal/shape/tokenize.go",
  "\t\t\t\tif s[i] == '`' {\n\t\t\t\t\tticks++\n\t\t\t\t}\n", "", "TestTokenizeTicks")
# Judged by the corpus alone, where the two lines of the PR #29 review are the
# only ones this break reopens: "$(echo "a;b")" ends at its first inner quote
# again, and the ; inside it ends the search for the () after the command word.
m("H-13 a $( ) inside double quotes is not a context in which quotes nest", "internal/shape/tokenize.go",
  "\t\t\t\tif program && s[i] == '$' && i+1 < len(s) && s[i+1] == '(' {",
  "\t\t\t\tif false && program && s[i] == '$' && i+1 < len(s) && s[i+1] == '(' {", "TestNoCorpusLineLeaksIntoProgram")
# The review of that change: each guard comsubEnd and its helpers keep against
# a reading the shell may not share. TPROG adds the tokenizer's own tests.
TPROG = "TestTokenizeProgram|" + PROG
C = "internal/shape/comsub.go"
m("H-13 a paren inside $[ ] counts as a group", C,
  "\t\tcase '(', ')', '{', '}', '\\'', '\"', '`', '\\\\', '\\n':", "\t\tcase '{', '}', '\\'', '\"', '`', '\\\\', '\\n':", TPROG)
m("H-13 a nested [ ] ends a $[ ]", C, "\t\tcase '[':\n\t\t\tdepth++\n", "", TPROG)
m("H-13 $[ is not read as arithmetic", C, "\t\treturn bracketEnd(r.s, i+2)", "\t\treturn i", TPROG)
m("H-13 an even run of $ opens an expansion in a substitution", C,
  "\t\tif dollars%2 == 0 {\n\t\t\treturn -1\n\t\t}", "\t\tif false {\n\t\t\treturn -1\n\t\t}", TPROG)
m("H-13 \"$$(\" opens a substitution", "internal/shape/tokenize.go",
  "\t\t\t\t\tif dq%2 == 1 {", "\t\t\t\t\tif true {", TPROG)
m("H-13 a $' inside double quotes in a substitution is a quote", C,
  "\t\t\t\tcontinue // no quote inside double quotes\n", "", TPROG)
m("H-13 a nested $( ) is counted as parens", C, "\t\treturn r.substEnd(i, nest)", "\t\treturn i", TPROG)
m("H-13 a backslash-newline the joining left is read past in a substitution", C,
  "\t\t\tif i+1 >= len(s) || r.continuesAt(i) {", "\t\t\tif i+1 >= len(s) {", TPROG)
m("H-13 a backslash-newline the joining left is read past in its double quotes", C,
  "\t\t\tif r.continuesAt(i) {", "\t\t\tif false {", TPROG)
m("H-13 the joining's offsets are not kept", "internal/shape/shape.go",
  "\t\t\t\tjoins = append(joins, b.Len())\n", "", TPROG)
m("H-13 a quoted span joined across a newline opens a here-document's body", C,
  " || r.joinedIn(from, to)))", "))", TPROG)
m("H-13 an unquoted here-document's body is read across backslash-newlines", C,
  "\t\t\tif !d.quoted && (r.joinedIn(i-1, e)", "\t\t\tif false && (r.joinedIn(i-1, e)", TPROG)
m("H-13 a quoted here-document's line as written may begin with its delimiter", C,
  "\t\tif strings.HasPrefix(line(r.s[from:p]), d.delim) {", "\t\tif false {", TPROG)
m("H-13 a quoted here-document's joined line is read as one line", C,
  "\t\tfrom = p\n", "\t\t_ = p\n", TPROG)
m("H-13 a quoted delimiter leaves its body unquoted", C,
  "\t\t\td.quoted = true\n\t\t\ti += j + 1", "\t\t\ti += j + 1", TPROG)
m("H-13 case is a keyword wherever it stands", C,
  "\t\tcase c == 'c' && cmd.begins() && isCase(s, i, body):", "\t\tcase c == 'c' && isCase(s, i, body):", TPROG)
m("H-13 a command's first word never names it", C, "\t\tp.named = true", "\t\tp.named = false", TPROG)
m("H-13 a separator does not begin a command", C,
  "\tif strings.IndexByte(\";&|()\\n\", c) >= 0 {", "\tif false {", TPROG)
m("H-13 zsh's } and ]] do not begin a command", C, "\tcase w == \"}\" || w == \"]]\":", "\tcase false:", TPROG)
m("H-13 time's options name the command", C, "\t\tp.prefix = true", "\t\tp.prefix = false", TPROG)
m("H-13 then names the command", C, "\"select\": true, \"then\": true,", "\"select\": true,", TPROG)
m("H-13 an assignment names the command", C, "commandKeywords[w], strings.IndexByte(w, '=') >= 0, ", "commandKeywords[w], ", TPROG)
m("H-13 a redirect's target names the command", C,
  "\t\tcase '<', '>':\n\t\t\treturn true", "\t\tcase '<', '>':\n\t\t\treturn false", TPROG)
m("H-13 a redirect's fd names the command", C,
  "\tif i+n < len(s) && (s[i+n] == '<' || s[i+n] == '>') {", "\tif false {", TPROG)
m("H-13 $(( is read as a $( ) with a group", C,
  "\tif i+2 < len(r.s) && r.s[i+2] == '(' {", "\tif false {", TPROG)
m("H-13 $(( whose ( closes alone is read as arithmetic", C,
  "\t\t\tif i+1 < len(s) && s[i+1] == ')' {\n\t\t\t\treturn i + 1\n\t\t\t}\n\t\t\treturn -1", "\t\t\treturn i + 1", TPROG)
m("H-13 arithmetic holds what only a command can", C,
  "\t\tcase '\\'', '\"', '`', '\\\\', '\\n', ';':\n\t\t\treturn -1\n", "\t\tcase '\\'', '\"', '`', '\\\\', '\\n', ';':\n", TPROG)
m("H-13 a # after a blank in arithmetic is read past", C,
  "\t\t\tif i == body || strings.IndexByte(wordBreak, s[i-1]) >= 0 {\n\t\t\t\treturn -1", "\t\t\tif false {\n\t\t\t\treturn -1", TPROG)
m("H-13 case in arithmetic is read past", C, "\t\t\tif isCase(s, i, body) {", "\t\t\tif false {", TPROG)
m("H-13 a run of $ in arithmetic is not counted", C, "\t\t\tdollars++\n\t\t\tend := r.dollarOpens(i, dollars, nest+1)", "\t\t\tdollars = 1\n\t\t\tend := r.dollarOpens(i, dollars, nest+1)", TPROG)
m("H-13 an expansion in arithmetic is read as parens", C,
  "\t\t\tend := r.dollarOpens(i, dollars, nest+1)\n\t\t\tif end < 0 {\n\t\t\t\treturn -1\n\t\t\t}\n\t\t\ti = end",
  "\t\t\tend := i\n\t\t\tif end < 0 {\n\t\t\t\treturn -1\n\t\t\t}\n\t\t\ti = end", TPROG)
m("H-13 the function-definition scan reads past a redirect with no target", "internal/shape/shape.go",
  "\t\t\tif noTarget(toks, end) {", "\t\t\tif false && noTarget(toks, end) {", PROG)
m("H-13 the program search reads past a redirect with no target", "internal/shape/shape.go",
  "\tif noTarget(toks, i) {\n\t\treturn 0, false\n\t}\n", "\tif false && noTarget(toks, i) {\n\t\treturn 0, false\n\t}\n", PROG)
m("H-13 a redirect takes its target from the next line", "internal/shape/shape.go",
  "\tcase next.nlBefore:\n\t\treturn true\n", "\tcase false && next.nlBefore:\n\t\treturn true\n", PROG)
m("H-13 a redirect followed by another is read past", "internal/shape/shape.go",
  "\treturn !procSub(toks, i+1)\n}", "\treturn !procSub(toks, i+1) && !isRedirect(next.text)\n}", PROG)
m("H-13 a paren is refused as a redirect's missing target", "internal/shape/shape.go",
  "\tcase !next.meta, next.text == \"(\":\n", "\tcase !next.meta:\n", "TestProgramIsAProgram")
m("H-13 a process substitution is refused as a redirect's target", "internal/shape/shape.go",
  "\treturn !procSub(toks, i+1)\n}", "\treturn true\n}", "TestProgramIsAProgram")
m("H-13 a paren spaced from its < is read as a process substitution", "internal/shape/shape.go",
  "\treturn next.meta && next.glued && next.text == \"(\"\n}", "\treturn next.meta && next.text == \"(\"\n}", PROG)
m("H-13 a comment where a redirect target belongs is taken for the target", "internal/shape/shape.go",
  "\tcase isComment(next):\n\t\treturn true\n", "\tcase false && isComment(next):\n\t\treturn true\n", PROG)
m("H-13 a C0 control other than NUL and CR is read past", "internal/shape/shape.go",
  "\t\tif c := cmd[i]; c < 0x20 && c != '\\t' && c != '\\n' || c == 0x7f {",
  "\t\tif c := cmd[i]; c == 0 || c == '\\r' || c == 0x7f {", PROG)
m("H-13 a DEL in the line is read past", "internal/shape/shape.go",
  "\t\tif c := cmd[i]; c < 0x20 && c != '\\t' && c != '\\n' || c == 0x7f {",
  "\t\tif c := cmd[i]; c < 0x20 && c != '\\t' && c != '\\n' {", PROG)
m("H-13 ~user is named", "internal/shape/shape.go",
  "\tcase strings.HasPrefix(s, \"~\") && !strings.Contains(s, \"/\"):",
  "\tcase false && strings.HasPrefix(s, \"~\") && !strings.Contains(s, \"/\"):", PROG)
m("H-13 zsh's ^ is read as part of a program name", "internal/shape/shape.go",
  "\t\tcase c == '^' || c == '#':", "\t\tcase c == '#':", PROG)
m("H-13 zsh's # is read as part of a program name", "internal/shape/shape.go",
  "\t\tcase c == '^' || c == '#':", "\t\tcase c == '^':", PROG)
m("H-13 an empty word is named `.`", "internal/shape/shape.go",
  "which reads as the source builtin.\n\t\treturn false", "which reads as the source builtin.\n\t\treturn true", PROG)
m("H-14 argc is counted over the program search's reading of $'...'", "internal/shape/shape.go",
  "\tshaped, err := tokenizeShape(cmd)", "\tshaped, err := tokenizeProgram(cmd)", "TestArgc")
m("H-13 the program search reads $'...' as argc does", "internal/shape/shape.go",
  "\tpshaped, perr := tokenizeProgram(cmd)", "\tpshaped, perr := tokenizeShape(cmd)", PROG)
m("H-13 any earlier $ opens a $'...' string", "internal/shape/tokenize.go",
  "\t\t\tif program && dollarAt >= 0 && dollarAt == i-1 {", "\t\t\tif program && dollarAt >= 0 {", "TestTokenizeProgramReadsANSICQuotes")
m("H-13 a quote at the start of the line is read as $'...'", "internal/shape/tokenize.go",
  "program && dollarAt >= 0 && dollarAt == i-1", "program && dollarAt == i-1", "TestTokenizeProgramReadsANSICQuotes|" + PROG)
m("H-13 bash's $$' is read as zsh's $'...'", "internal/shape/tokenize.go",
  "\t\t\t\tif dollars%2 == 0 {", "\t\t\t\tif false {", "TestTokenizeProgramReadsANSICQuotes|" + PROG)
m("H-13 an odd run of $ is read as bash's $$'", "internal/shape/tokenize.go",
  "\t\t\t\tif dollars%2 == 0 {", "\t\t\t\tif dollars > 1 {", "TestTokenizeProgramReadsANSICQuotes")
m("H-13 an unterminated quote after an expansion is read as a real one", "internal/shape/tokenize.go",
  "\t\tif program && (opaque || expanded) {", "\t\tif false {", "TestTokenizeProgramStops|" + PROG)
m("H-13 an expansion in an earlier word leaves a later quote certain", "internal/shape/tokenize.go",
  "\t\tif program && (opaque || expanded) {", "\t\tif program && opaque {", "TestTokenizeProgramStops|" + PROG)
m("H-13 the count's tokenizer stops where the program search does", "internal/shape/tokenize.go",
  "\t\tif program && (opaque || expanded) {", "\t\tif opaque || expanded {", "TestTokenize")
m("H-14 argc counts a leading assignment prefix again", "internal/shape/shape.go",
  "\t\tn := len(dropLeadingAssignments(toks))", "\t\tn := len(toks)",
  "TestArgcExcludesLeadingAssignments")
m("H-71 the plugin runs a model beside the recorder", "plugin/hooks/hooks.json",
  "\"args\": [\"hook\"],\n            \"timeout\": 5\n          }",
  "\"args\": [\"hook\"],\n            \"timeout\": 5\n          },\n          {\"type\": \"prompt\", \"prompt\": \"Is this call safe?\"}",
  "TestH71_PluginHooksRunOnlyTheRecorder")
# The same model, declared where the hooks.json walk does not look: the
# manifest's own hooks field, which Claude Code loads beside hooks.json. This
# passed every H-71 test until the manifest's key set was closed.
m("H-71 the manifest declares a model-run hook", "plugin/.claude-plugin/plugin.json",
  '"license": "Apache-2.0",',
  '"license": "Apache-2.0",\n  "hooks": {"PreToolUse": [{"matcher": "*", "hooks": [{"type": "prompt", "prompt": "x"}]}]},',
  "TestH71_ManifestDeclaresNoHooks")
m("H-71 a skill's frontmatter declares a model-run hook", "plugin/skills/report/SKILL.md",
  "name: report\n",
  "name: report\nhooks:\n  PreToolUse:\n    - matcher: \"*\"\n      hooks:\n        - type: prompt\n          prompt: x\n",
  "TestH71_MarkdownDeclaresNoHooks")
m("H-14 untokenizable command records argc 0", "internal/shape/shape.go",
  "\tif err == nil {\n\t\tn := len(dropLeadingAssignments(toks))\n\t\ts.Argc = &n\n\t}", "\tn := len(dropLeadingAssignments(toks))\n\tif err != nil {\n\t\tn = 0\n\t}\n\ts.Argc = &n", "TestH14_Untokenizable")
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
  "\t\tn, l, err := remove(doc)\n\t\tremoved, left = n, l\n\t\treturn n > 0, err",
  "\t\tn, l, err := remove(doc)\n\t\tremoved, left = n, l\n\t\treturn true, err", "TestH18_")
m("H-19 report re-reads today's config to judge a past run", "internal/report/report.go",
  "\t\tsess := build(run)\n", "\t\tsess := build(run)\n\t\tif p, err := settings.UserPath(); err == nil {\n\t\t\tif doc, err := settings.Load(p); err == nil {\n\t\t\t\tif ok, _ := install.Present(doc, st.InstallID(), install.EventPreToolUse); !ok {\n\t\t\t\t\tsess.Coverage.add(store.ReasonHookEntryAbsent)\n\t\t\t\t}\n\t\t\t}\n\t\t}\n", "TestH19_")

m("report text renders a count it does not have as 0", "internal/report/text.go",
  "\tif p == nil {\n\t\treturn notRead\n\t}", "\tif p == nil {\n\t\treturn \"0\"\n\t}", "TestReport_")
m("report text renders a comparison it could not make as an empty list", "internal/report/text.go",
  "\tif ids == nil {\n\t\treturn unknown\n\t}", "\tif ids == nil {\n\t\treturn none\n\t}", "TestReport_")
m("report --json is accepted and ignored", "cmd/rashomon/main.go",
  "\t\tcase \"--json\":\n\t\t\tasJSON = true\n\t\tcase \"--redact\":", "\t\tcase \"--json\":\n\t\t\tasJSON = false\n\t\tcase \"--redact\":", "TestReport_")
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
# The three-state join. Each break is a way a tokened run silently loses or
# gains rows, and none of them moves a number anyone is already watching.
# THE SEAM MUTATIONS. A review deleted each of these lines and the whole suite
# stayed green -- the feature could be disconnected at three separate points
# while rendering a line that reads as correct. Unit tests on both leaves,
# nothing on the wire between them.
m("TB1 a non-loopback proxy address is accepted", "internal/posture/posture.go",
  "\tif !isLoopback(v.File.ListenAddr) {", "\tif false {", "TestIsLoopback|TestRead_Refuses")
m("TB1 the loopback check matches a prefix", "internal/posture/posture.go",
  "\tswitch strings.ToLower(host) {\n\tcase \"127.0.0.1\", \"::1\", \"localhost\":\n\t\treturn true\n\t}\n\treturn false",
  "\treturn strings.HasPrefix(strings.ToLower(host), \"127.0.0.1\") ||\n\t\tstrings.HasPrefix(strings.ToLower(host), \"localhost\") || host == \"::1\"",
  "TestIsLoopback")
# The nono adapter. A verification review found the sweep had NO entry for any
# line of it, so the sweep certified nothing about a fourth evidence source --
# and separately found that five mutations to the plain-HTTP correction all
# survived the whole suite. These are the breaks with judging tests that exist.
m("NONO suppressTrail is removed, so forget --host is bypassed",
  "internal/report/nono.go", "\tobs = suppressTrail(obs, forgotten)", "\t_ = forgotten",
  "TestSeam_AForgotten")
m("NONO plain HTTP stops meaning ONLY plain HTTP", "internal/report/nono.go",
  "plainOnly[h] && !observable[h]", "plainOnly[h]", "TestSeam_PlainHTTP")
m("NONO the plain-HTTP list is never populated", "internal/report/nono.go",
  "n.PlainHTTP = append(n.PlainHTTP, h)", "_ = h", "TestSeam_PlainHTTP")
m("NONO a denied host is reported as missing from the trail",
  "internal/report/nono.go", "\t\tif denied[h] {", "\t\tif false {",
  "TestSeam_TheReconciliation")
m("NONO loopback is reported as traffic the sandbox missed",
  "internal/report/nono.go",
  "return loopbackHosts[h] || clientPlane[h]", "return clientPlane[h]",
  "TestSeam_TheReconciliation")
m("NONO the client plane is reported as traffic the sandbox missed",
  "internal/report/nono.go",
  "\tfor _, h := range dests.ClientPlane {\n\t\tclientPlane[h] = true\n\t}", "",
  "TestSeam_TheReconciliation")
m("NONO redaction skips the sandbox section", "internal/report/redact.go",
  "\t\ts.Nono = redactNono(sess.Nono, key)", "", "TestRedact_EveryDeclared")
# Simulates the defect the vocabulary exists for: somebody adds a host-bearing
# field and classifies nothing. Mutating the test's own collection loop was a
# no-op, because with a complete vocabulary there is nothing for it to collect.
m("NONO a new host field appears with no classification", "internal/report/nono.go",
  "\tSessions  int `json:\"sessions\"`",
  "\tSessions  int `json:\"sessions\"`\n\tNewHosts []string `json:\"new_hosts\"`",
  "TestRedact_TheHostBearingSet")
m("NONO a file of unlearned record types reads as a quiet sandbox",
  "internal/nono/nono.go", "\t\tswitch head.Type {", "\t\tparsed++\n\t\tswitch head.Type {",
  "TestRead_AFileOfUnlearned")
m("NONO the drop counters are discarded on the bail-out path",
  "internal/nono/nono.go", "\t\tobs.Reason = NotObservedNoRecords",
  "\t\tobs = Observation{Trail: path}\n\t\tobs.Reason = NotObservedNoRecords",
  "TestRead_AFileOfUnlearned")
m("NONO the trail is not read at all from the CLI", "cmd/rashomon/main.go",
  "report.WithNonoTrail(nonoTrail)", "report.WithNonoTrail(\"\")", "TestH31_NonoTrail")
m("TB1 SEAM the token never reaches the join", "internal/report/report.go",
  "\t\tw.RunID = cfg.runToken", "", "TestSeam")
m("TB1 SEAM the counters never cross into the report", "internal/report/destinations.go",
  "\t\tTokenMatched:        obs.TokenMatched,", "\t\tTokenMatched:        0,", "TestSeam")
m("TB1 SEAM the join line is never rendered", "internal/report/text.go",
  "\twriteJoin(b, d)", "", "TestSeam")
m("TB1 SEAM an unverifiable tag is treated as another session",
  "internal/wire/wire.go", "\tif w.IsOurs != nil && w.IsOurs(r.runID) {", "\tif true {",
  "TestSeam|TestToken")
m("TB1 SEAM a foreign dial failure reaches our outcome map", "internal/wire/wire.go",
  "\t\t\tif joinOf(r, w) == joinOther {\n\t\t\t\tcontinue\n\t\t\t}", "", "TestToken")
# RETIRED: "TB1 a token is minted from a posture that was refused". Its line,
# `if v.Export && v.File.SessionToken {`, no longer exists: the mint moved
# inside run's `if v.Export {` branch, so the refused path cannot reach it by
# construction and there is no conjunct left to drop. What the mutant guarded
# -- a refused posture leaking into the report -- is now held by
# "H-27 a refused posture's report reads the store it names" below.
m("TB1 untokened rows are dropped from a tokened run", "internal/wire/wire.go",
  "\tif w.RunID == \"\" || r.runID == \"\" {\n\t\treturn joinWindow\n\t}",
  "\tif w.RunID == \"\" {\n\t\treturn joinWindow\n\t}\n\tif r.runID == \"\" {\n\t\treturn joinOther\n\t}",
  "TestToken")
m("TB1 another run's rows are admitted", "internal/wire/wire.go",
  "\tcase joinOther:\n\t\treturn true", "\tcase joinOther:\n\t\treturn false", "TestToken")
m("TB1 a token-matched row outside the window is inherited", "internal/wire/wire.go",
  "\tcase joinToken:\n\t\t// The token was issued to this process and no other, so it outranks the\n\t\t// clock. A row carrying it outside the window is this run's row with a\n\t\t// bad timestamp -- which is a real case, since the proxy stamps rows\n\t\t// from its own clock and a session can outlive a skew correction.\n\t\treturn false",
  "\tcase joinToken:\n\t\tbreak", "TestToken")
# Re-anchored from `if v.Export && v.File.SessionToken {` to the capability
# check alone, now nested inside run's accepted branch.
m("TB1 the token is sent to a proxy that never advertised it", "cmd/rashomon/main.go",
  "\t\tif v.File.SessionToken {\n\t\t\ttoken = launch.NewToken(runKey)",
  "\t\tif true {\n\t\t\ttoken = launch.NewToken(runKey)", "TestH27")
m("TB1 the token is not carried into the child's environment", "internal/launch/launch.go",
  "\tif token != \"\" {\n\t\taddr = \"http://\" + ProxyUser + \":\" + token + \"@\" + listenAddr\n\t}", "",
  "TestEnv|TestTokenFromProxyURL")
m("TB1 somebody else's proxy credentials are read as a session tag",
  "internal/launch/token.go", "\tif !ok || user != ProxyUser {", "\tif !ok || user == \"\" {",
  "TestTokenFromProxyURL")
m("TB1 the render claims a token join over window-matched rows",
  "internal/report/text.go",
  "\t\tif d.WindowMatched > 0 {\n\t\t\tfmt.Fprintln(b, \"    some clients do not send the proxy credential and are \"+",
  "\t\tif false {\n\t\t\tfmt.Fprintln(b, \"    some clients do not send the proxy credential and are \"+",
  "TestJoinRender")
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
  "\t\t\tif !closed {\n\t\t\t\treturn toks, unterminated()",
  "\t\t\tif !closed {\n\t\t\t\tstarted = true\n\t\t\t\tflush()\n\t\t\t\treturn toks, unterminated()", "TestTokenize")
m("settings accepts a duplicate key", "internal/settings/document.go",
  "\t\tif seen[key] {\n\t\t\treturn nil, fmt.Errorf(\"duplicate key %q\", key)\n\t\t}\n\t\tseen[key] = true", "\t\tseen[key] = true",
  "TestParseRefuses|TestHookEntriesRefusesDuplicateEventKeys")
m("settings accepts trailing data after the object", "internal/settings/document.go",
  "\t\tif _, err := dec.Token(); err != io.EOF {\n\t\t\treturn nil, errors.New(\"trailing data after the top-level object\")\n\t\t}", "\t\t_ = io.EOF",
  "TestParseRefuses")
# The two breaks below live between shape.Hosts and the record, which is the
# only ground the H-70 acceptance items cover that the unit table does not.
# An audit measured the rest: across twenty mutations of hosts.go the
# acceptance layer caught none the unit layer missed, so these two are what
# the layer is for.
m("H-70 the ssh list is extracted and dropped", "internal/hook/handle.go",
  "\tdecl.Hosts, decl.SSHHosts = shape.Hosts(p.ToolName, p.ToolInput)",
  "\tdecl.Hosts, _ = shape.Hosts(p.ToolName, p.ToolInput)", "TestH70_")
m("H-70 a host of the extractor's own invention", "internal/shape/hosts.go",
  "\tsort.Strings(out)\n\treturn out\n}\n\n// sshDestPrograms",
  "\tout = append(out, \"fabricated.example.com\")\n\tsort.Strings(out)\n\treturn out\n}\n\n// sshDestPrograms",
  "TestH70_SSHDestinationsReachTheRecord")
m("H-70 a destination carrying a password is stripped, not refused", "internal/shape/hosts.go",
  "\t\tif strings.IndexByte(prefix[:at], ':') >= 0 {\n\t\t\treturn \"\", false\n\t\t}",
  "\t\tif false {\n\t\t\treturn \"\", false\n\t\t}", "TestH70_CommandText|TestSSHDestinationRefusesACredential")
m("H-70 a program's arguments run to the end of the line", "internal/shape/hosts.go",
  "\t\tend := i + 1\n\t\tfor end < len(toks) && !(end < len(meta) && meta[end]) {\n\t\t\tend++\n\t\t}",
  "\t\tend := len(toks)", "TestH70_ArgumentsStop|TestSSHHostsAreSorted")
m("H-70 every program is an ssh program", "internal/shape/hosts.go",
  "\t\tif !sshDestPrograms[prog] {\n\t\t\tcontinue\n\t\t}",
  "\t\tif false {\n\t\t\tcontinue\n\t\t}", "TestH70_Local|TestSSHDestinations")
m("H-70 the host list is not sorted", "internal/shape/hosts.go",
  "\tsort.Strings(out)\n\treturn out\n}\n\n// sshDestPrograms", "\treturn out\n}\n\n// sshDestPrograms",
  "TestSSHHostsAreSortedAndDeduped")
m("H-70 the destination is not required to be hostname shaped", "internal/shape/hosts.go",
  "\tif !hostnameShaped(hostPart) {\n\t\treturn \"\", false\n\t}",
  "\tif hostPart == \"\" {\n\t\treturn \"\", false\n\t}",
  "TestH70_CommandText|TestSSHDestinationIsHostnameShaped|TestSSHDestinationRefusesWhatIsNotAHost")
m("H-70 one flag table for all four programs", "internal/shape/hosts.go",
  "var valueFlags = map[string]string{\n\t\"ssh\":   \"BbcDEeFIiJLlmOoPpQRSWw\",\n\t\"scp\":   \"cDFiJloPSX\",\n\t\"sftp\":  \"BbcDFiJloPRSsX\",\n\t\"rsync\": \"eBTfM@\",\n}",
  "var valueFlags = map[string]string{\n\t\"ssh\":   \"pPioljJFe\",\n\t\"scp\":   \"pPioljJFe\",\n\t\"sftp\":  \"pPioljJFe\",\n\t\"rsync\": \"pPioljJFe\",\n}",
  "TestH70_Forwarding|TestSSHFlagTables")
m("H-70 ssh hosts come from URL schemes only", "internal/shape/hosts.go",
  "\t\tssh = mergeHosts(ssh, sshCommandHosts(text))", "\t\t_ = sshCommandHosts", "TestH70_SSH|TestSSHDestinations")
m("H-70 every non-flag argument is recorded as a host", "internal/shape/hosts.go",
  "\t\tif prog == \"ssh\" || prog == \"sftp\" {\n\t\t\tif h, ok := destinationHost(a, false); ok {\n\t\t\t\tout = append(out, h)\n\t\t\t}\n\t\t\treturn out\n\t\t}\n\t\tif h, ok := destinationHost(a, true); ok {\n\t\t\tout = append(out, h)\n\t\t}",
  "\t\tif h, ok := destinationHost(a, false); ok {\n\t\t\tout = append(out, h)\n\t\t}", "TestH70_Local|TestSSHDestinations")
m("H-70 the path is not split off before the user", "internal/shape/hosts.go",
  "\tprefix := tok\n\tif slash := strings.IndexByte(tok, '/'); slash >= 0 {\n\t\tprefix = tok[:slash]\n\t}",
  "\tprefix := tok", "TestSSHFlagTables|TestH70_SSH")
m("H-27 run reports the newest session whether or not the command produced it", "cmd/rashomon/main.go",
  "\tif newest == \"\" || writtenAt.Before(since) {", "\tif _ = writtenAt; newest == \"\" {",
  "TestH27_ReportsNothingWhenTheCommandRecordedNothing")
# The alpha's dormant proxy path. env's guard is the live hazard: without it,
# `eval $(rashomon env)` on a machine with no proxy points the shell's HTTPS at
# a port where nothing listens. Three breaks, because there are three ways to
# lose it -- dropping the check, weakening it to "a status file exists" (which
# the no-proxy test alone cannot tell from the real thing, and which would
# export at an enforcing proxy), and running it before the arguments, which
# makes `env --help` answer "no proxy".
m("H-22 env exports without checking the proxy", "cmd/rashomon/main.go",
  "\tv := posture.Read(posture.DefaultPath())\n\tif !v.Export {",
  "\tv := posture.Read(posture.DefaultPath())\n\tif false {",
  "TestH22_EnvRefuses")
m("H-22 env's check weakened to 'a status file exists'", "cmd/rashomon/main.go",
  "\tv := posture.Read(posture.DefaultPath())\n\tif !v.Export {",
  "\tv := posture.Read(posture.DefaultPath())\n\tif v.File.Product == \"\" {",
  "TestH22_EnvRefusesEveryPostureRunRefuses")
m("H-22 env consults the proxy before reading its own arguments", "cmd/rashomon/main.go",
  "\tport, portGiven, err := envPort(args)\n\tif err != nil {\n\t\treturn err\n\t}\n"
  "\tv := posture.Read(posture.DefaultPath())\n\tif !v.Export {\n"
  "\t\treturn fmt.Errorf(\"env: printing no proxy variables -- %s\", v.Reason)\n\t}\n",
  "\tv := posture.Read(posture.DefaultPath())\n\tif !v.Export {\n"
  "\t\treturn fmt.Errorf(\"env: printing no proxy variables -- %s\", v.Reason)\n\t}\n"
  "\tport, portGiven, err := envPort(args)\n\tif err != nil {\n\t\treturn err\n\t}\n",
  "TestH22_EnvArgumentsAnswerWithoutAProxy|TestH22_EnvRejectsABadPort")
# What env exports once the check passes. The verified address, never the
# default port env used to print as a constant; --port confirms it and cannot
# pick another; --token asks the verified file whether the proxy accepts a tag.
# Re-anchored in loop 2: env exports posture's parsed listener, rebuilt
# (v.Listen.String()), where it exported the file's raw v.File.ListenAddr; and
# --port compares against the parsed port, where it called main.go's own
# listenPort, which is gone.
m("H-22 env exports the default address instead of the verified one", "cmd/rashomon/main.go",
  "\tfor _, kv := range launch.Env(v.Listen.String(), token) {",
  "\tfor _, kv := range launch.Env(\"127.0.0.1:18080\", token) {",
  "TestH22_EnvExportsTheVerifiedAddress")
m("H-22 env accepts a --port the verified listener does not hold", "cmd/rashomon/main.go",
  "\tif portGiven && port != v.Listen.Port {",
  "\tif false && portGiven && port != v.Listen.Port {",
  "TestH22_EnvPortMustNameTheVerifiedListener")
# NOT A MUTANT, deliberately: "env exports the file's raw listen_addr instead
# of the rebuilt one". Under the grammar every accepted address IS its rebuilt
# form -- an exact host literal, ':' and plain digits -- so that break changes
# no output and would be reported as undetected when it is equivalent. The
# rebuild is defence in depth behind the parse; the parse mutants below are
# what hold the property, and they go red.
#
# The listener grammar (CWE-78). posture read only the HOST of listen_addr, so
# "127.0.0.1:1;cmd" passed and env printed the command into the operator's
# eval. Three ways to lose the fix: a portless address defaulted to a port, a
# port read up to its first non-digit (strconv-prefix style), and the parse
# not consulted at all.
m("CWE-78 a portless listen address falls back to a default port", "internal/posture/posture.go",
  "\tif i < 0 {\n\t\treturn Listen{}, false\n\t}",
  "\tif i < 0 {\n\t\taddr, i = addr+\":18080\", len(addr)\n\t}",
  "TestParseListen|TestRead_RefusesAListenAddress|TestH22_EnvRefusesAListenAddress")
m("CWE-78 a port is read up to its first non-digit, dropping the junk after it", "internal/posture/posture.go",
  "\t\tif c < '0' || c > '9' {\n\t\t\treturn 0, false\n\t\t}",
  "\t\tif c < '0' || c > '9' {\n\t\t\tbreak\n\t\t}",
  "TestParseListen|TestRead_RefusesAListenAddress|TestH22_EnvRefusesAListenAddress|TestH22_AListenAddressNeverReachesEval")
m("CWE-78 posture never consults the listener grammar", "internal/posture/posture.go",
  "\tlisten, ok := parseListen(v.File.ListenAddr)\n\tif !ok {",
  "\tlisten, ok := parseListen(v.File.ListenAddr)\n\tif false && !ok {",
  "TestRead_RefusesAListenAddress|TestH22_EnvRefusesAListenAddress|TestH22_AListenAddressNeverReachesEval|TestH27_LaunchesWithoutVariables")
m("H-22 env --token ignores the proxy's capability", "cmd/rashomon/main.go",
  "\tif !v.File.SessionToken {", "\tif false {",
  "TestH22_EnvTokenIsCapabilityGated")
# run's automatic report under a refused posture. posture.Read fills v.File
# before it decides, so without the gate a stale, crashed, enforcing or
# malformed status file chooses the database the report reads as the wire.
m("H-27 a refused posture's report reads the store it names", "cmd/rashomon/main.go",
  "\tif v.Export {\n\t\t// The proxy told us where it writes",
  "\tif true {\n\t\t// The proxy told us where it writes",
  "TestH27_ARefusedPostureReadsNoProxyStore")
# And under an ACCEPTED posture that named no causal_db, run falls back to the
# default path. Deleting the fallback left the whole suite green in loop 1:
# the accepted row named its store, so nothing read the default.
m("H-27 run's accepted report drops the default-store fallback", "cmd/rashomon/main.go",
  "\t\tproxyStore = v.File.CausalDB\n\t\tif proxyStore == \"\" {\n\t\t\tproxyStore = defaultProxyStore()\n\t\t}\n",
  "\t\tproxyStore = v.File.CausalDB\n",
  "TestH27_ARefusedPostureReadsNoProxyStore")
m("report falls back to the proxy's default store", "cmd/rashomon/main.go",
  "\trep, key, err := reportOrEmpty(sessionID, proxyStore, nonoTrail, time.Now())",
  "\tif proxyStore == \"\" {\n\t\tproxyStore = defaultProxyStore()\n\t}\n"
  "\trep, key, err := reportOrEmpty(sessionID, proxyStore, nonoTrail, time.Now())",
  "TestReport_ReadsAProxyStoreOnlyWhenNamed")
# The collapse. With no store named the proxy block is one line; forcing the
# full block back is what a reader without the proxy used to see.
m("report text renders the whole proxy block with no store named", "internal/report/text.go",
  "\tnamed := cfg.proxyStore != \"\"", "\tnamed := true",
  "TestReport_ReadsAProxyStoreOnlyWhenNamed|TestText_TheProxyBlock")
# The chain listing's share of the collapse: a state beside each host, the
# legend explaining it, "(not observable)" beside an ssh host -- and the seam
# that carries `named` into the listing at all.
m("report --chain prints a host state with no store named", "internal/report/text.go",
  "\t\tif !named {\n\t\t\tparts = append(parts, h.Host)",
  "\t\tif false {\n\t\t\tparts = append(parts, h.Host)",
  "TestText_TheCollapseReachesTheChainListing")
m("report --chain prints the host-state legend with no store named", "internal/report/text.go",
  "\tif len(c.Prompts) > 0 && named {", "\tif len(c.Prompts) > 0 {",
  "TestText_TheCollapseReachesTheChainListing|TestH31_ChainsGroupCallsUnderTheirPrompt|TestReport_ReadsAProxyStoreOnlyWhenNamed")
m("report --chain calls an ssh host not observable with no store named", "internal/report/text.go",
  "\tif !named {\n\t\treturn \"  ssh: \" + strings.Join(hosts, \", \")\n\t}",
  "",
  "TestText_TheCollapseReachesTheChainListing|TestH31_SSHHostsAreCarriedApartEndToEnd")
m("report --chain is never told whether a store was named", "internal/report/text.go",
  "\twriteChains(b, sess.Chains, cfg.chain, named)", "\twriteChains(b, sess.Chains, cfg.chain, true)",
  "TestText_TheCollapseReachesTheChainListing|TestH31_|TestReport_ReadsAProxyStoreOnlyWhenNamed")
# forget --host prints the baseline count and the proxy sentence only where a
# baseline directory shows a proxy store was read. Both directions.
m("forget --host speaks of a proxy on a machine that never read one", "cmd/rashomon/main.go",
  "\tif !hadBaselines {\n\t\tfmt.Fprintln(stdout)\n\t\treturn nil\n\t}",
  "\tif false && !hadBaselines {\n\t\tfmt.Fprintln(stdout)\n\t\treturn nil\n\t}",
  "TestH25_ForgetHostSaysNothingOfAProxyNeverRead")
m("forget --host drops the proxy sentence after a store was read", "cmd/rashomon/main.go",
  "\tif !hadBaselines {\n\t\tfmt.Fprintln(stdout)\n\t\treturn nil\n\t}",
  "\tif true || !hadBaselines {\n\t\tfmt.Fprintln(stdout)\n\t\treturn nil\n\t}",
  "TestH25_ForgottenHostStaysSuppressedInTheReport")
m("forget --host says \"1 runs\"", "cmd/rashomon/main.go",
  "\tif n == 1 {\n\t\treturn \"1 \" + noun\n\t}", "",
  "TestH25_ForgetHostSaysNothingOfAProxyNeverRead")
# Review of loop 2. Each of these is a way the collapse or the posture reasons
# regress without any line above noticing.
m("report --chain names hosts bare with no legend saying nothing observed them", "internal/report/text.go",
  "\t} else if !named && listsAHost(c) {", "\t} else if false {",
  "TestText_TheCollapseReachesTheChainListing|TestReport_ReadsAProxyStoreOnlyWhenNamed")
m("report text collapses an observed store when the render option was forgotten", "internal/report/text.go",
  "\tnamed := cfg.proxyStore != \"\" || sess.Destinations.Observed", "\tnamed := cfg.proxyStore != \"\"",
  "TestText_AnObservedStoreRendersInFullWithoutTheOption")
m("report takes --proxy-store \"\" as no store named", "cmd/rashomon/main.go",
  "\t\t\tif args[i+1] == \"\" {\n\t\t\t\treturn errors.New(\"--proxy-store needs a non-empty path\")\n\t\t\t}\n", "",
  "TestReport_RefusesAnEmptyProxyStore")
m("CWE-117 the mode refusal repeats connect_mode raw", "internal/posture/posture.go",
  "\"the proxy is in \" + strconv.Quote(v.File.ConnectMode) +", "\"the proxy is in \" + v.File.ConnectMode +",
  "TestRead_AReasonNeverRepeatsAFieldRaw|TestH22_EnvRefusesEveryPostureRunRefuses")
m("CWE-117 the non-loopback refusal repeats listen_addr raw", "internal/posture/posture.go",
  "\t\t\tstrconv.Quote(v.File.ListenAddr) + \"); refusing", "\t\t\tv.File.ListenAddr + \"); refusing",
  "TestRead_AReasonNeverRepeatsAFieldRaw|TestH22_EnvRefusesEveryPostureRunRefuses")
m("the surface check matches nothing", "test/acceptance/dormant_surface_test.go",
  "regexp.MustCompile(`(?i)\\brashomon (run|env)\\b|proxy-store`)", "regexp.MustCompile(`^$x`)",
  "TestSurface_|TestInstall_")
m("the README offers a dormant command", "README.md",
  "\nrashomon watch                      # install the recorders",
  "\nrashomon run -- claude              # record destinations too\nrashomon watch                      # install the recorders",
  "TestSurface_OffersNoDormantProxyPath")
# The installer places one binary. A second fetch is the loop-0 defect: the
# closed proxy's archive downloaded because a release happened to carry it.
m("install.sh fetches a second archive", "install.sh",
  "say \"  installed ${INSTALL_DIR}/rashomon\"\n",
  "say \"  installed ${INSTALL_DIR}/rashomon\"\n"
  "fetch \"${BASE_URL}/${TAG}/altrace_${VERSION}_${OS}_${ARCH}.tar.gz\" \"$TMP/altrace.tar.gz\" || true\n",
  "TestInstall_")
m("install.sh drops the alpha's closing line", "install.sh",
  "say \"Network destinations are not observed in this alpha. Declarations and\"\n",
  "say \"Declarations and\"\n",
  "TestInstall_")
# The README's example is pinned to the render. Either side drifting fails it.
m("the README excerpt drifts from the render", "README.md",
  "        agent-a41f (Explore): 3 declarations, 3 executions, 1 Bash\n",
  "        agent-a41f (Explore): 3 declarations, 3 executions\n",
  "TestReadme_")
m("the render drifts from the README excerpt", "internal/report/text.go",
  "\tfmt.Fprintln(b, \"    (these calls do not appear in the main transcript)\")",
  "\tfmt.Fprintln(b, \"    (these calls are not in the main transcript)\")",
  "TestReadme_")
m("usage advertises the dormant env command", "cmd/rashomon/main.go",
  "  rashomon version               print the version\n",
  "  rashomon env [--port N]        print the proxy variables to export\n"
  "  rashomon version               print the version\n",
  "TestUsage_AdvertisesNoDormantProxyPath")
m("H-6 detach has no way past an entry someone edited", "cmd/rashomon/main.go",
  "\t\tcase \"--force\":", "\t\tcase \"--force-disabled\":", "TestH6_DetachForceRemovesAnEditedEntry")
m("H-6 --force also removes hooks that are not ours", "internal/install/install.go",
  "\t\t\tid := Owner(e, event)\n\t\t\tif id == \"\" || !match(id) {", "\t\t\tid := Owner(e, event)\n\t\t\tif id == \"\" && !force || id != \"\" && !match(id) {",
  "TestH6_DetachForceRemovesAnEditedEntry|TestH4_")
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
  "\t\t\tbyLabel[knownLabel(*d.FileLabel)]++",
  "\t\t\tbyLabel[*d.FileLabel]++", "TestByLabel_")
m("H-46 labelling recovers after the append instead of before it", "internal/hook/handle.go",
  "\tdefer func() {\n\t\tif v := recover(); v != nil {\n\t\t\tlabel = shape.LabelUnknown\n\t\t}\n\t}()\n\tfault.Inject(fault.PointLabel)",
  "\tfault.Inject(fault.PointLabel)", "TestH46_NoFault")

m("a paused session probe counts as the session's start or end", "internal/report/report.go",
  "\t\tcase paused:\n", "\t\tcase paused && false:\n", "TestPausedSessionProbeIsNotReportedAsRecorded")
# Part 1 -- the plugin, and origin-aware ownership. H-71 through H-77 are
# provisional from H-71 (H-70 is the highest item on the merge target) and are
# grepped against it before they become the contract.
m("H-71 coverage reverts to the settings-only Present check", "internal/hook/coverage.go",
  "\tcase pluginPresent:\n\t\tres.HookEntry = store.EntryPresentPlugin",
  "\tcase false && pluginPresent:\n\t\tres.HookEntry = store.EntryPresentPlugin",
  "TestH71_|TestH72_")
m("H-73 watch's live-plugin refusal is dropped", "cmd/rashomon/main.go",
  "\t} else if p != nil && p.Enabled && len(p.Events) > 0 {",
  "\t} else if false && p != nil && p.Enabled && len(p.Events) > 0 {",
  "TestH73_")
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
m("H-75 status collapses the two origins into one line", "cmd/rashomon/main.go",
  "\tif settingsLive && pluginLive {",
  "\tif false && settingsLive && pluginLive {",
  "TestH75_")
m("H-76 the plugin's watch skill keeps the standalone three-path preamble", "plugin/skills/watch/SKILL.md",
  "1. The binary is `${CLAUDE_PLUGIN_ROOT}/bin/rashomon` — it ships with this\n   plugin, so there is nothing to resolve and nothing to build. Set\n   `$RASHOMON` to that path.",
  "1. Resolve the binary: `~/.local/bin/rashomon`, then `~/go/bin/rashomon`,\n   then `command -v rashomon`.",
  "TestH76_")
m("H-77 the manifest ships enabled by default", "plugin/.claude-plugin/plugin.json",
  '"defaultEnabled": false', '"defaultEnabled": true', "TestH77_")
m("H-103 the duplicate-declarations guard is dropped", "internal/report/report.go",
  "\tif HasDuplicateToolUseID(run.Declarations) {\n\t\tsess.Coverage.add(ReasonDuplicateDeclarations)\n\t}\n",
  "", "TestH103_")
m("H-103 the shared duplicate tool_use_id rule never fires", "internal/report/report.go",
  "\t\tif seen[d.ToolUseID] {\n\t\t\treturn true\n\t\t}",
  "\t\tif false && seen[d.ToolUseID] {\n\t\t\treturn true\n\t\t}",
  "TestH103_DuplicateDeclarationsAreNamedNotHidden|TestHasDuplicateToolUseID|TestBuild_DuplicateDeclarationsMakeTheTurnUnverified")
m("H-103 the turn digest drops the duplicate-declarations guard", "internal/digest/coverage.go",
  "\tif report.HasDuplicateToolUseID(w.declarations) {\n\t\ttc.add(report.ReasonDuplicateDeclarations)\n\t}\n",
  "", "TestBuild_DuplicateDeclarationsMakeTheTurnUnverified")
# Which report command the recap line points at is decided in cmdRecap, and
# nothing judged it until H-91's pointer test: H-91's first test checks only
# "--session <id>", which both commands contain. Three breaks, one per way the
# decision can go wrong -- ignore the marker (the shipped defect: a settings
# install made from the plugin's binary was sent to /rashomon:report), or
# replace the decision with either constant.
m("H-91 the recap pointer ignores the --install marker", "cmd/rashomon/main.go",
  "\t\tif installArg(args) == \"\" {\n\t\t\tif present, perr := install.PluginPresent(install.EventStop); perr == nil {",
  "\t\tif true {\n\t\t\tif present, perr := install.PluginPresent(install.EventStop); perr == nil {",
  "TestH91_TheReportPointer")
m("H-91 the recap pointer is always the CLI command", "cmd/rashomon/main.go",
  "\t\t\t\tfromPlugin = present\n", "\t\t\t\t_ = present\n", "TestH91_TheReportPointer")
m("H-91 the recap pointer is always the plugin command", "cmd/rashomon/main.go",
  "\t\tfromPlugin := false\n\t\tif installArg(args) == \"\" {",
  "\t\tfromPlugin := true\n\t\tif false && installArg(args) == \"\" {", "TestH91_TheReportPointer")
m("a legacy present record is treated with suspicion and renders unverified", "internal/report/report.go",
  "\t\t\tf.Reasons = append(f.Reasons, *c.Reason)\n\t\t}\n\t}\n\treturn f\n}",
  "\t\t\tf.Reasons = append(f.Reasons, *c.Reason)\n\t\t}\n\t\tif c.HookEntry == store.EntryPresent {\n\t\t\tf.Reasons = append(f.Reasons, ReasonRunNotClosed)\n\t\t}\n\t}\n\treturn f\n}",
  "TestLegacyPresentRendersVerified")
# Field findings from running the plugin against real sessions. Each break is
# the defect as it shipped, so the sweep proves the fix is still load-bearing.
m("FF a refusal outside the permission prompt is read as an execution", "internal/report/transcript.go",
  "\tfor _, p := range deniedPrefixes {", "\tfor _, p := range deniedPrefixes[:1] {",
  "TestTranscript_RefusalsOutside")
m("FF workflow subagent transcripts are not read", "internal/report/transcript.go",
  "\t\tif ok, _ := filepath.Match(\"agent-*.jsonl\", d.Name()); ok {",
  "\t\tif ok, _ := filepath.Match(\"agent-*.jsonl\", d.Name()); ok && filepath.Dir(p) == dir {",
  "TestTranscript_WorkflowSubagent")
m("FF a harness-completed input counts as a rewrite", "internal/report/destinations.go",
  "\t\tif d.Shape.Digest == \"\" || inputCompletedByHarness[d.ToolName] {",
  "\t\tif d.Shape.Digest == \"\" {", "TestRewritten_")
m("FF the program is cd for every cd-and-command line", "internal/shape/shape.go",
  "\t\tif i, ok = pastDirectoryChange(pshaped, i, uncertain); ok {", "\t\tif i, ok = i, true; ok {",
  "TestProgramLooksPastADirectoryChange|TestProgramIsAProgram")
m("FF the separator after cd is found without the command-end scan", "internal/shape/shape.go",
  "\tsep, ok := commandEnd(toks, i, uncertain)\n\tif !ok {\n\t\treturn 0, false\n\t}\n",
  "\tsep := -1\n\tfor j := i + 1; j < len(toks); j++ {\n\t\tif toks[j].meta {\n\t\t\tsep = j\n\t\t\tbreak\n\t\t}\n\t}\n",
  "TestProgramLooksPastADirectoryChange")
m("FF a separator in a comment after cd is followed", "internal/shape/shape.go",
  "\t\tif isComment(toks[j]) {\n\t\t\treturn i, true\n",
  "\t\tif false && isComment(toks[j]) {\n\t\t\treturn i, true\n",
  "TestProgramLooksPastADirectoryChange")
m("FF a refused turn is never reported", "internal/recap/state.go",
  "\treturn s.Checked[sessionID] != promptID", "\treturn s.Checked[sessionID] != promptID && false",
  "TestH106_")
m("FF the catch-up re-rules on a turn Stop checked", "internal/recap/state.go",
  "\t\ts.Checked[sessionID] = promptID\n", "\t\t_ = promptID\n",
  "TestH106_")

# `rashomon spend`. Each rule in internal/spend -- the read path, the dedupe,
# the window, the price table, the cold-cache heuristic, refusals and extra
# attempts, and the join to the store -- broken once.
m("SP a response's lines are summed, not counted once", "internal/spend/scan.go",
  "\t\tkeep(prev, cand)\n", "\t\tsc.Responses = append(sc.Responses, cand)\n\t\t_ = prev\n", "TestDedupe_")
m("SP the dedupe is per file, so a response carried into a second file counts twice", "internal/spend/scan.go",
  "\tbyID := map[string]*Response{}\n\tfor i, f := range found.Files {\n",
  "\tfor i, f := range found.Files {\n\t\tbyID := map[string]*Response{}\n", "TestDedupe_OneResponseInTwoFiles")
m("SP the first line of a response is kept, not the completed one", "internal/spend/scan.go",
  "\tif !better {\n\t\treturn\n\t}", "\tif !better || true {\n\t\treturn\n\t}", "TestDedupe_TheCompletedLine")
m("SP a response starts at its latest line, not its earliest", "internal/spend/scan.go",
  "\tif cand.StartMS != 0 && (prev.StartMS == 0 || cand.StartMS < prev.StartMS) {",
  "\tif cand.StartMS != 0 && (prev.StartMS == 0 || cand.StartMS > prev.StartMS) {", "TestDedupe_TheCompletedLine")
m("SP the transcript line decodes message.content", "internal/spend/scan.go",
  "\tModel       string       `json:\"model\"`\n", "\tModel       string       `json:\"model\"`\n\tContent     json.RawMessage `json:\"content\"`\n",
  "TestContentHasNoFieldToLandIn")
m("SP the --days window is ignored", "internal/spend/spend.go",
  "r.StartMS == 0 || r.StartMS < s.FromUnixMS {", "r.StartMS == 0 {", "TestWindow_")
m("SP an undated response is dropped without being counted", "internal/spend/scan.go",
  "\t\t\tsc.Undated++\n", "", "TestWindow_")
m("SP a file last written before the window is still read", "internal/spend/scan.go",
  "\t\t\tstale[resolved(p)] = true\n\t\t\treturn false", "\t\t\tstale[resolved(p)] = true\n\t\t\treturn true",
  "TestDiscover_Skips")
m("SP a file under subagents/ is main-agent spend unless its lines say sidechain", "internal/spend/scan.go",
  "Subagent:   f.Subagent || l.IsSidechain,", "Subagent:   l.IsSidechain,", "TestAgent_")
m("SP a main-file line marked isSidechain is main-agent spend", "internal/spend/scan.go",
  "Subagent:   f.Subagent || l.IsSidechain,", "Subagent:   f.Subagent,", "TestAgent_")
m("SP a workflow's subagent transcripts two levels down are not read", "internal/spend/scan.go",
  "\t\tif ok, _ := filepath.Match(\"agent-*.jsonl\", d.Name()); ok {",
  "\t\tif ok, _ := filepath.Match(\"agent-*.jsonl\", d.Name()); ok && filepath.Dir(p) == dir {", "TestAgent_")
m("SP any trailing hyphenated word is read as a dated suffix", "internal/spend/price.go",
  "\tif i < 0 || !isDate(model[i+1:]) {", "\tif i < 0 {", "TestPriceKey_")
m("SP a seven-digit suffix passes as a date", "internal/spend/price.go",
  "\tif len(s) != 8 {", "\tif len(s) < 7 {", "TestPriceKey_")
m("SP a 5m cache write is priced at the input rate", "internal/spend/price.go",
  "return r.Input * 5 / 4 }", "return r.Input }", "TestPricing_")
m("SP a 1h cache write is priced as a 5m one", "internal/spend/price.go",
  "return r.Input * 2 }", "return r.Input * 5 / 4 }", "TestPricing_")
m("SP Fable 5.1's cache read is 0.1x input, not the table's 0.25", "internal/spend/price.go",
  "\t\"claude-fable-5-1\": {Input: mtok(1000), Output: mtok(5000), CacheRead: mtok(25)},", "\t\"claude-fable-5-1\": {Input: mtok(1000), Output: mtok(5000), CacheRead: mtok(100)},", "TestPricing_TheTable")
m("SP an unsplit cache write is priced at the 1h rate", "internal/spend/scan.go",
  "\t\tout.CacheWrite5m += rest", "\t\tout.CacheWrite1h += rest", "TestPricing_CacheWrites")
m("SP an unknown model is priced at $0", "internal/spend/spend.go",
  "\tc.addUnpriced(r.Tokens.Total())\n", "\tc.addPriced(0)\n", "TestUnknownModel_")
m("SP an unpriced total marshals as usd 0 instead of null", "internal/spend/spend.go",
  "\tif !c.Wholly() {\n\t\treturn nil\n", "\tif false {\n\t\treturn nil\n", "TestUnknownModel_IsUnknown")
m("SP an unpriced total is headlined as a dollar figure", "internal/spend/text.go",
  "\tif !c.Wholly() {\n\t\treturn fmt.Sprintf(\"cost unknown (", "\tif false {\n\t\treturn fmt.Sprintf(\"cost unknown (",
  "TestUnknownModel_IsUnknown")
m("SP a non-claude model id is printed verbatim", "internal/spend/spend.go",
  "\treturn \"other\", false", "\treturn model, false", "TestUnknownModel_ANonClaude")
m("SP a claude-prefixed id is printed whatever it carries", "internal/spend/spend.go",
  "\t\tif !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {", "\t\tif c == 0 {", "TestUnknownModel_ANonClaude")
m("SP a zero-token synthetic response is listed as a model", "internal/spend/spend.go",
  "\t\tif r.Tokens.Total() == 0 {\n\t\t\tif r.StopReason == \"refusal\"", "\t\tif false {\n\t\t\tif r.StopReason == \"refusal\"", "TestZeroTokenResponses_")
m("SP an amount under a cent renders as $0.00", "internal/spend/text.go",
  "\tif nano > 0 && nano < 1e7 {", "\tif false && nano < 1e7 {", "TestSubCentAmounts_")
m("SP the plan-subscription sentence is not printed", "internal/spend/text.go",
  "\tfmt.Fprintf(&b, \"       %s\\n\", s.Pricing.Note)\n", "", "TestOutput_StatesTheBasis")
m("SP a 5m write is judged against the 1h TTL", "internal/spend/spend.go",
  "\t\t\tif gap > ttl5m {", "\t\t\tif gap > ttl1h {", "TestCacheExpiry_")
m("SP a 1h write is judged against the 5m TTL", "internal/spend/spend.go",
  "\t\t\tif gap > ttl1h {", "\t\t\tif gap > ttl5m {", "TestCacheExpiry_")
m("SP the previous response is taken across files", "internal/spend/spend.go",
  "\t\t\tk := stream{f.idx, r.Subagent}\n\t\t\tbyFile[k] = append(byFile[k], r)\n\t\t}\n\t}\n\tout := map[*Response]Tokens{}\n\tfor k, rs := range byFile {\n\t\tsort.SliceStable(rs, func(i, j int) bool { return rs[i].StartMS < rs[j].StartMS })\n\t\tfor i := 1; i < len(rs); i++ {\n\t\t\tif rs[i].file != k.file {",
  "\t\t\tk := stream{0 * f.idx, r.Subagent}\n\t\t\tbyFile[k] = append(byFile[k], r)\n\t\t}\n\t}\n\tout := map[*Response]Tokens{}\n\tfor k, rs := range byFile {\n\t\tsort.SliceStable(rs, func(i, j int) bool { return rs[i].StartMS < rs[j].StartMS })\n\t\tfor i := 1; i < len(rs); i++ {\n\t\t\tif false && rs[i].file != k.file {",
  "TestCacheExpiry_")
m("SP the window is applied before the previous response is found", "internal/spend/spend.go",
  "\tfor _, r := range sc.Responses {\n\t\tif r.StartMS == 0 || r.Tokens.Total() == 0 {\n\t\t\tcontinue\n\t\t}\n\t\tfor _, f := range r.files {",
  "\tfor _, r := range sc.Responses {\n\t\tif r.StartMS < time.Now().Add(-48*time.Hour).UnixMilli() {\n\t\t\tcontinue\n\t\t}\n\t\tfor _, f := range r.files {",
  "TestCacheExpiry_ThePredecessor")
m("SP a savings line is printed with no figure under it", "internal/spend/spend.go",
  "\tif s.SilentFailureTurns.Cost.Nano > 0 {", "\tif true {",
  "TestNoSavingsWithoutAFigure")
m("SP a refusal is not recognised", "internal/spend/spend.go",
  "\t\tif r.StopReason == \"refusal\" {\n\t\t\ts.Refusals.Responses++", "\t\tif r.StopReason == \"refused\" {\n\t\t\ts.Refusals.Responses++", "TestRefusalsAndExtraAttempts")
m("SP the returned attempt is counted again as an extra one", "internal/spend/scan.go",
  "\tout := make([]Tokens, 0, len(its)-1)\n\tfor _, it := range its[:len(its)-1] {", "\tout := make([]Tokens, 0, len(its)-1)\n\tfor _, it := range its {",
  "TestRefusalsAndExtraAttempts")
m("SP a turn with failures fires whatever its summary says", "internal/spend/join.go",
  "\t\tif !sf.Fires {", "\t\tif sf.Failed == 0 {",
  "TestJoin_AnHonestSummary")
m("SP a firing turn's spend is every main response of its transcript, not its prompt's", "internal/spend/join.go",
  "\t\tif f[prompt].Responses[r.ID] {", "\t\tif len(f[prompt].Responses) > 0 {",
  "TestJoin_ATurnsSpendIsKeyedByItsPrompt")
m("SP the transcript does not tie the response that made a turn's first call to it", "internal/report/transcript.go",
  "\t\t\tif head.Message.ID != \"\" {\n\t\t\t\ttie(head.Message.ID)", "\t\t\tif head.Message.ID != \"\" && bytes.Contains(raw, []byte(`\"text\"`)) {\n\t\t\t\ttie(head.Message.ID)",
  "TestJoin_SpendInside|TestSpend_JoinsSilentlyFailedTurns")
m("SP a subagent's spend is never its turn's", "internal/spend/join.go",
  "\t\treturn r.prompt == prompt && slices.Contains(mains, f.Main)", "\t\treturn false && r.prompt == prompt && slices.Contains(mains, f.Main)",
  "TestJoin_SpendInside")
m("SP a subagent response its transcript ties to another prompt is priced into the turn", "internal/spend/join.go",
  "\t\treturn r.prompt == prompt && slices.Contains(mains, f.Main)", "\t\treturn slices.Contains(mains, f.Main)",
  "TestJoin_ATurnsSpendIsKeyedByItsPrompt")
m("SP a subagent response under another conversation's transcript is priced into the turn", "internal/spend/join.go",
  "\t\treturn r.prompt == prompt && slices.Contains(mains, f.Main)", "\t\treturn r.prompt == prompt || slices.Contains(mains[:0], f.Main)",
  "TestJoin_OneSessionIDTwoConversations")
m("SP a subagent response's prompt is not read from its own transcript", "internal/spend/scan.go",
  "\t\t\tcand.prompt = prompt", "\t\t\tcand.prompt = prompt[:0]", "TestJoin_SpendInside")
m("SP a subagent transcript's user lines are skipped before their promptId is read", "internal/spend/scan.go",
  "\t\tif !usageLine && !f.Subagent {", "\t\tif !usageLine {",
  "TestJoin_SpendInside")
m("SP a subagent user line with no promptId does not end the tie", "internal/spend/scan.go",
  "\t\t\t} else if !l.IsMeta {\n\t\t\t\tprompt = \"\"", "\t\t\t} else if false {\n\t\t\t\tprompt = \"\"",
  "TestJoin_ATurnsSpendIsKeyedByItsPrompt")
m("SP a subagent meta line ends the tie", "internal/spend/scan.go",
  "\t\t\t} else if !l.IsMeta {\n\t\t\t\tprompt = \"\"", "\t\t\t} else {\n\t\t\t\tprompt = \"\"",
  "TestJoin_SpendInside")
m("SP a subagent line that does not decode keeps the tie", "internal/spend/scan.go",
  "\t\t\tprompt = \"\"\n\t\t\tcontinue\n\t\t}", "\t\t\tcontinue\n\t\t}",
  "TestJoin_ATurnsSpendIsKeyedByItsPrompt")
m("SP a subagent user line that does not decode is counted as unparsed usage", "internal/spend/scan.go",
  "\t\t\tif usageLine {\n\t\t\t\tsc.Unparsed++", "\t\t\tif true {\n\t\t\t\tsc.Unparsed++",
  "TestUnparsed_")
m("SP a failed turn with no words to judge goes uncounted", "internal/spend/join.go",
  "\t\tif !sf.FinalMessageAvailable {\n\t\t\tj.Unjudged++", "\t\tif !sf.FinalMessageAvailable {\n\t\t\t_ = sf",
  "TestJoin_AFailedTurnWithNoWords")
m("SP a failed turn whose records name no transcript goes uncounted", "internal/spend/join.go",
  "\t\tif len(mains) == 0 {\n\t\t\tj.Unjudged++", "\t\tif len(mains) == 0 {\n\t\t\t_ = mains",
  "TestJoin_CoverageIsPerTranscript")
m("SP unjudged turns marshal as a checked 0 with nothing covered", "internal/spend/join.go",
  "\tif j.CoveredTranscripts > 0 {\n\t\tout.Turns, out.Unjudged, out.Undeclared, out.Cost = &j.Turns, &j.Unjudged, &j.UndeclaredFailedCalls, &j.Cost",
  "\tout.Unjudged = &j.Unjudged\n\tif j.CoveredTranscripts > 0 {\n\t\tout.Turns, out.Undeclared, out.Cost = &j.Turns, &j.UndeclaredFailedCalls, &j.Cost",
  "TestJoin_NoStore")
m("SP an unjudged failed turn reads as a checked none", "internal/spend/text.go",
  "\tcase j.Turns == 0 && j.Unjudged == 0 && j.UndeclaredFailedCalls == 0:\n", "\tcase j.Turns == 0 && j.UndeclaredFailedCalls == 0:\n", "TestJoin_AFailedTurnWithNoWords")
m("SP the unjudged turn is not said", "internal/spend/text.go",
  "\tif j.Unjudged == 1 {", "\tif false {", "TestJoin_AFailedTurnWithNoWords")
m("SP unjudged turns are not said", "internal/spend/text.go",
  "\t} else if j.Unjudged > 1 {", "\t} else if false {", "TestJoin_AFailedTurnWithNoWords")
m("SP a silent turn before the window is counted", "internal/spend/join.go",
  "\t\t\tif t.lastMS < s.FromUnixMS {", "\t\t\tif false {", "TestJoin_ATurnBefore")
m("SP a transcript counts as covered when its session id has a run directory", "internal/spend/join.go",
  "\t\trun, err := st.ReadRun(id)\n\t\tif err != nil {\n\t\t\treturn err\n\t\t}\n", "\t\trun, err := st.ReadRun(id)\n\t\tif err != nil {\n\t\t\treturn err\n\t\t}\n\t\tfor m, rs := range byTranscript {\n\t\t\tif slices.Contains(rs[0].owners, id) {\n\t\t\t\tcovered[m] = true\n\t\t\t}\n\t\t}\n",
  "TestJoin_CoverageIsPerTranscript")
m("SP a row holding not-covered dollars is not named", "internal/spend/join.go",
  "\t\t\tnamed[displaySession(id)] = true\n", "",
  "TestJoin_SpendInside|TestJoin_CoverageIsPerTranscript")
m("SP a partly recorded session is marked recorded", "internal/spend/join.go",
  "\t\tdefault:\n\t\t\ts.PerSession[i].Coverage = CoveragePartly", "\t\tdefault:\n\t\t\ts.PerSession[i].Coverage = CoverageRecorded",
  "TestJoin_CoverageIsPerTranscript")
m("SP the text does not name the not-covered sessions", "internal/spend/text.go",
  "\tif len(n) == 0 {\n\t\treturn \"\"", "\tif true {\n\t\treturn \"\"",
  "TestJoin_SpendInside|TestJoin_CoverageIsPerTranscript|TestSpend_JoinsSilentlyFailedTurns")
m("SP a covered, clean record reads at least none across 0 turns", "internal/spend/text.go",
  "\tcase j.Turns == 0 && j.Unjudged == 0 && j.UndeclaredFailedCalls == 0:\n", "\tcase false:\n", "TestJoin_ACoveredZeroIsAZero")
m("SP a firing turn with nothing priced reads at least none", "internal/spend/text.go",
  "\tcase j.Cost.Priced == 0 && j.Cost.Unpriced == 0:\n", "\tcase false:\n", "TestJoin_AFiringTurnWithNoResponse")
m("SP the floor note is printed under a none", "internal/spend/text.go",
  "\tif j.Turns > 0 {\n\t\tfmt.Fprintf(&b, \"  (%s)\\n\", j.Bound)", "\tif true {\n\t\tfmt.Fprintf(&b, \"  (%s)\\n\", j.Bound)",
  "TestJoin_ACoveredZeroIsAZero")
m("SP an unrecorded session's spend is folded in as zero", "internal/spend/join.go",
  "\t\tcostOf(&j.NotCoveredCost, r)", "\t\t_ = r", "TestJoin_")
m("SP no store renders as a store that recorded nothing here", "internal/spend/text.go",
  "\tcase j.Store == StoreNone:", "\tcase j.Store == \"never\":", "TestJoin_NoStore|TestSpend_OpensNoStore")
m("SP spend creates a store to read one", "cmd/rashomon/main.go",
  "\tst, err := openStoreForRead()\n\tswitch {", "\tst, err := openStore()\n\tswitch {", "TestSpend_OpensNoStore")
m("SP spend ignores CLAUDE_CONFIG_DIR", "cmd/rashomon/main.go",
  "\tconfigDir, err := settings.ConfigDir()\n\tif err != nil {\n\t\treturn err\n\t}\n\tfiles, err := spend.Discover(configDir,",
  "\tconfigDir, err := os.UserHomeDir()\n\tconfigDir = filepath.Join(configDir, \".claude\")\n\tif err != nil {\n\t\treturn err\n\t}\n\tfiles, err := spend.Discover(configDir,",
  "TestSpend_ReadsWhere")
m("SP --days 0 is accepted", "cmd/rashomon/main.go",
  "if err != nil || n < 1 {", "if err != nil {", "TestSpend_RefusesAWindow")

m("SP a tool-less prompt's reply is read as the previous turn's summary", "internal/report/transcript.go",
  "\t\t\t\tcurrent = head.PromptID\n", "\t\t\t\tif want[head.PromptID] {\n\t\t\t\t\tcurrent = head.PromptID\n\t\t\t\t}\n",
  "TestJoin_AToolLessTurnBetween")
m("SP an unkeyed prompt's reply is credited to the turn before it", "internal/report/transcript.go",
  "\t\t\t} else if !head.IsMeta && !toolResultOnly(raw) {", "\t\t\t} else if false {",
  "TestJoin_AnUnkeyedPrompt|TestFinalAssistantTexts_")
m("SP a tool_result without promptId ends its turn's words", "internal/report/transcript.go",
  "\t\t\t} else if !head.IsMeta && !toolResultOnly(raw) {", "\t\t\t} else if !head.IsMeta {",
  "TestFinalAssistantTexts_")
m("SP a meta line without promptId ends its turn's words", "internal/report/transcript.go",
  "\t\t\t} else if !head.IsMeta && !toolResultOnly(raw) {", "\t\t\t} else if !toolResultOnly(raw) {",
  "TestFinalAssistantTexts_")
m("SP the bound says every unkeyed user line ends a tie", "internal/spend/join.go",
  "a user line with no promptId that is not a meta line or, in the main transcript, a tool result, or after a line that cannot be decoded (any in a subagent transcript; in the main transcript, unless it is a sidechain line), is tied to no turn", "a user line with no promptId is tied to no turn",
  "TestJoin_TheBoundNamesTheUnkeyedLinesThatKeepATie")
m("SP a subagent's sidechain line is read as the turn's summary", "internal/report/transcript.go",
  "\t\tif head.IsSidechain {\n", "\t\tif false && head.IsSidechain {\n",
  "TestFinalAssistantTexts_")
m("SP an unbilled synthetic line counts as the previous request", "internal/spend/spend.go",
  "\t\tif r.StartMS == 0 || r.Tokens.Total() == 0 {\n\t\t\tcontinue\n\t\t}\n\t\tfor _, f := range r.files {",
  "\t\tif r.StartMS == 0 {\n\t\t\tcontinue\n\t\t}\n\t\tfor _, f := range r.files {",
  "TestCacheExpiry_AnUnbilled")
m("SP an unchecked silent-failure line marshals as a checked $0", "internal/spend/join.go",
  "\tif j.CoveredTranscripts > 0 {\n\t\tout.Turns", "\tif true {\n\t\tout.Turns",
  "TestJoin_NoStore")
m("SP by-agent shares are printed beside an unknown", "internal/spend/text.go",
  "\tif main.Nano+sub.Nano == 0 || !main.Known() || !sub.Known() {", "\tif main.Nano+sub.Nano == 0 {",
  "TestAgent_NoShareBesideAnUnknown")
m("SP a user line's tool-result test decodes the text of its blocks", "internal/report/transcript.go",
  "\t\tContent []struct {\n\t\t\tType string `json:\"type\"`\n\t\t} `json:\"content\"`",
  "\t\tContent []struct {\n\t\t\tType string `json:\"type\"`\n\t\t\tText string `json:\"text\"`\n\t\t} `json:\"content\"`",
  "TestUserBlocks_")
m("SP a transcript's session id is printed whatever it carries", "internal/spend/spend.go",
  "\tif idShaped(id) {\n\t\treturn id", "\tif true {\n\t\treturn id", "TestSessionID_")
m("SP the window is a Duration of days, which overflows", "internal/spend/spend.go",
  "\treturn now.AddDate(0, 0, -days)", "\treturn now.Add(-time.Duration(days) * 24 * time.Hour)",
  "TestWindow_AVeryLong")
m("SP --days past the century is accepted", "cmd/rashomon/main.go",
  "\t\t\tif n > spend.MaxDays {", "\t\t\tif false && n > spend.MaxDays {", "TestSpend_RefusesAWindow")
m("SP a future-dated response counts as the last N days", "internal/spend/spend.go",
  "\t\tif r.StartMS > latest && r.Tokens.Total() > 0 {", "\t\tif false && r.StartMS > latest && r.Tokens.Total() > 0 {",
  "TestWindow_AFutureDated")
m("SP a response written while spend runs is future-dated", "internal/spend/spend.go",
  "\tlatest := now.Add(futureSlack).UnixMilli()", "\tlatest := now.UnixMilli()", "TestWindow_AFutureDated")
m("SP a transcript last written before the window is not counted", "internal/spend/scan.go",
  "\t\t\tstale[resolved(p)] = true\n\t\t\treturn false", "\t\t\treturn false",
  "TestWindow_OldTranscripts|TestDiscover_Skips")
m("SP old transcripts are reported as no transcripts found", "internal/spend/text.go",
  "\tcase s.Read.Files == 0 && s.Read.FilesBeforeWindow > 0:", "\tcase false:", "TestWindow_OldTranscripts")
m("SP a symlinked project folder is skipped", "internal/spend/scan.go",
  "\tif t&fs.ModeSymlink != 0 {", "\tif false && t&fs.ModeSymlink != 0 {", "TestDiscover_ASymlinked")
m("SP a recorded real path does not cover a transcript reached through a symlink", "internal/spend/join.go",
  "\t\tif real, err := filepath.EvalSymlinks(p); err == nil {", "\t\tif real, err := filepath.EvalSymlinks(p); err == nil && false {",
  "TestDiscover_ASymlinked")
m("SP a discovered transcript is resolved as written, not by its absolute spelling", "internal/spend/join.go",
  "\t\tif real, err := filepath.EvalSymlinks(p); err == nil {", "\t\tif real, err := filepath.EvalSymlinks(f.Path); err == nil {",
  "TestDiscover_ASymlinked")
m("SP a discovered transcript is not indexed by its absolute spelling", "internal/spend/join.go",
  "\tif abs, err := filepath.Abs(f.Path); err == nil {\n\t\t\tp = abs", "\tif abs, err := filepath.Abs(f.Path); err == nil {\n\t\t\t_ = abs",
  "TestDiscover_ASymlinked")
m("SP one transcript under two spellings is read twice", "internal/spend/scan.go",
  "\t\tif seen[real] {\n\t\t\tcontinue", "\t\tif false && seen[real] {\n\t\t\tcontinue",
  "TestDiscover_OneTranscriptUnderTwoSpellings")
m("SP a project folder that cannot be read is not counted", "internal/spend/scan.go",
  "\t\tif kind == kindUnreadable {\n\t\t\tunreadable[resolved(dir)] = true", "\t\tif kind == kindUnreadable {\n\t\t\t_ = dir",
  "TestDiscover_AnUnreadableFolder")
m("SP a session folder that cannot be read is not counted", "internal/spend/scan.go",
  "\t\t\tcase kind == kindUnreadable:\n\t\t\t\tunreadable[resolved(path)] = true", "\t\t\tcase kind == kindUnreadable:\n\t\t\t\t_ = path",
  "TestDiscover_AnUnreadableFolder")
m("SP folders that could not be read are not said", "internal/spend/text.go",
  "\tif s.Read.UnreadableDirs > 0 {", "\tif false {", "TestDiscover_AnUnreadableFolder")
m("SP a usage line that does not decode is dropped without a count", "internal/spend/scan.go",
  "\t\t\tif usageLine {\n\t\t\t\tsc.Unparsed++", "\t\t\tif usageLine {\n\t\t\t\t_ = sc",
  "TestUnparsed_")
m("SP an implausible usage is priced", "internal/spend/scan.go",
  "\t\tif !l.Message.Usage.plausible() {", "\t\tif false && !l.Message.Usage.plausible() {", "TestUnparsed_")
m("SP a negative token count is plausible", "internal/spend/scan.go",
  "\t\t\tif v < 0 || v > maxTokens {", "\t\t\tif v > maxTokens {", "TestUnparsed_")
m("SP an implausibly large token count is plausible", "internal/spend/scan.go",
  "\t\t\tif v < 0 || v > maxTokens {", "\t\t\tif v < 0 {", "TestUnparsed_")
m("SP an iteration's counts are not checked", "internal/spend/scan.go",
  "\tfor _, it := range u.Iterations {\n\t\tif !ok(it.tokens) {", "\tfor _, it := range u.Iterations {\n\t\tif false && !ok(it.tokens) {",
  "TestUnparsed_")
m("SP unparsed usage lines are not said", "internal/spend/text.go",
  "\tif s.Read.UnparsedUsageLines > 0 {", "\tif false {", "TestUnparsed_")
m("SP a main transcript's sidechain lines share the main agent's cache stream", "internal/spend/spend.go",
  "\t\t\tk := stream{f.idx, r.Subagent}\n", "\t\t\tk := stream{f.idx, false}\n", "TestCacheExpiry_ThePreviousResponseIsTheSameAgents")
m("SP each agent share is rounded on its own", "internal/spend/text.go",
  "\tif pa+pb < 100 {", "\tif false {", "TestAgent_SharesSumTo100")
m("SP the leftover share point goes to the smaller remainder", "internal/spend/text.go",
  "\t\tif ra >= rb {", "\t\tif ra < rb {", "TestAgent_SharesSumTo100")
m("SP a non-zero side under 1% prints 0%", "internal/spend/text.go",
  "\t\tcase p == 0 && n > 0:", "\t\tcase false:", "TestAgent_SharesSumTo100")
m("SP a share of 100% is printed beside a non-zero other side", "internal/spend/text.go",
  "\t\tcase p == 100 && uint64(n) < total:", "\t\tcase false:", "TestAgent_SharesSumTo100")
m("SP per-session spend is JSON-only", "internal/spend/text.go",
  "\t\tfor i, line := range sessionLines(s.PerSession) {", "\t\tfor i, line := range sessionLines(nil) {",
  "TestSessions_")
m("SP the text lists every session", "internal/spend/text.go",
  "\t\tif i == maxNamed {", "\t\tif i == -1 {", "TestSessions_")
m("SP a session rashomon did not record is not marked", "internal/spend/text.go",
  "\t\tcase CoverageNotRecorded:\n\t\t\tline +=", "\t\tcase \"never\":\n\t\t\tline +=", "TestSessions_")
m("SP a partly recorded session is not marked", "internal/spend/text.go",
  "\t\tcase CoveragePartly:\n\t\t\tline +=", "\t\tcase \"never\":\n\t\t\tline +=", "TestJoin_CoverageIsPerTranscript")
m("SP a recorded transcript_path is not resolved through a symlinked ancestor", "internal/spend/join.go",
  "\t\tif r, err := filepath.EvalSymlinks(recorded); err == nil {", "\t\tif r, err := filepath.EvalSymlinks(recorded); err == nil && false {",
  "TestDiscover_ASymlinked")
m("SP a retired model's row is priced at a current model's rates", "internal/spend/price.go",
  "\t\"claude-opus-4-1\": {Input: mtok(1500), Output: mtok(7500), CacheRead: mtok(150)},", "\t\"claude-opus-4-1\": {Input: mtok(500), Output: mtok(2500), CacheRead: mtok(50)},",
  "TestPricing_TheTable")
m("SP Haiku 3.5 has no row and prices as unknown", "internal/spend/price.go",
  "\t\"claude-3-5-haiku\": {Input: mtok(80), Output: mtok(400), CacheRead: mtok(8)},\n", "",
  "TestPricing_TheTable|TestPriceKey_")
m("SP a tool_use block's input is decoded with an assistant line's text blocks", "internal/report/transcript.go",
  "type textBlock struct {\n\tType string `json:\"type\"`\n\tText string `json:\"text\"`\n}",
  "type textBlock struct {\n\tType  string          `json:\"type\"`\n\tText  string          `json:\"text\"`\n\tInput json.RawMessage `json:\"input\"`\n}",
  "TestAssistantLine_")
m("SP a block of any type is kept as an assistant line's text", "internal/report/transcript.go",
  "\t\tif blk.Type == \"text\" && strings.TrimSpace(blk.Text) != \"\" {", "\t\tif strings.TrimSpace(blk.Text) != \"\" {",
  "TestAssistantLine_")
m("SP a firing turn's final words are printed to stderr", "internal/spend/join.go",
  "\t\tsf := report.BuildSilentFailures(t.run, report.AccountFromMessage(lastSaid(byFile, t.prompt)))\n",
  "\t\tsf := report.BuildSilentFailures(t.run, report.AccountFromMessage(lastSaid(byFile, t.prompt)))\n\t\tfmt.Fprintln(os.Stderr, lastSaid(byFile, t.prompt))\n",
  "TestSpend_NoMessageTextReachesTheOutput")
m("SP a firing turn's final words reach the JSON", "internal/spend/join.go",
  "\t\tif !sf.Fires {\n\t\t\tcontinue\n\t\t}\n\t\tj.Turns++\n",
  "\t\tif !sf.Fires {\n\t\t\tcontinue\n\t\t}\n\t\tj.Turns++\n\t\tj.Bound = lastSaid(byFile, t.prompt)\n",
  "TestJoin_NoMessageTextReachesTheOutput|TestSpend_NoMessageTextReachesTheOutput")
m("SP a main-transcript sidechain response is tied to no turn", "internal/report/transcript.go",
  "\t\t\tif head.Type == \"assistant\" && want[current] && head.Message.ID != \"\" {\n\t\t\t\ttie(head.Message.ID)",
  "\t\t\tif false && head.Type == \"assistant\" && want[current] && head.Message.ID != \"\" {\n\t\t\t\ttie(head.Message.ID)",
  "TestJoin_AMainTranscriptSidechain")
m("SP a main-transcript sidechain user line moves the tie", "internal/report/transcript.go",
  "\t\t\t\ttie(head.Message.ID)\n\t\t\t}\n\t\t\tcontinue\n",
  "\t\t\t\ttie(head.Message.ID)\n\t\t\t}\n\t\t\tif head.Type == \"user\" && head.PromptID != \"\" {\n\t\t\t\tcurrent = head.PromptID\n\t\t\t}\n\t\t\tcontinue\n",
  "TestJoin_AMainTranscriptSidechain")
m("SP a store that covers no transcript renders as a checked none", "internal/spend/text.go",
  "\tcase j.CoveredTranscripts == 0:\n", "\tcase false:\n", "TestJoin_AStoreThatCoversNoTranscript")
m("SP a transcript that cannot be read to the end is not counted", "internal/spend/scan.go",
  "\t\tif err := readFile(sc, byID, i, f); err != nil {\n\t\t\tsc.Unreadable++", "\t\tif err := readFile(sc, byID, i, f); err != nil {\n\t\t\t_ = sc",
  "TestRead_AnUnreadableTranscript")
m("SP a transcript that cannot be read to the end is not said", "internal/spend/text.go",
  "\tif s.Read.UnreadableFiles > 0 {", "\tif false {", "TestRead_AnUnreadableTranscript")
m("SP cmdSpend reads every transcript ever written", "cmd/rashomon/main.go",
  "\tfiles, err := spend.Discover(configDir, spend.WindowStart(now, days))", "\tfiles, err := spend.Discover(configDir, time.Time{})",
  "TestSpend_OldTranscriptsAreSkippedAndSaid")
m("SP a text line with no parseable timestamp is skipped, and the turn judged on earlier words", "internal/report/transcript.go",
  "\t\t\tif err != nil {\n\t\t\t\tunsay()\n\t\t\t\tcontinue\n\t\t\t}", "\t\t\tif err != nil {\n\t\t\t\tcontinue\n\t\t\t}",
  "TestFinalAssistantTexts_")
m("SP a main-transcript user line that does not decode keeps the tie", "internal/report/transcript.go",
  "\t\t\tunsay()\n\t\t\tcurrent = \"\"\n\t\t\tcontinue\n\t\tcase LineUndecodableSidechain:", "\t\t\tunsay()\n\t\t\tcontinue\n\t\tcase LineUndecodableSidechain:",
  "TestFinalAssistantTexts_")
m("SP a line that does not decode leaves the turn's earlier words standing", "internal/report/transcript.go",
  "\t\t\tunsay()\n\t\t\tcurrent = \"\"\n\t\t\tcontinue\n\t\tcase LineUndecodableSidechain:", "\t\t\tcurrent = \"\"\n\t\t\tcontinue\n\t\tcase LineUndecodableSidechain:",
  "TestFinalAssistantTexts_")
m("SP a tie between two main files' final words goes to the first", "internal/spend/join.go",
  "t.Said && t.AtMS >= bestMS {", "t.Said && t.AtMS > bestMS {", "TestJoin_TheLastWord")
m("SP an earlier main file's final word beats a later one", "internal/spend/join.go",
  "t.Said && t.AtMS >= bestMS {", "t.Said && (bestMS == math.MinInt64 || t.AtMS < bestMS) {", "TestJoin_TheLastWord")
m("SP a transcript not read to the end still gives a turn its words", "internal/report/transcript.go",
  "\tif sc.Err() != nil {\n\t\t// A file that cannot be read to the end may hold a later reply", "\tif false {\n\t\t// A file that cannot be read to the end may hold a later reply",
  "TestJoin_ATranscriptCut")
m("SP a turn's final word carries no time", "internal/report/transcript.go",
  "\t\t\ttf.Said, tf.Text, tf.AtMS = true, text, at.UnixMilli()", "\t\t\ttf.Said, tf.Text, tf.AtMS = true, text, 0*at.UnixMilli()",
  "TestFinalAssistantTexts_ATurnIsItsPrompt")
m("SP an id-less usage line with tokens is dropped without a count", "internal/spend/scan.go",
  "\t\t\tif l.Message.Usage.carriesTokens() {\n\t\t\t\tsc.Unparsed++", "\t\t\tif false {\n\t\t\t\tsc.Unparsed++",
  "TestUnparsed_")
m("SP an id-less usage line is counted as a usage line", "internal/spend/scan.go",
  "\t\tif l.Message.ID == \"\" {\n\t\t\t// No id to deduplicate against", "\t\tif l.Message.ID == \"\" {\n\t\t\tsc.UsageLines++\n\t\t\t// No id to deduplicate against",
  "TestUnparsed_")
m("SP a transcript that cannot be stat'ed is dropped without a count", "internal/spend/scan.go",
  "\t\t\treturn !errors.Is(err, fs.ErrNotExist)", "\t\t\treturn false", "TestDiscover_AFileThatCannotBeStated")
m("SP a transcript that vanished is still read", "internal/spend/scan.go",
  "\t\t\treturn !errors.Is(err, fs.ErrNotExist)", "\t\t\treturn true", "TestDiscover_AFileThatCannotBeStated")
m("SP a share's percentage overflows", "internal/spend/text.go",
  "\thi, lo := bits.Mul64(n, 100)\n\treturn bits.Div64(hi, lo, total)", "\t_, _ = bits.Mul64(n, 100)\n\treturn n * 100 / total, n * 100 % total",
  "TestAgent_Shares")
m("SP a declaration with no prompt_id is grouped as the turn \"\"", "internal/spend/join.go",
  "\t\tif d.PromptID == nil || *d.PromptID == \"\" {\n\t\t\tcontinue\n\t\t}",
  "\t\tif d.PromptID == nil {\n\t\t\tnone := \"\"\n\t\t\td.PromptID = &none\n\t\t}",
  "TestJoin_ADeclarationWithNoPromptID")
m("SP a declaration with no prompt_id is dereferenced", "internal/spend/join.go",
  "\t\tif d.PromptID == nil || *d.PromptID == \"\" {\n\t\t\tcontinue\n\t\t}", "",
  "TestJoin_ADeclarationWithNoPromptID")
m("SP a wholly unpriced silent-failure cost reads at least unknown", "internal/spend/text.go",
  "\tcase !j.Cost.Wholly():\n", "\tcase false:\n", "TestJoin_AWhollyUnpricedTurn")
m("SP an empty window reads unknown over 0 transcripts", "internal/spend/text.go",
  "\tcase j.Transcripts == 0:\n", "\tcase false:\n", "TestJoin_NoTranscriptInTheWindow")
m("SP a fast-mode response is not counted", "internal/spend/scan.go",
  "\t\t\tFast:       l.Message.Usage.Speed == \"fast\",", "\t\t\tFast:       l.Message.Usage.Speed == \"turbo\",",
  "TestFastMode_")
m("SP fast-mode responses are not said", "internal/spend/text.go",
  "\tif s.FastMode.Responses > 0 {", "\tif false {", "TestFastMode_")
m("SP the out-of-scope line does not name web-search fees", "internal/spend/text.go",
  "; web-search fees (%s)\\n\", WebSearchFee)", "; %s\\n\", WebSearchFee)", "TestFastMode_")
m("SP the web-search fee is not the dated table's", "internal/spend/price.go",
  "const WebSearchFee = \"$10 per 1,000 searches\"", "const WebSearchFee = \"$5 per 1,000 searches\"", "TestFastMode_")
m("SP Mythos 5 prices its cache reads at Mythos 5.1's rate", "internal/spend/price.go",
  "\t\"claude-mythos-5\":   {Input: mtok(1000), Output: mtok(5000), CacheRead: mtok(100)},", "\t\"claude-mythos-5\":   {Input: mtok(1000), Output: mtok(5000), CacheRead: mtok(25)},",
  "TestPricing_TheTable")
m("SP a decoded field under an allowed tag holds whatever it is handed", "internal/spend/scan.go",
  "\tType        string  `json:\"type\"`", "\tType        any     `json:\"type\"`", "TestContentHasNoFieldToLandIn")
m("SP a zero-usage refusal line is not counted", "internal/spend/spend.go",
  "\t\t\t\ts.Refusals.WithoutUsage++", "\t\t\t\t_ = s", "TestRefusals_")
m("SP refusals without usage read as refusals none", "internal/spend/text.go",
  "\tif r.Responses == 0 && r.BeforeOutput == 0 && r.WithoutUsage == 0 {", "\tif r.Responses == 0 && r.BeforeOutput == 0 {",
  "TestRefusals_")
m("SP a refusal without usage is not said when nothing else was billed", "internal/spend/text.go",
  "\t} else if s.Refusals.WithoutUsage > 0 || s.Refusals.BeforeOutput > 0 {", "\t} else if false {",
  "TestRefusals_")
m("SP the silent-failure line says the turns ended with a failure", "internal/spend/text.go",
  'const lead = "in turns with a failed call the summary never mentioned: "', 'const lead = "in turns that ended with a failure the summary never mentioned: "',
  "TestJoin_SpendInsideASilentlyFailedTurn")
m("SP the silent-failure saving says it bought a done", "internal/spend/text.go",
  '"%s spent in turns with a failed call the summary never mentioned"', '"%s bought a \\"done\\" in turns whose recorded failures the summary never mentioned"',
  "TestJoin_SpendInsideASilentlyFailedTurn")
m("SP a write on a response that read back the previous cache is counted cold", "internal/spend/spend.go",
  "short := max(0, cached-cur.CacheRead)", "short := max(0, cached-0*cur.CacheRead)",
  "TestCacheExpiry_AWriteOnAWarmCacheIsNotCold")
m("SP a cold write is counted whole, not its shortfall", "internal/spend/spend.go",
  "\t\t\tshort := max(0, cached-cur.CacheRead)", "\t\t\tshort := cur.CacheWrite5m + cur.CacheWrite1h + 0*cached",
  "TestCacheExpiry_APartialExpiry")
m("SP a write with any cache read is not cold", "internal/spend/spend.go",
  "\t\t\tprev, cur := rs[i-1].Tokens, rs[i].Tokens\n", "\t\t\tprev, cur := rs[i-1].Tokens, rs[i].Tokens\n\t\t\tif cur.CacheRead > 0 {\n\t\t\t\tcontinue\n\t\t\t}\n",
  "TestCacheExpiry_APartialExpiry")
m("SP a cold write is priced at the full write rate, not over a cache read", "internal/spend/spend.go",
  "w.CacheWrite5m*(rt.CacheWrite5m()-rt.CacheRead) + w.CacheWrite1h*(rt.CacheWrite1h()-rt.CacheRead)", "w.CacheWrite5m*rt.CacheWrite5m() + w.CacheWrite1h*(rt.CacheWrite1h()-rt.CacheRead)",
  "TestCacheExpiry_")
m("SP a cold 1h write is priced at the full write rate", "internal/spend/spend.go",
  "w.CacheWrite5m*(rt.CacheWrite5m()-rt.CacheRead) + w.CacheWrite1h*(rt.CacheWrite1h()-rt.CacheRead)", "w.CacheWrite5m*(rt.CacheWrite5m()-rt.CacheRead) + w.CacheWrite1h*rt.CacheWrite1h()",
  "TestCacheExpiry_")
m("SP a response keeps only the file it was first seen in", "internal/spend/scan.go",
  "\tif !slices.ContainsFunc(prev.files, func(s sighting) bool { return s.idx == cand.file }) {\n\t\tprev.files = append(prev.files, cand.files[0])\n\t}\n", "\t_ = slices.ContainsFunc[[]sighting]\n",
  "TestJoin_ADuplicatedResponse")
m("SP a shared response's coverage follows the file that sorts first", "internal/spend/join.go",
  "\t\t\tfor _, m := range s.mainsOf(r) {\n\t\t\t\tif !covered[m] {\n\t\t\t\t\tnotCovered = true", "\t\t\tfor _, m := range s.mainsOf(r)[:1] {\n\t\t\t\tif !covered[m] {\n\t\t\t\t\tnotCovered = true",
  "TestJoin_ADuplicatedResponse")
m("SP a transcript holding only shared responses is not a transcript", "internal/spend/join.go",
  "\t\t\tfor _, m := range s.mainsOf(r) {\n\t\t\t\tbyTranscript[m] = append(byTranscript[m], r)", "\t\t\tfor _, m := range s.mainsOf(r)[:1] {\n\t\t\t\tbyTranscript[m] = append(byTranscript[m], r)",
  "TestJoin_ADuplicatedResponse")
m("SP a cold-cache stream holds only the responses first seen in its file", "internal/spend/spend.go",
  "\t\tfor _, f := range r.files {\n\t\t\tk := stream{f.idx, r.Subagent}", "\t\tfor _, f := range r.files[:1] {\n\t\t\tk := stream{f.idx, r.Subagent}",
  "TestJoin_ADuplicatedResponse")
m("SP an iteration entry's type is decoded as counts only", "internal/spend/scan.go",
  "\tType string `json:\"type\"`\n\t// Model is the model that ran the attempt.", "\tType string `json:\"-\"`\n\t// Model is the model that ran the attempt.",
  "TestExtraAttempts_")
m("SP an iteration entry's model is not decoded", "internal/spend/scan.go",
  "\tModel string `json:\"model\"`\n}", "\tModel string `json:\"-\"`\n}",
  "TestExtraAttempts_")
m("SP a fallback-served response is not counted", "internal/spend/spend.go",
  "\tif r.Fallback && r.StopReason != \"refusal\" {\n\t\te.FallbackServed++", "\tif false && r.Fallback {\n\t\te.FallbackServed++",
  "TestExtraAttempts_")
m("SP the headline does not say the total leaves out the extra attempts", "internal/spend/text.go",
  "\tif a := s.ExtraAttempts; a.Attempts > 0 {", "\tif a := s.ExtraAttempts; false && a.Attempts > 0 {",
  "TestExtraAttempts_TheFallbackPagesExample|TestRefusalsAndExtraAttempts")
m("SP a refusal's category is not decoded", "internal/spend/scan.go",
  "\tCategory *string `json:\"category\"`", "\tCategory *string `json:\"-\"`",
  "TestRefusals_AreSplitByCategoryAndModel")
m("SP a category outside the vocabulary is kept as read", "internal/spend/scan.go",
  "\tswitch c := *d.Category; c {\n\tcase CategoryCyber, CategoryBio, CategoryFrontierLLM, CategoryReasoningExtraction, CategoryGeneralHarms:\n\t\treturn c\n\t}\n\treturn CategoryOther\n",
  "\treturn *d.Category\n",
  "TestRefusals_AreSplitByCategoryAndModel|TestContentNeverReachesTheOutput")
m("SP a null category is read as other", "internal/spend/scan.go",
  "\tif d == nil || d.Category == nil {\n\t\treturn CategoryUncategorized", "\tif d == nil || d.Category == nil {\n\t\treturn CategoryOther",
  "TestRefusals_")
m("SP a zero-usage refusal is not split by category", "internal/spend/spend.go",
  "\t\t\t\trefusal(r).WithoutUsage++\n", "",
  "TestRefusals_")
m("SP refusals are not split by model", "internal/spend/spend.go",
  "\t\tk := [2]string{r.Category, refusalModel(r.Model)}", "\t\tk := [2]string{r.Category, \"other\"}",
  "TestRefusals_AreSplitByCategoryAndModel")
m("SP a refusal's model is printed as read", "internal/spend/spend.go",
  "\t\tk := [2]string{r.Category, refusalModel(r.Model)}", "\t\tk := [2]string{r.Category, r.Model}",
  "TestContentNeverReachesTheOutput")
m("SP the refusal categories are not printed", "internal/spend/text.go",
  "\tfor _, g := range r.ByCategory {\n\t\tvar p []string", "\tfor _, g := range r.ByCategory[:0] {\n\t\tvar p []string",
  "TestRefusals_")
m("SP a refusal's category reaches the output end to end", "internal/spend/scan.go",
  "\tswitch c := *d.Category; c {\n\tcase CategoryCyber, CategoryBio, CategoryFrontierLLM, CategoryReasoningExtraction, CategoryGeneralHarms:\n\t\treturn c\n\t}\n\treturn CategoryOther\n",
  "\treturn *d.Category\n",
  "TestSpend_NoMessageTextReachesTheOutput")
m("SP a shared response's not-covered cost is counted in every unrecorded transcript", "internal/spend/join.go",
  "\t\tcostOf(&j.NotCoveredCost, r)\n", "\t\tfor _, m := range s.mainsOf(r) {\n\t\t\tif !covered[m] {\n\t\t\t\tcostOf(&j.NotCoveredCost, r)\n\t\t\t}\n\t\t}\n",
  "TestJoin_ASharedResponse")
m("SP a response a covered turn counted is also not covered", "internal/spend/join.go",
  "\t\tif !counted[r] {\n\t\t\tfor _, m := range s.mainsOf(r) {", "\t\tif true {\n\t\t\tfor _, m := range s.mainsOf(r) {",
  "TestJoin_ASharedResponseIsNeverBoth")
m("SP a shared response's session is the first sighting's", "internal/spend/scan.go",
  "\t\tr.owners = sc.ownerSessions(r)", "\t\tr.owners = []string{r.files[0].session}",
  "TestJoin_ADuplicatedResponse|TestJoin_ASharedResponseBelongs|TestJoin_ARecordedOriginal")
m("SP a transcript is tallied by the session of its first-seen response", "internal/spend/join.go",
  "\tfor m := range byTranscript {\n\t\tid := sessionOf[m]", "\tfor m, rs := range byTranscript {\n\t\tid := rs[0].files[0].session",
  "TestJoin_ADuplicatedResponse|TestJoin_ASharedResponse")
m("SP a refusal's synthetic line is counted again as a pre-output refusal", "internal/spend/scan.go",
  "\t\t\tif into := billed[key{r.file, r.requestID}]; into != nil {", "\t\t\tif into := billed[key{r.file, r.requestID}]; false && into != nil {",
  "TestRefusals_AMidStream")
m("SP a synthetic line is folded into a response in another file", "internal/spend/scan.go",
  "\t\tfor _, f := range r.files {\n\t\t\tbilled[key{f.idx, r.requestID}] = r", "\t\tfor range r.files {\n\t\t\tbilled[key{0, r.requestID}] = r",
  "TestRefusals_AMidStream")
m("SP a requestId is read whatever its shape", "internal/spend/scan.go",
  "\t\tif requestIDShaped(l.RequestID) {", "\t\tif l.RequestID != \"\" {", "TestRefusals_AMidStream")
m("SP a folded synthetic line does not make its response a refusal", "internal/spend/scan.go",
  "\t\t\t\tif into.StopReason != \"refusal\" {\n", "\t\t\t\tif false {\n", "TestRefusals_AMidStream")
m("SP a pre-output refusal with usage is priced into the total", "internal/spend/scan.go",
  "\t\tif r.StopReason == \"refusal\" && r.Tokens.Output == 0 && r.Tokens.Total() > 0 {", "\t\tif false {",
  "TestRefusals_APreOutputRefusalIsLeftOutOfTheTotal|TestRefusals_AreSplit")
m("SP the header does not say the total leaves out pre-output refusals", "internal/spend/text.go",
  "\tif line := preOutputLine(s.Refusals); line != \"\" {", "\tif line := preOutputLine(s.Refusals); false && line != \"\" {",
  "TestRefusals_")
m("SP a synthetic refusal group reads as a model named other", "internal/spend/spend.go",
  "\tif model == \"<synthetic>\" {\n\t\treturn ModelNotRecorded", "\tif false {\n\t\treturn ModelNotRecorded",
  "TestRefusals_")
m("SP the served model is read from message.model, not the fallback entry", "internal/spend/scan.go",
  "\t\tif served != \"\" {\n\t\t\tcand.Model = served", "\t\tif false && served != \"\" {\n\t\t\tcand.Model = served",
  "TestExtraAttempts_TheServedModelIsTheFallbackEntrys")
m("SP a chain where every model declined is reported as served", "internal/spend/spend.go",
  "\tif r.Fallback && r.StopReason != \"refusal\" {", "\tif r.Fallback {", "TestExtraAttempts_AnAllDeclinedChain")
m("SP an undecodable line ends the tie only when it holds the byte string user", "internal/report/transcript.go",
  "\t\tcase LineUndecodable:\n", "\t\tcase LineUndecodable:\n\t\t\tif !bytes.Contains(raw, []byte(`\"user\"`)) {\n\t\t\t\tcontinue\n\t\t\t}\n",
  "TestFinalAssistantTexts_AnUndecodableLine")
m("SP an undecodable sidechain line ends the tie", "internal/report/transcript.go",
  "\tif topLevelSidechain(raw) {", "\tif false && topLevelSidechain(raw) {",
  "TestFinalAssistantTexts_AnUndecodableLine")
m("SP the bound does not say a sidechain user line keeps the tie", "internal/spend/join.go",
  "and a sidechain user line does not end the tie; ", "", "TestJoin_TheBoundNamesTheUnkeyedLinesThatKeepATie")

m("SP a refusal's category is not kept from the completed line", "internal/spend/scan.go",
  "\tprev.Category = cand.Category\n", "", "TestDedupe_TheCompletedLineWins")
m("SP a fallback's attempts are not kept from the completed line", "internal/spend/scan.go",
  "\tprev.Attempts, prev.Fallback = cand.Attempts, cand.Fallback\n", "",
  "TestDedupe_TheCompletedLineWins")
m("SP the served model is not kept from the completed line", "internal/spend/scan.go",
  "\tif cand.Model != \"\" && (prev.Model == \"\" || cand.Fallback) {", "\tif cand.Model != \"\" && prev.Model == \"\" {",
  "TestDedupe_TheCompletedLineWins")
m("SP an assistant line whose content does not decode leaves the earlier words standing", "internal/report/transcript.go",
  "\t\t\tif err != nil || line.Message.Role != \"assistant\" {\n\t\t\t\tunsay()\n", "\t\t\tif err != nil || line.Message.Role != \"assistant\" {\n",
  "TestFinalAssistantTexts_AnUnreadableLine")
m("SP a response is judged cold in a file it was copied into", "internal/spend/spend.go",
  "\t\t\tif rs[i].file != k.file {", "\t\t\tif false && rs[i].file != k.file {", "TestCacheExpiry_AResponseIsJudged")
m("SP an assistant line's content is decoded whole into a RawMessage again", "internal/report/transcript.go",
  "\t\t\tline, err := decodeAssistantLine(raw)\n\t\t\tif err != nil || line.Message.Role != \"assistant\" {\n\t\t\t\tunsay()\n\t\t\t\tcontinue\n\t\t\t}\n\t\t\ttext, ok := line.Message.Content.text()",
  "\t\t\tvar line struct {\n\t\t\t\tMessage struct {\n\t\t\t\t\tRole    string          `json:\"role\"`\n\t\t\t\t\tContent json.RawMessage `json:\"content\"`\n\t\t\t\t} `json:\"message\"`\n\t\t\t}\n\t\t\tif json.Unmarshal(raw, &line) != nil || line.Message.Role != \"assistant\" {\n\t\t\t\tunsay()\n\t\t\t\tcontinue\n\t\t\t}\n\t\t\ttext, ok := assistantText(line.Message.Content)",
  "TestAssistantLine_")
m("SP a non-firing turn's final words are printed to stderr", "internal/spend/join.go",
  "\t\tif !sf.Fires {\n\t\t\tcontinue\n\t\t}\n\t\tj.Turns++\n", "\t\tif !sf.Fires {\n\t\t\tfmt.Fprintln(os.Stderr, lastSaid(byFile, t.prompt))\n\t\t\tcontinue\n\t\t}\n\t\tj.Turns++\n",
  "TestSpend_NoMessageTextReachesTheOutput")
m("SP a non-firing turn's final words reach the JSON", "internal/spend/join.go",
  "\t\tif !sf.Fires {\n\t\t\tcontinue\n\t\t}\n\t\tj.Turns++\n", "\t\tif !sf.Fires {\n\t\t\tj.Bound = lastSaid(byFile, t.prompt)\n\t\t\tcontinue\n\t\t}\n\t\tj.Turns++\n",
  "TestJoin_NoMessageTextReachesTheOutput")
m("SP cmdSpend reads a 30-day window whatever --days says", "cmd/rashomon/main.go",
  "\tfiles, err := spend.Discover(configDir, spend.WindowStart(now, days))", "\tfiles, err := spend.Discover(configDir, spend.WindowStart(now, 30))",
  "TestSpend_OldTranscriptsAreSkippedAndSaid")
m("SP the decoded message hands its bytes to its own decoder", "internal/spend/scan.go",
  "\tUsage       *usage       `json:\"usage\"`\n}\n", "\tUsage       *usage       `json:\"usage\"`\n}\n\nfunc (m *message) UnmarshalJSON(b []byte) error {\n\ttype plain message\n\treturn json.Unmarshal(b, (*plain)(m))\n}\n",
  "TestContentHasNoFieldToLandIn")

m("SP an unkeyed main-transcript sidechain user line ends the tie", "internal/report/transcript.go",
  "\t\t\t\ttie(head.Message.ID)\n\t\t\t}\n\t\t\tcontinue\n", "\t\t\t\ttie(head.Message.ID)\n\t\t\t}\n\t\t\tif head.Type == \"user\" && head.PromptID == \"\" {\n\t\t\t\tcurrent = \"\"\n\t\t\t}\n\t\t\tcontinue\n",
  "TestJoin_AMainTranscriptSidechain")
m("SP a main-transcript sidechain line's words are the turn's final word", "internal/report/transcript.go",
  "\t\t\tif head.Type == \"assistant\" && want[current] && head.Message.ID != \"\" {\n\t\t\t\ttie(head.Message.ID)\n\t\t\t}\n\t\t\tcontinue\n",
  "\t\t\tif head.Type == \"assistant\" && want[current] && head.Message.ID != \"\" {\n\t\t\t\ttie(head.Message.ID)\n\t\t\t}\n\t\t\tif head.Type == \"assistant\" && want[current] {\n\t\t\t\tif l, err := decodeAssistantLine(raw); err == nil {\n\t\t\t\t\tif text, ok := l.Message.Content.text(); ok {\n\t\t\t\t\t\ttf := out[current]\n\t\t\t\t\t\ttf.Said, tf.Text = true, text\n\t\t\t\t\t\tout[current] = tf\n\t\t\t\t\t}\n\t\t\t\t}\n\t\t\t}\n\t\t\tcontinue\n",
  "TestJoin_AMainTranscriptSidechain")

m("SP an extra attempt's tokens are not counted", "internal/spend/spend.go",
  "\t\te.Tokens.add(a)\n", "\t\t_ = a\n",
  "TestRefusalsAndExtraAttempts|TestExtraAttempts_")
m("SP an extra attempt's tokens are counted in the token total", "internal/spend/spend.go",
  "\ts.ExtraAttempts.add(r)\n", "\ts.ExtraAttempts.add(r)\n\tfor _, a := range r.Attempts {\n\t\ts.Tokens.add(a)\n\t}\n",
  "TestRefusalsAndExtraAttempts|TestExtraAttempts_")
m("SP a pre-output refusal is counted in the breakdowns", "internal/spend/spend.go",
  "\t\t\ts.refused = append(s.refused, r)\n\t\t\tcontinue\n", "\t\t\ts.refused = append(s.refused, r)\n",
  "TestRefusals_APreOutputRefusalIsLeftOutOfTheTotal")
m("SP an old transcript is counted once per spelling", "internal/spend/scan.go",
  "\t\t\tstale[resolved(p)] = true\n", "\t\t\tstale[p] = true\n",
  "TestDiscover_OneTranscriptUnderTwoSpellings")
m("SP a model-less fallback line replaces a known model", "internal/spend/scan.go",
  "\tif cand.Model != \"\" && (prev.Model == \"\" || cand.Fallback) {", "\tif prev.Model == \"\" || cand.Fallback {",
  "TestExtraAttempts_AModelLessFallbackLineKeepsAKnownModel")
m("SP a synthetic refusal line is folded after pre-output refusals are marked", "internal/spend/scan.go",
  "\tsc.foldRefusalMessages()\n\tfor _, r := range sc.Responses {", "\tdefer sc.foldRefusalMessages()\n\tfor _, r := range sc.Responses {",
  "TestRefusals_")
m("SP a cold write is priced on a response whose cost is unknown", "internal/spend/spend.go",
  "\t\t\tif key, ok := PriceKey(r.Model); ok && !r.costUnknown {", "\t\t\tif key, ok := PriceKey(r.Model); ok {",
  "TestCacheExpiry_")
m("SP a write on another model is judged against the previous model's cache", "internal/spend/spend.go",
  "\t\t\tif rs[i].Model != rs[i-1].Model || ", "\t\t\tif ",
  "TestCacheExpiry_")
m("SP a smaller prompt is judged as a re-write of the previous cache", "internal/spend/spend.go",
  " || cur.Input+cur.CacheRead+cur.CacheWrite5m+cur.CacheWrite1h < cached {", " {",
  "TestCacheExpiry_")
m("SP a cold 1h write ignores the 5m write already counted cold", "internal/spend/spend.go",
  "min(cur.CacheWrite1h, short-w.CacheWrite5m)", "min(cur.CacheWrite1h, short)",
  "TestCacheExpiry_")
m("SP a subagent transcript's lines are prefiltered on the byte string user again", "internal/spend/scan.go",
  "\t\tif !usageLine && !f.Subagent && dated {", "\t\tif !usageLine && (f.Subagent && !bytes.Contains(raw, []byte(`\"user\"`)) || !f.Subagent && dated) {",
  "TestJoin_ATurnsSpendIsKeyedByItsPrompt")
m("SP an undecodable line is a sidechain one by its bytes anywhere, not its top-level key", "internal/report/transcript.go",
  "\tif topLevelSidechain(raw) {", "\tif bytes.Contains(raw, []byte(`\"isSidechain\":true`)) {",
  "TestFinalAssistantTexts_AnUndecodableLine")
m("SP a whitespace-only line ends a main transcript's tie", "internal/report/transcript.go",
  "\tif len(bytes.TrimSpace(raw)) == 0 {", "\tif len(raw) == 0 {",
  "TestFinalAssistantTexts_AnUndecodableLine")
m("SP the bound does not say an undecodable sidechain line keeps the tie", "internal/spend/join.go",
  " (any in a subagent transcript; in the main transcript, unless it is a sidechain line), is tied", ", is tied",
  "TestJoin_TheBoundNamesTheUnkeyedLinesThatKeepATie")
m("SP a shared response's owner ignores its file's first dated line", "internal/spend/scan.go",
  "\t\tif ms := sc.firstMS[s.idx]; ms != 0 {", "\t\tif ms := sc.firstMS[s.idx]; false && ms != 0 {",
  "TestJoin_")
m("SP the store is read only for the sessions a response belongs to", "internal/spend/join.go",
  "\t\t\tfor _, f := range r.files {\n\t\t\t\tsessions[f.session] = true\n\t\t\t}", "\t\t\tfor _, id := range r.owners {\n\t\t\t\tsessions[id] = true\n\t\t\t}",
  "TestJoin_")
m("SP a file is dated by its literal first line", "internal/spend/scan.go",
  "\t\tif !dated {\n\t\t\tif ms, ok := parseTimestamp(l.Timestamp); ok {\n\t\t\t\tsc.firstMS[idx], dated = ms, true", "\t\tif !dated {\n\t\t\tdated = true\n\t\t\tif ms, ok := parseTimestamp(l.Timestamp); ok {\n\t\t\t\tsc.firstMS[idx] = ms",
  "TestJoin_")
m("SP a tie between two first-dated files goes to the lower session id", "internal/spend/scan.go",
  "\tsort.Strings(out)\n\treturn out\n}", "\tsort.Strings(out)\n\treturn out[:1]\n}",
  "TestJoin_")
m("SP a session holding a response it does not own has no row", "internal/spend/spend.go",
  "\t\tfor _, f := range r.files {\n\t\t\trow(f.session)\n\t\t}\n", "",
  "TestJoin_")
m("SP a row holding not-covered dollars reads recorded", "internal/spend/join.go",
  "\t\t\ttallyOf(id).out++", "\t\t\ttallyOf(id).in++",
  "TestJoin_")
m("SP a lost declaration's failed call is placed in a turn by recorded time", "internal/spend/join.go",
  "\t\t\tif !declared[x.ToolUseID] && x.Outcome == store.ExecFailed {\n\t\t\t\tlost = append(lost, x.RecordedAtMS)\n\t\t\t}\n\t\t\tcontinue\n",
  "\t\t\tif declared[x.ToolUseID] || x.Outcome != store.ExecFailed {\n\t\t\t\tcontinue\n\t\t\t}\n\t\t\tfor _, c := range byPrompt {\n\t\t\t\tif c.firstMS <= x.RecordedAtMS && (t == nil || c.firstMS > t.firstMS) {\n\t\t\t\t\tt = c\n\t\t\t\t}\n\t\t\t}\n\t\t\tif t == nil {\n\t\t\t\tlost = append(lost, x.RecordedAtMS)\n\t\t\t\tcontinue\n\t\t\t}\n",
  "TestJoin_AFailedCallWhoseDeclarationWasLost")
m("SP a failed call whose declaration was lost is not counted", "internal/spend/join.go",
  "\t\t\t\tj.UndeclaredFailedCalls++", "\t\t\t\t_ = ms",
  "TestJoin_AFailedCallWhoseDeclarationWasLost")
m("SP a lost declaration's failed call before the window is counted", "internal/spend/join.go",
  "\t\t\tif ms >= s.FromUnixMS {\n\t\t\t\tj.UndeclaredFailedCalls++", "\t\t\tif ms >= 0 {\n\t\t\t\tj.UndeclaredFailedCalls++",
  "TestJoin_AFailedCallWhoseDeclarationWasLost")
m("SP a lost declaration's successful call is counted", "internal/spend/join.go",
  "\t\t\tif !declared[x.ToolUseID] && x.Outcome == store.ExecFailed {", "\t\t\tif !declared[x.ToolUseID] {",
  "TestJoin_AFailedCallWhoseDeclarationWasLost")
m("SP an undeclared failed call reads as a checked none", "internal/spend/text.go",
  "\tcase j.Turns == 0 && j.Unjudged == 0 && j.UndeclaredFailedCalls == 0:\n", "\tcase j.Turns == 0 && j.Unjudged == 0:\n",
  "TestJoin_AFailedCallWhoseDeclarationWasLost")
m("SP the undeclared failed call is not said", "internal/spend/text.go",
  "\tif j.UndeclaredFailedCalls == 1 {", "\tif false {", "TestJoin_AFailedCallWhoseDeclarationWasLost")
m("SP undeclared failed calls are not said", "internal/spend/text.go",
  "\t} else if j.UndeclaredFailedCalls > 1 {", "\t} else if false {", "TestJoin_AFailedCallWhoseDeclarationWasLost")
m("SP undeclared failed calls marshal as a checked 0 with nothing covered", "internal/spend/join.go",
  "\tif j.CoveredTranscripts > 0 {\n\t\tout.Turns, out.Unjudged, out.Undeclared, out.Cost = &j.Turns, &j.Unjudged, &j.UndeclaredFailedCalls, &j.Cost",
  "\tout.Undeclared = &j.UndeclaredFailedCalls\n\tif j.CoveredTranscripts > 0 {\n\t\tout.Turns, out.Unjudged, out.Cost = &j.Turns, &j.Unjudged, &j.Cost",
  "TestJoin_NoStore")
m("SP a session holding only pre-output refusals has no row", "internal/spend/spend.go",
  "\t\tfor _, f := range r.files {\n\t\t\trow(f.session)\n\t\t}\n\t\tif r.costUnknown {\n\t\t\ts.Refusals.BeforeOutput++\n\t\t\ts.Refusals.BeforeOutputTokens += r.Tokens.Total()\n\t\t\trefusal(r).BeforeOutput++\n\t\t\ts.refused = append(s.refused, r)\n\t\t\tcontinue\n\t\t}\n",
  "\t\tif r.costUnknown {\n\t\t\ts.Refusals.BeforeOutput++\n\t\t\ts.Refusals.BeforeOutputTokens += r.Tokens.Total()\n\t\t\trefusal(r).BeforeOutput++\n\t\t\ts.refused = append(s.refused, r)\n\t\t\tcontinue\n\t\t}\n\t\tfor _, f := range r.files {\n\t\t\trow(f.session)\n\t\t}\n",
  "TestSessions_ASessionHoldingOnlyPreOutputRefusals")
m("SP a pre-output refusal's transcript is not one of the window's", "internal/spend/join.go",
  "\tfor _, rs := range [][]*Response{s.window, s.refused} {", "\tfor _, rs := range [][]*Response{s.window} {",
  "TestSessions_ASessionHoldingOnlyPreOutputRefusals")
m("SP a response a covered turn counted marks every row holding it covered", "internal/spend/join.go",
  "\t\tif !notCovered {\n\t\t\tcontinue\n\t\t}\n",
  "\t\tif !notCovered {\n\t\t\tfor _, id := range r.owners {\n\t\t\t\ttallyOf(id).in++\n\t\t\t}\n\t\t\tcontinue\n\t\t}\n",
  "TestJoin_")
m("SP a not-covered transcript names its file's own session", "internal/spend/join.go",
  "\t\t\tj.NotCoveredTranscripts++\n\t\t\tt.out++\n", "\t\t\tj.NotCoveredTranscripts++\n\t\t\tt.out++\n\t\t\tnamed[displaySession(id)] = true\n",
  "TestJoin_")
m("SP a row with no tally has no coverage label", "internal/spend/join.go",
  "\t\tcase t == nil:\n\t\t\ts.PerSession[i].Coverage = CoverageNotRecorded\n", "\t\tcase t == nil:\n",
  "TestJoin_")
m("SP the pre-output refusals' rule is not in the JSON", "internal/spend/spend.go",
  "Refusals{BeforeOutputPricing: PreOutputRefusalPricing, ByCategory", "Refusals{ByCategory",
  "TestRefusalsAndExtraAttempts_")
m("SP the extra attempts' rule is not in the JSON", "internal/spend/spend.go",
  "\t\tExtraAttempts: ExtraAttempts{Pricing: ExtraAttemptsPricing},\n", "",
  "TestRefusalsAndExtraAttempts_")
m("SP the pre-output refusals' rule does not say they are out of the total", "internal/spend/spend.go",
  "\"tokens only, cost unknown, out of total and tokens: whether", "\"tokens only, cost unknown: whether",
  "TestRefusalsAndExtraAttempts_")
# Import additions some mutants need.
IMPORTS = {
  "SP the transcript line decodes message.content": ("internal/spend/scan.go", '\t"bytes"\n', '\t"bytes"\n\t"encoding/json"\n'),
  "SP the decoded message hands its bytes to its own decoder": ("internal/spend/scan.go", '\t"bytes"\n', '\t"bytes"\n\t"encoding/json"\n'),
  "SP a firing turn's final words are printed to stderr": ("internal/spend/join.go", '\t"encoding/json"\n', '\t"encoding/json"\n\t"fmt"\n\t"os"\n'),
  "SP a non-firing turn's final words are printed to stderr": ("internal/spend/join.go", '\t"encoding/json"\n', '\t"encoding/json"\n\t"fmt"\n\t"os"\n'),
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
