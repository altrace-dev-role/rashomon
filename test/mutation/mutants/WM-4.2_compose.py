# WM-4.2: the frontier's composition (internal/store/frontiercompose.go, internal/graph/session/compose.go,
# internal/fold/mark.go). A compose reads each run file to the size its first hold of the records lock read,
# and those sizes are the cursors' SizeSeen; it saves only inside the records lock under an unmoved gaps
# generation, takes a missed budget in silence, and continues a saved frontier only from an envelope of its
# schema, read under the generation of now, with both cursors valid, the records line that ends at its
# cursor the one the envelope's mark names, and no late line; a forget of a window removes the frontier.
# Each break below is one of those promises undone. A run whose records file is gone is guarded twice (the
# first take's missing records file and the cursor check), so one row undoes both at once.
m('WM-4.2 a compose takes no records lock', 'internal/store/frontiercompose.go',
  '\tunlock, err := s.lockAppend(f, s.lockWait(lockBudget))\n\tif err != nil {\n\t\treturn err\n\t}\n\tdefer unlock()\n',
  '',
  'TestComposeFrontier_ForgetHoldsOffTheSave')
m('WM-4.2 a compose saves across a moved generation', 'internal/store/frontiercompose.go',
  '\t\tif g != b0.Generation {\n\t\t\treturn errGenerationMoved\n\t\t}\n',
  '',
  'TestComposeFrontier_ForgetBetweenTheHoldsSavesNothing')
m('WM-4.2 a compose reports a missed lock budget', 'internal/store/frontiercompose.go',
  'errors.Is(err, ErrLockTimeout) || ',
  '',
  'TestComposeFrontier_HeldLockSavesNothing')
m('WM-4.2 a frontier read under another generation is continued', 'internal/graph/session/compose.go',
  'env.Records.Generation != b.Generation || ',
  '',
  'TestCompose_MovedGenerationFoldsWhole')
m('WM-4.2 a steer cursor with a generation is continued', 'internal/graph/session/compose.go',
  ' || env.Steer.Generation != 0 {',
  ' {',
  'TestCompose_MovedGenerationFoldsWhole')
m('WM-4.2 a records cursor its file refuses is continued', 'internal/graph/session/compose.go',
  'env.Records, env.Mark); err != nil || !valid {',
  'env.Records, env.Mark); err != nil || !valid && err != nil {',
  'TestCompose_ReplacedRecordsFoldWhole')
m('WM-4.2 a catch-up reads the records from the start', 'internal/graph/session/compose.go',
  'readRun(st, sessionID, env.Records.Offset, env.Steer.Offset, b)',
  'readRun(st, sessionID, 0, env.Steer.Offset, b)',
  'TestCompose_EqualsTheWholeFold')
m('WM-4.2 an envelope takes an unknown field', 'internal/graph/session/compose.go',
  '\tdec.DisallowUnknownFields()\n\tvar e envelope\n',
  '\tvar e envelope\n',
  'TestParseEnvelope_RefusesAnyOtherFile')
m('WM-4.2 an envelope takes data after the object', 'internal/graph/session/compose.go',
  'if _, err := dec.Token(); err != io.EOF {',
  'if _, err := dec.Token(); err != io.EOF && false {',
  'TestParseEnvelope_RefusesAnyOtherFile')
m('WM-4.2 an envelope of another schema is taken', 'internal/graph/session/compose.go',
  '\tif e.Schema != envelopeSchema {',
  '\tif false {',
  'TestParseEnvelope_RefusesAnyOtherFile')
m('WM-4.2 a catch-up continues past a late steer line', 'internal/graph/session/fold.go',
  '\tif len(sorted) > 0 && !f.Through.before(sorted[0]) {',
  '\tif false {',
  'TestCompose_LateSteerLineFoldsWhole')
m('WM-4.2 a forget of a window leaves its frontier', 'internal/store/gaps.go',
  '\tif err := removeFrontier(dir); err != nil {\n\t\treturn nil, err\n\t}\n\tif afterFrontierRemoved != nil {\n\t\tafterFrontierRemoved()\n\t}\n\tif sessionID == "" {\n\t\tsessionID = filepath.Base(dir)\n\t}\n\n\t// The gap spans what was removed.',
  '\tif afterFrontierRemoved != nil {\n\t\tafterFrontierRemoved()\n\t}\n\tif sessionID == "" {\n\t\tsessionID = filepath.Base(dir)\n\t}\n\n\t// The gap spans what was removed.',
  'TestCompose_ForgetRemovesTheFrontier')
