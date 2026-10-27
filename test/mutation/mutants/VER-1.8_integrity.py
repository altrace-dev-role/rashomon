# VER-1.8: integrity from edit marks and test bending (internal/verify/
# integrity.go, 2.2.6; D39-D42). One break per predicate of the window, the
# marks' pick and basis, the marker table and its path classes, the net and
# its last adverse write, the pairs, the counts and the order. The kill row is
# the named INT test's. No fuzz target: the input is rashomon's own record.
I = "internal/verify/integrity.go"
SORT = ("\tslices.SortFunc(out.Unresolved, func(a, b IntegrityEntry) int {\n"
        "\t\treturn cmp.Or(cmp.Compare(a.Seq, b.Seq), cmp.Compare(slices.Index(IntegrityKinds, a.Check), slices.Index(IntegrityKinds, b.Check)),\n"
        "\t\t\tcmp.Compare(a.Marker, b.Marker))\n"
        "\t})\n")

# The window (from, at] (D40).
m("VER-1.8 a write after the point counts", I, " || w.Seq > at {\n", " || w.Seq > at+1 {\n", "TestINT5_")
m("VER-1.8 a write at the point is cut", I, " || w.Seq > at {\n", " || w.Seq >= at {\n", "TestINT1_")
m("VER-1.8 a write at the window start counts", I, " || w.Seq <= from || ", " || w.Seq < from || ", "TestINT13_")
m("VER-1.8 a pair at the window start counts", I, "\t\tif from < p[1] && ", "\t\tif from <= p[1] && ", "TestINT13_")
m("VER-1.8 a pair after the point counts", I, " && p[1] <= at {\n", " && p[1] <= at+1 {\n", "TestINT4_")

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
  " && p[1] <= at {\n", " && p[1] <= at && !slices.ContainsFunc(writes, func(w write) bool { return w.Seq > p[1] && w.Seq <= at }) {\n",
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
IMPORTS["VER-1.8 the entries are unsorted"] = (I, "import (\n\t\"cmp\"\n\t\"slices\"\n\n", "import (\n")
m("VER-1.8 the check is not a sort key", I,
  "cmp.Compare(slices.Index(IntegrityKinds, a.Check), slices.Index(IntegrityKinds, b.Check)),\n", "0,\n", "TestINT8_")
m("VER-1.8 the marker is not a sort key", I,
  "\t\t\tcmp.Compare(a.Marker, b.Marker))\n", "\t\t\t0)\n", "TestINT8_")
