# TRU-29: purge refuses a root it cannot prove is a store, or that it must
# never remove, before it prints a plan or touches anything.
m("TRU-29 a filesystem root is not refused", "internal/purge/purge.go",
  "\tif filepath.Dir(root) == root {\n",
  "\tif false {\n",
  "TestTRU29_")
m("TRU-29 a root equal to home is not refused", "internal/purge/purge.go",
  "\tif root == home {\n",
  "\tif false {\n",
  "TestTRU29_")
m("TRU-29 a root enclosing home is not refused", "internal/purge/purge.go",
  "\tif Encloses(root, home) {\n",
  "\tif false {\n",
  "TestTRU29_")
m("TRU-29 Encloses counts a sibling sharing a name prefix", "internal/purge/purge.go",
  "\treturn rel != \"..\" && !strings.HasPrefix(rel, \"..\"+string(filepath.Separator))\n",
  "\treturn !strings.HasPrefix(rel, \"..\") || len(rel) > 0\n",
  "TestTRU29_")
m("TRU-29 a root without install.key is accepted", "cmd/rashomon/purge.go",
  "\tfor _, anchor := range []string{purge.InstallMeta, purge.InstallKey} {\n",
  "\tfor _, anchor := range []string{purge.InstallMeta} {\n",
  "TestTRU29_")
m("TRU-29 a root in an unconfirmed work tree is accepted", "cmd/rashomon/purge.go",
  "\tif top := purge.WorkTreeTop(resolvedRoot); top != \"\" && !confirmedWorkTree(t.rec.RootInWorkTree, top) {\n",
  "\tif top := purge.WorkTreeTop(resolvedRoot); top != \"\" && false {\n",
  "TestTRU29_")
m("TRU-29 a missing root_in_work_tree counts as confirmed", "cmd/rashomon/purge.go",
  "\tif recorded == \"\" {\n\t\treturn false\n\t}\n",
  "\tif recorded == \"\" {\n\t\treturn true\n\t}\n",
  "TestTRU29_")
m("TRU-29 WorkTreeTop looks only at the directory itself", "internal/purge/purge.go",
  "\t\tdir = parent\n\t}\n}\n",
  "\t\treturn \"\"\n\t}\n}\n",
  "TestTRU29_")
m("TRU-29 a root left holding only install.key gets no command", "cmd/rashomon/purge.go",
  "\tif err != nil || len(entries) != 1 || entries[0].Name() != purge.InstallKey {\n",
  "\tif true {\n",
  "TestTRU29_")