m('WM-4.2 SizeSeen is read at the read, not in the hold', 'internal/graph/session/compose.go',
  '\trl, raw, rend, _, err := st.RunRegionFrom(sessionID, store.FileRecords, records, b.RecordsSize)\n\tif err != nil {\n\t\treturn runRead{}, err\n\t}\n\tsl, send, _, err := st.RunLinesFrom(sessionID, store.FileSteer, steer, b.SteerSize)\n\tif err != nil {\n\t\treturn runRead{}, err\n\t}\n\tr := runRead{\n\t\trecords: fold.Cursor{Offset: rend, SizeSeen: b.RecordsSize, Generation: b.Generation},',
  '\trl, raw, rend, rsize, err := st.RunRegionFrom(sessionID, store.FileRecords, records, b.RecordsSize)\n\tif err != nil {\n\t\treturn runRead{}, err\n\t}\n\tsl, send, _, err := st.RunLinesFrom(sessionID, store.FileSteer, steer, b.SteerSize)\n\tif err != nil {\n\t\treturn runRead{}, err\n\t}\n\tr := runRead{\n\t\trecords: fold.Cursor{Offset: rend, SizeSeen: rsize, Generation: b.Generation},',
  'TestComposeFrontier_ReadsToTheHeldBounds')
m('WM-4.2 the records are read past the hold', 'internal/graph/session/compose.go',
  'store.FileRecords, records, b.RecordsSize)',
  'store.FileRecords, records, -1)',
  'TestComposeFrontier_ReadsToTheHeldBounds')
m('WM-4.2 the steer file is read past the hold', 'internal/graph/session/compose.go',
  'store.FileSteer, steer, b.SteerSize)',
  'store.FileSteer, steer, -1)',
  'TestComposeFrontier_ReadsToTheHeldBounds')
m("WM-4.2 a run read stops at the file's end, not the limit", 'internal/store/frontiercompose.go',
  '\tif limit >= 0 && limit < stop {\n\t\tstop = limit\n\t}\n',
  '\tif limit >= 0 && limit < stop {\n\t\t_ = limit\n\t}\n',
  'TestComposeFrontier_ReadsToTheHeldBounds')
