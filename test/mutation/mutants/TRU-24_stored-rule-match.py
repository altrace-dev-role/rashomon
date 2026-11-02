# TRU-24 (#216 ruling 2): the consent screen's STORED paragraph names the
# rule match, by the rule's id and the spec file's digest, never the command.
# Without the clause a person agrees to a record the screen never told of.
m("TRU-24 the STORED paragraph drops the rule-match clause", "internal/disclose/paragraphs.go",
  "which spec rule a call matched, by the rule's id and the spec file's digest, never the command; and the time",
  "and the time", "TestTRU24b_")
m("TRU-24 the STORED paragraph lets the rule match carry the command", "internal/disclose/paragraphs.go",
  "by the rule's id and the spec file's digest, never the command; and the time",
  "by the rule's id and the spec file's digest, and the command; and the time", "TestTRU24b_")
