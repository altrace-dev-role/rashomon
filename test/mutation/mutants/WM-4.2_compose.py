# WM-4.2: the frontier's composition (internal/store/frontiercompose.go, internal/graph/session/compose.go).
# A compose reads each run file to the size its first hold of the records lock read, and those sizes are
# the cursors' SizeSeen; it saves only inside the records lock under an unmoved gaps generation, takes a
# missed budget in silence, and continues a saved frontier only from an envelope of its schema, read under
# the generation of now, with both cursors valid and no late line; a forget of a window removes the
# frontier. Each break below is one of those promises undone. A run whose files are gone is guarded twice
# (the first take's missing records file and the cursor check), so no row here breaks it alone.
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
m('WM-4.2 a cursor its file refuses is continued', 'internal/graph/session/compose.go',
  'err != nil || !valid {',
  'err != nil || !valid && err != nil {',
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
  '\trl, rend, _, err := st.RunLinesFrom(sessionID, store.FileRecords, records, b.RecordsSize)\n\tif err != nil {\n\t\treturn runRead{}, err\n\t}\n\tsl, send, _, err := st.RunLinesFrom(sessionID, store.FileSteer, steer, b.SteerSize)\n\tif err != nil {\n\t\treturn runRead{}, err\n\t}\n\tr := runRead{\n\t\trecords: fold.Cursor{Offset: rend, SizeSeen: b.RecordsSize, Generation: b.Generation},',
  '\trl, rend, rsize, err := st.RunLinesFrom(sessionID, store.FileRecords, records, b.RecordsSize)\n\tif err != nil {\n\t\treturn runRead{}, err\n\t}\n\tsl, send, _, err := st.RunLinesFrom(sessionID, store.FileSteer, steer, b.SteerSize)\n\tif err != nil {\n\t\treturn runRead{}, err\n\t}\n\tr := runRead{\n\t\trecords: fold.Cursor{Offset: rend, SizeSeen: rsize, Generation: b.Generation},',
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
