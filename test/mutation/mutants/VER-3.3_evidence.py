# VER-3.3: expected evidence by task kind (internal/verify/evidence.go) and the
# done flag's clause (internal/recap/done.go), D117 and Gate B 1. One break
# per rule: supply read from the gates' supplies, the gate_after_write reading,
# the kinds' order, the two text kinds, the Stop readings (D-A2), INV-2 on an
# incomplete record, the rows reaching the Stop view and the ledger, and the
# clause's gate, claim and list. The kill row is the named test's.
E = "internal/verify/evidence.go"
C = "internal/verify/claims.go"
S = "internal/verify/stop.go"
O = "internal/verify/obligations.go"
F = "internal/recap/done.go"

# Run kinds.
m("VER-3.3 supplies ignored", E,
  '\t\tif !slices.Contains(d.Gate.Supplies, kind) {\n\t\t\tcontinue\n\t\t}\n\t\tstate := gateAfterWrite(snap, d.ID)\n',
  '\t\tstate := gateAfterWrite(snap, d.ID)\n', "TestEV1_")
m("VER-3.3 a green before the last write is met", E,
  '\t\t\tstate = snap.Gates[i].State\n', '\t\t\tstate = snap.Gates[i].State\n\t\t\tif state == Stale {\n\t\t\t\tstate = Green\n\t\t\t}\n', "TestEV1_")
m("VER-3.3 no supplier at Stop stays open", E,
  '\tcase !found && in.atStop:\n', '\tcase !found && in.atStop && false:\n', "TestEV1_")
m("VER-3.3 an open supplier at Stop stays open", E,
  'first.State == ObligationOpen && in.atStop:\n', 'first.State == ObligationOpen && in.atStop && false:\n', "TestEV1_")
m("VER-3.3 the first supplier's state wins over a later met one", E,
  '\t\tif state == ObligationMet {\n\t\t\treturn Expected{State: ObligationMet, Gate: d.ID}\n\t\t}\n', '', "TestEV1_")

# Order and INV-2.
m("VER-3.3 rows in the task file's order", E,
  '\tfor _, kind := range spec.EvidenceKinds {\n\t\tif !slices.Contains(in.sp.Task.ExpectedEvidence, kind) {\n',
  '\tfor _, kind := range in.sp.Task.ExpectedEvidence {\n\t\tif !slices.Contains(spec.EvidenceKinds, kind) {\n', "TestEV1b_")
m("VER-3.3 an incomplete record keeps its states", E,
  '\t\tif snap.Recording != Complete {\n', '\t\tif snap.Recording != Complete && false {\n', "TestEV1c_")
m("VER-3.3 an unknown row is named unmet", E,
  'o.Kind == ExpectedEvidence && o.State == ObligationUnmet && o.ID',
  'o.Kind == ExpectedEvidence && (o.State == ObligationUnmet || o.State == ObligationUnknown) && o.ID', "TestEV1c_")

# The text kinds.
m("VER-3.3 an empty mechanism is met", E,
  '\tcase in.sp.Task.Mechanism == "":\n', '\tcase in.sp.Task.Mechanism == "" && false:\n', "TestEV2_")
m("VER-3.3 a mechanism on a targeted task file is accepted", E,
  '\tcase taskTargeted(snap):\n', '\tcase taskTargeted(snap) && false:\n', "TestEV3_")
m("VER-3.3 any store write reads as the task file's", E,
  '\t\tif e.Check == StoreTouched && spec.IsTaskFile(e.Path) {\n', '\t\tif e.Check == StoreTouched {\n', "TestEV3_")
m("VER-3.3 disclosed() replaced by a word match", C,
  '\t\tif s.disclosed() {\n', '\t\tif strings.Contains(t, "error") {\n', "TestEV4_")
m("VER-3.3 judgment read as met before Stop", E,
  '\tcase !in.atStop:\n\t\treturn Expected{State: ObligationOpen}\n\tcase in.unread:\n',
  '\tcase in.unread:\n', "TestEV4_")

# The rows reach the Stop view and the ledger.
m("VER-3.3 EvaluateStop leaves Expected unset", S,
  '\tv.Expected = expected(snap, evidenceIn{sp: sp, atStop: true, unread: in.MessageUnread, disclosed: v.Scan.Disclosed, wrote: wrote})\n', '', "TestEV5b_")
m("VER-3.3 the ledger drops the expected rows", O,
  'append(obligations(snap, c.runs, c.writes), expectedObligations(scope, snap, ex)...)',
  'append(obligations(snap, c.runs, c.writes), expectedObligations(scope, snap, ex)[:0]...)', "TestEV5b_")

# The done flag (Gate B 1).
m("VER-3.3 Gate B: the gate ignores unmet kinds", F,
  '\tif (v.Summary.Row == verify.RowVerified || !unbacked(v.Scan.Claims)) && len(unmet) == 0 {\n',
  '\tif (v.Summary.Row == verify.RowVerified || !unbacked(v.Scan.Claims)) && len(unmet) >= 0 {\n', "TestEV6_")
m("VER-3.3 Gate B: the gate fires on an unmet kind with no done claim", F,
  '\t\tif c.Strict && c.Class == verify.ClaimDone {\n', '\t\tif c.Strict || c.Class == verify.ClaimDone {\n', "TestEV6_")
m("VER-3.3 Gate B: the clause lists one kind when two are unmet", F,
  '\tfor _, e := range unmet {\n', '\tfor _, e := range unmet[:min(1, len(unmet))] {\n', "TestEV6_")
m("VER-3.3 the clause dropped", F,
  ' + "." + expectedClause(unmet) + "\\n" + ', ' + "." + "\\n" + ', "TestEV6_")

# 3.3b: a met run row cites its supplying gate's green run (D143).
m("VER-3.3 a met run row cites nothing", E, "Evidence: expectedCites(snap, e)})", "Evidence: []int64{}})",
  "TestEV8_")
