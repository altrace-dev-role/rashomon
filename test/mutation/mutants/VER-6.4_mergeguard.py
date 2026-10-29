# VER-6.4: the report-only merge guard (scripts/mergeguard). One break per
# promise: it never fails a build, it fetches the notes it is told to, the
# note must be the record the trailer names, each declared gate reruns in its
# dir and is compared with the checkpoint's own state, ci gates map to their
# commands, and nothing the note carries runs. The kill row is the named
# test's.
P = "scripts/mergeguard/main.go"
G = "scripts/mergeguard/mergeguard.sh"

m("VER-6.4 the guard fails the build on a difference", G,
  'sh "$tmp/rerun.sh"\nexit 0\n',
  'sh "$tmp/rerun.sh" | tee "$tmp/out"\n! grep -q \' [1-9][0-9]* differ\' "$tmp/out"\n', "TestRG2a_")
m("VER-6.4 the notes are not fetched", G,
  "\tgit fetch -q \"$remote\" '+refs/notes/rashomon:refs/notes/rashomon' ||\n", "\ttrue ||\n", "TestRG2b_")
m("VER-6.4 a note the trailer does not name is read", P,
  "\tif got := receipt.Digest([]byte(record)); got != trailer {\n",
  "\tif got := receipt.Digest([]byte(record)); false && got != trailer {\n", "TestRG2a_")
m("VER-6.4 a gate runs outside its dir", P, '\t\tcd += " && cd " + q(c.dir)\n', '\t\tcd += ""\n', "TestRG2b_")
m("VER-6.4 a rerun is compared with green, not the checkpoint's state", P,
  '\tfmt.Fprintf(&p.body, "if [ \\"$r\\" = %s ]; then a=$((a + 1)); else d=$((d + 1)); fi\\n", q(state))\n',
  '\tfmt.Fprintf(&p.body, "if [ \\"$r\\" = %s ]; then a=$((a + 1)); else d=$((d + 1)); fi\\n", q("green"))\n',
  "TestRG2b_")
m("VER-6.4 a ci gate maps to the next command", P,
  '\t\tdeclared["ci:"+strconv.Itoa(i)] = command{"", argv}\n',
  '\t\tdeclared["ci:"+strconv.Itoa(i+1)] = command{"", argv}\n', "TestRG2b_")
m("VER-6.4 the note's text is quoted no further than a pair of quotes", P,
  "func q(s string) string { return \"'\" + strings.ReplaceAll(s, \"'\", `'\"'\"'`) + \"'\" }\n",
  "func q(s string) string { return \"'\" + s + \"'\" }\n", "TestRG2c_TheNoteNeverRuns")
m("VER-6.4 a NUL in a dir is carried", P,
  "\tif len(c.argv) == 0 || strings.ContainsRune(c.dir, 0) {\n", "\tif len(c.argv) == 0 {\n",
  "TestRG2c_ACommandShCannotCarryIsNotRun")
m("VER-6.4 a NUL in a word is carried", P,
  "\t\tif strings.ContainsRune(w, 0) {\n", "\t\tif false && strings.ContainsRune(w, 0) {\n", "TestRG2c_ACommandShCannotCarryIsNotRun")
