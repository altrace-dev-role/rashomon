# presizeRun (internal/store/presize.go): the reader's three call slices and
# the clamp that bounds what a line count can ask for before a single line
# is parsed. Not row-pinned; this is a performance change the measured
# allocation gates (test/perf/budgets.json) judge, not a numbered
# specification row.
m("PRESIZE the clamp on a line count is skipped", "internal/store/presize.go",
  "\tif lines > maxPresizeLines {\n\t\tlines = maxPresizeLines\n\t}\n",
  "", "TestPresizeBoundedOnAdversarialFile")
m("PRESIZE presizeRun never presizes the call slices", "internal/store/presize.go",
  "\tif n := callSlicePresize(calls); n > 0 {",
  "\tif n := callSlicePresize(calls); n > 0 && false {",
  "TestPresizeMatchesLineCount")
m("PRESIZE countLinesCapped counts past its own limit", "internal/store/presize.go",
  "\t\t\tif count >= limit {\n\t\t\t\treturn limit, nil\n\t\t\t}\n",
  "", "TestCountLinesCappedStopsAtCap")
m("PRESIZE cappedLineCount skips the file-size floor", "internal/store/presize.go",
  "\t\tif bySize := int(info.Size() / minPresizeRecordBytes); bySize < limit {\n\t\t\tlimit = bySize\n\t\t}\n",
  "", "TestPresizeBoundedByFileSizeOnShortLinePadding")
