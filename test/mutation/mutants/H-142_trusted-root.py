# H-142 (SPEC-TRUST2): hook.TrustedRoot has two makers and no exported field.
# One break per way a caller could get a raw root in: a third exported maker
# and an exported field beside the root.
m("H-142 a third maker of a TrustedRoot", "internal/hook/specdiscover.go",
  "// TrustRoot is the store root DiscoverSpec",
  "func RawRoot(root string) TrustedRoot { return TrustedRoot{root: root} }\n\n// TrustRoot is the store root DiscoverSpec",
  "TestH142_TrustedRootHasTwoMakers")
m("H-142 an exported field is added", "internal/hook/specdiscover.go",
  "type TrustedRoot struct{ root string }", "type TrustedRoot struct {\n\troot string\n\tRaw  string\n}",
  "TestH142_TrustedRootHasTwoMakers")
m("H-142 laststop discovery takes NoRun in place of TrustRoot", "cmd/rashomon/laststop.go",
  "hook.TrustRoot(run, st.Root(), pt.Seq)", "hook.NoRun(st.Root())",
  "TestH142_NoRunHasOneProductionCaller")
