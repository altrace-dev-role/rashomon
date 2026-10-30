# VER-1.9: the obligation ledger (internal/verify/obligations.go). One break
# per row of the mapping and per Gate A and Gate B ruling: which gates owe,
# each kind's table, the watched kind's red, green and write, the forced
# unknown, the id, the evidence lists, the order, the scope and the source.
# The kill row is the named test's.
O = "internal/verify/obligations.go"

# Which gates owe.
m("VER-1.9 an advisory gate owes rows", O, '\t\tif !g.Required {\n\t\t\tcontinue\n\t\t}\n\t\tsource, known', '\t\tsource, known', "TestPG5a_")
m("VER-1.9 an Absent gate with no write owes rows", O,
  '\t\towed := g.State != Absent || g.LastWrite != nil\n', '\t\towed := true\n', "TestPG5b_")
m("VER-1.9 evidence_bound on an Absent gate with no write", O,
  '\t\tif owed {\n\t\t\tl.fromTable(g, source, EvidenceBound, evidenceBound(g))\n',
  '\t\tif true {\n\t\t\tl.fromTable(g, source, EvidenceBound, evidenceBound(g))\n', "TestPG5b_")

# gate_after_write.
m("VER-1.9 Red reads met", O, '\t{Red, ObligationUnmet, citeRun},\n', '\t{Red, ObligationMet, citeRun},\n', "TestPG5b_")
m("VER-1.9 Stale reads open", O,
  '\t{Stale, ObligationUnmet, citeRun | citeWrite},\n', '\t{Stale, ObligationOpen, citeRun | citeWrite},\n', "TestPG5b_")
m("VER-1.9 Backgrounded reads met", O,
  '\t{Backgrounded, ObligationOpen, citeRun},\n', '\t{Backgrounded, ObligationMet, citeRun},\n', "TestPG5b_")
m("VER-1.9 Masked reads met", O,
  '\t{MaskedInconclusive, ObligationUnknown, citeRun},\n', '\t{MaskedInconclusive, ObligationMet, citeRun},\n', "TestPG5b_")
m("VER-1.9 a nil cited pointer reads its row's state", O,
  '\t\tif c.seq == nil {\n\t\t\tl.add(g, source, kind, ObligationUnknown)\n\t\t\treturn\n\t\t}\n',
  '\t\tif c.seq == nil {\n\t\t\tcontinue\n\t\t}\n', "TestPG5a_")

# exit_unmasked (Gate B F2).
m("VER-1.9 exit_unmasked met on Masked", O,
  '\t{MaskedInconclusive, ObligationUnmet, citeRun},\n', '\t{MaskedInconclusive, ObligationMet, citeRun},\n', "TestPG3a_")
m("VER-1.9 exit_unmasked met on Backgrounded", O,
  '\t{Backgrounded, ObligationUnknown, citeRun},\n', '\t{Backgrounded, ObligationMet, citeRun},\n', "TestPG3a_")
m("VER-1.9 exit_unmasked met on Unknown", O,
  '\t{Backgrounded, ObligationUnknown, citeRun},\n\t{Unknown, ObligationUnknown, citeRun},\n',
  '\t{Backgrounded, ObligationUnknown, citeRun},\n\t{Unknown, ObligationMet, citeRun},\n', "TestPG3a_")

# The forced unknown.
m("VER-1.9 an incomplete record keeps a met row", O,
  '\t\tif snap.Recording != Complete || Rank(g.State) < 0 || !known {\n', '\t\tif Rank(g.State) < 0 || !known {\n', "TestINV2_")
m("VER-1.9 a state outside States keeps a met row", O,
  '\t\tif snap.Recording != Complete || Rank(g.State) < 0 || !known {\n', '\t\tif snap.Recording != Complete || !known {\n', "TestPG5b_")
m("VER-1.9 a source outside the three keeps a met row", O,
  '\t\tif snap.Recording != Complete || Rank(g.State) < 0 || !known {\n', '\t\tif snap.Recording != Complete || Rank(g.State) < 0 || !known && known {\n', "TestPG5e_")

