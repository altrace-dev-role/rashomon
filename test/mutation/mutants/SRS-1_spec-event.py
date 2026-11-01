# SRS-1 (card SPEC-RULES-STOP1, D-e as ruled): one record kind, spec_event,
# with a closed reason set. A reason this reader does not know is passed over
# by the critical key its head carries (*bool, absent is must-understand), and
# the rule_hit the hook writes is critical, so a reader that cannot read it
# reads a gap, never green past it.
m("SRS-1 the reason check accepts any reason", "internal/store/specevent.go",
  "func knownSpecReason(reason string) bool { return slices.Contains(specEventReasons, reason) }",
  "func knownSpecReason(reason string) bool { return true }", "TestSRS1g_AReaderPassesOverWhatItCannotRead")
m("SRS-1 the reason check reads EventHeader's bool, so an absent critical is Ignored", "internal/store/specevent.go",
  "\t\tpassOver(run, head.Critical)\n", "\t\tpassOver(run, &rec.Critical)\n", "TestSRS1g_AReaderPassesOverWhatItCannotRead")
m("SRS-1 the rule_hit is stamped not critical", "internal/store/specevent.go",
  "SchemaVersion: EventSchemaVersion, Critical: true,", "SchemaVersion: EventSchemaVersion, Critical: false,",
  "TestSRS1l_TheHookAppendIsAccepted")
# The Stop's fold (verify/rules.go) and the call's hits (verify/rules_raise.go),
# and the one record-green reading the live Stop and the replay share
# (verify/stop.go, #173 F4).
R = "internal/verify/rules.go"
m("SRS-1 the Stop does not read the hits", R,
  "\t\t\tr.seqs = append(r.seqs, ruleHitSeqs(run, dr.ID, sourceDigest(sp, dr.Source), at)...)\n", "",
  "TestSRS1_ARuleHitIsAViolationAtStop|TestSRS1d_")
m("SRS-1 the Stop compares the merged spec digest", R,
  "ruleHitSeqs(run, dr.ID, sourceDigest(sp, dr.Source), at)", "ruleHitSeqs(run, dr.ID, sp.Digest, at)",
  "TestSRS1_ARuleHitIsAViolationAtStop|TestSRS1d_")
m("SRS-1 a hit's reason is not read", R,
  "h.Reason == store.SpecEventRuleHit && h.RuleID == id", "h.RuleID == id", "TestSRS1d_")
m("SRS-1 a hit counts under any digest of its file", R,
  "h.RuleID == id && h.SourceDigest == digest && h.LandedSeq > 0", "h.RuleID == id && h.LandedSeq > 0", "TestSRS1d_")
m("SRS-1 a hit past the point counts", R,
  "h.LandedSeq > 0 && h.LandedSeq <= at {", "h.LandedSeq > 0 {", "TestSRS1d_")
m("SRS-1 a hit cites its landed seq, not its declaration's", R,
  "\t\t\t\treturn run.Declarations[j].Seq\n", "\t\t\t\treturn landed\n", "TestSRS1_ARuleHitIsAViolationAtStop")
m("SRS-1 the call hits only at PostToolUse", "internal/verify/rules_raise.go",
  "\t\t\tview.Hits = append(view.Hits, RuleHit{RuleID: dr.ID, SourceDigest: sourceDigest(sp, dr.Source)})\n\t\t\tif event != string(steer.EventPostToolUse) {\n\t\t\t\tcontinue\n\t\t\t}\n",
  "\t\t\tif event != string(steer.EventPostToolUse) {\n\t\t\t\tcontinue\n\t\t\t}\n\t\t\tview.Hits = append(view.Hits, RuleHit{RuleID: dr.ID, SourceDigest: sourceDigest(sp, dr.Source)})\n",
  "TestSRS1h_")
m("SRS-1 the replay recounts record-green without binding violations", "internal/verify/stop.go",
  "\tsnap.RecordGreen, snap.RecordGreenAtSeq = recordGreenOf(snap)\n",
  "\tsnap.RecordGreen, snap.RecordGreenAtSeq = recordGreen(snap.Gates, len(snap.Integrity.Unresolved))\n",
  "TestSRS1e2_")
