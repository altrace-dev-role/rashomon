m("status reads its own shell's cap instead of a hook's recorded one", "cmd/rashomon/status_store.go",
  '\tif st != nil {\n\t\tif rc, ok, err := st.ResolvedCap(); err == nil && ok {\n\t\t\treturn rc.Bytes, rc.Problem, ""\n\t\t}\n\t}\n',
  "",
  "TestRubricPERF7_StatusShowsTheStoreAgainstItsCap")
m("the probe never records the cap it resolved", "internal/hook/probe.go",
  "\tcapSetting := store.ResolveCap()\n\t_ = st.RecordResolvedCap(capSetting)\n",
  "\tcapSetting := store.ResolveCap()\n",
  "TestRubricPERF7_StatusShowsTheStoreAgainstItsCap")
