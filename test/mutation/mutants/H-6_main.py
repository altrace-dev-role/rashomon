m("H-6  watch appends a second entry of ours", "internal/install/install.go", "\t\tif !found {\n\t\t\tout = append(out, want)", "\t\tif true {\n\t\t\tout = append(out, want)", "TestH6_")
m("H-6  detach removes every install's entries", "internal/install/install.go",
  "\treturn RemoveIf(doc, func(id string) bool { return id == spec.InstallID }, force)",
  "\treturn RemoveIf(doc, func(string) bool { return true }, force)", "TestH6_")
m("H-6  another install's entry records into this store", "cmd/rashomon/main.go",
  "\tif id == \"\" || id == st.InstallID() {", "\tif id == \"\" || true {", "TestH6_ForeignInstallEntryStandsDown")
m("H-6  watch does not say another install's entries are present", "cmd/rashomon/watch.go",
  "\tif len(foreign) > 0 {", "\tif false && len(foreign) > 0 {", "TestH6_ForeignInstallEntryStandsDown")
m("H-6  detach --install ignores the id it was given", "internal/install/install.go",
  "\t\t\tif id == \"\" || !match(id) {", "\t\t\tif id == \"\" {", "TestH6_DetachByInstallIDNeedsNoStore")
m("H-6  detach --all removes only this machine's install", "cmd/rashomon/main.go",
  "\t\t\treturn install.RemoveIf(doc, func(string) bool { return true }, force)",
  "\t\t\treturn install.Remove(doc, install.Spec{InstallID: installID}, force)", "TestH6_DetachAllRemovesEveryInstall")
m("H-6  removing by predicate skips the intact check", "internal/install/install.go",
  "\t\t\t\tif !force {\n\t\t\t\t\treturn 0, nil, &ErrModified{Event: event, Detail: detail}\n\t\t\t\t}",
  "\t\t\t\tif false {\n\t\t\t\t\treturn 0, nil, &ErrModified{Event: event, Detail: detail}\n\t\t\t\t}", "TestH6_DetachAllRefusesAnEditedEntry")
m("H-6  detach drops the entries it is not removing", "internal/install/install.go",
  "\t\t\tif id == \"\" || !match(id) {\n\t\t\t\tout = append(out, e)\n\t\t\t\tcontinue\n\t\t\t}",
  "\t\t\tif id == \"\" || !match(id) {\n\t\t\t\tcontinue\n\t\t\t}", "TestH6_Detach")
m("H-6  detach opens the store although it was given the id", "cmd/rashomon/main.go",
  "\tpath, err := settings.UserPath()\n\tif err != nil {\n\t\treturn err\n\t}\n\trec := selfCheckRecord(installID, all)\n",
  "\tpath, err := settings.UserPath()\n\tif err != nil {\n\t\treturn err\n\t}\n\tif _, err := openStore(); err != nil {\n\t\treturn err\n\t}\n\trec := selfCheckRecord(installID, all)\n", "TestH6_Detach")
m("H-6  plain detach creates a store to learn the id", "cmd/rashomon/main.go",
  "\tif _, err := os.Stat(filepath.Join(root, installMetaFile)); err != nil {",
  "\tif _, err := os.Stat(filepath.Join(root, installMetaFile)); err == nil && false {", "TestH6_PlainDetachLeavesNoStoreBehind")
m("H-6  watch does not print the undo line", "cmd/rashomon/watch.go",
  "\tfmt.Fprintf(stdout, \"rashomon detach --install %s\\n\", installID)\n", "", "TestH6_WatchPrintsTheUndoLine")

m("H-6 detach has no way past an entry someone edited", "cmd/rashomon/main.go",
  "\t\tcase \"--force\":", "\t\tcase \"--force-disabled\":", "TestH6_DetachForceRemovesAnEditedEntry")
m("H-6 --force also removes hooks that are not ours", "internal/install/install.go",
  "\t\t\tid := Owner(e, event)\n\t\t\tif id == \"\" || !match(id) {", "\t\t\tid := Owner(e, event)\n\t\t\tif id == \"\" && !force || id != \"\" && !match(id) {",
  "TestH6_DetachForceRemovesAnEditedEntry|TestH4_")
m("settings reads a non-object top level as an empty document", "internal/settings/document.go",
  "\t\treturn nil, errors.New(\"not a JSON object\")", "\t\treturn nil, nil", "TestParseRefuses")
