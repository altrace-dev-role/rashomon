# A comment ends its physical line, so it is no part of an import on that
# line. Read with its comment, a from-import's last name carries the comment
# into the name it references, a plain import with a comment matches nothing,
# a comment inside a bracket ends the import before its names, and a
# backslash at the end of a comment joins the next line into the import.
m("pylite reads an import line with its comment", "internal/graph/lang/pylite/scan.go",
  "\tc, _, _ := strings.Cut(line, \"#\")\n",
  "\tc := line\n",
  "TestLogicalLine_BracketsJoinLinesAndACommentEndsItsLine")

# A bracket still open joins the next line to the import (Python's implicit
# line joining). Without the count the import ends on its opening line, and a
# from-import whose names sit in brackets over several lines reads none of
# the names past that line.
m("pylite ends a bracketed import on its opening line", "internal/graph/lang/pylite/scan.go",
  "\t\tdepth += strings.Count(c, \"(\") - strings.Count(c, \")\")\n",
  "",
  "TestLogicalLine_BracketsJoinLinesAndACommentEndsItsLine")

# Inside an import's bracket only names, commas, as, backslashes and the
# closing bracket occur, so a line with any other character ends the import
# before it. Strings are blanked before the scan (blank.go), so the stop
# guards a bracket that balances but holds a line no import can hold, a file
# Python rejects that an edit in progress can leave. Without the stop that
# line and every line after it up to the closing bracket join the import, and
# the real import among them is lost.
m("pylite joins past a string's close into the code after it", "internal/graph/lang/pylite/scan.go",
  "\t\tif depth > 0 && strings.IndexFunc(c, notName) >= 0 {\n\t\t\tbreak\n\t\t}\n",
  "",
  "TestLogicalLine_AnImportEndsBeforeALineNoImportHolds")
