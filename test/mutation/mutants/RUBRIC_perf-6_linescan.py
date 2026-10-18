# perf-6: linescan reads lines as bufio.Scanner does, under the same bound,
# and steps over a line over it instead of ending the read.
m("perf-6  linescan takes a line of max bytes", "internal/linescan/linescan.go",
  "\t\tcase n > s.max || (err != nil && n >= s.max):", "\t\tcase n > s.max+1 || (err != nil && n > s.max):", "TestLinescan_")
m("perf-6  linescan takes a final line of max bytes", "internal/linescan/linescan.go",
  "\t\tcase n > s.max || (err != nil && n >= s.max):", "\t\tcase n > s.max:", "TestLinescan_")
m("perf-6  linescan keeps assembling a line over the bound", "internal/linescan/linescan.go",
  "\t\t\ts.tooLong, s.buf = true, s.buf[:0]\n", "\t\t\ts.tooLong = true\n\t\t\ts.buf = append(s.grow(len(frag)), frag...)\n", "TestLinescan_")
m("perf-6  linescan hands back a fragment as the line", "internal/linescan/linescan.go",
  "\t\tcase err == nil && len(s.buf) == 0:", "\t\tcase len(s.buf) == 0:", "TestLinescan_")
m("perf-6  linescan grows past its bound", "internal/linescan/linescan.go",
  "\tc := min(max(2*cap(s.buf), need), s.max)\n", "\tc := max(2*cap(s.buf), need)\n", "TestLinescan_")
m("perf-6  linescan keeps a \\r before the newline", "internal/linescan/linescan.go",
  "\tif len(l) > 0 && l[len(l)-1] == '\\r' {", "\tif false && len(l) > 0 && l[len(l)-1] == '\\r' {", "TestLinescan_")
m("perf-6  linescan does not count a line too long", "internal/linescan/linescan.go",
  "\t\ts.skipped++\n", "", "TestLinescan_")
m("perf-6  linescan hands back a line its read error cut short", "internal/linescan/linescan.go",
  "\t\t\ts.done, s.err, s.line, s.tooLong = true, err, nil, false\n\t\t\treturn false\n",
  "\t\t\ts.done, s.err = true, err\n\t\t\treturn len(s.line) > 0\n", "TestLinescan_")
m("perf-6  linescan reads on after a read error", "internal/linescan/linescan.go",
  "\t\t\ts.done, s.err, s.line, s.tooLong = true, err, nil, false\n",
  "\t\t\ts.err, s.line, s.tooLong = err, nil, false\n", "TestLinescan_")
m("perf-6  linescan drops a final line with no newline", "internal/linescan/linescan.go",
  "\t\t\tif n == 0 {\n\t\t\t\treturn false\n\t\t\t}\n\t\t\treturn s.emit()",
  "\t\t\treturn false", "TestLinescan_")
