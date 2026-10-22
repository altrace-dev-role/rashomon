# H-148: detach --harness codex deletes Codex's hooks.json when watch created
# it and our entries were all it held, and in no other case. settings.Remove's
# own refusals, a file changed since it was read and a symlink, are judged by
# its unit test.
m("H-148 watch does not record creating hooks.json", "cmd/rashomon/codex_install.go",
  "\tif done.created {", "\tif false {", "TestH148_")
m("H-148 detach ignores the creation marker", "cmd/rashomon/codex_install.go",
  "\tcreated := rootErr == nil && store.CodexHooksCreated(root, path)", "\tcreated := rootErr == nil && false && store.CodexHooksCreated(root, path)", "TestH148_")
m("H-148 detach deletes a hooks.json it did not create", "cmd/rashomon/codex_install.go",
  "\tcreated := rootErr == nil && store.CodexHooksCreated(root, path)", "\tcreated := rootErr == nil || store.CodexHooksCreated(root, path)", "TestH148_")
m("H-148 detach leaves the creation marker behind", "cmd/rashomon/codex_install.go",
  "\tif done.removed {\n\t\tif err := store.ClearCodexHooksCreated(root)", "\tif false {\n\t\tif err := store.ClearCodexHooksCreated(root)",
  "TestH148_")
m("H-148 the marker is believed for any path", "internal/store/codex.go",
  "\treturn err == nil && strings.TrimSuffix(string(data), \"\\n\") == path", "\treturn err == nil && (strings.TrimSuffix(string(data), \"\\n\") != \"\" || path == \"\")",
  "TestH148_")
m("H-148 a file still holding the user's hooks is deleted", "cmd/rashomon/main.go",
  "\t\tif removeIfEmpty && original != nil && doc.Empty() {", "\t\tif removeIfEmpty && original != nil {", "TestH148_")
m("H-148 an existing file is recorded as created", "cmd/rashomon/main.go",
  "created: err == nil && original == nil,", "created: err == nil,", "TestH148_")
m("H-148 Remove deletes a file that changed since it was read", "internal/settings/write.go",
  "\tif !bytes.Equal(current, original) {\n\t\treturn false, ErrChanged", "\tif len(current) < 0 {\n\t\treturn false, ErrChanged", "TestRemove")
m("H-148 Remove deletes a symlink", "internal/settings/write.go",
  "\tif !fi.Mode().IsRegular() {\n\t\treturn false, nil", "\tif fi == nil {\n\t\treturn false, nil", "TestRemove")
