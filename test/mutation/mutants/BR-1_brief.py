# BR-1: the project brief at session start (card BRIEF1, R2). Each break
# removes one of the brief's promises: the walk's entry cap, nothing guessed
# without a manifest, every line sourced, and a startup print the model never
# reads.
B = "internal/brief/brief.go"
C = "cmd/rashomon/brief.go"

m("BR-1 the walk reads past its entry cap", B, "\t\tif n >= left || over() {\n", "\t\tif over() {\n", "TestBR1c_")
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
# /rashomon:report last only from the plugin's own entry (the recap
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
m("BR-1 the brief names rashomon, not this binary", B, "\treturn exe\n}\n", '\treturn "rashomon"\n}\n',
  "TestBR1k_|TestBR1f_|TestBR1l_")
