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
