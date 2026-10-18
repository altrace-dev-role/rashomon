# TRU-1: watch writes nothing before y on /dev/tty, asks every question
# before its first write, reads answers only from the terminal, holds a
# signal during the write and undoes a failed write in reverse.
m("TRU-1 a declined first question still installs", "cmd/rashomon/watch.go",
  "\t\tcase i == 0 && !yes:\n", "\t\tcase false:\n",
  "TestTRU1_WatchWritesNothingBeforeYes")
m("TRU-1 EOF at a question counts as yes", "cmd/rashomon/watch.go",
  "\t\tyes := !eof && consent.IsYes(reply)\n", "\t\tyes := eof || consent.IsYes(reply)\n",
  "TestTRU1_WatchWritesNothingBeforeYes")
m("TRU-1 any answer starting with y is yes", "internal/consent/tty.go",
  "\treturn strings.EqualFold(a, \"y\") || strings.EqualFold(a, \"yes\")\n",
  "\treturn strings.HasPrefix(strings.ToLower(a), \"y\")\n",
  "TestTRU1_WatchWritesNothingBeforeYes")
m("TRU-1 the Agent SDK answer is recorded as yes whatever was typed", "cmd/rashomon/watch.go",
  "\t\t\tans.sdk, ans.sdkAsked = yes, true\n", "\t\t\tans.sdk, ans.sdkAsked = true, true\n",
  "TestTRU1_WatchWritesNothingBeforeYes")
m("TRU-1 answers are read from stdin", "cmd/rashomon/watch.go",
  "\tterm := consent.NewTerminal(tty)\n",
  "\tterm := consent.NewTerminal(struct {\n\t\tio.Reader\n\t\tio.Writer\n\t}{os.Stdin, tty})\n",
  "TestTRU1_WatchWritesNothingBeforeYes")
m("TRU-1 a signal before the write is held instead of ending watch", "cmd/rashomon/signals.go",
  "\t\t\tif !g.holding {\n", "\t\t\tif false {\n",
  "TestTRU1_WatchWritesNothingBeforeYes")
m("TRU-1 allowManagedHooksOnly is not refused", "cmd/rashomon/watch.go",
  "\tif only {\n", "\tif false && only {\n",
  "TestTRU1_WatchWritesNothingBeforeYes")
m("TRU-1 allowManagedHooksOnly is read from the user layer", "internal/settings/locate.go",
  "\tdoc, err := Load(loc.Managed)\n\tif err != nil {\n\t\treturn false, fmt.Errorf(\"%s settings at %s: %w\", LayerManaged, loc.Managed, err)",
  "\tdoc, err := Load(loc.User)\n\tif err != nil {\n\t\treturn false, fmt.Errorf(\"%s settings at %s: %w\", LayerManaged, loc.Managed, err)",
  "TestTRU1_WatchWritesNothingBeforeYes")
m("TRU-1 a signal during the write is not held", "cmd/rashomon/watch.go",
  "\tgate.hold()\n", "",
  "TestTRU1_SignalDuringTheWriteIsHeld")
m("TRU-1 a failed write undoes nothing", "cmd/rashomon/watch.go",
  "\tfor i := len(w.done) - 1; i >= 0; i-- {\n\t\tif uerr := w.done[i].undo(); uerr != nil {",
  "\tfor i := len(w.done) - 1; i >= len(w.done); i-- {\n\t\tif uerr := w.done[i].undo(); uerr != nil {",
  "TestTRU1_AFailedWriteIsUndoneInReverse")
m("TRU-1 undoing the store leaves the directories it made", "cmd/rashomon/watch.go",
  "\t\terrs = append(errs, removeDirs(created))\n", "",
  "TestTRU1_AFailedWriteIsUndoneInReverse")
m("TRU-1 undoing the settings edit leaves the edit", "cmd/rashomon/watch.go",
  "\t\treturn settings.Write(path, res.after, res.before)\n", "\t\treturn nil\n",
  "TestTRU1_AFailedWriteIsUndoneInReverse")
m("TRU-1 undoing a new file leaves it", "cmd/rashomon/watch.go",
  "\t\tif !existed {\n\t\t\tif err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {",
  "\t\tif false {\n\t\t\tif err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {",
  "TestTRU1_AFailedWriteIsUndoneInReverse")
m("TRU-1 a failure does not name its step", "cmd/rashomon/watch.go",
  "\tmsg := fmt.Sprintf(\"%s failed: %v\", step, err)\n", "\tmsg := fmt.Sprintf(\"a write failed: %v\", err)\n",
  "TestTRU1_AFailedWriteIsUndoneInReverse")
m("TRU-1 --yes overwrites an Agent SDK answer on file", "cmd/rashomon/watch.go",
  "\t\tif !ans.sdkAsked {\n\t\t\treturn consent.WriteSDKDefault(plan.root)\n\t\t}\n", "",
  "TestTRU1_WatchWritesNothingBeforeYes")
m("TRU-1 an answered Agent SDK question records only the default", "cmd/rashomon/watch.go",
  "\t\t\tans.sdk, ans.sdkAsked = yes, true\n", "\t\t\tans.sdk = yes\n",
  "TestTRU1_WatchWritesNothingBeforeYes")
m("TRU-1 the Agent SDK default overwrites an answer on file", "internal/consent/harnesses.go",
  "\tif _, ok := h.sdk[\"record\"]; ok {\n\t\treturn nil\n\t}\n", "",
  "TestTRU1_WatchWritesNothingBeforeYes")
