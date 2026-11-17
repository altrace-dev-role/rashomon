# UO-1: the Stop's unit rows (W-M3.3). UnitOf maps a path the session wrote
# to the unit the graph index files it in, by the index's own claim rule
# (internal/graph/index UnitOf), after verify places the path under the
# Stop's project (placed); the Stop and its replay both fill it
# (stopUnitOf). Each break is one part of that chain.
I = "internal/graph/index/index.go"
m("UO-1 UnitOf files a path under a directory Build skips", I,
  "\t\t\tif skipDir(d) {\n",
  "\t\t\tif false && skipDir(d) {\n",
  "TestUnitOf_FilesAPathAsBuildDoes")
m("UO-1 a file is claimed outside its language's units", I,
  "\treturn i, under(rel, c.ps[i].paths)\n",
  "\treturn i, true\n",
  "TestUnitOf_FilesAPathAsBuildDoes|TestBuild_UnitScopesItsLanguage")
L = "internal/verify/ledger_turn.go"
m("UO-1 a unit row reads the path as recorded, not under the root", L,
  "\t\tif rel, ok := placed(root, x); ok {\n",
  "\t\tif rel, ok := *x.Path, true; ok {\n",
  "TestLedger_UnitRowReadsThePathUnderTheStopsRoot")
m("UO-1 a project inside the root is not placed under it", L,
  "\tif sub, in := under(proj, root); in {\n",
  "\tif sub, in := under(proj, root); in && sub == \"\" {\n",
  "TestPlaced_PutsAWriteUnderTheStopsRoot|TestLedger_UnitRowReadsThePathUnderTheStopsRoot")
m("UO-1 a root inside the project keeps the root's directory on the path", L,
  "\t\treturn p[len(rel)+1:], true\n",
  "\t\treturn p + rel[:0], true\n",
  "TestPlaced_PutsAWriteUnderTheStopsRoot")
m("UO-1 a write whose project the record does not say is placed as recorded", L,
  "\tif project == nil || *project == \"\" || p == \"..\" || strings.HasPrefix(p, \"../\") {\n",
  "\tif project == nil {\n\t\treturn p, true\n\t}\n\tif *project == \"\" || p == \"..\" || strings.HasPrefix(p, \"../\") {\n",
  "TestPlaced_PutsAWriteUnderTheStopsRoot")
m("UO-1 the live Stop fills no UnitOf", "cmd/rashomon/recap_stop.go",
  "\tin.UnitOf = stopUnitOf(key)\n",
  "",
  "TestStopUnitOf_TheStopAndTheReplayNameOneUnit")
m("UO-1 the replay fills no UnitOf and drifts from the Stop", "cmd/rashomon/laststop.go",
  "\tin.UnitOf = stopUnitOf(root)\n",
  "",
  "TestStopUnitOf_TheStopAndTheReplayNameOneUnit")
U = "cmd/rashomon/stop_unitof.go"
m("UO-1 the index is found from the written file, below the root", U,
  "\t\t\tif at = graph.FindIndex(root); at != \"\" {\n",
  "\t\t\tif at = graph.FindIndex(filepath.Dir(filepath.Join(root, filepath.FromSlash(rel)))); at != \"\" {\n",
  "TestStopUnitOf_AnIndexBelowTheRootNamesNoUnit")
m("UO-1 a unit row's unit names no repository", U,
  "\t\treturn string(graph.GlobalID(rid, u))\n",
  "\t\treturn string(u)\n",
  "TestStopUnitOf_TheStopAndTheReplayNameOneUnit")
# ReadConfig is on the Stop's path (stopUnitOf), so it reads as
# internal/store's config does: through regfile.Open, so a named pipe at
# graph.json cannot hold the Stop (TestReadConfig_ANamedPipeReadsUnreadableAtOnce;
# index.go imports no os, so a break back to os.Open does not build, and
# regfile's SRR-1 rows judge the pipe), and at most MaxConfigBytes.
m("UO-1 ReadConfig's cap lets one byte over through", I,
  "len(b) > MaxConfigBytes {",
  "len(b) > MaxConfigBytes+1 {",
  "TestReadConfig_RefusesAConfigOverItsCap")
