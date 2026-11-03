# SUF-1 (STOP-UNBOUND-FIRST, QA-2 e2e/live/PR-196.txt items 1 and 13): when
# a spec declares gates that read untrusted, the Stop's no-gate line says
# they do not count, never that none is known, within 80 columns; the line
# under it says why: the spec is not trusted and the step that counts it, or,
# for a spec the session edited after a trust, SPEC-TRUST-DROP1's two lines
# alone. After a session write under the store root nothing offers the step.
V = "internal/recap/value.go"
S = "cmd/rashomon/recap_stop.go"

m("SUF-1 the no-gate line says none is known beside an untrusted gate", V,
  "\t\tif s := notCounted(v, k, short); s != \"\" {\n",
  "\t\tif s := notCounted(v, k, short); false && s != \"\" {\n", "TestSUF1_|TestSUF1b_")
m("SUF-1 no line says the spec is not trusted", V,
  "\tif v.Summary.Row == verify.RowNoGate && untrustedOnly(v) {\n",
  "\tif false && v.Summary.Row == verify.RowNoGate && untrustedOnly(v) {\n", "TestSUF1_|TestSUF1b_")
m("SUF-1 the review step under an unbound spec's lines", V,
  "return slices.ContainsFunc(v.Untrusted, func(u verify.UntrustedGate) bool { return !slices.Contains(v.Unbound, u.Spec) })",
  "return len(v.Untrusted) > 0", "TestSUF1_")
m("SUF-1 the trust step offered after a store write", S,
  "\tif out != nil && verify.StoreWritten(run, root, seq) {\n",
  "\tif false && out != nil && verify.StoreWritten(run, root, seq) {\n", "TestSUF1_")
m("SUF-1 the names carry the line past 80 columns", V,
  "\t\tif short || !ok || p.Validate() != nil {\n", "\t\tif !ok || p.Validate() != nil {\n", "TestSUF1b_")
m("SUF-1 every gate named however many", V,
  "\tcase len(names) > k:\n", "\tcase false:\n", "TestSUF1b_")
