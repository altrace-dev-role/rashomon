# VER-1.6: the project index (internal/verify/index.go, PF-PG1). One break per
# predicate of Required, Observe, ReadIndex, WriteIndex and its eviction, and
# IndexPath; and the store's ReplaceFile mode, which is the index file's. The
# kill row is the named IDX test's (not PG1x: TestPG1c_ already names five
# tests in internal/shape and internal/hook). No fuzz target: ReadIndex's
# input is rashomon's own file, read under a cap with a closed shape.
I = "internal/verify/index.go"
S = "internal/store/install_record.go"
INDEX_IMPORTS = ("\t\"github.com/altrace-dev-role/rashomon/internal/baseline\"\n"
                 "\t\"github.com/altrace-dev-role/rashomon/internal/store\"\n")

# Required.
m("VER-1.6 the session threshold is one too high", I,
  "\treturn e.Sessions >= RequiredSessions && ", "\treturn e.Sessions > RequiredSessions && ", "TestIDX1_")
m("VER-1.6 the session threshold is one too low", I,
  "\treturn e.Sessions >= RequiredSessions && ", "\treturn e.Sessions >= RequiredSessions-1 && ", "TestIDX2_")
m("VER-1.6 the window closes a millisecond early", I,
  "nowMS-e.LastSeen <= RequiredWindowMS\n", "nowMS-e.LastSeen < RequiredWindowMS\n", "TestIDX3_")
m("VER-1.6 the window closes a millisecond late", I,
  "nowMS-e.LastSeen <= RequiredWindowMS\n", "nowMS-e.LastSeen <= RequiredWindowMS+1\n", "TestIDX3_")
m("VER-1.6 a nil index requires every family", I,
  "\tif idx == nil {\n\t\treturn false\n", "\tif idx == nil {\n\t\treturn true\n", "TestIDX2_")
m("VER-1.6 a family not in the index is required", I,
  "\tif !ok {\n\t\treturn false\n", "\tif !ok {\n\t\treturn true\n", "TestIDX2_")

# Observe.
m("VER-1.6 the same session is counted twice", I,
  "\t\tif e.LastSession != sessionID {\n", "\t\tif true {\n", "TestIDX2_")
m("VER-1.6 a new family's first session is not counted", I,
  "Kind: kind, Sessions: 1,\n", "Kind: kind, Sessions: 0,\n", "TestIDX1_|TestIDX2_")
m("VER-1.6 LastSeen is not moved", I,
  "e.LastSeen, e.FirstSeen = max(held, nowMS), min(e.FirstSeen, nowMS)", "e.FirstSeen = min(e.FirstSeen, nowMS)",
  "TestIDX3_")
m("VER-1.6 FirstSeen is overwritten", I, ", min(e.FirstSeen, nowMS)\n", ", nowMS\n", "TestIDX3_|TestIDX4_")
m("VER-1.6 a known family keeps its old kind", I, "\t\t\te.Kind = kind\n", "", "TestIDX7_")
# 1.6b: sessions recapped out of order, each at its own time.
m("VER-1.6 LastSeen moves back to an older session", I,
  "e.LastSeen, e.FirstSeen = max(held, nowMS), ", "e.LastSeen, e.FirstSeen = nowMS, ", "TestIDX3b_")
m("VER-1.6 FirstSeen does not move to an older session", I,
  ", min(e.FirstSeen, nowMS)\n", ", e.FirstSeen\n", "TestIDX3b_")
m("VER-1.6 an older session's kind replaces the newer's", I,
  "\t\tif nowMS >= held {\n\t\t\te.Kind = kind\n\t\t}\n", "\t\te.Kind = kind\n", "TestIDX3b_")
m("VER-1.6 a tie keeps the held kind", I, "\t\tif nowMS >= held {\n", "\t\tif nowMS > held {\n", "TestIDX3b_")
m("VER-1.6 LastSession kept at the newer session", I,
  "\t\t\te.LastSession = sessionID\n", "\t\t\tif nowMS >= held {\n\t\t\t\te.LastSession = sessionID\n\t\t\t}\n",
  "TestIDX3b_")
m("VER-1.6 the runs are not in Seq order for kindOf", I,
  "\t\tslices.SortFunc(fr, func(a, b gateRun) int { return cmp.Compare(a.Seq, b.Seq) })\n", "",
  "TestIDX6_|TestIDX7_")
m("VER-1.6 the kind is the last run's", I,
  "kind, family := kindOf(fr), ", "kind, family := fr[len(fr)-1].Kind, ", "TestIDX7_")
m("VER-1.6 a run with no family adds an entry", I,
  "\t\tif k, ok := r.family(); ok {\n", "\t\tif k, _ := r.family(); true {\n", "TestIDX8_")
m("VER-1.6 Observe leaves the entries unsorted", I,
  "\tslices.SortFunc(idx.Entries, entryOrder)\n", "", "TestIDX6_")

