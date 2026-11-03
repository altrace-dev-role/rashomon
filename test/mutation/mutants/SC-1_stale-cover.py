# SC-1: a gate is stale only by a file it covers (card STALE-COVER1): covered
# is inside the gate's project root, read lexically from the record (covers);
# a learned family's root is its last run's project, a declared or inferred
# gate's the snapshot's; the turn counts covered writes; a done claim does not
# answer to a gate that never ran and covers nothing written. Fail closed
# where the record cannot place a gate or a write. Each break removes one rule
# and names the tests that hold it.
C = "internal/verify/cover.go"
G = "internal/verify/gates.go"
D = "internal/verify/declared.go"
S = "internal/verify/stop.go"
L = "internal/verify/claims.go"
V = "internal/verify/steer.go"
O = "internal/verify/obligations.go"
T = "internal/verify/tree.go"
m("SC-1 every write is covered", C,
  "\tif root == \"\" || w.Project == nil {\n\t\treturn true\n", "\tif true {\n\t\treturn true\n", "TestSC1")
m("SC-1 an absolute path is covered anywhere", C,
  "\t\t_, in := under(p, root)\n\t\treturn in\n", "\t\treturn true\n", "TestSC1a_|TestSC1b_")
m("SC-1 a path that climbs out is placed in its call's project", C,
  "\tif proj == \"\" || p == \"..\" || strings.HasPrefix(p, \"../\") {\n", "\tif proj == \"\" {\n", "TestSC1c_")
m("SC-1 a write in the gate's own project is not covered", C,
  "\tif _, in := under(proj, root); in || proj == root {\n", "\tif _, in := under(proj, root); in {\n", "TestSC1g_")
m("SC-1 a parent session's write into the worktree is not covered", C,
  "\treturn in && (p == rel || strings.HasPrefix(p, rel+\"/\"))\n",
  "\treturn false && in && (p == rel || strings.HasPrefix(p, rel+\"/\"))\n", "TestSC1d_")
m("SC-1 a write proven in no project reads not known", C,
  "\t\treturn &noProject\n", "\t\treturn nil\n", "TestSC1a_")
m("SC-1 a withheld project reads proven in none", C,
  "\tcase d.ProjectWithheld == nil && d.CWD != nil:\n", "\tcase d.CWD != nil:\n", "TestSC1f_")
m("SC-1 a gate with no root is left out of the turn's roots", C,
  "\t\tif r == \"\" {\n\t\t\treturn nil\n\t\t}\n", "", "TestSC1k_")
m("SC-1 an incomplete turn counts by root", C,
  "\tif snap.Recording != Complete || snap.ProjectRoot == \"\" {\n", "\tif snap.ProjectRoot == \"\" {\n", "TestSC1k_")
m("SC-1 a learned family covers every write", G,
  "keys.key(fr[len(fr)-1].ProjectID)", "keys.key(0)", "TestSC1c_|TestSC1d_|TestSC1h_|TestSC1i_|TestSC1j_")
m("SC-1 a learned family counts an uncovered write", G,
  "w.Seq > f.ref.Seq && w.Seq <= at && covers(root, &w) {\n", "w.Seq > f.ref.Seq && w.Seq <= at {\n",
  "TestSC1b_|TestSC1c_|TestSC1d_")
m("SC-1 an absent gate counts an uncovered write", D,
  "w.Seq <= at && covers(root, &w) {\n", "w.Seq <= at {\n", "TestSC1a_")
m("SC-1 the turn counts every write", S,
  "window[w.Seq] && coveredByAny(roots, &w) {\n", "window[w.Seq] {\n", "TestSC1a_")
m("SC-1 a done claim answers to a gate with nothing to verify", L,
  " || r.class == ClaimDone && owesNothing(&g) {\n", " {\n", "TestSC1a_")
m("SC-1 a never-run gate with covered writes owes nothing", L,
  "g.LastRun == nil && g.WritesSinceGreen == 0 && g.UnknownTarget == 0\n", "g.LastRun == nil && g.UnknownTarget == 0\n",
  "TestSC1f_")
m("SC-1 the nudge counts an uncovered write", V,
  "w.Path != nil && Relevant(w.Class) && covers(g.root, w) {\n", "w.Path != nil && Relevant(w.Class) {\n", "TestSC1h_")
m("SC-1 the watched row takes an uncovered write for the fix", O,
  "x.Path != nil && Relevant(x.Class) && covers(root, x) {\n", "x.Path != nil && Relevant(x.Class) {\n", "TestSC1i_")
m("SC-1 the run's tree holds an uncovered file", T,
  "w.Path != nil && Relevant(w.Class) && covers(root, &w) {\n", "w.Path != nil && Relevant(w.Class) {\n", "TestSC1j_")
