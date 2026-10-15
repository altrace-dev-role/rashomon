m("statusSize hides a partial total behind unknown", "cmd/rashomon/status_store.go",
  "\tswitch n, unlisted, err := st.RunsBytes(); {\n\tcase err == nil:\n\t\tsize = store.FormatBytes(n)\n\tcase unlisted == 1:\n\t\tsize, note = store.FormatBytes(n), \" (1 run not listed)\"\n\tcase unlisted > 1:\n\t\tsize, note = store.FormatBytes(n), fmt.Sprintf(\" (%d runs not listed)\", unlisted)\n\t}\n",
  "\tif n, _, err := st.RunsBytes(); err == nil {\n\t\tsize = store.FormatBytes(n)\n\t}\n",
  "TestStatusSize_NotesAnUnlistableRunRatherThanHidingTheTotal")
m("RunsBytes drops the count of what it could not list", "internal/store/capbytes.go",
  "\treturn plan.total, len(plan.unlisted), errors.Join(plan.unlisted...)\n",
  "\treturn plan.total, 0, errors.Join(plan.unlisted...)\n",
  "TestRunsBytes_CountsWhatItCouldNotList")