# ReadIndex.
m("VER-1.6 a missing file reads as an error", I,
  "\tif errors.Is(err, fs.ErrNotExist) {\n\t\treturn nil, nil\n",
  "\tif errors.Is(err, fs.ErrNotExist) {\n\t\treturn nil, err\n", "TestIDX5_")
m("VER-1.6 the read limit is the cap, not a byte past it", I,
  "N: MaxIndexBytes + 1}", "N: MaxIndexBytes}", "TestIDX5_")
m("VER-1.6 a file over the cap reads", I, "\tif lr.N == 0 {\n", "\tif false {\n", "TestIDX5_")
m("VER-1.6 an unknown field reads", I, "\tdec.DisallowUnknownFields()\n", "", "TestIDX5_")
m("VER-1.6 data after the one value reads", I,
  "\tif _, err := dec.Token(); !errors.Is(err, io.EOF) {\n", "\tif _, err := dec.Token(); false && err != nil {\n",
  "TestIDX5_")
m("VER-1.6 entries out of order read", I,
  "\t\tif idx.Entries[i-1].Family >= idx.Entries[i].Family {\n", "\t\tif false {\n", "TestIDX5_")
m("VER-1.6 a family twice reads", I,
  "\t\tif idx.Entries[i-1].Family >= idx.Entries[i].Family {\n",
  "\t\tif idx.Entries[i-1].Family > idx.Entries[i].Family {\n", "TestIDX5_")

# WriteIndex and its eviction.
m("VER-1.6 WriteIndex writes the entries unsorted", I, "\tslices.SortFunc(entries, entryOrder)\n", "", "TestIDX6_")
m("VER-1.6 an empty index writes entries null", I,
  "\tif entries == nil {\n\t\tentries = []IndexEntry{}\n\t}\n", "", "TestIDX6_")
m("VER-1.6 nothing is evicted over the cap", I,
  "\t\tif total <= MaxIndexBytes {\n\t\t\tbreak\n", "\t\tif true {\n\t\t\tbreak\n", "TestIDX4_")
m("VER-1.6 an encoding exactly at the cap is evicted from", I,
  "\t\tif total <= MaxIndexBytes {\n\t\t\tbreak\n", "\t\tif total < MaxIndexBytes {\n\t\t\tbreak\n", "TestIDX4_")
m("VER-1.6 eviction goes by Family first", I,
  "cmp.Or(cmp.Compare(entries[a].LastSeen, entries[b].LastSeen), cmp.Compare(entries[a].Family, entries[b].Family))",
  "cmp.Or(cmp.Compare(entries[a].Family, entries[b].Family), cmp.Compare(entries[a].LastSeen, entries[b].LastSeen))",
  "TestIDX4_")
m("VER-1.6 between equals the last family goes", I,
  "cmp.Compare(entries[a].Family, entries[b].Family))", "cmp.Compare(entries[b].Family, entries[a].Family))",
  "TestIDX4_")
m("VER-1.6 an entry's comma is not counted", I, "\t\tsize[i] = len(b) + 1\n", "\t\tsize[i] = len(b)\n", "TestIDX4_")
m("VER-1.6 the frame is counted with a comma too many", I,
  "\ttotal := len(frame) - 1 ", "\ttotal := len(frame) ", "TestIDX4_")
m("VER-1.6 the evicted count is not returned", I, "\treturn kept, n, nil\n", "\treturn kept, 0, nil\n", "TestIDX4_")
m("VER-1.6 a write over the cap is not refused", I,
  "\tif len(encoded) > MaxIndexBytes {\n", "\tif false {\n", "TestIDX4_")
m("VER-1.6 the projects directory is made 0755", I,
  "os.MkdirAll(filepath.Dir(path), 0o700)", "os.MkdirAll(filepath.Dir(path), 0o755)", "TestIDX4_")
m("VER-1.6 the index is written in place, not renamed over", I,
  "\tif err := store.ReplaceFile(path, encoded); err != nil {\n",
  "\tif err := os.WriteFile(path, encoded, 0o600); err != nil {\n", "TestIDX4_")
IMPORTS["VER-1.6 the index is written in place, not renamed over"] = (I,
  INDEX_IMPORTS, "\t\"github.com/altrace-dev-role/rashomon/internal/baseline\"\n")
m("VER-1.6 the index file is written 0644", S,
  "\tif err := tmp.Chmod(fileMode); err != nil {\n", "\tif err := tmp.Chmod(0o644); err != nil {\n", "TestIDX4_")

# IndexPath.
m("VER-1.6 the key is used raw as the file name", I,
  "ProjectsDir, baseline.FileName(projectKey))", "ProjectsDir, projectKey)", "TestIDX9_")
m("VER-1.6 the file name carries a second .json", I,
  "ProjectsDir, baseline.FileName(projectKey))", "ProjectsDir, baseline.FileName(projectKey)+\".json\")", "TestIDX9_")
