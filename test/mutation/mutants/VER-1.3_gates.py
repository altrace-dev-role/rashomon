# VER-1.3: the state of each gate family at a point (internal/verify/gates.go).
# One break per table row, per row-order pair, per predicate term, per count,
# per sort and per cut in time; and classify's two inputs to it, the family key
# and ExecSeq. The kill row is the named test's. No fuzz target: the input is
# the builder's records, not untrusted bytes.
G = "internal/verify/gates.go"
C = "internal/verify/classify.go"

# Each row dropped from the table.
m("VER-1.3 the backgrounded row is dropped", G,
  '\t{lastBackgrounded, Backgrounded, func(*fam) Reason { return BackgroundLaunch }},\n', '', "TestVER14_")
m("VER-1.3 the masked row is dropped", G, '\t{lastMasked, MaskedInconclusive, maskedReason},\n', '', "TestVER13_")
m("VER-1.3 the red row is dropped", G, '\t{lastFailed, Red, redReason},\n', '', "TestVER12_")
m("VER-1.3 the unknown row is dropped", G,
  '\t{lastOutcomeNotKnown, Unknown, func(*fam) Reason { return OutcomeUnknown }},\n', '', "TestVER15b_")
m("VER-1.3 the stale row is dropped", G,
  '\t{writtenSince, Stale, func(*fam) Reason { return WritesAfterGreen }},\n', '', "TestVER11_")
m("VER-1.3 the green row is dropped", G,
  '\t{func(*fam) bool { return true }, Green, func(*fam) Reason { return OK }},\n', '', "TestVER10_")

# Row order and each row's terms.
m("VER-1.3 masked ranks below failed", G,
  '\t{lastMasked, MaskedInconclusive, maskedReason},\n\t{lastEmptyRun, Absent, func(*fam) Reason { return EmptyRun }},\n\t{lastFailed, Red, redReason},\n',
  '\t{lastFailed, Red, redReason},\n\t{lastEmptyRun, Absent, func(*fam) Reason { return EmptyRun }},\n\t{lastMasked, MaskedInconclusive, maskedReason},\n', "TestVER16_")
m("VER-1.3 a hidden build status is not masked", G,
  ' || *r.Masked == shape.MaskedBuild)', ')', "TestVER13_")
m("VER-1.3 an interrupt with a code is not red", G,
  ' ||\n\t\tr.Outcome == store.ExecInterrupted && r.ExitCode != nil && *r.ExitCode != 0\n', '\n', "TestVER12b_")
m("VER-1.3 a fail line is not red", G, ' || r.Output.Kind == outFailLine)', ')', "TestVER12b_")
m("VER-1.3 an outcome not ok is known", G,
  'return r.Outcome != store.ExecOK || r.Output != nil && r.Output.Kind == outUnread',
  'return r.Output != nil && r.Output.Kind == outUnread', "TestVER15d_")

# Reasons.
m("VER-1.3 a red with writes after it ignores them", G,
  '\tfor _, w := range f.counted {\n\t\tif w.Seq > f.last.Seq {\n\t\t\treturn RedThenWrites\n\t\t}\n\t}\n', '', "TestVER12b_")
m("VER-1.3 a write before the red is a write after it", G,
  '\t\tif w.Seq > f.last.Seq {\n', '\t\tif w.Seq > 0 {\n', "TestVER12b_")
m("VER-1.3 a red is never by output alone", G,
  '\tif !failedByExit(f.last) {\n\t\treturn FailedOutput\n\t}\n', '', "TestVER12_")
m("VER-1.3 a masked success line is not read", G,
  '\tcase outSummaryPass:\n\t\treturn MaskedWithSuccessLine\n', '', "TestVER13b_")
m("VER-1.3 a masked no_summary is unattributable", G,
  '\tcase outNoSummary, outEmptyRun:\n', '\tcase outEmptyRun:\n', "TestVER13b_")

# The reference run.
m("VER-1.3 the reference run is always the last", G,
  'f.ref = cmp.Or(f.green, f.last)', 'f.ref = f.last', "TestVER23_")
m("VER-1.3 the reference green is the first green", G,
  '\t\t\tf.green = &fr[i]\n\t\t\tbreak\n', '\t\t\tf.green = &fr[i]\n', "TestVER15_")
m("VER-1.3 an unread run can be the reference green", G,
  '!failed(r) && !outcomeNotKnown(r)', '!failed(r)', "TestVER15c_")

