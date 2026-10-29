# VER-5.5: the commit receipt (internal/receipt, the store's LatestRecordBytes,
# `rashomon receipt`). One break per promise: the trailer is the stored
# record's digest, the note is that record on that commit, the newest
# checkpoint of this project is the one named, and a digest the latest
# checkpoint lacks prints nothing. The kill row is the named test's.
R = "internal/receipt/receipt.go"
S = "internal/store/latestrecord.go"
C = "cmd/rashomon/receipt.go"

# The trailer and the note.
m("VER-5.5 the trailer digest is over the wrong bytes", R,
  '\treturn TrailerKey + ": " + Digest(record)\n', '\treturn TrailerKey + ": " + Digest(append(record, \'\\n\'))\n',
  "TestRG1b_|TestRG1e_")
m("VER-5.5 the record is a re-marshal of the stored line", R,
  "\t\t\trecord, ok, bestAt = line, true, h.RecordedAt\n",
  "\t\t\trecord, _ = json.Marshal(json.RawMessage(line))\n\t\t\tok, bestAt = true, h.RecordedAt\n", "TestRG1e_")
m("VER-5.5 the note is written for the wrong commit", R,
  "git notes --ref rashomon add -f -F - HEAD || true\n", "git notes --ref rashomon add -f -F - HEAD^ || true\n",
  "TestRG1b_")
m("VER-5.5 an older receipt is kept beside the new one", R,
  "--if-exists replace", "--if-exists add", "TestRG1a_")
m("VER-5.5 receipt note prints the record whatever the digest", C,
  "\t\tif !ok || receipt.Digest(record) != rest[0] {\n", "\t\tif !ok {\n", "TestRG1b_")

# Which checkpoint: this project's, the newest recorded, the stored bytes.
m("VER-5.5 the first checkpoint found is taken", R,
  "\t\tif !ok || h.RecordedAt > bestAt {\n", "\t\tif !ok {\n", "TestRG1e_")
m("VER-5.5 the walk reads every run", R, "\t\tif ok && r.at < bestAt {\n", "\t\tif false {\n", "TestRG1e_")
m("VER-5.5 another project's checkpoint is named", R,
  " || h.ProjectRoot != project {\n", " {\n", "TestRG1e_|TestRG1d_")
m("VER-5.5 a run's first record of the kind is handed back", S,
  "\t\tif head.Type == kind && AcceptsKind(kind, head.SchemaVersion) {\n",
  "\t\tif head.Type == kind && AcceptsKind(kind, head.SchemaVersion) && !ok {\n", "TestRG1f_")
m("VER-5.5 a refused version is handed back", S,
  "\t\tif head.Type == kind && AcceptsKind(kind, head.SchemaVersion) {\n", "\t\tif head.Type == kind {\n", "TestRG1f_")
m("VER-5.5 a refused read fails the receipt", S,
  "\terr = refusedAsSkipped(eachLine(filepath.Join(s.RunDir(sessionID), FileRecords), fn), s.refusal(), fn)\n",
  "\terr = eachLine(filepath.Join(s.RunDir(sessionID), FileRecords), fn)\n", "TestRG1f_ARefused")
