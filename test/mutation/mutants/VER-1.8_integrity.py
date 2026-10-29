# VER-1.8: integrity from edit marks and test bending (internal/verify/
# integrity.go, 2.2.6; D39-D42). One break per predicate of the window, the
# marks' pick and basis, the marker table and its path classes, the net and
# its last adverse write, the pairs, the counts and the order. The kill row is
# the named INT test's. No fuzz target: the input is rashomon's own record.
I = "internal/verify/integrity.go"
SORT = ("\tslices.SortFunc(out.Unresolved, func(a, b IntegrityEntry) int {\n"
        "\t\treturn cmp.Or(cmp.Compare(a.Seq, b.Seq), cmp.Compare(slices.Index(IntegrityKinds, a.Check), slices.Index(IntegrityKinds, b.Check)),\n"
        "\t\t\tcmp.Compare(a.Marker, b.Marker), cmp.Compare(a.Path, b.Path))\n"
        "\t})\n")

# The window (from, at] (D40).
m("VER-1.8 a write after the point counts", I, " || w.Seq > at {\n", " || w.Seq > at+1 {\n", "TestINT5_")
m("VER-1.8 a write at the point is cut", I, " || w.Seq > at {\n", " || w.Seq >= at {\n", "TestINT1_")
m("VER-1.8 a write at the window start counts", I, " || w.Seq <= from || ", " || w.Seq < from || ", "TestINT13_")
P = "internal/verify/pairs.go"
m("VER-1.8 a pair at the window start counts", P, " && from < d.Seq && ", " && from <= d.Seq && ", "TestINT13_|TestINT17_")
m("VER-1.8 a pair after the point counts", P, " && d.Seq <= at {\n", " && d.Seq <= at+1 {\n", "TestINT4_|TestINT17_")

# The marks: which calls carry them, and on which basis (D42).
m("VER-1.8 nil marks are read", I, "\t\tif em == nil || w.Path == nil || ", "\t\tif w.Path == nil || ", "TestINT6_")
m("VER-1.8 a write with no path is read", I, "\t\tif em == nil || w.Path == nil || ", "\t\tif em == nil || ", "TestINT6_")
m("VER-1.8 marks on a shell line are folded", I, "e != nil && d.FilePath != nil && ", "e != nil && ", "TestINT6_")
m("VER-1.8 a failed execution's marks are read", I, " && e.Outcome == store.ExecOK {\n", " {\n", "TestINT6_")
m("VER-1.8 the pick ignores outranks", I,
  "ok && (cur == nil || !outranks(cur, e)) {\n", "ok && (cur == nil || cur != nil) {\n", "TestINT6_")
m("VER-1.8 a nil run is read", I, "\tif run == nil {\n\t\treturn marks\n\t}\n", "", "TestINT6_")
m("VER-1.8 the basis filter is dropped", I,
  "\t\tif em.Basis != basisEditDelta && em.Basis != basisWriteVsBefore {\n", "\t\tif false {\n", "TestINT6_")
m("VER-1.8 write_vs_before is not a delta", I,
  " && em.Basis != basisWriteVsBefore {\n", " {\n", "TestINT2_")
m("VER-1.8 the unread-basis counter is dropped", I, "\t\t\tout.UnreadBases++\n", "", "TestINT6_")

# The marker table (D41).
m("VER-1.8 an unknown id is counted as a skip", I,
  "\tif !ok {\n\t\tout.UnknownMarks++\n\t\treturn\n\t}\n", "\tif !ok {\n\t\tclass = markSkip\n\t}\n", "TestINT6_")
m("VER-1.8 the unknown counter is dropped", I, "\t\tout.UnknownMarks++\n", "", "TestINT6_")
m("VER-1.8 an unknown id is read on", I, "\t\tout.UnknownMarks++\n\t\treturn\n", "\t\tout.UnknownMarks++\n", "TestINT6_")
m("VER-1.8 the table misses an id", I, ", \"dotnet.test_def\": markTestDef,\n", ",\n", "TestINT12_")

# Path classes.
m("VER-1.8 a skip on a source path fires", I,
  "\tif c == markSuppression {\n\t\treturn p == ClassSource", "\tif c == markSuppression || c == markSkip {\n\t\treturn p == ClassSource",
  "TestINT7_")
m("VER-1.8 a test definition on a source path counts", I,
  "\tif c == markSuppression {\n\t\treturn p == ClassSource", "\tif c == markSuppression || c == markTestDef {\n\t\treturn p == ClassSource",
  "TestINT7_|TestINT9_")
m("VER-1.8 an assertion on a source path fires", I,
  "\tif c == markSuppression {\n\t\treturn p == ClassSource", "\tif c == markSuppression || c == markAssert {\n\t\treturn p == ClassSource",
  "TestINT7_")
