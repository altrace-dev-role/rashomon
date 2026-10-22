# The destinations, families and chains keys changed incompatibly in the
# move off the proxy, with nothing added to say so. report_version names the
# shape; these two mutants are the two places it is actually set rather
# than merely declared.
m("Build renders a report with no version", "internal/report/report.go",
  "\trep := &Report{\n\t\tReportVersion:         CurrentReportVersion,\n",
  "\trep := &Report{\n",
  "TestReportVersion_IsCarriedOnEveryReport")
m("Empty renders a report with no version", "internal/report/report.go",
  "return &Report{ReportVersion: CurrentReportVersion, GeneratedAtUnixMS: now.UnixMilli(), CallsWithoutSessionID: &none, Sessions: []Session{}}",
  "return &Report{GeneratedAtUnixMS: now.UnixMilli(), CallsWithoutSessionID: &none, Sessions: []Session{}}",
  "TestReportVersion_IsCarriedOnEveryReport")
