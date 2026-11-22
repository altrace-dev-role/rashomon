# A def's header ends at the first colon outside a bracket, and a bracket
# still open joins the next line to it (Python's implicit line joining), so
# a return annotation may run over lines after the parameters close. Without
# the count an open bracket at a line end ends the header with no colon, and
# a header whose annotation runs over lines is refused, the whole file with it.
m("pylite counts no open bracket in a def's header", "internal/graph/lang/pylite/scan.go",
  "\t\t\tcase '(', '[', '{':\n\t\t\t\tdepth++\n\t\t\tcase ')', ']', '}':\n\t\t\t\tdepth--\n\t\t\tcase ':':\n",
  "\t\t\tcase '(', '[', '{':\n\t\t\tcase ')', ']', '}':\n\t\t\t\tdepth--\n\t\t\tcase ':':\n",
  "TestDefHeader_AHeaderOpenPastItsParametersIsRead")

# A backslash that ends the code joins the next line to the header
# (explicit line joining), after the parameters, after the arrow or before
# the colon. Without the join the header ends at that line end with no colon
# and the file is refused.
m("pylite joins no line after a backslash in a def's header", "internal/graph/lang/pylite/scan.go",
  "\t\tif depth == 0 && !strings.HasSuffix(strings.TrimRight(line[from:], \" \\t\\r\"), `\\`) {\n",
  "\t\tif depth == 0 {\n",
  "TestDefHeader_AHeaderOpenPastItsParametersIsRead")

# A colon inside a bracket of the annotation (a dict display, a slice) is
# no end of the header. Taken as the end, the header stops inside the
# bracket, the bracket's last line at the def's indent ends the def, and its
# body is read as module lines: the def's calls are lost and its end is wrong.
m("pylite ends a def's header at a colon inside a bracket", "internal/graph/lang/pylite/scan.go",
  "\t\t\tcase ':':\n\t\t\t\tif depth == 0 {\n\t\t\t\t\treturn j, true\n\t\t\t\t}\n",
  "\t\t\tcase ':':\n\t\t\t\treturn j, true\n",
  "TestDefHeader_AHeaderOpenPastItsParametersIsRead")

# A colon on the line the parameters close on ends the header there, as it
# always has, so every file read before gives the same records under the
# same version. Walking past it instead moves the end of a header whose
# close line holds a colon inside a bracket still open, and the call on the
# line after is no longer the def's.
m("pylite walks past a colon on a def's close line", "internal/graph/lang/pylite/scan.go",
  "\tif !strings.Contains(lines[end][closeAt+1:], \":\") {\n",
  "\tif true {\n",
  "TestDefHeader_AColonOnTheCloseLineEndsTheHeaderAsBefore")
