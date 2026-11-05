# F2-1: a gate answers for a turn only where a call of the turn ran in its
# project (card F2): the turn's projects are its calls' (turnProjects), a gate
# is the turn's where one of them is its root, inside it or holds it
# (touches), and the row, the done verdict and the turn's record-green point
# pass over the rest (markTurn, offTurn). Fail closed where the record cannot
# place a call or a gate, and a turn in no project reads every gate. Each
# break removes one rule and names the tests that hold it.
C = "internal/verify/cover.go"
E = "internal/verify/evaluate.go"
L = "internal/verify/claims.go"
S = "internal/verify/stop.go"
m("F2-1 every gate is the turn's", S,
  "\tif snap.Recording == Complete {\n\t\tmarkTurn(", "\tif false {\n\t\tmarkTurn(", "TestF2")
m("F2-1 a turn in no project reads none of the session's gates", C,
  "\t\t\tps = append(ps, *p)\n\t\t}\n\t}\n\treturn ps, true\n",
  "\t\t\tps = append(ps, *p)\n\t\t}\n\t}\n\tif ps == nil {\n\t\tps = []string{}\n\t}\n\treturn ps, true\n", "TestF2a_")
m("F2-1 a call the record cannot place is in no project", C,
  "\t\tcase p == nil:\n\t\t\treturn nil, false\n", "\t\tcase p == nil:\n", "TestF2a_")
m("F2-1 a gate with no root is no turn's", C,
  "\tif projects == nil || root == \"\" {\n", "\tif projects == nil {\n", "TestF2a_")
m("F2-1 a call in the gate's own project is not in it", C,
  "\t\tif _, in := under(p, root); in || p == root {\n", "\t\tif _, in := under(p, root); in {\n", "TestF2")
m("F2-1 a call in a repository inside the gate's is not in it", C,
  "\t\tif _, in := under(p, root); in || p == root {\n", "\t\tif p == root {\n", "TestF2a_")
m("F2-1 a call in the directory holding the gate's is not in it", C,
  "\t\tif _, in := under(root, p); in {\n", "\t\tif _, in := under(root, p); false && in {\n", "TestF2a_")
m("F2-1 the done verdict reads every gate", L,
  "\t\tif !g.Required || g.offTurn || r.class != ClaimDone", "\t\tif !g.Required || r.class != ClaimDone", "TestF2a_")
m("F2-1 the row reads every gate", E,
  "\treturn !g.offTurn && (g.Required || advisory && g.Source == sourceLearned)\n",
  "\treturn g.Required || advisory && g.Source == sourceLearned\n", "TestF2b_|TestF2c_")
m("F2-1 the turn's record-green point is the session's", E,
  "\t\tgreen, _ = recordGreen(snap.Gates, k)\n", "", "TestF2c_")
m("F2-1 record green waits on a gate not the turn's", E,
  "\t\tif !g.Required || g.offTurn {\n", "\t\tif !g.Required {\n", "TestF2c_")
