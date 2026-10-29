# TRU-48: one consent.json, one record per line, three readers. Each mutant
# breaks one rule a reader or the writer keeps: a torn line ends the read, an
# append completes a torn line, a reader takes the wrong record, a line from
# outside the closed via set counts, or compaction changes what a reader says.
m("TRU-48 a line that does not parse ends the read", "internal/consent/record.go",
  "\t\tif json.Unmarshal(line, &r) != nil || !r.parses() {\n\t\t\tcontinue\n\t\t}",
  "\t\tif json.Unmarshal(line, &r) != nil || !r.parses() {\n\t\t\treturn out\n\t\t}",
  "TestTRU48_OneConsentFileThreeReaders")
m("TRU-48 an append completes a torn last line", "internal/consent/record.go",
  "\t\tif last[0] != '\\n' {", "\t\tif false && last[0] != '\\n' {",
  "TestTRU48_OneConsentFileThreeReaders")
m("TRU-48 Current needs a disclosure above the build's", "internal/consent/record.go",
  "\treturn n >= steer.DisclosureVersion, err", "\treturn n > steer.DisclosureVersion, err",
  "TestTRU48_OneConsentFileThreeReaders")
m("TRU-48 RecordScope takes the earliest record", "internal/consent/record.go",
  "\tfor i := len(recs) - 1; i >= 0; i-- {\n\t\tif s := recs[i].RecordScope; s != \"\" {",
  "\tfor i := 0; i < len(recs); i++ {\n\t\tif s := recs[i].RecordScope; s != \"\" {",
  "TestTRU48_OneConsentFileThreeReaders")
m("TRU-48 any consent covers this build's scope", "internal/consent/record.go",
  "\treturn ok && best >= disclose.ScopeVersion, nil", "\treturn ok && best >= 0*disclose.ScopeVersion, nil",
  "TestTRU48_OneConsentFileThreeReaders")
m("TRU-48 a via outside the closed set counts", "internal/consent/record.go",
  "\treturn r.V == RecordVersion && r.Via.Valid()", "\treturn r.V == RecordVersion",
  "TestTRU48_OneConsentFileThreeReaders")
m("TRU-48 compaction drops the highest disclosure", "internal/consent/record.go",
  "\t\tif latest[key{r.Via, r.ScopeVersion, r.Unit}] == i || i == maxDisclosure || i == scoped {",
  "\t\tif latest[key{r.Via, r.ScopeVersion, r.Unit}] == i || i == scoped {",
  "TestTRU48_CompactionKeepsEveryReadersAnswer")
m("TRU-48 compaction keeps one record per via only", "internal/consent/record.go",
  "\t\tlatest[key{r.Via, r.ScopeVersion, r.Unit}] = i\n", "\t\tlatest[key{r.Via, 0, r.Unit}] = i\n",
  "TestTRU48_CompactionKeepsEveryReadersAnswer")
m("TRU-48 the file is never compacted", "internal/consent/record.go",
  "info.Size()+int64(len(line))+1 > CapBytes {", "info.Size()+int64(len(line))+1 > 64*CapBytes {",
  "TestTRU48_CompactionKeepsEveryReadersAnswer")
m("TRU-48 the disclosure version moves without a review", "internal/steer/disclosure.go",
  "const DisclosureVersion = 2", "const DisclosureVersion = 3",
  "TestTRU48_DisclosureVersionIsPinned")
