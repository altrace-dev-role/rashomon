# VER-1.2: classify turns a run's calls into gate runs keyed by family and
# writes with their path class (internal/verify/classify.go). One break per
# rule: the family predicate, the gate-run predicate, the kind order and its
# inference, the rewritten check, each unnamed case, the execution pick, the
# as-ran fields, each relativise step and each output sort. The kill row is
# the named test's.
C = "internal/verify/classify.go"

# The family: both the runner and the location known.
m("VER-1.2 a run at no known location has a family", C,
  '\tif g.Runner == nil || g.Location == "" {\n', '\tif g.Runner == nil {\n', "TestVER5b_|TestVER6_")

# Which calls are gate runs.
m("VER-1.2 a hidden runner's line is not a gate run", C,
  ' || masked != nil && (*masked == shape.MaskedTest || *masked == shape.MaskedBuild) ||', ' ||', "TestVER5_")
m("VER-1.2 a test-class line with no runner is not a gate run", C,
  ' ||\n\t\td.Shape.VerbClass == shape.VerbTest\n', '\n', "TestVER5_")

# Kind: the recorded kind first, then the inference, then custom.
m("VER-1.2 a recorded runner kind is not read", C,
  '\t\tcase kind != nil:\n', '\t\tcase kind != nil && false:\n', "TestVER1_")
m("VER-1.2 an old record's kind is never inferred", C,
  '\t\tcase runner != nil && schema < 6:\n', '\t\tcase runner != nil && schema < 0:\n', "TestVER1b_")
IMPORTS["VER-1.2 kind inference pools by location"] = (C,
  '\t\tswitch r := *runs[i].Runner; {\n', '\t\tswitch r := runs[i].Location + *runs[i].Runner; {\n')
m("VER-1.2 kind inference pools by location", C,
  '\t\tm, r, _, _ := asRan(d, e)\n\t\tif r == nil {\n\t\t\tcontinue\n\t\t}\n',
  '\t\tm, r, _, _ := asRan(d, e)\n\t\tif r == nil {\n\t\t\tcontinue\n\t\t}\n\t\tpooled := d.CWDDigest + *r\n\t\tr = &pooled\n',
  "TestVER1b_")
m("VER-1.2 a sibling masked build infers nothing", C,
  '\t\t} else if m != nil && *m == shape.MaskedBuild {\n', '\t\t} else if false {\n', "TestVER1b_")

# A line rewritten after its declaration.
m("VER-1.2 a rewritten gate line keeps its location", C,
  '\t\tif e.ExecutedDigest != "" && e.ExecutedDigest != d.Shape.Digest {\n\t\t\tg.Location = ""\n\t\t\trewritten = append(rewritten, d.Seq)\n\t\t}\n',
  '', "TestVER6_")

# Unnamed writes.
m("VER-1.2 a write with no known target is not listed", C,
  '\t\tunnamed = append(unnamed, d.Seq)\n\t}\n\tswitch {\n', '\t}\n\tswitch {\n', "TestVER2_|TestVER4_")
m("VER-1.2 an incomplete named list is not unnamed", C,
  '\t\tif d.WrittenPathsComplete == nil || !*d.WrittenPathsComplete {\n\t\t\tunnamed = append(unnamed, d.Seq)\n\t\t}\n',
  '', "TestVER4_")
m("VER-1.2 an MCP call is not an unnamed write", C,
  '\tcase d.Shape.VerbClass == shape.VerbMCP:\n\t\tblind()\n', '', "TestVER4_")

# The execution a call is judged on, and its fields as the line ran.
m("VER-1.2 the pick is the last record read", C,
  '\tcf, ef := cur.Outcome == store.ExecFailed, e.Outcome == store.ExecFailed\n\tif cf != ef {\n\t\treturn cf\n\t}\n\treturn execSeq(cur) > execSeq(e)\n',
  '\treturn false\n', "TestVER9_")
m("VER-1.2 a failed record does not outrank an ok one", C,
  '\tif cf != ef {\n\t\treturn cf\n\t}\n', '\t_, _ = cf, ef\n', "TestVER9_")
m("VER-1.2 the runner is the declaration's, not the line's as it ran", C,
  '\tif e.SchemaVersion >= 4 && e.ExecutedDigest != "" {\n', '\tif false {\n', "TestVER9_")

# relativise.
m("VER-1.2 relativise strips the cwd before the project", C,
  '[2]*string{project, cwd}', '[2]*string{cwd, project}', "TestVER7_")
m("VER-1.2 separators are not normalised", C,
  'func slashed(s string) string { return strings.ReplaceAll(s, `\\`, "/") }',
  'func slashed(s string) string { return s }', "TestVER7_")
IMPORTS["VER-1.2 relativise reads the filesystem"] = (C, '\t"sort"\n', '\t"os"\n\t"sort"\n')
m("VER-1.2 relativise reads the filesystem", C,
  '\tp := slashed(filePath)\n',
  '\tp := slashed(filePath)\n\tif _, err := os.Stat(filePath); err != nil {\n\t\treturn p\n\t}\n', "TestVER7_")
m("VER-1.2 a leading ./ is kept", C,
  '\tfor strings.HasPrefix(p, "./") {\n\t\tp = p[2:]\n\t}\n', '', "TestVER7_")
m("VER-1.2 a prefix need not end at a separator", C,
  " || p[len(dir)] != '/'", '', "TestVER7_")
m("VER-1.2 the root is a prefix", C,
  '\tif dir == "" || len(p)', '\tif len(p)', "TestVER7_")
m("VER-1.2 a drive letter compares with its case", C,
  'a[0]|0x20 == b[0]|0x20', 'a[0] == b[0]', "TestVER7_")

# Seq order, whatever order the records are read in.
m("VER-1.2 runs are left in read order", C,
  '\tsort.SliceStable(runs, func(i, j int) bool { return runs[i].Seq < runs[j].Seq })\n', '', "TestVER8_")
m("VER-1.2 writes are left in read order", C,
  '\tsort.SliceStable(writes, func(i, j int) bool { return writes[i].Seq < writes[j].Seq })\n', '', "TestVER8_")
m("VER-1.2 unnamed is left in read order", C, '\tslices.Sort(unnamed)\n', '', "TestVER8_")
m("VER-1.2 rewritten is left in read order", C, '\tslices.Sort(rewritten)\n', '', "TestVER8_")
