# The SessionStart probe sets the record_scope marker from consent.json once
# per session, writes only on a difference, removes the marker and names the
# reason when consent.json cannot be read, and is the only hook that reads
# consent.json. Each rule broken once, judged by the RS11 row that names it.
# Not broken here: the marker-write failure branch, which no row can reach
# without also failing the probe's own coverage write in the same root.
m("VER-1.2q the session start never sets the marker", "internal/hook/probe.go",
  "\t\t\tif !syncRecordScope(st.Root()) {\n\t\t\t\treason = store.ReasonRecordScopeUnresolved\n\t\t\t}\n", "",
  "TestRS11_|TestRS11c_")
m("VER-1.2q an unresolved record scope names no reason", "internal/hook/probe.go",
  "\t\t\t\treason = store.ReasonRecordScopeUnresolved\n", "\t\t\t\t_ = store.ReasonRecordScopeUnresolved\n",
  "TestRS11c_")
m("VER-1.2q the binary hands the probe no consent reader", "cmd/rashomon/consent.go",
  "\thook.ConsentRecordScope = consent.RecordScope\n", "",
  "TestRS11_")
m("VER-1.2q an unreadable consent.json keeps the marker", "internal/hook/recordscope.go",
  "\tif err != nil {\n\t\t_ = store.RemoveRecordScopeMarker(root)\n\t\treturn false\n\t}\n\tif want",
  "\tif err != nil {\n\t\treturn false\n\t}\n\tif want",
  "TestRS11c_")
m("VER-1.2q the session start rewrites a matching marker", "internal/hook/recordscope.go",
  "\tif have == want && (err == nil || errors.Is(err, fs.ErrNotExist)) {\n",
  "\tif have == want && (err == nil || errors.Is(err, fs.ErrNotExist)) && want == \"\" {\n",
  "TestRS11d_")
m("VER-1.2q no marker counts as a difference under no consent", "internal/hook/recordscope.go",
  "\tif have == want && (err == nil || errors.Is(err, fs.ErrNotExist)) {\n",
  "\tif have == want && (err == nil || errors.Is(err, fs.ErrNotExist) && want != store.RecordScopeV4) {\n",
  "TestRS11b_")
m("VER-1.2q PreToolUse reads consent.json", "internal/hook/handle.go",
  "\tscope, _ := store.RecordScopeMarker(st.Root())\n",
  "\tsyncRecordScope(st.Root())\n\tscope, _ := store.RecordScopeMarker(st.Root())\n",
  "TestRS11e_")
m("VER-1.2q SessionEnd reads consent.json", "internal/hook/probe.go",
  "\t\tcase store.PhaseEnd:\n", "\t\tcase store.PhaseEnd:\n\t\t\tsyncRecordScope(st.Root())\n",
  "TestRS11e_")
