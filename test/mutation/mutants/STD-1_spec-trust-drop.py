# STD-1 (card SPEC-TRUST-DROP1, #153 D1 and d4): the trust screen says the
# spec's state as the bindings hold it, and the Stop names a spec the
# session edited after the user trusted it, with the verb that trusts it
# again.
S = "cmd/rashomon/spec.go"
R = "cmd/rashomon/recap_stop.go"
B = "internal/spec/trust.go"

# The trust screen.
m("STD-1 the screen says untrusted at the digest already bound", S,
  "\tcase ok && x.SpecDigest == digest:\n", "\tcase false:\n", "TestSTD1_")
m("STD-1 the screen says never trusted after an edit", S,
  "\tcase ok:\n\t\treturn \"now untrusted: edited", "\tcase false:\n\t\treturn \"now untrusted: edited", "TestSTD1_")

# Of.
m("STD-1 Of reads the path alone", B,
  "\t\treturn x.ProjectRoot == projectRoot && x.SpecPath == specPath\n\t})\n\tif i < 0 {\n",
  "\t\treturn x.SpecPath == specPath\n\t})\n\tif i < 0 {\n", "TestSTD1d_")
m("STD-1 Of reads the project alone", B,
  "\t\treturn x.ProjectRoot == projectRoot && x.SpecPath == specPath\n\t})\n\tif i < 0 {\n",
  "\t\treturn x.ProjectRoot == projectRoot\n\t})\n\tif i < 0 {\n", "TestSTD1d_")
m("STD-1 Of reads through a nil Bindings", B,
  "\tif b == nil {\n\t\treturn Binding{}, false\n\t}\n\ti := slices.IndexFunc", "\ti := slices.IndexFunc", "TestSTD1d_")

# The Stop's spec list.
m("STD-1 the Stop reads no binding", R,
  "\t\tif _, ok := b.Of(project, s.Path); ok {\n", "\t\tif false {\n", "TestSTD1b_")
m("STD-1 the Stop names a trusted spec", R,
  "\t\tif s.Trust != spec.Untrusted || !specWritten(snap, s.Path) {\n", "\t\tif !specWritten(snap, s.Path) {\n", "TestSTD1b_")
m("STD-1 the Stop names a spec the session did not write", R,
  "\treturn slices.ContainsFunc(snap.Integrity.Unresolved,", "\treturn true || slices.ContainsFunc(snap.Integrity.Unresolved,",
  "TestSTD1b_")
m("STD-1 a project-relative entry names no spec", R,
  "strings.HasSuffix(slashed, \"/\"+e.Path)", "strings.HasSuffix(slashed, \"/\"+e.Path+\"/\")", "TestSTD1b_")
m("STD-1 the Stop names a spec after a store write", R,
  "err != nil || verify.StoreWritten(run, root, seq) {\n", "err != nil {\n", "TestSTD1b_")

# The line.
m("STD-1 the value line leaves the spec out", "internal/recap/value.go",
  "\tb.WriteString(unboundLines(v))\n", "", "TestSTD1c_")
m("STD-1 the done flag leaves the spec out", "internal/recap/done.go",
  "\"\\n\" + unboundLines(v) + pointer(sessionID, fromPlugin), true", "\"\\n\" + pointer(sessionID, fromPlugin), true",
  "TestSTD1c_")
m("STD-1 the line prints a path the slot refuses", "internal/recap/value.go",
  "if s := slot.Path(p); s.Validate() == nil {", "if s := slot.Path(p); true {", "TestSTD1c_")
