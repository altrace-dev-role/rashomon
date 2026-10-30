# VER-3.2: the task file (internal/spec/task.go), its discovery
# (internal/spec/discover.go) and the store_touched check (card 3.2, D131;
# internal/verify/integrity.go, pathclass.go's inStore). One break per rule of README 3.2's list and
# per Gate A ruling: the cap, the keys, the closed and distinct kinds, the
# rune caps, the empty mechanism, the digest, current.json beside the spec
# that applied, a refusal costing the spec nothing, the implied rule, the
# active-file test, and the store check's class, segment, window and order.
# The kill row is the named test's.
T = "internal/spec/task.go"
D = "internal/spec/discover.go"
I = "internal/verify/integrity.go"
P = "internal/verify/pathclass.go"
S = "internal/verify/snapshot.go"
R = "internal/verify/tosteer.go"

# ParseTask.
m("VER-3.2 cap off by one", T, '\tif len(data) > MaxTaskBytes {\n', '\tif len(data) > MaxTaskBytes+1 {\n', "TestTF2_")
m("VER-3.2 unknown key accepted", T,
  '\tf := r.object(root, "id", "goal", "decision", "expected_evidence", "mechanism")\n', '\tf := r.index(root)\n', "TestTF3_")
m("VER-3.2 open evidence list", T, '\tout := list(r, n, r.oneOf(EvidenceKinds))\n', '\tout := list(r, n, r.plain)\n', "TestTF4_")
m("VER-3.2 duplicate accepted", T,
  '\t\tif slices.Contains(out[:i], out[i]) {\n', '\t\tif slices.Contains(out[:i], out[i]) && false {\n', "TestTF4_")
m("VER-3.2 rune cap on bytes", T,
  '\tt.Goal = field(r, f, "goal", true, r.why)\n',
  '\tt.Goal = field(r, f, "goal", true, func(n *node) string {\n\t\tif s := r.why(n); len(s) <= MaxWhyRunes {\n\t\t\treturn s\n\t\t}\n\t\tr.fail(ReasonTooLong, MaxWhyRunes)\n\t\treturn ""\n\t})\n',
  "TestTF5_")
m("VER-3.2 mechanism uncapped", T, '\treturn r.why(n)\n', '\treturn r.plain(n)\n', "TestTF5_")
m("VER-3.2 an empty mechanism refused", T,
  '\tif n.kind == kString && n.str == "" {\n', '\tif n.kind == kString && n.str == "" && false {\n', "TestTF1_")
m("VER-3.2 digest over raw bytes", T, '\tsum := sha256.Sum256(appendJCS(nil, root))\n', '\tsum := sha256.Sum256(data)\n', "TestTF6_")

# IsTaskFile.
m("VER-3.2 the active file by suffix alone", T,
  'strings.HasSuffix(p, "/"+TaskFile)', 'strings.HasSuffix(p, TaskFile)', "TestTF1b_")
m("VER-3.2 the active file uncleaned", T, '\tp = path.Clean(p)\n', '\t_ = path.Clean(p)\n', "TestTF1b_")

# Discovery.
m("VER-3.2 sibling read", T,
  'filepath.Join(filepath.Dir(filepath.Dir(filepath.Dir(spec))), filepath.FromSlash(TaskFile))',
  'filepath.Join(filepath.Dir(filepath.Dir(filepath.Dir(spec))), ".claude", "rashomon", "task", "abc.json")', "TestTF7_")
m("VER-3.2 the task beside the root, not the spec that applied", D, '\t\tnear = path\n', '', "TestTF7_")
m("VER-3.2 a task alone is no Compiled", D,
  '\tif len(d.c.Sources) == 0 && len(d.c.Errors) == 0 && d.c.Task == nil {\n',
  '\tif len(d.c.Sources) == 0 && len(d.c.Errors) == 0 {\n', "TestTF7_")
m("VER-3.2 refusal fatal", T,
  '\t\td.c.Errors = append(d.c.Errors, p+": "+err.Error())\n',
  '\t\td.c.Errors, d.c.Sources, d.files = append(d.c.Errors, p+": "+err.Error()), nil, nil\n', "TestTF8_")

# The implied rule.
m("VER-3.2 implied rule missing", D,
  '\tif c.Task != nil {\n\t\tc.Rules = append(c.Rules, taskRule())\n', '\tif c.Task != nil && false {\n\t\tc.Rules = append(c.Rules, taskRule())\n', "TestTF9_")
m("VER-3.2 implied rule over the wrong path", T,
  'Paths: []string{".claude/rashomon/task/**"}', 'Paths: []string{".claude/rashomon/**"}', "TestTF9_")
m("VER-3.2 implied rule takes the first source's trust", T,
  'DeclaredRule{ID: "builtin:task-file", Source: SourceImplied,', 'DeclaredRule{ID: "builtin:task-file", Source: 0,', "TestTF9_")

# store_touched (D131).
m("VER-3.2 class test dropped", I,
  '\tfor _, w := range writes {\n\t\tif w.Path != nil && w.Seq > from && w.Seq <= at && (inStore(*w.Path) || underStore(w.raw, storeRoot)) {\n',
  '\tfor _, w := range writes[:0] {\n\t\tif w.Path != nil && w.Seq > from && w.Seq <= at && (inStore(*w.Path) || underStore(w.raw, storeRoot)) {\n', "TestTF10_")
m("VER-3.2 segment loosened to .claude/", P, '"/.claude/rashomon/")', '"/.claude/")', "TestTF10_")
m("VER-3.2 the store check ignores the window", I,
  'if w.Path != nil && w.Seq > from && w.Seq <= at && (inStore(*w.Path) || underStore(w.raw, storeRoot)) {', 'if w.Path != nil && (inStore(*w.Path) || underStore(w.raw, storeRoot)) {', "TestTF10_")
m("VER-3.2 store entries unordered by path", I,
  'cmp.Compare(a.Marker, b.Marker), cmp.Compare(a.Path, b.Path))', 'cmp.Compare(a.Marker, b.Marker))', "TestTF10_")
m("VER-3.2 store_touched outside IntegrityKinds", S, '\tStoreTouched,\n}\n', '}\n', "TestVER52b_")
m("VER-3.2 store-touched unmapped for STEER", R, '\t\t\tStoreTouched:       "store-touched",\n', '', "TestVER54f_")
