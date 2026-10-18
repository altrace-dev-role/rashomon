# STR-9b: a steer line writes evidence_seqs and slots as [] and {}, never
# null, however the Line was built: Line's own MarshalJSON fills a nil
# slice or map, on the value so a Line, a *Line and a Line inside another
# value all go through it.
# RETIRED: "STR-9b NewLine leaves EvidenceSeqs nil" and "STR-9b NewLine
# leaves Slots nil". NewLine is gone: a Line{} literal skipped it and still
# wrote null, so the rule moved into MarshalJSON, which no caller can skip.
m("STR-9b a zero line writes evidence_seqs as null", "internal/steer/line.go",
  "\tif w.EvidenceSeqs == nil {\n\t\tw.EvidenceSeqs = []int64{}\n\t}\n", "",
  "TestLine_ZeroValueMarshalsEmptyNotNull")
m("STR-9b a zero line writes slots as null", "internal/steer/line.go",
  "\tif w.Slots == nil {\n\t\tw.Slots = map[string]json.RawMessage{}\n\t}\n", "",
  "TestLine_ZeroValueMarshalsEmptyNotNull")
m("STR-9b only a pointer to a line marshals through the method", "internal/steer/line.go",
  "func (l Line) MarshalJSON() ([]byte, error) {\n\ttype wire Line // the same fields without this method, so no recursion\n\tw := wire(l)\n",
  "func (l *Line) MarshalJSON() ([]byte, error) {\n\ttype wire Line\n\tw := wire(*l)\n",
  "TestLine_ZeroValueMarshalsEmptyNotNull")
