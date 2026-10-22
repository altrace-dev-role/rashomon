m("H-10 accounting compares counts, not sets", "internal/report/report.go",
  "\tt.MissingFromStore = []string{}\n\tfor id := range ids {", "\tt.MissingFromStore = []string{}\n\tfor id := range ids {\n\t\tif len(ids) == len(recorded) {\n\t\t\tbreak\n\t\t}", "TestH10_SetsNotCounts")
m("H-10 every transcript is checked against the union of the run's ids", "internal/report/report.go",
  "\t\t\tt = accounting(path, byPath[path], executed, hooked)",
  "\t\t\tt = accounting(path, func() map[string]bool {\n\t\t\t\tunion := map[string]bool{}\n\t\t\t\tfor _, ids := range byPath {\n\t\t\t\t\tfor id := range ids {\n\t\t\t\t\t\tunion[id] = true\n\t\t\t\t\t}\n\t\t\t\t}\n\t\t\t\treturn union\n\t\t\t}(), executed, hooked)", "TestH10_PerTranscript")
m("H-10 declarations naming no transcript are not counted", "internal/report/report.go",
  "\t\tif d.TranscriptPath == \"\" {\n\t\t\tsess.Declarations.WithoutTranscript++\n\t\t\tcontinue\n\t\t}",
  "\t\tif d.TranscriptPath == \"\" {\n\t\t\tcontinue\n\t\t}", "TestH10_DeclarationsWithoutATranscript")
