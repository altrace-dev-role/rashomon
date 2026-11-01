# DG-2: a trusted or managed spec's required gate raises stale-evidence with
# no learned index (card D222). One break per rule: the declared gate alone
# raises where no family is established (a learned row, required only by the
# fail-closed prior, does not: D164), the spec's required gate is what counts
# (trust and Required), the note names the gate by its id, and the hook reads
# the spec for a project with no index file and takes the arm whatever it
# says.
V = "internal/verify/steer.go"
D = "internal/verify/declared.go"
H = "internal/hook/steerstep.go"
m("DG-2 a learned gate raises with no index", V,
  "\t\tif declaredOnly && g.Source == sourceLearned || !staleHere(g, writes, decl) {\n",
  "\t\tif !staleHere(g, writes, decl) {\n", "TestDG2b_")
m("DG-2 a declared required gate does not open the raise", V,
  "\t\tif g := &snap.Gates[i]; g.Required && g.Source != sourceLearned {\n",
  "\t\tif g := &snap.Gates[i]; false && g.Required && g.Source != sourceLearned {\n", "TestDG2_")
m("DG-2 the learned-only filter reads the index backwards", V,
  "staleEvidence(snap, c.runs, c.writes, decl, !c.learned, pt, event)",
  "staleEvidence(snap, c.runs, c.writes, decl, c.learned, pt, event)", "TestDG2_|TestDG2b_|TestSN1_")
m("DG-2 the note names the gate by its program", V,
  "ok && slot.Gate(id).Validate() == nil {", "ok && false && slot.Gate(id).Validate() == nil {", "TestDG2_")
m("DG-2 an untrusted spec opens the trigger", D,
  "(tr == spec.Trusted || tr == spec.Managed) && (d.Gate.Required == nil",
  "(tr == spec.Trusted || tr == spec.Managed || tr == spec.Untrusted) && (d.Gate.Required == nil", "TestDG2c_")
m("DG-2 an advisory declared gate opens the trigger", D,
  "&& (d.Gate.Required == nil || *d.Gate.Required) {\n\t\t\treturn true",
  "{\n\t\t\treturn true", "TestDG2c_")
m("DG-2 the hook never reads the spec of an unindexed project", H,
  "\t\treturn key, false, true\n", "\t\treturn \"\", false, false\n", "TestDG2d_")
m("DG-2 an unindexed write leaves the arm when no gate is declared", H,
  "\ttookIndexless := indexless && p.st.TakeStaleArm(p.sessionID)\n",
  "\ttookIndexless := indexless\n", "TestDG2e_|TestSN1g2_")
m("DG-2 an unindexed write takes the arm only after the run is read", H,
  "\ttookIndexless := indexless && p.st.TakeStaleArm(p.sessionID)\n\treadRun := stepped || tookIndexless && specFileAt(p.cwd, key, env.ProjectDir, env.ManagedDir)\n\tif p.landedStore {\n\t\tp.st.ClearNoSpec(p.sessionID)\n\t}\n\tif !readRun && (!ruled(p.event, p.landedSeq, p.toolName) || p.noSpecNoted(env)) {\n\t\treturn steer.Context{}, verify.SteerView{}, false\n\t}\n\tvar run *store.Run\n\tif readRun {\n\t\tif r, err := p.st.ReadRunConsistent(p.sessionID, SteerReadBudget); err == nil {\n\t\t\trun = r\n\t\t}\n\t}\n",
  "\treadRun := stepped || indexless && specFileAt(p.cwd, key, env.ProjectDir, env.ManagedDir)\n\tif p.landedStore {\n\t\tp.st.ClearNoSpec(p.sessionID)\n\t}\n\tif !readRun && (!ruled(p.event, p.landedSeq, p.toolName) || p.noSpecNoted(env)) {\n\t\treturn steer.Context{}, verify.SteerView{}, false\n\t}\n\tvar run *store.Run\n\tvar tookIndexless bool\n\tif readRun {\n\t\tif r, err := p.st.ReadRunConsistent(p.sessionID, SteerReadBudget); err == nil {\n\t\t\trun = r\n\t\t\ttookIndexless = indexless && p.st.TakeStaleArm(p.sessionID)\n\t\t}\n\t}\n", "TestDG2f_")