m("VER-1.8 a suppression on a docs path fires", I,
  "\t\treturn p == ClassSource || p == ClassTest\n", "\t\treturn p == ClassSource || p == ClassTest || p == ClassDocs\n",
  "TestINT7_")
m("VER-1.8 a suppression on a source path does not fire", I,
  "\t\treturn p == ClassSource || p == ClassTest\n", "\t\treturn p == ClassTest\n", "TestINT7_")

# The net, its direction and its last adverse write.
m("VER-1.8 a restoring write does not resolve", I,
  "\t\tif adverse(n.class, n.net) && ", "\t\tif n.last > 0 && ", "TestINT3_")
m("VER-1.8 the entry cites the first adverse write", I,
  "\t\tn.last = max(n.last, w.Seq)\n", "\t\tn.last = cmp.Or(n.last, w.Seq)\n", "TestINT3_")
m("VER-1.8 a skip net of zero is unresolved", I, "\t\treturn d > 0\n", "\t\treturn d >= 0\n", "TestINT3_")
m("VER-1.8 a test count that did not move is unresolved", I, "\treturn d < 0\n}\n", "\treturn d <= 0\n}\n", "TestINT11_")
m("VER-1.8 a suppression is adverse when it goes down", I,
  "\tif c == markSkip || c == markSuppression {\n", "\tif c == markSkip {\n", "TestINT7_")
m("VER-1.8 an id in Added and Removed moves twice", I,
  "\t\t\tif _, both := em.Added[id]; !both {\n", "\t\t\tif true {\n", "TestINT9_")
m("VER-1.8 Added is not netted against Removed", I,
  "\t\t\tmoveMark(nets, out, w, id, n-em.Removed[id])\n", "\t\t\tmoveMark(nets, out, w, id, n)\n", "TestINT9_")
m("VER-1.8 assertions fire while tests fell", I,
  "(n.class != markAssert || tests[k.path] >= 0)", "(n.class != markAssert || true)", "TestINT11_")
m("VER-1.8 assertions fire only while the test count is unchanged", I,
  "(n.class != markAssert || tests[k.path] >= 0)", "(n.class != markAssert || tests[k.path] == 0)", "TestINT11_")

# The pairs.
m("VER-1.8 tests_only_then_green is resolved by a later write", I,
  "\t\tout.Unresolved = append(out.Unresolved, IntegrityEntry{Check: TestsOnlyThenGreen, Seq: p[1]})\n",
  "\t\tif !slices.ContainsFunc(writes, func(w write) bool { return w.Seq > p[1] && w.Seq <= at }) {\n"
  "\t\t\tout.Unresolved = append(out.Unresolved, IntegrityEntry{Check: TestsOnlyThenGreen, Seq: p[1]})\n\t\t}\n",
  "TestINT4_")

# Counts.
m("VER-1.8 TestsDelta is floored at zero", I,
  "\t\t\tout.Counts.TestsDelta += n.net\n", "\t\t\tout.Counts.TestsDelta += max(n.net, 0)\n", "TestINT9_")
m("VER-1.8 SkipsAdded counts negatives", I,
  "\t\t\tout.Counts.SkipsAdded += max(n.net, 0)\n", "\t\t\tout.Counts.SkipsAdded += n.net\n", "TestINT9_")
m("VER-1.8 SuppressionsAdded counts negatives", I,
  "\t\t\tout.Counts.SuppressionsAdded += max(n.net, 0)\n", "\t\t\tout.Counts.SuppressionsAdded += n.net\n", "TestINT9_")

# Order and shape.
m("VER-1.8 an empty Unresolved is null", I,
  "\tout := Integrity{Unresolved: []IntegrityEntry{}}\n", "\tout := Integrity{}\n", "TestINT6_")
m("VER-1.8 the entries are unsorted", I, SORT, "", "TestINT8_")
IMPORTS["VER-1.8 the entries are unsorted"] = (I, "import (\n\t\"cmp\"\n\t\"slices\"\n\t\"sync\"\n", "import (\n\t\"sync\"\n")
m("VER-1.8 the check is not a sort key", I,
  "cmp.Compare(slices.Index(IntegrityKinds, a.Check), slices.Index(IntegrityKinds, b.Check)),\n", "0,\n", "TestINT8_")
m("VER-1.8 the marker is not a sort key", I,
  "\t\t\tcmp.Compare(a.Marker, b.Marker), cmp.Compare(a.Path, b.Path))\n", "\t\t\t0, cmp.Compare(a.Path, b.Path))\n", "TestINT8_")

