# Issue #102: silent_failure and masked_run read failureVocabulary as
# substrings, so "bug" in "debug" or "fail" in "pipefail" counted as the agent
# naming a failure and the line stayed silent. An entry now counts only where
# it opens a word (namesFailure).
A = "internal/report/account.go"
SF = "TestSilentFailures_AWordInsideAnotherWordNamesNoFailure"
MR = "TestMasked_AWordInsideAnotherWordNamesNoFailure"
m("I-102 an entry inside another word names a failure", A,
  "; !unicode.Is(unicode.Latin, r) {\n", "; r >= 0 {\n", SF + "|" + MR)
m("I-102 a digit before an entry is part of the word", A,
  "; !unicode.Is(unicode.Latin, r) {\n", "; !unicode.Is(unicode.Latin, r) && !unicode.IsDigit(r) {\n", SF + "|" + MR)
m("I-102 a Han letter before an entry is part of the word", A,
  "; !unicode.Is(unicode.Latin, r) {\n", "; !unicode.IsLetter(r) {\n", SF + "|" + MR)
m("I-102 an entry opening the message names nothing", A,
  "; !unicode.Is(unicode.Latin, r) {\n", "; k > 0 && !unicode.Is(unicode.Latin, r) {\n", SF)
m("I-102 the scan stops at the first entry inside a word", A,
  "\t\ti = k + 1 // a match", "\t\treturn false // a match", SF + "|" + MR)
m("I-102 silent_failure reads entries as substrings", A,
  "if acct.Available && namesFailure(lower, w) {", "if acct.Available && strings.Contains(lower, w) {", SF)
m("I-102 masked_run reads entries as substrings", A,
  "\t\tif namesFailure(lower, w) {\n\t\t\tfailure = true", "\t\tif strings.Contains(lower, w) {\n\t\t\tfailure = true", MR)
