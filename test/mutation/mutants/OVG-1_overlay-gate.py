# OVG-1: the overlay gate's cost (the hook's allocation budgets,
# test/perf/budgets.json). A post evaluates the gate once, in the overlay
# writer, and its unit-brief read takes the writer's answer; the hook's gate
# takes the path its declaration decoded; no input is decoded before the
# session-content marker is read; and the walk for an index stats each
# directory without building a FileInfo or a *PathError. Each break below is
# one of those undone. Not anchored: indexAt's EINTR retry, which no test
# can provoke on demand, and indexstat_other.go, which the sweep's host does
# not build.
L = "internal/hook/landededit.go"
O = "internal/hook/overlaywrite.go"
m("OVG-1 the unit-brief read evaluates its own gate", L,
  "\tg := p.gate\n",
  "\tvar g keptGate\n\t_, g.root, g.rel, g.ok = keptGoEdit(p.st, p.sessionID, p.toolName, p.toolInput, nil, p.cwd, p.landedSeq)\n",
  "TestHook_LandedGoEditTakesTheWritersGate")
m("OVG-1 the writer keeps no gate for the read", O,
  "*c.gate = keptGate{root: at, rel: under, ok: kept}",
  "*c.gate = keptGate{}",
  "TestHook_LandedGoEditTakesTheWritersGate")
m("OVG-1 the gate decodes the input whatever path it was passed", O,
  "\tif declared == nil {\n",
  "\tif true {\n",
  "TestHook_TheHookGateTakesTheDeclaredPath")
m("OVG-1 the input is decoded before the marker", O,
  "\tif !overlayTool(tool) || seq <= 0 || session == store.UnattributedSession || !store.SessionContentMarker(st.Root()) {\n",
  "\tif declared == nil {\n\t\tdeclared = shape.DeclaredPath(tool, input)\n\t}\n"
  "\tif !overlayTool(tool) || seq <= 0 || session == store.UnattributedSession || !store.SessionContentMarker(st.Root()) {\n",
  "TestHook_NoDecodeBeforeTheMarker")
m("OVG-1 the walk for an index probes with os.Stat", "internal/graph/persist.go",
  "\t\tif indexAt(filepath.Join(dir, filepath.FromSlash(GraphFile))) {\n",
  "\t\tif _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(GraphFile))); err == nil {\n",
  "TestFindIndex_AWalkWithNoIndexAllocatesTwoPerDirectory")
