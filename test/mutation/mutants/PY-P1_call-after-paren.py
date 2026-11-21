# A call whose name directly follows the open paren of another call takes
# that paren as the byte before its name. One FindAll pass over the line, as
# the reader did before PY-P, has consumed that paren with the match before
# and reads ^ only where the read starts, so it loses every such call: add
# in print(add(2, 3), total()), the middle call of three deep, a call after
# return( and a method call inside another call. A search that starts one
# byte after the paren is no such mutant: ^ matches at the start of its own
# slice and reads the same calls.
m("pylite readCalls reads the line in one FindAll pass again", "internal/graph/lang/pylite/scan.go",
  "\tfor pos := from; ; {\n"
  "\t\tm := pats().call.FindStringSubmatchIndex(line[pos:])\n"
  "\t\tif m == nil {\n"
  "\t\t\tbreak\n"
  "\t\t}\n"
  "\t\tname := line[pos+m[2] : pos+m[3]]\n"
  "\t\topen := pos + m[1] - 1\n"
  "\t\tpos = open\n",
  "\tfor _, m := range pats().call.FindAllStringSubmatchIndex(line[from:], -1) {\n"
  "\t\tpos := from\n"
  "\t\tname := line[pos+m[2] : pos+m[3]]\n"
  "\t\topen := pos + m[1] - 1\n",
  "TestPYP_ACallRightAfterAnOpenParenIsRead")
