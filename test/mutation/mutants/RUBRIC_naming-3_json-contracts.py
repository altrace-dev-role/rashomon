# Rubric naming-3 / test-6 / perf-5: report --json carries a report_version
# and is byte-compared with a committed golden, and the digest and spend
# documents' key paths are snapshots. A renamed key is a diff, not a pass.
m("RUBRIC naming-3 an empty report carries no report_version", "internal/report/report.go",
  "\treturn &Report{ReportVersion: CurrentReportVersion, GeneratedAtUnixMS: now.UnixMilli(), Sessions: []Session{}}",
  "\treturn &Report{GeneratedAtUnixMS: now.UnixMilli(), Sessions: []Session{}}",
  "TestRubricNAMING3_TheReportCarriesItsReportVersion")
m("RUBRIC naming-3 report's redacted key is renamed", "internal/report/report.go",
  "\tRedacted bool `json:\"redacted\"`", "\tRedacted bool `json:\"redact\"`",
  "TestRubricNAMING3_ReportJSONIsTheGolden")
m("RUBRIC naming-3 report's programs_unknown key is renamed", "internal/report/report.go",
  "\tProgramsUnknown int            `json:\"programs_unknown\"`", "\tProgramsUnknown int            `json:\"unknown_programs\"`",
  "TestRubricNAMING3_ReportJSONIsTheGolden")
m("RUBRIC naming-3 digest's skipped_records key is renamed", "internal/digest/digest.go",
  "\tSkippedRecords int `json:\"skipped_records\"`", "\tSkippedRecords int `json:\"records_skipped\"`",
  "TestRubricNAMING3_DigestJSONKeysAreTheSnapshot")
m("RUBRIC naming-3 spend's by_kind key is renamed", "internal/spend/spend.go",
  "\tByKind  KindSplit    `json:\"by_kind\"`", "\tByKind  KindSplit    `json:\"by_kinds\"`",
  "TestRubricNAMING3_SpendJSONKeysAreTheSnapshot")
m("RUBRIC naming-3 the version guard lets any removal through",
  "test/acceptance/schema_guard_test.go",
  "\treturn removed, len(removed) > 0 && !bumped\n",
  "\t_ = bumped\n\treturn removed, false\n",
  "TestGuardDropsAKey_BlocksARemovedKeyWithNoVersionBump")
m("RUBRIC naming-3 any changed version passes for a bump",
  "test/acceptance/schema_guard_test.go",
  "\tbumped := newVersion >= 0 && newVersion > oldVersion\n",
  "\tbumped := newVersion != oldVersion\n",
  "TestGuardDropsAKey_BlocksARemovedKeyWithNoVersionBump")
m("RUBRIC naming-3 an unparseable version reads as unchanged",
  "test/acceptance/schema_guard_test.go",
  "\tif json.Unmarshal(raw, &doc) != nil || json.Unmarshal(doc[key], &v) != nil || v == nil {\n\t\treturn -1\n\t}\n",
  "\tif false {\n\t\treturn -1\n\t}\n\tjson.Unmarshal(raw, &doc)\n\tjson.Unmarshal(doc[key], &v)\n",
  "TestVersionOf_UnknownNeverReadsAsUnchanged")
m("RUBRIC naming-3 a map's key paths pin a run's own data as schema",
  "test/acceptance/type_key_paths_test.go",
  "\tcase reflect.Map:\n\t\tkey := prefix + \".{}\"\n\t\tif prefix == \"\" {\n\t\t\tkey = \"{}\"\n\t\t}\n\t\twalkType(key, t.Elem(), seen)\n",
  "\tcase reflect.Map:\n\t\twalkType(prefix+\".k\", t.Elem(), seen)\n",
  "TestTypeKeyPaths_WalksTheTypeNotAnInstance")
m("RUBRIC naming-3 a type with its own MarshalJSON is read by its field names",
  "test/acceptance/type_key_paths_test.go",
  "\tif t.Implements(marshalerType) || reflect.PointerTo(t).Implements(marshalerType) {\n\t\twalkMarshaler(prefix, t, seen)\n\t\treturn\n\t}\n",
  "",
  "TestRubricNAMING3_SpendJSONKeysAreTheSnapshot")
m("RUBRIC naming-3 an unexported field is read as if marshalled",
  "test/acceptance/type_key_paths_test.go",
  "\t\t\tif f.PkgPath != \"\" && !f.Anonymous {\n\t\t\t\tcontinue // unexported, never marshalled\n\t\t\t}\n",
  "",
  "TestTypeKeyPaths_WalksTheTypeNotAnInstance")
