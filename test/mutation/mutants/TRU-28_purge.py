# TRU-28: purge removes what rashomon put on the machine, in an order that
# lets an interrupted purge be run again, and nothing it did not list first.
m("TRU-28 the anchors are removed first, not last", "internal/purge/purge.go",
  "\tp.Files = append(files, filepath.Join(root, InstallMeta), filepath.Join(root, InstallKey))\n",
  "\tp.Files = append([]string{filepath.Join(root, InstallMeta), filepath.Join(root, InstallKey)}, files...)\n",
  "TestTRU28_")
m("TRU-28 a path that could not go does not keep the anchors", "internal/purge/purge.go",
  "\tif kept != nil {\n\t\to.Remaining = append(o.Remaining, kept...)\n",
  "\tif false {\n\t\to.Remaining = append(o.Remaining, kept...)\n",
  "TestTRU28_")
m("TRU-28 the root is planned for removal while it holds the user's files", "internal/purge/purge.go",
  "\tp.RemoveRoot = len(p.Foreign) == 0\n",
  "\tp.RemoveRoot = true\n",
  "TestTRU28_")
m("TRU-28 a name rashomon does not write is planned for removal", "internal/purge/purge.go",
  "\t\tdefault:\n\t\t\tp.Foreign = append(p.Foreign, path)\n",
  "\t\tdefault:\n\t\t\tfiles = append(files, path)\n",
  "TestTRU28_")
m("TRU-28 created dirs are not ordered deepest first", "internal/purge/purge.go",
  "\tsort.SliceStable(p.CreatedDirs, func(i, j int) bool { return depth(p.CreatedDirs[i]) > depth(p.CreatedDirs[j]) })\n",
  "",
  "TestTRU28_")
m("TRU-28 created_dirs may name home", "internal/purge/purge.go",
  "(home != \"\" && (d == home || Encloses(d, home)))",
  "false",
  "TestTRU28_")
m("TRU-28 a detached entry opens, and so creates, the purged store", "cmd/rashomon/main.go",
  "\tif installArg(args) != \"\" && !storeExistsAt(root) {\n",
  "\tif false {\n",
  "TestTRU28_")
m("TRU-28 purge goes ahead under an enabled plugin", "cmd/rashomon/purge.go",
  "\tif p != nil && p.Enabled {\n",
  "\tif false {\n",
  "TestTRU28_")
m("TRU-28 an empty answer is yes", "cmd/rashomon/purge.go",
  "\tcase \"y\", \"yes\":\n",
  "\tcase \"y\", \"yes\", \"\":\n",
  "TestTRU28_")
m("TRU-28 purge removes without asking or --yes", "cmd/rashomon/purge.go",
  "\tif !yes {\n",
  "\tif !yes && false {\n",
  "TestTRU28_")
m("TRU-28 --binary leaves the binary", "cmd/rashomon/purge.go",
  "\tif on {\n\t\treturn t.binary\n\t}\n",
  "\tif on {\n\t\treturn \"\"\n\t}\n",
  "TestTRU28_")
m("TRU-28 purge skips its detach", "cmd/rashomon/purge.go",
  "\tif err := cmdDetach([]string{\"--install\", t.rec.InstallID}, stdout); err != nil {\n",
  "\tif err := error(nil); err != nil {\n",
  "TestTRU28_")
m("TRU-28 watch does not record the configuration directory it creates", "cmd/rashomon/watch.go",
  "\tcreated, err := missingDirs(plan.root, filepath.Dir(plan.loc.User))\n",
  "\tcreated, err := missingDirs(plan.root)\n",
  "TestTRU28_")
m("TRU-28 purge refuses only a plugin whose hooks it can read", "cmd/rashomon/purge.go",
  "\tif p != nil && p.Enabled {\n",
  "\tif p != nil && p.Enabled && len(p.Events) > 0 {\n",
  "TestTRU28_")
m("TRU-28 a created dir that is now a symlink is planned for removal", "internal/purge/purge.go",
  "\t\tif dir, err := realDir(d); err == nil && !dir {\n",
  "\t\tif dir, err := realDir(d); err == nil && !dir && false {\n",
  "TestTRU28_")
m("TRU-28 a created dir that became a symlink after the plan is removed", "internal/purge/purge.go",
  "\tcase !dir:\n",
  "\tcase !dir && false:\n",
  "TestTRU28_")
m("TRU-28 anything at a created_dirs path counts as a directory", "internal/purge/purge.go",
  "\treturn info.Mode().IsDir(), nil\n",
  "\treturn info.Mode().IsDir() || true, nil\n",
  "TestTRU28_")
