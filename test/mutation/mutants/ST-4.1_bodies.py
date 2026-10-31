# Card 4.1: transcript copies (PF-RC14). The copy is off unless the switch
# lists the project, byte exact and 0600 through a temporary file and a
# rename, held within a cap by evicting the oldest first, refused whole above
# the cap, and taken at SessionEnd alone; purge takes the copies and forget
# takes one project's. Each guard is broken once (R5).
SB = "internal/store/bodies.go"
HP = "internal/hook/probe.go"
PG = "internal/purge/purge.go"

m("ST-4.1 eviction removes the newest first", SB,
  '\tif c := a.mod.Compare(b.mod); c != 0 {\n\t\treturn c\n',
  '\tif c := a.mod.Compare(b.mod); c != 0 {\n\t\treturn -c\n', "TestRC14c_CapEvictsOldestFirst")
m("ST-4.1 eviction takes the user's file", SB,
  ' || path == except || !strings.HasSuffix(e.Name(), ".jsonl") {\n',
  ' || path == except {\n', "TestRC14c_CapEvictsOldestFirst")
m("ST-4.1 the over-cap refusal is dropped", SB,
  '\tif info.Size() > set.MaxBytes {\n\t\treturn false, ErrBodyOverCap\n',
  '\tif false && info.Size() > set.MaxBytes {\n\t\treturn false, ErrBodyOverCap\n', "TestRC14c_CapEvictsOldestFirst")
m("ST-4.1 a source at the cap is refused", SB,
  '\tif info.Size() > set.MaxBytes {\n',
  '\tif info.Size() >= set.MaxBytes {\n', "TestRC14c_CapEvictsOldestFirst")
m("ST-4.1 the copy is world-readable", SB,
  '\terr = tmp.Chmod(fileMode)\n',
  '\terr = tmp.Chmod(0o644)\n', "TestRC14b_CopyIsByteExactAnd0600")
m("ST-4.1 the rename is skipped", SB,
  '\treturn os.Rename(tmp.Name(), dst)\n',
  '\treturn nil\n', "TestRC14b_CopyIsByteExactAnd0600")
m("ST-4.1 the project check is dropped", SB,
  '\tif err != nil || !on || !slices.Contains(set.Projects, projectKey) {\n',
  '\tif err != nil || !on {\n', "TestRC14f_ProjectNotListedWritesNothing")
m("ST-4.1 a directory that is not one element is taken", SB,
  '\tif !isPlainSegment(projectDir) {\n\t\treturn false, fmt.Errorf("bodies: project directory %q is not one path element", projectDir)\n',
  '\tif false && !isPlainSegment(projectDir) {\n\t\treturn false, fmt.Errorf("bodies: project directory %q is not one path element", projectDir)\n', "TestRC14b_CopyIsByteExactAnd0600")
m("ST-4.1 forget leaves the project's copies", SB,
  '\t\tif err := os.Remove(filepath.Join(dir, name)); err != nil {\n\t\t\treturn removed, err\n',
  '\t\tif err := error(nil); err != nil {\n\t\t\treturn removed, err\n', "TestRC14h_ForgetBodiesRemovesTheProjectOnly")
m("ST-4.1 the copy moves to SessionStart", HP,
  '\tif phase == store.PhaseEnd && transcriptPath != "" && bodiesConsented(st.Root()) {\n',
  '\tif phase == store.PhaseStart && transcriptPath != "" && bodiesConsented(st.Root()) {\n', "TestRC14g_SessionEndCopiesWhenOn")
m("ST-4.1 purge leaves the copies", PG,
  '\t\treturn level == 1 && (strings.HasSuffix(name, ".jsonl") || strings.HasPrefix(name, store.BodiesTempPrefix))\n',
  '\t\treturn false\n', "TestRC14d_PurgeLeavesNoFile")
m("ST-4.1 the Stop path gains a copy", "cmd/rashomon/recap_stop.go",
  '\tlines, _, _ := st.ReadSteer(p.SessionID)\n',
  '\tlines, _, _ := st.ReadSteer(p.SessionID)\n\tif false {\n\t\t_, _ = st.CopyBody("", "", "", "", now)\n\t}\n', "TestRC14e_OnlySessionEndTouchesBodies")
m("ST-4.1 purge does not plan bodies.json", PG,
  '\trecap.StateFileName, recap.StateTempName, store.FileBodies, store.FileConfig, store.FileConfigLock}', '\trecap.StateFileName, recap.StateTempName, store.FileConfig, store.FileConfigLock}',
  "TestRC14d_PurgeLeavesNoFile|TestTRU28_PurgeListsEveryNameItsWritersExport")
m("ST-4.1 purge does not plan bodies.json's temporary file", PG,
  ', "." + store.FileBodies + ".tmp-", "." + store.FileConfig + ".tmp-", "." + store.FileLab + ".tmp-"}', ', "." + store.FileConfig + ".tmp-", "." + store.FileLab + ".tmp-"}', "TestRC14d_PurgeLeavesNoFile")
m("ST-4.1 the bodies rows left out of the inventory", "internal/disclose/disclose.go",
  'append(bodiesFiles(), labFiles()...)', 'labFiles()', "TestTRU23_BodiesPathsAreInTheInventory")
