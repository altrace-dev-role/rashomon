# The destinations, families and chains keys changed incompatibly in the
# move off the proxy, with nothing added to say so. report_version names the
# shape; these mutants are the places it is actually set rather than merely
# declared: gather.Build, which assembles one session at a time without
# Assemble, Assemble itself, and Empty.
m("Build renders a report with no version", "internal/report/gather/gather.go",
  "\trep := &report.Report{\n\t\tReportVersion:         report.CurrentReportVersion,\n",
  "\trep := &report.Report{\n",
  "TestReport_JSONFormIsUnchanged")
m("Assemble renders a report with no version", "internal/report/assemble.go",
  "\trep := &Report{\n\t\tReportVersion:         CurrentReportVersion,\n",
  "\trep := &Report{\n",
  "TestReportVersion_IsCarriedOnEveryReport")
m("Empty renders a report with no version", "internal/report/report.go",
  "return &Report{ReportVersion: CurrentReportVersion, GeneratedAtUnixMS: now.UnixMilli(), CallsWithoutSessionID: &none, Sessions: []Session{}}",
  "return &Report{GeneratedAtUnixMS: now.UnixMilli(), CallsWithoutSessionID: &none, Sessions: []Session{}}",
  "TestReportVersion_IsCarriedOnEveryReport")
