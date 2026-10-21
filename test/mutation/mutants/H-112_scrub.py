# H-112, the scrubber a contributor runs on a capture before committing it.
# Each break leaves one shape in the output, or breaks a join, or rewrites a
# byte that was not a shape, and H-112's items each read the output for
# exactly that.
SCRUB = "test/tools/hygiene/scrub.go"
H112 = "TestH112_"

m("H-112 the inventory lists are kept", SCRUB,
  "\t}, func(string, int, int) bool { return true })",
  "\t}, func(string, int, int) bool { return false })", H112)
m("H-112 a value under a secret-named member is kept", SCRUB,
  "\tcase member != \"\" && v != \"\" && secretshape.SecretNamed(member):\n\t\treturn secretshape.Redacted",
  "\tcase member != \"\" && v != \"\" && secretshape.SecretNamed(member):\n\t\treturn z.text(v)", H112)
m("H-112 a payload held as text is scrubbed as text", SCRUB,
  "\tcase isDocument(v):\n\t\treturn z.document(v, named)\n", "", H112)
m("H-112 a digest the file names is redacted", SCRUB,
  "\tcase named[v]:\n\t\treturn v\n", "", H112)
m("H-112 secret shapes are kept", SCRUB,
  "\ts, _ = secretshape.Redact(s)", "\ts, _ = s, 0", H112)
m("H-112 the scratchpad is left to the temp rule", SCRUB,
  "\ts = scratchpadRE.ReplaceAllLiteralString(s, \"$SCRATCHPAD\")\n", "", H112)
m("H-112 temp roots are kept", SCRUB,
  "\ts = tempRE.ReplaceAllString(s, \"${1}$$TMPDIR\")\n", "", H112)
m("H-112 the scrubbing machine's own home is kept", SCRUB,
  "\ts = replaceDir(s, z.home, \"~\")\n", "", H112)
m("H-112 home directories are kept", SCRUB,
  "\ts = homeRE.ReplaceAllString(s, \"${1}~\")\n", "", H112)
m("H-112 email addresses are kept", SCRUB,
  "\treturn assign(s, emailSpans(s), z.emails, \"user-%d@example.invalid\")",
  "\treturn s", H112)
m("H-112 every occurrence of an id takes a fresh placeholder", "test/tools/hygiene/shapes.go",
  "\t\tp, ok := seen[key]", "\t\tp, ok := \"\", false", H112 + "|TestScrubIsStable")
m("H-112 a literal is re-encoded when nothing in it changed", "test/tools/hygiene/json.go",
  "\tif s == old {\n\t\treturn raw\n\t}\n", "", H112 + "|TestRewriteCopies")
m("H-112 what scrub leaves behind exits 0", "test/tools/scrub/main.go",
  "\t\t\tfmt.Fprintln(stderr, \"scrub: still there:\", f)\n\t\t\tstatus = 1",
  "\t\t\tfmt.Fprintln(stderr, \"scrub: still there:\", f)", H112)
