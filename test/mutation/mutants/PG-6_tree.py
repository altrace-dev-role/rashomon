# PG-6 tree: a gate run is bound to the tree it ran on (card P5A, D140;
# PF-PG6, INV-14). Each break removes one link: the post's sha of a named
# write, the write carrying it, the run's tree, the claim point's tree the
# Stop reads, the compare, the cap, the two Stale rows, the evidence_bound
# row, or the stale_foreign reason's place on the pinned list; and names the
# test that holds it.
T = "internal/verify/tree.go"
K = "internal/verify/classify.go"
G = "internal/verify/gates.go"
O = "internal/verify/obligations.go"
Q = "internal/verify/state.go"
H = "internal/hook/treesha.go"
P = "internal/hook/post.go"
R = "cmd/rashomon/recap_stop.go"
S = "internal/store/scope.go"
PG6a = "TestPG6a_EditAfterTheRunChangesTheDigest"
PG6b = "TestPG6b_OverCapIsUnknown"
PG6c = "TestPG6c_PostRecordsTheWrittenFilesSHA"
PG6d = "TestPG6d_TreeShasReadsEachTarget"
PG6e = "TestPG6e_StopBindsTheTree"
CONC1 = "TestCONC1_ForeignEditIsUnknownNotUnmet"
m("PG-6 the run's tree holds a docs path", T, "\t\tif w.Path != nil && Relevant(w.Class) {\n\t\t\trun[*w.Path] = w.SHA\n", "\t\tif w.Path != nil {\n\t\t\trun[*w.Path] = w.SHA\n", PG6a)
m("PG-6 the first write before the run is the tree", T, "\t\t\trun[*w.Path] = w.SHA\n", "\t\t\tif _, ok := run[*w.Path]; !ok {\n\t\t\t\trun[*w.Path] = w.SHA\n\t\t\t}\n", PG6a)
m("PG-6 a gone file reads unchanged", T, "case now == nil || *now != *sha:", "case now != nil && *now != *sha:", CONC1)
m("PG-6 a mismatch reads met", T, "\t\t\tmoved = true\n", "\t\t\tmoved = false\n", CONC1)
m("PG-6 a path with no sha reads met", T, "\t\t\tknown = false\n", "\t\t\tknown = true\n", PG6b)
m("PG-6 no tree at the point reads met", T, "\tif tree == nil {\n\t\treturn boundUnknown\n\t}\n", "", PG6b)
m("PG-6 the session's own move reads foreign", T, "case moved && slices.ContainsFunc(unnamed,", "case moved && false && slices.ContainsFunc(unnamed,", PG6a)
m("PG-6 the cap off by one, low", T, "if len(last) > MaxTreeFiles {", "if len(last) >= MaxTreeFiles {", PG6b)
m("PG-6 the cap off by one, high", T, "if len(last) > MaxTreeFiles {", "if len(last) > MaxTreeFiles+1 {", PG6b)
m("PG-6 the targets out of path order", T, "return strings.Compare(a.Path, b.Path) })", "return strings.Compare(a.Path, b.Path) * 0 })", PG6b)
m("PG-6 a relative path not joined to its cwd", T, "file = filepath.Join(*d.CWD, file)", "file = filepath.Join(\"\", file)", PG6b)
m("PG-6 a relative path with no cwd is a target", T, "last[*w[0].Path] = named{d.Seq, file, ok}", "last[*w[0].Path] = named{d.Seq, file, true}", PG6b)
m("PG-6 the first write names the file", T, "if cur, seen := last[*w[0].Path]; !seen || d.Seq > cur.seq {", "if _, seen := last[*w[0].Path]; !seen {", PG6b)
m("PG-6 a docs path is a target", T, "; !Relevant(w[0].Class) {", "; false {", PG6b)
m("PG-6 the write carries no sha", K, "\t\t\twrites[n].SHA = e.ContentSHA256\n", "\t\t\twrites[n].SHA = nil\n", PG6a)
m("PG-6 the session's own move is no row", G, "\t{treeMovedOwn, Stale, func(*fam) Reason { return WritesAfterGreen }},\n", "", PG6a)
m("PG-6 a foreign move is no row", G, "\t{treeMovedForeign, Stale, func(*fam) Reason { return StaleForeign }},\n", "", CONC1)
m("PG-6 a foreign move reads the session's", G, "{treeMovedForeign, Stale, func(*fam) Reason { return StaleForeign }}", "{treeMovedForeign, Stale, func(*fam) Reason { return WritesAfterGreen }}", CONC1)
m("PG-6 evidence_bound met with no binding", O, "case g.State == Green && g.bound == boundMet:", "case g.State == Green:", PG6b)
m("PG-6 evidence_bound never unmet", O, "\tcase g.Reason == WritesAfterGreen:\n\t\treturn evidenceUnmet\n", "\tcase false:\n\t\treturn evidenceUnmet\n", PG6a)
m("PG-6 stale_foreign off the pinned list", Q, "WriteDuringRun, StaleForeign, NeverRunAfterWrite,", "WriteDuringRun, NeverRunAfterWrite,", "TestDEL22_GateReasonsAreDistinct")
m("PG-6 a file of exactly 1 MiB unhashed", H, "fi.Size() > MaxHashBytes {", "fi.Size() >= MaxHashBytes {", PG6c)
m("PG-6 a relative path read from the hook's cwd", H, "file = filepath.Join(pl.CWD, file)", "file = filepath.Join(\"\", file)", PG6c)
m("PG-6 a directory read as a file", H, "if !fi.Mode().IsRegular() || fi.Size() > MaxHashBytes {", "if fi.Size() > MaxHashBytes {", PG6d)
m("PG-6 no targets reads as the cap", H, "\tif len(targets) == 0 {\n\t\treturn tree\n\t}\n", "\tif len(targets) == 0 {\n\t\treturn nil\n\t}\n", PG6d)
m("PG-6 the hash not reset between files", H, "\ths.h.Reset()\n", "", PG6d)
m("PG-6 an unreadable file reads as moved", H, "if sum, err := fileSHA256(t.File, hs); err == nil {", "if sum, err := fileSHA256(t.File, hs); err == nil || true {", PG6d)
m("PG-6 the post records no sha", P, "\t\trec.ContentSHA256 = writtenSHA(&pl)\n", "", PG6c)
m("PG-6 the Stop reads no tree", R, "\tpt.Tree = hook.TreeShas(verify.TreeTargets(run))\n", "", PG6e)
m("PG-6 content_sha256 kept below record_scope v5", S, "\tx.ContentSHA256 = nil\n", "", "TestRS1u_GateNullsEveryContentFieldOnlyBelowV5")
