# LEARN-2 (transcript learning PR 2, docs/autosteer/LEARNING.md section 3):
# episodes are found in a session's trace steps by pure functions that name no
# agent, language or tool; a key is a kind, a kind of act, a scope and a
# fingerprint of the failure's own sentence.
D = "internal/episode/detect.go"
F = "internal/episode/fingerprint.go"
C = "internal/episode/command.go"
S = "internal/episode/session.go"
T = "internal/trace/claudecode/claudecode.go"

# The detectors.
m("LEARN-2 a regression reads as a plain failure", D,
  "\t\tif s.regressed(i) {\n", "\t\tif false {\n", "TestTypeScript_")
m("LEARN-2 a one-off failure is an episode", D,
  "\t\t} else if s.next(i, a.sig, func(*act) bool { return true }) < 0 {\n",
  "\t\t} else if s.next(i, a.sig, func(*act) bool { return true }) < -1 {\n", "TestDetect_NoEpisodeForAOneOff")
m("LEARN-2 a subagent's failure is the session's", D,
  "\t\tif !a.failed() || a.side || open[a.sig] {\n", "\t\tif !a.failed() || open[a.sig] {\n", "TestDetect_NoEpisodeForAOneOff")
m("LEARN-2 an interrupt with no act since counts", D,
  "\t\tif l.after < 0 || !acted {\n", "\t\tif l.after < 0 {\n", "TestTerraform_")
m("LEARN-2 a machine's line counts as pushback", D,
  "l.orig == trace.OriginPerson && pushback != nil", "l.orig != trace.OriginMachine && pushback != nil", "TestTerraform_")
m("LEARN-2 an edit in another file reads as a revert", D,
  "\t\t\t\tif p.c.Path == c.Path && p.c.Old != \"\"", "\t\t\t\tif p.c.Path != c.Path && p.c.Old != \"\"", "TestTerraform_")
m("LEARN-2 one identical retry is a loop", D,
  "\t\tif j-i >= 3 {\n", "\t\tif j-i >= 2 {\n", "TestRetryLoop_")

# The fingerprint.
m("LEARN-2 a line a passing run prints is picked", F,
  "\t\tif _, ok := seen[t]; ok {\n\t\t\tcontinue\n\t\t}\n", "", "TestFingerprint_BaselineAndTies")
m("LEARN-2 a tie goes to the earlier line", F,
  "\t\tif n < minFixed || n < most {\n", "\t\tif n < minFixed || n <= most {\n", "TestFingerprint_BaselineAndTies")
m("LEARN-2 a hex id is a fixed word", F,
  "\tif len(w) >= 8 && strings.Trim(", "\tif len(w) >= 800 && strings.Trim(", "TestFingerprint_StableAcrossWhatVaries")
m("LEARN-2 decoration is a word", F,
  "\t\t\tcontinue // punctuation or a rule of symbols: decoration, not a word\n",
  "\t\t\t// punctuation or a rule of symbols: decoration, not a word\n", "TestPython_")

# The command, the scope and the paths.
m("LEARN-2 a wrapper is the program", C,
  "\t\tcase wrappers[path.Base(words[0])]:\n", "\t\tcase false:\n", "TestCommandSig")
m("LEARN-2 a setup segment is the program", C,
  "\t\tif len(words) == 0 || setup[words[0]] {\n", "\t\tif len(words) == 0 {\n", "TestCommandSig")
m("LEARN-2 a scope runs deeper than three levels", S,
  "const maxScopeDepth = 3\n", "const maxScopeDepth = 4\n", "TestScopeOf")
m("LEARN-2 the root itself is a path in scope", S,
  "\t\tif ok && r != \".\" && !seen[r] {\n", "\t\tif ok && !seen[r] {\n", "TestTypeScript_")

# The adapter's content fields.
m("LEARN-2 a call's content is not read", T,
  "\t\t\tif content {\n\t\t\t\tfill(s.Call)\n\t\t\t}\n", "", "TestReadContent_Calls")
m("LEARN-2 a result's output is not read", T,
  "\tif content {\n\t\tr.Output, _, _ = report.ResultClass(b.IsError, b.Content)\n\t}\n", "", "TestReadContent_Outputs")