# What counts as a write against the family.
m("VER-1.3 relevance is ignored", G, 'w.Path != nil && Relevant(w.Class) && ', 'w.Path != nil && ', "TestVER11c_")
m("VER-1.3 an unnamed write counts as a named one", G, 'w.Path != nil && Relevant(w.Class) && ', 'Relevant(w.Class) && ', "TestVER25_")
m("VER-1.3 unnamed writes are not counted", G, '\t\t\tg.UnknownTarget++\n', '', "TestVER17_")
m("VER-1.3 the writers cap is off by one", G, '\t\tif i == MaxWriters {\n', '\t\tif i == MaxWriters+1 {\n', "TestVER17_")
m("VER-1.3 writers are left in read order", G,
  '\tslices.SortFunc(as, func(a, b agent) int { return cmp.Compare(a.first, b.first) })\n', '', "TestVER18_")
m("VER-1.3 the writer list is nil with no writers", G,
  'list = make([]AgentCount, 0, min(len(as), MaxWriters))', 'list = nil', "TestVER10_")
m("VER-1.3 the last write ignores the path among one call's paths", G,
  ' || w.Seq == last.Seq && *w.Path > *last.Path', '', "TestVER18_")

# Time: the point, a run's execution, a write's declaration.
m("VER-1.3 the point does not cut the future", G,
  '\t\tfr = slices.DeleteFunc(fr, func(r gateRun) bool { s, _ := landed(r); return s > at })\n', '', "TestVER15_")
m("VER-1.3 a run exists at its declaration", G,
  '\tif r.ExecSeq != nil {\n\t\treturn *r.ExecSeq, true\n\t}\n\treturn r.Seq, false\n', '\treturn r.Seq, r.ExecSeq != nil\n', "TestVER24_")
m("VER-1.3 writes count after the reference's execution", G,
  'w.Seq > f.ref.Seq && w.Seq <= at', 'w.Seq > func() int64 { s, _ := landed(*f.ref); return s }() && w.Seq <= at', "TestVER24b_")
m("VER-1.3 ExecSeq is the declaration's", C, 'Seq: d.Seq, ExecSeq: e.Seq, ', 'Seq: d.Seq, ExecSeq: &d.Seq, ', "TestVER9b_")

# Families and their order.
m("VER-1.3 no-family runs are admitted", G, '\t\tif k, ok := r.family(); ok {\n', '\t\tif k, ok := r.family(); ok || true {\n', "TestVER21_")
m("VER-1.3 the family key has no location", C,
  'familyKey{location: g.Location, runner: *g.Runner}', 'familyKey{runner: *g.Runner}', "TestVER18c_")
m("VER-1.3 a family's runs are left in read order", G,
  '\t\tslices.SortFunc(fr, func(a, b gateRun) int { return cmp.Compare(a.Seq, b.Seq) })\n', '', "TestVER18_")
m("VER-1.3 the rows are left in map order", G,
  '\tslices.SortFunc(out, func(a, b GateState) int {\n\t\treturn cmp.Or(cmp.Compare(a.Gate, b.Gate), cmp.Compare(a.Family, b.Family))\n\t})\n',
  '', "TestVER18_")
m("VER-1.3 rows sharing a Gate name are not ordered by Family", G,
  'cmp.Or(cmp.Compare(a.Gate, b.Gate), cmp.Compare(a.Family, b.Family))', 'cmp.Compare(a.Gate, b.Gate)', "TestVER18c_")

# The v0 constants.
m("VER-1.3 a run's scope is not unknown", G, 'GateOutput: r.Output, Scope: scopeUnknown,\n', 'GateOutput: r.Output,\n', "TestVER10_")

# VER-1.3b (D33): a spilled last run's order, and the Gate's kind.
m("VER-1.3b a spilled run's order is known", G, '\treturn r.Seq, false\n', '\treturn r.Seq, true\n', "TestVER26_")
m("VER-1.3b the order row is dropped", G,
  '\t{lastOrderNotKnown, Unknown, func(*fam) Reason { return OrderUnknown }},\n', '', "TestVER26_")
m("VER-1.3b the order row ranks above red", G,
  '\t{lastFailed, Red, redReason},\n\t{lastOutcomeNotKnown, Unknown, func(*fam) Reason { return OutcomeUnknown }},\n\t{lastOrderNotKnown, Unknown, func(*fam) Reason { return OrderUnknown }},\n',
  '\t{lastOrderNotKnown, Unknown, func(*fam) Reason { return OrderUnknown }},\n\t{lastFailed, Red, redReason},\n\t{lastOutcomeNotKnown, Unknown, func(*fam) Reason { return OutcomeUnknown }},\n',
  "TestVER26_")
