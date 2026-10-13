# The destinations, families and chains keys changed incompatibly in the
# move off the proxy, with nothing added to say so. report_version names the
# shape; these two mutants are the two places it is actually set rather
# than merely declared.
m("Build renders a report with no version", "internal/report/assemble.go",
  "rep := &Report{ReportVersion: CurrentReportVersion, GeneratedAtUnixMS: now.UnixMilli(), Sessions: make([]Session, 0, len(in.Sessions))}",
  "rep := &Report{GeneratedAtUnixMS: now.UnixMilli(), Sessions: make([]Session, 0, len(in.Sessions))}",
  "TestReportVersion_IsCarriedOnEveryReport")
m("Empty renders a report with no version", "internal/report/report.go",
  "return &Report{ReportVersion: CurrentReportVersion, GeneratedAtUnixMS: now.UnixMilli(), Sessions: []Session{}}",
  "return &Report{GeneratedAtUnixMS: now.UnixMilli(), Sessions: []Session{}}",
  "TestReportVersion_IsCarriedOnEveryReport")