m("TRU-28 an empty created dir that cannot go is left as not empty", "internal/purge/purge.go",
  "\tcase notEmpty(d):\n",
  "\tcase true:\n",
  "TestTRU28_")
m("TRU-28 install.key goes although install.json stays", "internal/purge/purge.go",
  "\t\tif remove(anchors[0], &o.Removed) == nil {\n",
  "\t\tif remove(anchors[0], &o.Removed) == nil || true {\n",
  "TestTRU28_")
m("TRU-28 a path that fails after the anchors counts as rerunnable", "internal/purge/purge.go",
  "\t\t\to.Stranded = append(o.Stranded, path)\n",
  "\t\t\to.Remaining = append(o.Remaining, path)\n",
  "TestTRU28_")
m("TRU-28 a stranded path does not fail the purge", "cmd/rashomon/purge.go",
  "\tcase len(out.Stranded) > 0:\n",
  "\tcase false:\n",
  "TestTRU28_")
m("TRU-28 --binary is not refused on Windows", "cmd/rashomon/purge.go",
  "\tif goos == \"windows\" {\n\t\treturn fmt.Errorf(\"--binary: on Windows",
  "\tif false {\n\t\treturn fmt.Errorf(\"--binary: on Windows",
  "TestTRU28_")
m("TRU-28 --binary is not refused when the binary's path is unknown", "cmd/rashomon/purge.go",
  "\tif binary == \"\" {\n",
  "\tif false {\n",
  "TestTRU28_")
m("TRU-28 watch records no created_dirs before its settings write", "cmd/rashomon/watch.go",
  "\tif err := recordCreatedDirs(st.Root(), created); err != nil {\n",
  "\tif err := recordCreatedDirs(st.Root(), created[:0]); err != nil {\n",
  "TestTRU28_")
# The store writes resolved-cap at its root (hld-7) on every hook call that
# resolves a cap. A purge that does not plan it leaves it, and the store
# root that holds it, behind.
m("TRU-28 purge does not plan the resolved cap", "internal/purge/purge.go",
  "store.FileRecordingPaused, store.FileResolvedCap,\n", "store.FileRecordingPaused,\n",
  "TestTRU28_PurgeLeavesHomeAsFound")

# watch writes consent.json and harnesses.json at the root (TRU-1), each
# replaced through a ".<name>.tmp-" file; the disclosure names purge as what
# removes all four. A purge that does not plan one leaves it, and the root.
m("TRU-28 purge does not plan consent.json", "internal/purge/purge.go",
  "\tstore.FileConsent, store.FileHarnesses, ", "\tstore.FileHarnesses, ",
  "TestTRU28_PlanListsRashomonsNamesWithTheAnchorsLast")
m("TRU-28 purge does not plan harnesses.json", "internal/purge/purge.go",
  "\tstore.FileConsent, store.FileHarnesses, ", "\tstore.FileConsent, ",
  "TestTRU28_PlanListsRashomonsNamesWithTheAnchorsLast")
m("TRU-28 purge does not plan consent.json's temporary file", "internal/purge/purge.go",
  "\t\".\" + store.FileConsent + \".tmp-\", ", "\t",
  "TestTRU28_PlanListsRashomonsNamesWithTheAnchorsLast")
m("TRU-28 purge does not plan harnesses.json's temporary file", "internal/purge/purge.go",
  ", \".\" + store.FileHarnesses + \".tmp-\",\n", ",\n",
  "TestTRU28_PlanListsRashomonsNamesWithTheAnchorsLast")

# A hook that stood down writes harness_counts.json at the root, under its
# harness_counts.lock sidecar, through a ".harness_counts.json.rashomon-"
# replacement; the disclosure names purge as what removes all three.
m("TRU-28 purge does not plan harness_counts.json", "internal/purge/purge.go",
  " store.FileHarnessCounts, store.FileHarnessCountsLock,\n", " store.FileHarnessCountsLock,\n",
  "TestTRU28_PlanListsRashomonsNamesWithTheAnchorsLast")
m("TRU-28 purge does not plan harness_counts.lock", "internal/purge/purge.go",
  " store.FileHarnessCounts, store.FileHarnessCountsLock,\n", " store.FileHarnessCounts,\n",
  "TestTRU28_PlanListsRashomonsNamesWithTheAnchorsLast")
m("TRU-28 purge does not plan harness_counts.json's temporary file", "internal/purge/purge.go",
  "\n\t\".\" + store.FileHarnessCounts + \".rashomon-\"}", "}",
  "TestTRU28_PlanListsRashomonsNamesWithTheAnchorsLast")