m("VER-1.3b a spilled run can be the reference green", G, ' && !orderNotKnown(r)', '', "TestVER26_")
m("VER-1.3b the kind is the last run's", G, '\t\tkind := kindOf(fr)\n', '\t\tkind := fr[len(fr)-1].Kind\n', "TestVER27b_")
m("VER-1.3b the kind is read at the point", G,
  '\t\tkind := kindOf(fr)\n\t\tfr = slices.DeleteFunc(fr, func(r gateRun) bool { s, _ := landed(r); return s > at })\n\t\tif len(fr) == 0 {\n\t\t\tcontinue\n\t\t}\n',
  '\t\tfr = slices.DeleteFunc(fr, func(r gateRun) bool { s, _ := landed(r); return s > at })\n\t\tif len(fr) == 0 {\n\t\t\tcontinue\n\t\t}\n\t\tkind := kindOf(fr)\n',
  "TestVER27_")
m("VER-1.3b the kind is the first recorded", G,
  '\tfor i := len(fr) - 1; i >= 0; i-- {\n\t\tif fr[i].KindRecorded {\n', '\tfor i := 0; i < len(fr); i++ {\n\t\tif fr[i].KindRecorded {\n', "TestVER27b_")
m("VER-1.3b a recorded kind is not marked", C, 'g.Kind, g.KindRecorded = *kind, true', 'g.Kind = *kind', "TestVER27b_")

# 1.3c: an empty run is Absent (2.2.4 step 5; D46), no green (D47-a), no red
# for the watched row (D47-b), and its status was read (D47).
OB = "internal/verify/obligations.go"
EMPTY_ROW = '\t{lastEmptyRun, Absent, func(*fam) Reason { return EmptyRun }},\n'
m("VER-1.3c the empty-run row is dropped", G, EMPTY_ROW, '', "TestVER13c_")
m("VER-1.3c the empty-run row ranks below writes since", G,
  EMPTY_ROW + '\t{lastFailed, Red, redReason},\n\t{lastOutcomeNotKnown, Unknown, func(*fam) Reason { return OutcomeUnknown }},\n'
  '\t{lastOrderNotKnown, Unknown, func(*fam) Reason { return OrderUnknown }},\n\t{writtenSince, Stale, func(*fam) Reason { return WritesAfterGreen }},\n',
  '\t{lastFailed, Red, redReason},\n\t{lastOutcomeNotKnown, Unknown, func(*fam) Reason { return OutcomeUnknown }},\n'
  '\t{lastOrderNotKnown, Unknown, func(*fam) Reason { return OrderUnknown }},\n\t{writtenSince, Stale, func(*fam) Reason { return WritesAfterGreen }},\n' + EMPTY_ROW,
  "TestVER13c_")
m("VER-1.3c the empty-run row ranks below failed", G,
  EMPTY_ROW + '\t{lastFailed, Red, redReason},\n', '\t{lastFailed, Red, redReason},\n' + EMPTY_ROW, "TestVER13c_")
m("VER-1.3c an empty run reads green", G, '\t{lastEmptyRun, Absent, ', '\t{lastEmptyRun, Green, ', "TestVER13c_")
m("VER-1.3c an empty run's reason is ok", G, EMPTY_ROW, '\t{lastEmptyRun, Absent, func(*fam) Reason { return OK }},\n', "TestVER13c_")
m("VER-1.3c no output reads as an empty run", G,
  '\treturn r.Output != nil && r.Output.Kind == outEmptyRun\n', '\treturn r.Output == nil || r.Output.Kind == outEmptyRun\n', "TestVER13c_")
m("VER-1.3c no summary reads as an empty run", G,
  '\treturn r.Output != nil && r.Output.Kind == outEmptyRun\n',
  '\treturn r.Output != nil && (r.Output.Kind == outEmptyRun || r.Output.Kind == outNoSummary)\n', "TestVER13c_")
m("VER-1.3c an empty run is a green", G, ' && !masked(r) && !emptyRun(r) && !failed(r) && ', ' && !masked(r) && !failed(r) && ', "TestVER13c_")
m("VER-1.3c a row above Stale has no term in green()", G,
  '\t{writtenSince, Stale, func(*fam) Reason { return WritesAfterGreen }},\n',
  '\t{func(f *fam) bool { return f.last.Output != nil && f.last.Output.Kind == outNoSummary }, Absent, func(*fam) Reason { return EmptyRun }},\n'
  '\t{writtenSince, Stale, func(*fam) Reason { return WritesAfterGreen }},\n', "TestVER13f_")
m("VER-1.3c a failed empty run is the watched red", OB,
  '\t\tcase !r.Backgrounded && !masked(r) && !emptyRun(r) && failed(r):\n', '\t\tcase !r.Backgrounded && !masked(r) && failed(r):\n', "TestVER13d_")
m("VER-1.3c an Absent gate with no write owes exit_unmasked", OB,
  '\t\tif g.LastRun != nil && owed {\n', '\t\tif g.LastRun != nil {\n', "TestVER13d_")
m("VER-1.3c an empty run's status is not read", OB, '\t{Absent, ObligationMet, citeRun},\n', '', "TestVER13d_")