# Part two: Evaluate's wiring (PF-IV3, D39, D40) and the findings row (Gate B F1).
E = "internal/verify/evaluate.go"
m("VER-1.8 Evaluate does not fold integrity", E, "\tsnap.Integrity = integrity(run, picked, writes, 0, at)\n", "", "TestINT10_")
m("VER-1.8 Evaluate's window starts at the point", E,
  "integrity(run, picked, writes, 0, at)", "integrity(run, picked, writes, at, at)", "TestINT10_")
m("VER-1.8 RecordGreen ignores integrity", E, "\tif unresolved > 0 {\n\t\treturn false, nil\n\t}\n", "", "TestINT10_")
m("VER-1.8 the findings row is not taken", E,
  "\tdefault:\n\t\trow = RowFindings\n", "\tdefault:\n\t\trow = RowVerified\n", "TestINT10_")
m("VER-1.8 the count sentence is off by one", E,
  "\treturn strconv.Itoa(k) + \" record", "\treturn strconv.Itoa(k+1) + \" record", "TestINT10_")
m("VER-1.8 the count sentence is on every row", E, "\tcase row == RowFindings:\n", "\tdefault:\n", "TestINT10_")

# 1.8r: the pair read in verify over classify's pick (pairs.go), report's
# definition (Gate A R1-R5, D44), and the import pin. INT14 holds the pairs
# equal to report.DetectTestBending's on any record; its rows are each built
# where a break would part the two.
m("VER-1.8r Evaluate hands integrity no pick", E,
  "integrity(run, picked, writes, 0, at)", "integrity(run, nil, writes, 0, at)", "TestINT15_")
m("VER-1.8r verify imports report again", I,
  '\t"github.com/altrace-dev-role/rashomon/internal/store"\n',
  '\t_ "github.com/altrace-dev-role/rashomon/internal/report"\n\t"github.com/altrace-dev-role/rashomon/internal/store"\n', "TestRES2d_")
m("VER-1.8r an other edit between still pairs", P, " && others == prev.others && ", " && ", "TestINT14_|TestINT15_")
m("VER-1.8r the test count need not rise", P, " && tests > prev.tests && ", " && ", "TestINT14_")
m("VER-1.8r the order of the pair is not read", P, " && prev.failed && ok && ", " && prev.failed != failed && ", "TestINT14_")
m("VER-1.8r another command closes the pair", P,
  "\t\tkey := command{d.Shape.Digest, d.CWDDigest}\n", "\t\tkey := command{\"\", d.CWDDigest}\n", "TestINT14_")
m("VER-1.8r the same command elsewhere closes the pair", P,
  "\t\tkey := command{d.Shape.Digest, d.CWDDigest}\n", "\t\tkey := command{d.Shape.Digest, \"\"}\n", "TestINT14_")
m("VER-1.8r the pair is cut on the red", P,
  " && from < d.Seq && d.Seq <= at {\n", " && from < prev.seq && prev.seq <= at {\n", "TestINT17_")
m("VER-1.8r a masked line read as a run", P, " && !hidden && (ok || failed)\n", " && (ok || failed)\n", "TestINT14_")
m("VER-1.8r masking is the declaration's (D44)", P, "\tif e != nil {\n\t\tm, _, _, _ = asRan(d, e)\n\t}\n", "", "TestINT14_")
m("VER-1.8r two reds pair the first", P,
  '\t\tif isRun && d.CWDDigest != "" {\n', '\t\tif isRun && d.CWDDigest != "" && !(failed && last[key].failed) {\n', "TestINT18_")
m("VER-1.8r a green leaves the red open (two pairs)", P,
  '\t\tif isRun && d.CWDDigest != "" {\n', '\t\tif failed && d.CWDDigest != "" {\n', "TestINT14_")
m("VER-1.8r a run at no known location pairs", P, '\t\tif isRun && d.CWDDigest != "" {\n', "\t\tif isRun {\n", "TestINT14_")
m("VER-1.8r the run's own edit is between", P, "others: others}", "others: others - 1}", "TestINT4_|TestINT14_")
m("VER-1.8r a failed test edit counts", P, "\t\t\tcase ok:\n\t\t\t\ttests++\n", "\t\t\tdefault:\n\t\t\t\ttests++\n", "TestINT14_")
m("VER-1.8r a failed test-file edit stops the pair", P,
  "\t\t\tcase ok:\n\t\t\t\ttests++\n", "\t\t\tcase ok:\n\t\t\t\ttests++\n\t\t\tdefault:\n\t\t\t\tothers++\n", "TestINT14_")
m("VER-1.8r a test edit needs no label", P,
  " || d.FileLabel == nil || *d.FileLabel != shape.LabelTestFile:\n", ":\n", "TestINT14_")
