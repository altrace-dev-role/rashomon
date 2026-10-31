# VER-40, SPEC-TRUST2: DiscoverSpec's root is a TrustedRoot that only
# hook.TrustRoot and hook.NoRun make. The replay of a session's last Stop
# (laststop.go) holds a run and reads trust through TrustRoot; reading it
# as NoRun, the root as it is, keeps a spec trusted past a store write.
# The break builds; the test is the kill.
m("VER-40 the last-Stop replay reads trust past a store write", "cmd/rashomon/laststop.go",
  "hook.TrustRoot(run, st.Root(), pt.Seq)", "hook.NoRun(st.Root())", "TestST2_LastStopReadsTrustPastAStoreWrite")
m("VER-40 the replay reads trust at the session's start", "cmd/rashomon/laststop.go",
  "hook.TrustRoot(run, st.Root(), pt.Seq)", "hook.TrustRoot(run, st.Root(), 0)", "TestST2_LastStopReadsTrustPastAStoreWrite")
# NoRun hands the root back as it is; one that returned "" would leave the
# brief reading no binding.
m("VER-40 NoRun binds nothing", "internal/hook/specdiscover.go",
  "func NoRun(root string) TrustedRoot { return TrustedRoot{root: root} }",
  "func NoRun(root string) TrustedRoot { return TrustedRoot{} }", "TestST2b_BriefAtSessionStartReadsBindings")
