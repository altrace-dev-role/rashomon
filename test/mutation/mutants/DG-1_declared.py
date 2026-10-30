# DG-1: a trusted spec's declared gates become snapshot gates (card
# DECLARED-GATES1, PRD PF-PG1, PG5). One break per promise the adapter keeps:
# an untrusted spec declares nothing, prefix matches the runner's words, Dir
# binds the location and an empty Dir the project root (D158), a declared
# gate absorbs the learned family whose runs it took and no other, Required
# is the spec's, the row says where it came from, an absent gate reads the
# writes it owes, the ledger ids a declared row by its gate, and the Stop view
# passes the key. Each row is killed by the card's own tests, or by the VER18
# shuffle tests that carry a declared gate.
D = "internal/verify/declared.go"
E = "internal/verify/evaluate.go"
O = "internal/verify/obligations.go"
S = "internal/verify/stop.go"

m("DG-1 an untrusted spec declares a gate", D, "\t\tif tr != spec.Trusted && tr != spec.Managed {\n",
  "\t\tif tr != spec.Trusted && tr != spec.Managed && tr != spec.Untrusted {\n", "TestDG1b_")
m("DG-1 prefix is read as exact", D, "\t\t\tif d.Gate.Match != \"exact\" && s.RunnerDigest != nil {\n",
  "\t\t\tif false && s.RunnerDigest != nil {\n", "TestDG1c_")
m("DG-1 exact is read as prefix", D, "\t\t\tif d.Gate.Match != \"exact\" && s.RunnerDigest != nil {\n",
  "\t\t\tif s.RunnerDigest != nil {\n", "TestDG1c_")
m("DG-1 Dir is ignored", D, "\t\t\tg.location = shape.CWDDigest(key, filepath.Join(root, d.Gate.Dir))\n",
  "\t\t\tg.location = shape.CWDDigest(key, filepath.Clean(root))\n", "TestDG1d_")
m("DG-1 an empty Dir reads any location", D, "\tif g.location == \"\" || r.Location != g.location {\n",
  "\tif g.location == \"\" {\n", "TestDG1d2_|TestVER18c_")
m("DG-1 absorption is dropped", D,
  "\tgates = slices.DeleteFunc(gates, func(g GateState) bool { return g.Source == sourceLearned && !open[g.Family] })\n",
  "\tgates = slices.DeleteFunc(gates, func(g GateState) bool { return false })\n", "TestDG1_|TestVER18_")
m("DG-1 a family with an unmatched run is absorbed", D, "\t\tif k, ok := r.family(); ok && !matched[r.Seq] {\n",
  "\t\tif k, ok := r.family(); ok && false {\n", "TestDG1c_|TestVER18_")
m("DG-1 Required is forced true", D, "required: d.Gate.Required == nil || *d.Gate.Required,",
  "required: true,", "TestDG1e_")
m("DG-1 a ci_commands row reads declared", D, "\t\t\tg.source = sourceCICommands\n",
  "\t\t\tg.source = sourceDeclared\n", "TestDG1g_")
m("DG-1 an absent gate owes no write", D, "\t\tg.Reason = NeverRunAfterWrite\n",
  "\t\tg.Reason = EmptyRun\n", "TestDG1e_")
m("DG-1 an empty key declares", D, "\tif sp == nil || len(key) == 0 {\n",
  "\tif sp == nil {\n", "TestDG1h_")
m("DG-1 evaluate joins no declared row", E, "\tif dgs := declaredGates(sp, key, snap.ProjectRoot); len(dgs) > 0 {\n",
  "\tif dgs := declaredGates(sp, key, snap.ProjectRoot); false && len(dgs) > 0 {\n", "TestDG1f_|TestDG1_")
m("DG-1 the Stop view drops the key", S, "\tsnap := evaluate(run, idx, sp, in.Key, pt, now, &c)\n",
  "\tsnap := evaluate(run, idx, sp, nil, pt, now, &c)\n", "TestDG1f_")
m("DG-1 two declared rows share their ids", O, "\tif g.Family == familyDeclared {\n\t\treturn g.Gate\n",
  "\tif false {\n\t\treturn g.Gate\n", "TestDG1_")
# D209: the line digest moved from gateRun to lineRun; an exact gate reads it.
m("DG-1 an exact line is not matched", D, "r.digest != \"\" && g.lines[r.digest]",
  "false && g.lines[r.digest]", "TestDG1c_")

# D211: the clearing that makes the digest's execution arm unobservable.
m("DG-1 a rewritten line keeps its location", D,
  "\t\t\tg.Location = \"\"\n\t\t}\n\t\tout = append(out, lineRun{",
  "\t\t\t_ = 0\n\t\t}\n\t\tout = append(out, lineRun{", "TestDG1c2_")
