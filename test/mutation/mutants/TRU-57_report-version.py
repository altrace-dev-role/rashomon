# The destinations, families and chains keys changed incompatibly in the
# move off the proxy, with nothing added to say so. report_version names the
# shape; these mutants are the places it is actually set rather than merely
# declared: gather.Build, which assembles one session at a time without
# Assemble, Assemble itself, and Empty.
m("Build renders a report with no version", "internal/report/gather/gather.go",
  "rep := &report.Report{ReportVersion: report.CurrentReportVersion, GeneratedAtUnixMS: now.UnixMilli(), Sessions: make([]report.Session, 0, len(names))}",
  "rep := &report.Report{GeneratedAtUnixMS: now.UnixMilli(), Sessions: make([]report.Session, 0, len(names))}",
  "TestReport_JSONFormIsUnchanged")
m("Assemble renders a report with no version", "internal/report/assemble.go",
  "rep := &Report{ReportVersion: CurrentReportVersion, GeneratedAtUnixMS: now.UnixMilli(), Sessions: make([]Session, 0, len(in.Sessions))}",
  "rep := &Report{GeneratedAtUnixMS: now.UnixMilli(), Sessions: make([]Session, 0, len(in.Sessions))}",
  "TestReportVersion_IsCarriedOnEveryReport")
m("Empty renders a report with no version", "internal/report/report.go",
  "return &Report{ReportVersion: CurrentReportVersion, GeneratedAtUnixMS: now.UnixMilli(), Sessions: []Session{}}",
  "return &Report{GeneratedAtUnixMS: now.UnixMilli(), Sessions: []Session{}}",
  "TestReportVersion_IsCarriedOnEveryReport")
