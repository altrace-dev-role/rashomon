# The record-scope gate: the hook writes a v5 content field only when the
# consent on file says record_scope v5, learned from the record_scope marker
# every consent write keeps equal to consent.json's answer. The marker
# ignored or read open, consent.json's fold reversed or the marker leading it,
# compaction dropping the record the fold reads, the gate after the caps or
# inverted, and each content field left ungated, each judged by the row that
# names that rule. There is no "compact skips the marker" break: compaction
# runs only inside Append, after the marker is removed and before it is
# written again, so no path writes consent.json without the marker write.
m("VER-1.2q the hook ignores the marker and records at v5", "internal/hook/handle.go",
  "\tscope, _ := store.RecordScopeMarker(st.Root())\n\treturn scope\n", "\treturn store.RecordScopeV5\n",
  "TestRS1_|TestRS1b_|TestRS2b_")
m("VER-1.2q no marker reads v5", "internal/store/scope.go",
  "\tif err != nil {\n\t\treturn RecordScopeV4, err\n\t}\n", "\tif err != nil {\n\t\treturn RecordScopeV5, err\n\t}\n",
  "TestRS1_|TestRS5_|TestRS6_")
m("VER-1.2q a marker holding neither word reads v5", "internal/store/scope.go",
  "\treturn RecordScopeV4, ErrRecordScopeWord\n", "\treturn RecordScopeV5, ErrRecordScopeWord\n",
  "TestRS5_")
m("VER-1.2q the earliest record naming a scope wins", "internal/consent/record.go",
  "\tfor i := len(recs) - 1; i >= 0; i-- {\n\t\tif s := recs[i].RecordScope; s != \"\" {\n",
  "\tfor i := 0; i < len(recs); i++ {\n\t\tif s := recs[i].RecordScope; s != \"\" {\n",
  "TestRS3_")
m("VER-1.2q the marker is not removed before the consent line", "internal/consent/record.go",
  "\tif err := store.RemoveRecordScopeMarker(root); err != nil {\n\t\treturn err\n\t}\n", "",
  "TestRS8_")
m("VER-1.2q the marker is written before the consent line", "internal/consent/record.go",
  "\tif err := appendRecord(root, line); err != nil {\n\t\treturn err\n\t}\n\tvar scopeErr, steerErr error\n"
  "\tif err := syncMarker(root); err != nil {\n\t\tscopeErr = fmt.Errorf(\"%w: %w\", ErrScopeMarker, err)\n\t}\n",
  "\tvar scopeErr, steerErr error\n"
  "\tif err := syncMarker(root); err != nil {\n\t\tscopeErr = fmt.Errorf(\"%w: %w\", ErrScopeMarker, err)\n\t}\n"
  "\tif err := appendRecord(root, line); err != nil {\n\t\treturn err\n\t}\n",
  "TestRS3_|TestRS4_")
m("VER-1.2q a failed marker write reads as success", "internal/consent/record.go",
  "\t\tscopeErr = fmt.Errorf(\"%w: %w\", ErrScopeMarker, err)\n", "\t\tscopeErr = nil\n",
  "TestRS8_")
m("VER-1.2q compaction drops the record RecordScope reads", "internal/consent/record.go",
  " || i == maxDisclosure || i == scoped {", " || i == maxDisclosure || scoped < -1 {",
  "TestRS10_")
m("VER-1.2q the gate runs after the caps", "internal/hook/handle.go",
  "\tstore.GateDeclaration(&decl, recordScope(h.st))\n"
  "\t// The caps: a value over its cap is left out of the record rather than\n"
  "\t// cut, and said so on the coverage record (REC-52), so no payload inside\n"
  "\t// the intake limit can make a record the store would refuse.\n"
  "\tif h.capDecl(&decl) {\n\t\th.withheld = true\n\t}\n",
  "\tif h.capDecl(&decl) {\n\t\th.withheld = true\n\t}\n"
  "\tstore.GateDeclaration(&decl, recordScope(h.st))\n",
  "TestRS1b_")
m("VER-1.2q the declaration gate is inverted", "internal/store/scope.go",
  "func GateDeclaration(d *Declaration, scope string) {\n\tif scope == RecordScopeV5 {",
  "func GateDeclaration(d *Declaration, scope string) {\n\tif scope != RecordScopeV5 {",
  "TestRS1_|TestRS2_|TestRS1u_")
