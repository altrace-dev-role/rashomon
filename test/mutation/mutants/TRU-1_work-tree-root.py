# TRU-1: a store root inside a git work tree needs its top directory's name
# typed at a third question before anything is written; --yes cannot answer
# it, and the confirmed answer is recorded in install.json.
m("TRU-1 the third question is never asked", "cmd/rashomon/watch.go",
  "\tif wt.top != \"\" {\n\t\treply, eof, err := term.Ask(", "\tif false {\n\t\treply, eof, err := term.Ask(",
  "TestTRU1_AStoreRoot")
m("TRU-1 EOF after the name confirms it", "cmd/rashomon/watch.go",
  "\t\tif eof || strings.TrimSpace(reply) != filepath.Base(wt.top) {\n",
  "\t\tif strings.TrimSpace(reply) != filepath.Base(wt.top) {\n",
  "TestTRU1_AStoreRoot")
m("TRU-1 the name is matched without case", "cmd/rashomon/watch.go",
  "\t\tif eof || strings.TrimSpace(reply) != filepath.Base(wt.top) {\n",
  "\t\tif eof || !strings.EqualFold(strings.TrimSpace(reply), filepath.Base(wt.top)) {\n",
  "TestTRU1_AStoreRoot")
m("TRU-1 --yes installs in a work tree unconfirmed", "cmd/rashomon/watch.go",
  "\t\tif wt.top != \"\" {\n\t\t\treturn nil, fmt.Errorf(refuseWorkTreeYes,", "\t\tif false {\n\t\t\treturn nil, fmt.Errorf(refuseWorkTreeYes,",
  "TestTRU1_AStoreRoot")
m("TRU-1 the work tree is not looked for", "cmd/rashomon/watch.go",
  "\tif plan.workTree, err = store.WorkTreeTop(plan.root); err != nil {\n",
  "\tif _, err = store.WorkTreeTop(plan.root); err != nil {\n",
  "TestTRU1_AStoreRoot")
m("TRU-1 a confirmed work tree is not recorded", "cmd/rashomon/watch.go",
  "\tif ans.workTree != \"\" {\n\t\tundo, err = restorable(", "\tif false {\n\t\tundo, err = restorable(",
  "TestTRU1_AStoreRoot")
m("TRU-1 the work tree write is not undone", "cmd/rashomon/watch.go",
  "\t\tw.done = append(w.done, undoStep{stepWorkTree, undo})\n", "",
  "TestTRU1_AStoreRoot")
m("TRU-1 the work tree is recorded under another key", "internal/store/worktree.go",
  "const KeyRootInWorkTree = \"root_in_work_tree\"\n", "const KeyRootInWorkTree = \"work_tree\"\n",
  "TestTRU1_")
m("TRU-1 the walk does not go up past the root", "internal/store/worktree.go",
  "\tfor d := resolved; ; d = filepath.Dir(d) {\n\t\tif _, err := os.Lstat(filepath.Join(d, \".git\")); err == nil {\n\t\t\treturn d, nil\n\t\t}\n\t\tif filepath.Dir(d) == d {\n",
  "\tfor d := resolved; ; d = filepath.Dir(d) {\n\t\tif _, err := os.Lstat(filepath.Join(d, \".git\")); err == nil {\n\t\t\treturn d, nil\n\t\t}\n\t\tif filepath.Dir(d) == d || true {\n",
  "TestTRU1_")
m("TRU-1 a symlinked parent is not resolved", "internal/store/worktree.go",
  "\t\tr, err := filepath.EvalSymlinks(d)\n", "\t\tr, err := filepath.Abs(d)\n",
  "TestTRU1_")
m("TRU-1 SetRootInWorkTree drops the other keys", "internal/store/install_record.go",
  "\tfor _, k := range installRecordKeys {\n\t\tdelete(raw, k)\n\t}\n",
  "\traw = map[string]json.RawMessage{}\n",
  "TestTRU1_|TestTRU44_InstallRecordUpdateKeepsTheIDAndUnknownKeys")
m("TRU-1 SetRootInWorkTree creates a missing install.json", "internal/store/install_record.go",
  "\tb, err := os.ReadFile(path)\n\tif errors.Is(err, os.ErrNotExist) {\n\t\treturn ErrNoStore\n\t}\n",
  "\tb, err := os.ReadFile(path)\n\tif errors.Is(err, os.ErrNotExist) {\n\t\tb, err = []byte(`{\"install_id\":\"x\"}`), nil\n\t}\n",
  "TestTRU1_")
