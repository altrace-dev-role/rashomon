# STR-9b: NewLine starts evidence_seqs and slots at empty, not nil, so
# marshalling a freshly built Line writes [] and {} rather than null for
# either key.
m("STR-9b NewLine leaves EvidenceSeqs nil", "internal/steer/line.go",
  "\t\tEvidenceSeqs: []int64{},\n", "\t\tEvidenceSeqs: nil,\n",
  "TestNewLine_EvidenceSeqsAndSlotsAreEmptyNotNull")
m("STR-9b NewLine leaves Slots nil", "internal/steer/line.go",
  "\t\tSlots:        map[string]json.RawMessage{},\n", "\t\tSlots:        nil,\n",
  "TestNewLine_EvidenceSeqsAndSlotsAreEmptyNotNull")
