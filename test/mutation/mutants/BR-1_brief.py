# BR-1: the project brief at session start (card BRIEF1, R2). Each break
# removes one of the brief's promises: the walk's entry cap, nothing guessed
# without a manifest, every line sourced, and a startup print the model never
# reads.
B = "internal/brief/brief.go"
C = "cmd/rashomon/brief.go"

m("BR-1 the walk reads past its entry cap", B, "ReadDir(min(walkChunk, left-n))", "ReadDir(walkChunk)", "TestBR1c_")
m("BR-1 Go packages are counted without go.mod (a guess from file extensions)", B,
  '\tcase !slices.Contains(b.Manifests, "go.mod"):\n\t\treturn newFact(KindPackages, "not read (no go.mod)", b.Root)\n', "",
  "TestBR1b_")
m("BR-1 the tests line drops its source with no index", B, '\tsource := "no store"\n', '\tsource := ""\n', "TestBR1d_")
m("BR-1 startup hands the brief to the model as context", C,
  'json.Marshal(map[string]string{"systemMessage": msg})',
  'json.Marshal(map[string]any{"systemMessage": msg, "hookSpecificOutput": map[string]string{'
  '"hookEventName": "SessionStart", "additionalContext": msg}})', "TestBR1e_")
m("BR-1 the verify line drops the session", B,
  '\treturn newFact(KindVerify, "rashomon report --session "+report.SessionID, "the payload\'s session_id")\n',
  '\treturn newFact(KindVerify, "rashomon report", "the payload\'s session_id")\n',
  "TestBR1e_")

# D175: the verify line names the command its origin has, the plugin's
# /autosteer:report last only from the plugin's own entry (the recap
# pointer's rule, H-91). D173: an unsigned bindings file is named, and only
# that: no other read error, no spec the bindings do not decide, and no file
# outside the store.
m("BR-1 the plugin's entry is pointed at the CLI", B, "\tcase report.Plugin:\n", "\tcase false:\n",
  "TestBR1j_|TestBR1i_")
m("BR-1 a settings install made from the plugin's binary is pointed at the plugin", C,
  '\tif installArg(args) != "" {\n\t\treturn false\n\t}\n\tpresent, err := install.PluginFires(install.EventSessionStart)\n',
  "\tpresent, err := install.PluginFires(install.EventSessionStart)\n", "TestBR1i_")
m("BR-1 an unsigned bindings file is not named", B,
  "\tif f, ok := trustFact(storeRoot, sp); ok {\n\t\tb.Facts = append(b.Facts, f)\n\t}\n", "", "TestBR1k_")
m("BR-1 any bindings read error reads unsigned", B,
  "!errors.Is(err, spec.ErrBindingsUnsigned) {", "err == nil {", "TestBR1k_")
IMPORTS["BR-1 any bindings read error reads unsigned"] = (B, '\t"errors"\n', "")
m("BR-1 the bindings are read for a spec they do not decide", B,
  "func(s spec.Source) bool { return s.Trust == spec.Untrusted }", "func(s spec.Source) bool { return true }",
  "TestBR1k_")
m("BR-1 no store root reads the working directory's file", B,
  'if storeRoot == "" || sp == nil || ', "if sp == nil || ", "TestBR1k_")
# D189: the trust line's command and the resume line name this binary by its
# path, which a plugin install has off PATH.
m("BR-1 the brief names rashomon, not this binary", B, "\treturn exe, true\n}\n", '\treturn "rashomon", true\n}\n',
  "TestBR1k_|TestBR1f_|TestBR1l_")

# BRIEF1 QA (PR #148): a pipe named go.mod must not hold SessionStart; every
# printed line names its source; a binary path is shown only when it is safe;
# the repo's task goal is labelled; exactly the entry cap reads complete.
m("BR-1 a pipe named go.mod is read as a manifest", B,
  "\t\tcase !fi.Mode().IsRegular():\n", "\t\tcase false:\n", "TestBR1n_")
m("BR-1 go.mod is opened blocking", B,
  "os.O_RDONLY|syscall.O_NONBLOCK", "os.O_RDONLY", "TestBR1n_")
IMPORTS["BR-1 go.mod is opened blocking"] = (B, '\t"syscall"\n', "")
m("BR-1 go.mod is read whatever its mode", B,
  "\tif fi, err := f.Stat(); err != nil || !fi.Mode().IsRegular() {\n", "\tif fi, err := f.Stat(); err != nil || fi == nil {\n", "TestBR1n_")
m("BR-1 the tests line names no spec", B,
  '\t\t\ttail := " (spec, " + string(trust) + ")"\n', '\t\t\ttail := " (" + string(trust) + ")"\n', "TestBR1o_|TestBR1_")
m("BR-1 the packages line names no walk", B,
  '\treturn newFact(KindPackages, n+" (walk)", b.Root)\n', '\treturn newFact(KindPackages, n, b.Root)\n', "TestBR1o_")
m("BR-1 the task goal is not said to be the repo's", B,
  '\tconst lead, tail = "from the repo: ", " (task/current.json)"\n', '\tconst lead, tail = "", ""\n', "TestBR1o_")
m("BR-1 a binary path with a shell character is printed bare", B,
  "\tif !safeBare(exe) {\n", "\tif false {\n", "TestBR1l_")
m("BR-1 an unsafe binary path is mangled, not withheld", B,
  "\tif !utf8.ValidString(exe) || strings.IndexFunc(exe, func(r rune) bool { return !unicode.IsPrint(r) }) >= 0 {\n",
  "\tif false {\n", "TestBR1m_")
m("BR-1 a path that is not UTF-8 is shown", B,
  "\tif !utf8.ValidString(exe) || strings.IndexFunc(", "\tif strings.IndexFunc(", "TestBR1m_")
m("BR-1 a walk of exactly the entry cap reads capped", B,
  "\t\t\treturn subs, hasGo, n, len(ents) > 0\n", "\t\t\t_ = ents\n\t\t\treturn subs, hasGo, n, true\n", "TestBR1p_")
