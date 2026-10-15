m("LongestLockWait fails instead of skipping a refused note read", "internal/store/lockwait.go",
  "\t\tif perr != nil {\n\t\t\tvar num *strconv.NumError\n\t\t\tif errors.As(perr, &num) || refused(perr) {\n\t\t\t\tcontinue\n\t\t\t}\n\t\t\treturn 0, \"\", perr\n\t\t}\n",
  "\t\tif perr != nil {\n\t\t\tvar num *strconv.NumError\n\t\t\tif errors.As(perr, &num) {\n\t\t\t\tcontinue\n\t\t\t}\n\t\t\treturn 0, \"\", perr\n\t\t}\n",
  "TestLongestLockWait_SkipsARefusedNoteRead")