m("VER-1.8r the record is read in slice order", P,
  "\tif slices.IsSortedFunc(run.Declarations, bySeq) {\n", "\tif true {\n", "TestINT14_|TestINT8_")
m("VER-1.8r a call with no execution is a result", P,
  "\tcase e == nil:\n\t\treturn false, false\n", "\tcase e == nil:\n\t\treturn true, false\n", "TestINT14_")
m("VER-1.8r a timeout-124 red pairs", P, "\t\treturn false, !stopped\n", "\t\treturn false, !stopped || stopped\n", "TestINT14_")
m("VER-1.8r a backgrounded green is a result", P, " && !e.Backgrounded, false\n", ", false\n", "TestINT14_")
m("VER-1.8r a noWrite tool stops the pair", P, "\tif slices.Contains(noWrite, d.ToolName) {\n\t\treturn false\n\t}\n", "", "TestINT14_")
m("VER-1.8r the noWrite list misses a name", P, '"TodoWrite", ', "", "TestINT14_")
m("VER-1.8r a shell line that may write is read by its class", P,
  "\tif d.Shape.MayWrite || hidden {\n", "\tif hidden {\n", "TestINT14_")
m("VER-1.8r a masked line is read by its class", P,
  "\tif d.Shape.MayWrite || hidden {\n", "\tif d.Shape.MayWrite {\n", "TestINT14_")
m("VER-1.8r an MCP call is no edit", P,
  "\tcase shape.VerbRead, shape.VerbNetwork, shape.VerbAgent:\n", "\tcase shape.VerbRead, shape.VerbNetwork, shape.VerbAgent, shape.VerbMCP:\n",
  "TestINT14_|TestINT15_")
m("VER-1.8r a read is an edit", P,
  "\tcase shape.VerbRead, shape.VerbNetwork, shape.VerbAgent:\n", "\tcase shape.VerbNetwork, shape.VerbAgent:\n", "TestINT14_|TestINT15_")
m("VER-1.8r an undeclared call is ignored", P,
  "\tif len(pairs) > 0 && undeclared(run, at) {\n", "\tif len(pairs) > 0 && false {\n", "TestINT14_")
m("VER-1.8r an orphan execution after the point cuts the pair", P, "; !ok && byPoint(e.Seq, at) {\n", "; !ok {\n", "TestINT17_")
m("VER-1.8r an orphan terminal after the point cuts the pair", P, "; !ok && byPoint(t.Seq, at) {\n", "; !ok {\n", "TestINT17_")
m("VER-1.8r a flush_failed marker with no id cuts the pair", P,
  '\t\tif t.FlushFailed() && t.ToolUseID == "" {\n\t\t\tcontinue\n\t\t}\n', "", "TestINT14_")

# 1.8r2: integrity reads testPairs on its precondition, that recording reads
# the record Complete, whose outcome_without_declaration is a superset of the
# guard; testsOnlyThenGreen keeps the guard for the detector on its own. The
# card's "testsOnlyThenGreen drops the guard" is "an undeclared call is
# ignored" above, and its "undeclared() stops skipping the flush_failed
# terminal" is "a flush_failed marker with no id cuts the pair".
m("VER-1.8r2 integrity reads the guarded pairs", I,
  "\tfor _, p := range testPairs(run, picked, from, at) {\n",
  "\tfor _, p := range testsOnlyThenGreen(run, picked, from, at) {\n", "TestINT19c_")
m("VER-1.8r2 testPairs keeps the guard", P,
  "\t\t\tvisit(&run.Declarations[i])\n\t\t}\n\t}\n\treturn pairs\n}\n",
  "\t\t\tvisit(&run.Declarations[i])\n\t\t}\n\t}\n\tif len(pairs) > 0 && undeclared(run, at) {\n\t\treturn nil\n\t}\n\treturn pairs\n}\n",
  "TestINT19b_|TestINT19c_")
m("VER-1.8r2 evaluate reads integrity before recording's return", E,
  "\tif snap.Recording != Complete {\n\t\treturn snap\n\t}\n\tpicked := pickExecutions(run)\n\truns, writes, unnamed, rewritten := classifyWith(run, picked)\n",
  "\tpicked := pickExecutions(run)\n\truns, writes, unnamed, rewritten := classifyWith(run, picked)\n"
  "\tif snap.Recording != Complete {\n\t\tsnap.Integrity = integrity(run, picked, writes, 0, at)\n\t\treturn snap\n\t}\n",
  "TestINT19b_")
m("VER-1.8r2 undeclared finds no orphan", P,
  "func undeclared(run *store.Run, at int64) bool {\n",
  "func undeclared(run *store.Run, at int64) bool {\n\treturn false\n", "TestINT19b_")
