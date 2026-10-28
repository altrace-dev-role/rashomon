# VER-1.10: the checkpoint cut (internal/verify/checkpoint.go). One break per
# row of the cut order and per Gate A and Gate B ruling: the order of the
# categories (D45 included), the order within each (Q3), the omitted counts,
# the budget and its seq reserve (Q1), the flag decided unflagged, the record
# json cannot encode, the header and the projection (Q4), and the copies that
# keep the record apart from the snapshot (Q2, R5). The kill row is the named
# test's.
C = "internal/verify/checkpoint.go"

# The order of the categories.
m("VER-1.10 practice cut after the green gates", C,
  '\t\t{cutPractice, every, inPlace},\n\t\t{cutGates, func(i int) bool { return g[i].State == Green }, older},\n',
  '\t\t{cutGates, func(i int) bool { return g[i].State == Green }, older},\n\t\t{cutPractice, every, inPlace},\n', "TestVER51b_")
m("VER-1.10 green gates cut after the loose claims", C,
  '\t\t{cutGates, func(i int) bool { return g[i].State == Green }, older},\n\t\t{cutClaims, supported, fromEnd},\n',
  '\t\t{cutClaims, supported, fromEnd},\n\t\t{cutGates, func(i int) bool { return g[i].State == Green }, older},\n', "TestVER51b_")
m("VER-1.10 background cut after integrity", C,
  '\t\t{cutBackground, every, inPlace},\n\t\t{cutIntegrity, every, inPlace},\n',
  '\t\t{cutIntegrity, every, inPlace},\n\t\t{cutBackground, every, inPlace},\n', "TestVER51g_")
m("VER-1.10 integrity cut after spec", C,
  '\t\t{cutIntegrity, every, inPlace},\n\t\t{cutSpec, every, inPlace},\n',
  '\t\t{cutSpec, every, inPlace},\n\t\t{cutIntegrity, every, inPlace},\n', "TestVER51g_")
GATES7 = ('\t\t{cutGates, func(i int) bool { return g[i].State != Green }, func(a, b int) int {\n'
          '\t\t\treturn cmp.Or(cmp.Compare(Rank(g[b].State), Rank(g[a].State)), older(a, b))\n'
          '\t\t}},\n')
CLAIMS8 = ('\t\t{cutClaims, func(i int) bool { return !supported(i) }, func(a, b int) int {\n'
           '\t\t\treturn cmp.Or(cmp.Compare(Rank(State(cl[b].Verdict)), Rank(State(cl[a].Verdict))), fromEnd(a, b))\n'
           '\t\t}},\n')
m("VER-1.10 D45 reversed: strict claims cut before non-green gates", C, GATES7 + CLAIMS8, CLAIMS8 + GATES7, "TestVER51c_")

# Which entries a category takes.
m("VER-1.10 a non-green gate cut by age with the green", C,
  'return g[i].State == Green }, older},\n', 'return g[i].State == Green || true }, older},\n', "TestVER51c_")
m("VER-1.10 a strict red claim cut with the loose ones", C,
  '|| cl[i].Verdict == NoGateKnown\n', '|| cl[i].Verdict == NoGateKnown || true\n', "TestVER51c_")
m("VER-1.10 a supported claim kept as strict non-green", C,
  'return !cl[i].Strict || cl[i].Verdict == Verdict(Green) || cl[i].Verdict == NoGateKnown\n',
  'return !cl[i].Strict || cl[i].Verdict == NoGateKnown\n', "TestVER51c_")
m("VER-1.10 a no_gate_known claim kept as strict non-green", C,
  'return !cl[i].Strict || cl[i].Verdict == Verdict(Green) || cl[i].Verdict == NoGateKnown\n',
  'return !cl[i].Strict || cl[i].Verdict == Verdict(Green)\n', "TestVER51c_")

# The order within a category.
m("VER-1.10 the newest green gate cut first", C,
  '\t\t{cutGates, func(i int) bool { return g[i].State == Green }, older},\n',
  '\t\t{cutGates, func(i int) bool { return g[i].State == Green }, func(a, b int) int { return older(b, a) }},\n',
  "TestVER51b_")
m("VER-1.10 a Red gate cut before a less severe one", C,
  'cmp.Compare(Rank(g[b].State), Rank(g[a].State))', 'cmp.Compare(Rank(g[a].State), Rank(g[b].State))', "TestVER51c_")
m("VER-1.10 a defect state cut with the Red", C,
  'cmp.Compare(Rank(g[b].State), Rank(g[a].State))', 'cmp.Compare(max(Rank(g[b].State), 0), max(Rank(g[a].State), 0))',
  "TestVER51c_")
m("VER-1.10 a gate never run is the newest", C,
  '\tif g.LastRunSeq == nil {\n\t\treturn math.MinInt64\n', '\tif g.LastRunSeq == nil {\n\t\treturn math.MaxInt64\n', "TestVER51c_")
m("VER-1.10 ties in a state fall to the place, not the run", C,
  'cmp.Or(cmp.Compare(runKey(g[a]), runKey(g[b])), cmp.Compare(a, b))', 'cmp.Or(cmp.Compare(runKey(g[a]), runKey(g[a])), cmp.Compare(a, b))',
  "TestVER51c_")
m("VER-1.10 the red claim cut first among the strict", C,
  'cmp.Compare(Rank(State(cl[b].Verdict)), Rank(State(cl[a].Verdict)))',
  'cmp.Compare(Rank(State(cl[a].Verdict)), Rank(State(cl[b].Verdict)))', "TestVER51c_")
