# REC-33: a gap line that does not parse is counted, and every session reads
# unverified while the count is not zero: records_unreadable in the report,
# records_skipped in the turn digest.
m("REC-33 ReadGaps drops the gap-skip count", "internal/store/read.go",
  "\t\t\t*skipped++\n\t\t\treturn\n", "\t\t\treturn\n", "TestREC33_")
m("REC-33 the report ignores unreadable gap lines", "internal/report/report.go",
  "\t\tif gapsSkipped > 0 {\n\t\t\tsess.Coverage.add(ReasonRecordsUnreadable)\n",
  "\t\tif gapsSkipped < 0 {\n\t\t\tsess.Coverage.add(ReasonRecordsUnreadable)\n", "TestREC33_")
m("REC-33 the digest ignores unreadable gap lines", "internal/digest/digest.go",
  "\tif gapsSkipped > 0 {\n\t\td.Coverage.add(ReasonRecordsSkipped)\n",
  "\tif gapsSkipped < 0 {\n\t\td.Coverage.add(ReasonRecordsSkipped)\n", "TestREC33_")
