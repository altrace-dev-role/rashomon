# WRITE-SAFE1, the write cap: rootio.ReplaceFile holds the file it writes to
# max bytes, so a writer holds its reader's cap by the reader's name.
# SaveFile passes MaxGraphFileBytes, the cap LoadFile reads under, and
# WriteFile passes MaxKnowledgeFileBytes, ReadDir's (WC1, WC2). The writer
# makes one comparison, the bytes asked over max, at each Write and once
# more after write returns, so a write func that drops a refusal is refused
# all the same, and the refusal wins over an error of the func's own (RF9);
# no max is unlimited (RF10). One row per test's "Break:" clause. Written,
# not run here: no Go build runs on this machine below the disk floor, so
# the rows are judged by the end sweep.
P = "internal/graph/persist.go"
K = "internal/graph/knowledge/schema.go"
R = "internal/rootio/rootio.go"
m("WRITE-SAFE1 SaveFile writes an index a byte past MaxGraphFileBytes", P,
  "0o600, MaxGraphFileBytes, func(w io.Writer) error { return Save(w, s, h) })\n",
  "0o600, MaxGraphFileBytes+1, func(w io.Writer) error { return Save(w, s, h) })\n",
  "TestWS1_WC1_")
m("WRITE-SAFE1 WriteFile writes a row file a byte past MaxKnowledgeFileBytes", K,
  "rootio.ReplaceFile(r, rel, 0o644, MaxKnowledgeFileBytes, func(w io.Writer) error {\n",
  "rootio.ReplaceFile(r, rel, 0o644, MaxKnowledgeFileBytes+1, func(w io.Writer) error {\n",
  "TestWS1_WC2_")
m("WRITE-SAFE1 the writer refuses the bytes asked at max", R,
  "\tif w.asked > w.max {\n", "\tif w.asked >= w.max {\n",
  "TestWS1_RF8_|TestWS1_RF10_")
m("WRITE-SAFE1 the writer counts the last Write's bytes alone", R,
  "\tw.asked += int64(len(p))\n", "\tw.asked = int64(len(p))\n",
  "TestWS1_RF8_|TestWS1_RF9_")
m("WRITE-SAFE1 the replace takes write's nil as the last word", R,
  "\tif err := cw.end(write(cw)); err != nil {\n", "\tif err := write(cw); err != nil {\n",
  "TestWS1_RF9_|TestWS1_RF10_")
m("WRITE-SAFE1 a refused Write puts its bytes in the file", R,
  "\tw.asked += int64(len(p))\n\tif err := w.over(); err != nil {\n\t\treturn 0, err\n\t}\n\treturn w.f.Write(p)\n",
  "\tn, werr := w.f.Write(p)\n\tw.asked += int64(len(p))\n\tif err := w.over(); err != nil {\n\t\treturn 0, err\n\t}\n\treturn n, werr\n",
  "TestWS1_RF9_")
m("WRITE-SAFE1 a refused Write says it wrote its bytes", R,
  "\t\treturn 0, err\n", "\t\treturn len(p), err\n",
  "TestWS1_RF9_")
m("WRITE-SAFE1 the refusal holds max as its size", R,
  "&regfile.OverCapError{Size: w.asked, Max: w.max}", "&regfile.OverCapError{Size: w.max, Max: w.max}",
  "TestWS1_RF8_")
m("WRITE-SAFE1 the write func's own error wins over the refusal", R,
  "\tif over := w.over(); over != nil {\n", "\tif over := w.over(); over != nil && err == nil {\n",
  "TestWS1_RF9_")
m("WRITE-SAFE1 a refusal keeps its temporary file beside name", R,
  "\t\tr.Remove(tmp) //nolint:errcheck // best effort: the write's error is the one to report\n",
  "\t\tif cw.over() == nil {\n\t\t\tr.Remove(tmp) //nolint:errcheck // best effort: the write's error is the one to report\n\t\t}\n",
  "TestWS1_RF9_")
m("WRITE-SAFE1 the check after write is made once the file is renamed over name", R,
  "\tif err := cw.end(write(cw)); err != nil {\n\t\tf.Close()     //nolint:errcheck // the write failed; the temp is removed\n\t\tr.Remove(tmp) //nolint:errcheck // best effort: the write's error is the one to report\n\t\treturn err\n\t}\n\tif err := f.Close(); err != nil {\n\t\tr.Remove(tmp) //nolint:errcheck // best effort: the close's error is the one to report\n\t\treturn err\n\t}\n\tif err := r.Rename(tmp, name); err != nil {\n\t\tr.Remove(tmp) //nolint:errcheck // best effort: the rename's error is the one to report\n\t\treturn err\n\t}\n\treturn nil\n}\n",
  "\tif err := write(cw); err != nil {\n\t\tf.Close()     //nolint:errcheck // the write failed; the temp is removed\n\t\tr.Remove(tmp) //nolint:errcheck // best effort: the write's error is the one to report\n\t\treturn err\n\t}\n\tif err := f.Close(); err != nil {\n\t\tr.Remove(tmp) //nolint:errcheck // best effort: the close's error is the one to report\n\t\treturn err\n\t}\n\tif err := r.Rename(tmp, name); err != nil {\n\t\tr.Remove(tmp) //nolint:errcheck // best effort: the rename's error is the one to report\n\t\treturn err\n\t}\n\treturn cw.end(nil)\n}\n",
  "TestWS1_RF9_")
m("WRITE-SAFE1 a refused Write takes its bytes back out of the count", R,
  "\tw.asked += int64(len(p))\n\tif err := w.over(); err != nil {\n\t\treturn 0, err\n\t}\n",
  "\tw.asked += int64(len(p))\n\tif err := w.over(); err != nil {\n\t\tw.asked -= int64(len(p))\n\t\treturn 0, err\n\t}\n",
  "TestWS1_RF9_")