m('WM-4.2 a run whose records file is gone is continued', 'internal/graph/session/compose.go',
  "\t\tif locked {\n\t\t\tf, r, hit = catchUp(st, sessionID, b)\n\t\t} else {\n\t\t\tb = unbounded\n\t\t}\n\t\tif !hit {\n\t\t\tvar err error\n\t\t\tif r, err = readRun(st, sessionID, 0, 0, b); err != nil {\n\t\t\t\treturn nil, err\n\t\t\t}\n\t\t\tf, stats.full = Fold(sessionID, r.evs), true\n\t\t}\n\t\tstats.tail = len(r.evs)\n\t\tif !locked {\n\t\t\treturn nil, nil\n\t\t}\n\t\treturn marshalEnvelope(f, r)\n\t})\n\treturn f, stats, saved, err\n}\n\n// catchUp is the saved frontier continued by the lines after its cursors,\n// and whether it hit. Every failure on the way is a miss: the caller folds\n// the whole record, and a read that fails there too is its error.\n//\n// The records cursor is checked with its mark (fold.CursorTailValid), the\n// steer cursor by fold.CursorValid alone. The mark's limit is the limit of\n// this check: a records file replaced by one whose line ending at the cursor\n// is byte for byte the marked line, at the same offset, is continued as if\n// it were the file the frontier was folded from. Every records line carries\n// its own tool use id, so two runs' files do not share one by chance.\nfunc catchUp(st *store.Store, sessionID string, b store.Bounds) (Frontier, runRead, bool) {\n\tdata, ok, err := st.LoadFrontier(sessionID)\n\tif err != nil || !ok {\n\t\treturn Frontier{}, runRead{}, false\n\t}\n\tenv, f, err := parseEnvelope(data)\n\tif err != nil || env.Records.Generation != b.Generation || env.Steer.Generation != 0 {\n\t\treturn Frontier{}, runRead{}, false\n\t}\n\tdir := st.RunDir(sessionID)\n\tif valid, err := fold.CursorTailValid(filepath.Join(dir, store.FileRecords), env.Records, env.Mark); err != nil || !valid {\n\t\treturn Frontier{}, runRead{}, false\n",
  "\t\tif true {\n\t\t\tf, r, hit = catchUp(st, sessionID, b)\n\t\t} else {\n\t\t\tb = unbounded\n\t\t}\n\t\tif !hit {\n\t\t\tvar err error\n\t\t\tif r, err = readRun(st, sessionID, 0, 0, b); err != nil {\n\t\t\t\treturn nil, err\n\t\t\t}\n\t\t\tf, stats.full = Fold(sessionID, r.evs), true\n\t\t}\n\t\tstats.tail = len(r.evs)\n\t\tif !locked {\n\t\t\treturn nil, nil\n\t\t}\n\t\treturn marshalEnvelope(f, r)\n\t})\n\treturn f, stats, saved, err\n}\n\n// catchUp is the saved frontier continued by the lines after its cursors,\n// and whether it hit. Every failure on the way is a miss: the caller folds\n// the whole record, and a read that fails there too is its error.\n//\n// The records cursor is checked with its mark (fold.CursorTailValid), the\n// steer cursor by fold.CursorValid alone. The mark's limit is the limit of\n// this check: a records file replaced by one whose line ending at the cursor\n// is byte for byte the marked line, at the same offset, is continued as if\n// it were the file the frontier was folded from. Every records line carries\n// its own tool use id, so two runs' files do not share one by chance.\nfunc catchUp(st *store.Store, sessionID string, b store.Bounds) (Frontier, runRead, bool) {\n\tdata, ok, err := st.LoadFrontier(sessionID)\n\tif err != nil || !ok {\n\t\treturn Frontier{}, runRead{}, false\n\t}\n\tenv, f, err := parseEnvelope(data)\n\tif err != nil || env.Records.Generation != b.Generation || env.Steer.Generation != 0 {\n\t\treturn Frontier{}, runRead{}, false\n\t}\n\tdir := st.RunDir(sessionID)\n\tif valid, err := fold.CursorTailValid(filepath.Join(dir, store.FileRecords), env.Records, env.Mark); err != nil || !valid {\n\t\treturn f, runRead{}, true\n",
  'TestCompose_ReplacedRecordsFoldWhole')
m('WM-4.2 a records line of the marked length is taken whatever its bytes', 'internal/fold/mark.go',
  '\treturn hex.EncodeToString(h[:]) == m.SHA256, nil\n',
  '\treturn hex.EncodeToString(h[:]) != "", nil\n',
  'TestCompose_RecreatedRecordsFoldWhole')
m('WM-4.2 a marked line need not start after a line end', 'internal/fold/mark.go',
  " || (start > 0 && b[0] != '\\n')",
  '',
  'TestCursorTailValid')
m('WM-4.2 a catch-up that reads no records line drops the mark', 'internal/graph/session/compose.go',
  '\tif r.mark == nil {\n\t\tr.mark = env.Mark\n\t}\n',
  '',
  'TestCompose_EqualsTheWholeFold')
m('WM-4.2 the envelope is saved without its mark', 'internal/graph/session/compose.go',
  ' Mark: r.mark,',
  '',
  'TestCompose_EqualsTheWholeFold')
# The fold's Touched list (internal/graph/session/fold.go): a run of events gathers the files it wrote that Touched
# does not hold and merges them into Touched once, at the run's end: each kept once, sorted, in a new list, so a
# Frontier copied before the run keeps its own. A merge into the list's own array still gives the stepped frontier
# the right list, so only the copy taken before the run can see that break.
m('WM-4.2 a run merges its files into Touched in place', 'internal/graph/session/fold.go',
  'out := make([]string, len(touched)+len(ids))',
  'out := slices.Grow(touched, len(ids))[:len(touched)+len(ids)]',
  'TestStepIn_LeavesACopyTakenBeforeTheRun')
m('WM-4.2 a file a run wrote twice is kept twice', 'internal/graph/session/fold.go',
  'slices.Compact(ids)',
  'ids',
  'TestStepIn_LeavesACopyTakenBeforeTheRun')
m('WM-4.2 a run adds a file Touched already holds', 'internal/graph/session/fold.go',
  'slices.BinarySearch(f.Touched, id)',
  'slices.BinarySearch(f.Touched[:0], id)',
  'TestStepIn_LeavesACopyTakenBeforeTheRun')