m("VER-1.2q the execution gate is inverted", "internal/store/scope.go",
  "func GateExecution(x *Execution, scope string) {\n\tif scope == RecordScopeV5 {",
  "func GateExecution(x *Execution, scope string) {\n\tif scope != RecordScopeV5 {",
  "TestRS1u_")
m("VER-1.2q command is ungated", "internal/store/scope.go",
  "\t\td.Command, d.CommandCut, d.Masked, d.CommandWithheld = nil, nil, nil, noConsent()\n",
  "\t\td.CommandCut, d.Masked, d.CommandWithheld = nil, nil, noConsent()\n",
  "TestRS1u_")
m("VER-1.2q command_cut is ungated", "internal/store/scope.go",
  "\t\td.Command, d.CommandCut, d.Masked, d.CommandWithheld = nil, nil, nil, noConsent()\n",
  "\t\td.Command, d.Masked, d.CommandWithheld = nil, nil, noConsent()\n",
  "TestRS1u_")
m("VER-1.2q masked is ungated", "internal/store/scope.go",
  "\t\td.Command, d.CommandCut, d.Masked, d.CommandWithheld = nil, nil, nil, noConsent()\n",
  "\t\td.Command, d.CommandCut, d.CommandWithheld = nil, nil, noConsent()\n",
  "TestRS1u_")
m("VER-1.2q executed_command is ungated", "internal/store/scope.go",
  "\t\tx.ExecutedCommand, x.ExecutedCommandCut, x.ExecutedMasked, x.ExecutedCommandWithheld = nil, nil, nil, noConsent()\n",
  "\t\tx.ExecutedCommandCut, x.ExecutedMasked, x.ExecutedCommandWithheld = nil, nil, noConsent()\n",
  "TestRS1u_")
m("VER-1.2q cwd is ungated", "internal/store/scope.go",
  "\td.CWD = unconsented(d.CWD, &d.CWDWithheld)\n", "",
  "TestRS1_|TestRS1u_|TestRS2b_")
m("VER-1.2q project is ungated", "internal/store/scope.go",
  "\td.Project = unconsented(d.Project, &d.ProjectWithheld)\n", "",
  "TestRS1_|TestRS1u_")
m("VER-1.2q file_path is ungated", "internal/store/scope.go",
  "\td.FilePath = unconsented(d.FilePath, &d.FilePathWithheld)\n", "",
  "TestRS1_|TestRS1u_")
m("VER-1.2q written_paths is ungated", "internal/store/scope.go",
  "\t\td.WrittenPaths, d.WrittenPathsComplete, d.WrittenPathsWithheld = nil, &incomplete, noConsent()\n",
  "\t\td.WrittenPathsComplete, d.WrittenPathsWithheld = &incomplete, noConsent()\n",
  "TestRS1u_")
m("VER-1.2q target is ungated", "internal/store/scope.go",
  "\t\td.Target, d.TargetWithheld = nil, noConsent()\n", "\t\td.TargetWithheld = noConsent()\n",
  "TestRS1u_")
m("VER-1.2q head_after is ungated", "internal/store/scope.go",
  "\t\tx.HeadAfter, x.HeadAfterWithheld = nil, noConsent()\n", "\t\tx.HeadAfterWithheld = noConsent()\n",
  "TestRS1u_")
m("VER-1.2q status omits consent.json's answer", "cmd/rashomon/status_recording.go",
  "\t\tsource += \"; consent.json says \" + fold\n", "",
  "TestRS9_")
m("VER-1.2q purge does not plan the record_scope marker", "internal/purge/purge.go",
  "\tstore.FileRecordScope, store.FileSteerDisclosure,\n", "\tstore.FileSteerDisclosure,\n",
  "TestTRU28_PlanListsRashomonsNamesWithTheAnchorsLast|TestTRU28_PurgeListsEveryNameItsWritersExport")
m("VER-1.2q purge does not plan the record_scope marker's temporary file", "internal/purge/purge.go",
  " \".\" + store.FileRecordScope + \".tmp-\",", "",
  "TestTRU28_PlanListsRashomonsNamesWithTheAnchorsLast")
