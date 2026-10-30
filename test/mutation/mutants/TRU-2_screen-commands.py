# TRU-2: every command the consent screen, its unit screens and watch's
# prints name is one main.go dispatches, with flags its usage lines carry.
# The first four put back a command a user ran and got the general help for;
# the last three remove one of the sweep's own checks, which a text that is
# already right never shows, so TestTRU2d_ feeds each its defect.
m("TRU-2 a screen paragraph names steer off again", "internal/disclose/paragraphs.go",
  "\"`rashomon detach` removes the hook entries, and the notes stop with them.",
  "\"`rashomon steer off --user` turns them off everywhere, and they stay off.",
  "TestTRU2c_")
m("TRU-2 the closing table names a verb main does not dispatch", "internal/consent/screen.go",
  "\t{\"What is installed and recorded\", \"rashomon status\"},",
  "\t{\"What is installed and recorded\", \"rashomon disclose\"},",
  "TestTRU2c_")
m("TRU-2 watch's last line names disclose again", "cmd/rashomon/watch.go",
  "\"rashomon: rashomon status shows what is installed and what the store holds\"",
  "\"rashomon: rashomon disclose shows everything rashomon does here\"",
  "TestTRU2c_")
m("TRU-2 a unit screen names a flag forget does not take", "internal/disclose/paragraphs.go",
  "`rashomon forget --bodies` removes a project's copies",
  "`rashomon forget --commands` removes a project's copies",
  "TestTRU2c_")
m("TRU-2 a flag in running text is not checked", "cmd/rashomon/consent_text_test.go",
  "\t\tif strict || strings.HasPrefix(w, \"-\") {", "\t\tif strict {",
  "TestTRU2d_")
m("TRU-2 any word after rashomon in running text passes as prose", "cmd/rashomon/consent_text_test.go",
  "\t\tif !strict && proseAfterRashomon[verb] {", "\t\tif !strict {",
  "TestTRU2d_")
m("TRU-2 a sub-command the usage screen lacks is not checked", "cmd/rashomon/consent_text_test.go",
  " || usage[verb][w] {", " || true {",
  "TestTRU2d_")