# watched_red_then_green.
m("VER-1.9 watched met without the write between", O,
  '\treturn ObligationUnmet, [3]int64{w.red.Seq, w.green.Seq}, 2\n', '\treturn ObligationMet, [3]int64{w.red.Seq, w.green.Seq}, 2\n', "TestPG7b_")
m("VER-1.9 watched met by a docs write", O,
  '\t\tif x := &writes[i]; x.Path != nil && Relevant(x.Class) {\n', '\t\tif x := &writes[i]; x.Path != nil {\n', "TestPG7b_")
m("VER-1.9 watched met by an in-flight write", O,
  '\tfor i := hi - 1; i >= 0 && writes[i].Seq > lo; i-- {\n', '\tfor i := hi - 1; i >= 0 && writes[i].Seq > min(lo, w.red.Seq); i-- {\n', "TestPG7b_")
m("VER-1.9 watched row from another family's red", O,
  '\t\tif w, ok := reds[g.Family]; ok {\n', '\t\tfor _, w := range reds {\n', "TestPG7b_")
m("VER-1.9 the first red used instead of the last", O,
  '\t\t\tbyKey[k] = watch{red: r}\n', '\t\t\tif w.red == nil {\n\t\t\t\tbyKey[k] = watch{red: r}\n\t\t\t}\n', "TestPG7b_")
m("VER-1.9 a green before the last red counted", O,
  '\t\t\tbyKey[k] = watch{red: r}\n', '\t\t\tbyKey[k] = watch{red: r, green: w.green}\n', "TestPG7b_")
m("VER-1.9 a masked failure counts as the red", O,
  '\t\tcase !r.Backgrounded && !masked(r) && !emptyRun(r) && failed(r):\n', '\t\tcase !r.Backgrounded && !emptyRun(r) && failed(r):\n', "TestPG7b_")
m("VER-1.9 a green not yet landed counts", O,
  '!ok || !known || s > at {\n', '!ok || !known || s > at+1 {\n', "TestPG7b_")

# evidence_bound and the kinds.
m("VER-1.9 evidence_bound reads met", O,
  '\t\t\tl.fromTable(g, source, EvidenceBound, evidenceBound(g))\n', '\t\t\tl.add(g, source, EvidenceBound, ObligationMet)\n', "TestPG5a_")
m("VER-1.9 a Phase 3 kind emitted", O,
  '\t\t\tl.fromTable(g, source, EvidenceBound, evidenceBound(g))\n', '\t\t\tl.fromTable(g, source, SpecRule, evidenceBound(g))\n', "TestPG5e_")

# The id.
m("VER-1.9 the id over the gate's label", O,
  '\treturn g.Family\n}\n', '\treturn g.Gate\n}\n', "TestPG5d_")
m("VER-1.9 the id without the kind", O,
  "\tb := append(append(append(append(append(a[:0], scope...), '\\n'), kind...), '\\n'), family...)\n",
  "\tb := append(append(append(append(a[:0], scope...), '\\n'), '\\n'), family...)\n", "TestPG5d_")

# Evidence, order, scope, source.
m("VER-1.9 an empty Evidence is nil", O,
  '\t\tEvidence: l.evidence[from:len(l.evidence):len(l.evidence)],\n', '\t\tEvidence: append([]int64(nil), l.evidence[from:]...),\n', "TestPG5a_")
m("VER-1.9 rows out of gates order", O,
  '\tfor i := range snap.Gates {\n\t\tg := &snap.Gates[i]\n', '\tfor i := range snap.Gates {\n\t\tg := &snap.Gates[len(snap.Gates)-1-i]\n', "TestPG5e_")
m("VER-1.9 a commit point owes in the commit scope", O,
  '\tPointCommit: ScopeSession, PointPush', '\tPointCommit: ScopeCommit, PointPush', "TestPG5e_")
m("VER-1.9 a ci_commands gate is not spec", O, '\tcase "declared", "ci_commands":\n', '\tcase "declared":\n', "TestPG5e_")