m("VER-1.10 integrity cut newest first", C,
  '\t\t{cutIntegrity, every, inPlace},\n', '\t\t{cutIntegrity, every, fromEnd},\n', "TestVER51g_")
m("VER-1.10 integrity kept in the snapshot's order", C,
  '\tslices.SortStableFunc(p.unresolved, func(a, b IntegrityEntry) int { return cmp.Compare(a.Seq, b.Seq) })\n', '',
  "TestVER51g_")
m("VER-1.10 a spec entry ordered by its largest seq", C, '\treturn slices.Min(v.Seqs)\n', '\treturn slices.Max(v.Seqs)\n',
  "TestVER51g_")
m("VER-1.10 a spec entry with no seqs the newest", C,
  '\tif len(v.Seqs) == 0 {\n\t\treturn math.MinInt64\n', '\tif len(v.Seqs) == 0 {\n\t\treturn math.MaxInt64\n', "TestVER51g_")

# The omitted counts.
m("VER-1.10 practice_omitted counted as one", C,
  '\t\tr.Practice, r.PracticeOmitted = nil, practiceComponents\n', '\t\tr.Practice, r.PracticeOmitted = nil, 1\n', "TestVER51b_")
m("VER-1.10 gates_omitted not counted", C, '\tr.Gates, r.GatesOmitted = uncut(', '\tr.Gates, _ = uncut(', "TestVER51b_")
m("VER-1.10 claims_omitted not counted", C, '\tr.Claims, r.ClaimsOmitted = uncut(', '\tr.Claims, _ = uncut(', "TestVER51c_")
m("VER-1.10 background_omitted not counted", C,
  '\tr.BackgroundInFlight, r.BackgroundOmitted = uncut(', '\tr.BackgroundInFlight, _ = uncut(', "TestVER51g_")
m("VER-1.10 integrity_omitted not counted", C,
  '\tr.Integrity.Unresolved, r.IntegrityOmitted = uncut(', '\tr.Integrity.Unresolved, _ = uncut(', "TestVER51g_")
m("VER-1.10 spec_verdicts_omitted not counted", C,
  '\tr.SpecVerdicts, r.SpecVerdictsOmitted = uncut(', '\tr.SpecVerdicts, _ = uncut(', "TestVER51g_")

# The budget and the flag.
m("VER-1.10 Truncated never set", C, '\trec.Truncated = !fits(n)\n', '\trec.Truncated = !fits(n) && false\n', "TestVER51b_")
m("VER-1.10 Truncated only when something is cut", C,
  '\t\tn = size(&rec)\n\t}\n\treturn rec\n', '\t\tn = size(&rec)\n\t}\n\trec.Truncated = rec.Truncated && len(plan) > 0\n\treturn rec\n',
  "TestVER51b_")
m("VER-1.10 the cuts measured unflagged", C,
  '\tif rec.Truncated {\n\t\tn = size(&rec)\n', '\tif rec.Truncated && false {\n\t\tn = size(&rec)\n', "TestVER51b_")
m("VER-1.10 a record at the budget is over", C,
  '{ return n <= CheckpointMaxBytes-seqReserve }', '{ return n < CheckpointMaxBytes-seqReserve }', "TestVER51b_")
m("VER-1.10 the seq reserve dropped", C,
  '{ return n <= CheckpointMaxBytes-seqReserve }', '{ return n <= CheckpointMaxBytes }', "TestVER51b_")
m("VER-1.10 a record json cannot encode reads as fitting", C, '\t\treturn math.MaxInt\n', '\t\treturn 0\n', "TestVER51b_")
m("VER-1.10 the size read once, before the cut", C, '\t\tc.fill()\n\t\tn = size(&rec)\n\t}\n', '\t\tc.fill()\n\t}\n', "TestVER51b_")

# The header and the projection.
m("VER-1.10 the header carries the point's seq", C,
  'SchemaVersion: store.EventSchemaVersion,\n', 'SchemaVersion: store.EventSchemaVersion, Seq: copyOf(&pt.Seq),\n', "TestVER51d_")
m("VER-1.10 background from the Green gates", C, '\t\tif g.State == Backgrounded {\n', '\t\tif g.State == Green {\n', "TestVER51d_")
m("VER-1.10 an advisory gate's background dropped", C,
  '\t\tif g.State == Backgrounded {\n', '\t\tif g.State == Backgrounded && g.Required {\n', "TestVER51d_")
m("VER-1.10 an empty list null", C, '\tif dst == nil {\n\t\tdst = make([]T, 0, len(all))\n\t}\n', '', "TestVER51e_")

# The record shares no memory with the snapshot.
m("VER-1.10 a claim's gates shared with the snapshot", C,
  '\t\tc.Gates = slots[from:len(slots):len(slots)]\n', '\t\t_ = slots[from:len(slots):len(slots)]\n', "TestVER51f_")
m("VER-1.10 the integrity list sorted in place", C,
  'unresolved: append(make([]IntegrityEntry, 0, len(snap.Integrity.Unresolved)), snap.Integrity.Unresolved...)}',
  'unresolved: snap.Integrity.Unresolved}', "TestVER51f_")
m("VER-1.10 a gate's seq points into the snapshot", C, '\t*dst = r.Seq\n\treturn dst\n', '\t*dst = r.Seq\n\treturn &r.Seq\n',
  "TestVER51f_")
m("VER-1.10 the practice shared with the snapshot", C, '\t\tp.practice = &c\n', '\t\tp.practice, _ = pr, c\n', "TestVER51f_")
