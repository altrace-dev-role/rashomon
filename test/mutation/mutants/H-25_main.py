# forget --host's own line, re-anchored from H-27_main.py, which retired whole
# with `rashomon run`: the code and the tests survived, renamed. The baseline
# count follows only where a report has written a baseline directory, and the
# line's grammar is the line's.
m("forget --host prints a baseline count where no report ever ran", "cmd/rashomon/main.go",
  "\tif !hadBaselines {\n\t\tfmt.Fprintln(stdout)\n\t\treturn nil\n\t}",
  "\tif false && !hadBaselines {\n\t\tfmt.Fprintln(stdout)\n\t\treturn nil\n\t}",
  "TestH25_ForgetHostIsOneLineBeforeAnyReport")
m("forget --host drops the baseline count after a report ran", "cmd/rashomon/main.go",
  "\tif !hadBaselines {\n\t\tfmt.Fprintln(stdout)\n\t\treturn nil\n\t}",
  "\tif true || !hadBaselines {\n\t\tfmt.Fprintln(stdout)\n\t\treturn nil\n\t}",
  "TestH25_ForgottenHostStaysOutOfTheReport")
m("forget --host says \"1 runs\"", "cmd/rashomon/main.go",
  "\tif n == 1 {\n\t\treturn \"1 \" + noun\n\t}", "",
  "TestH25_ForgetHostIsOneLineBeforeAnyReport")
