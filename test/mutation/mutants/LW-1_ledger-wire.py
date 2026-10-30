# LW-1: the obligation ledger on the live Stop path and the ledger verb (card
# LEDGER-WIRE1, PF-PG5). One break per link: the ledger's scope, the Stop's
# expected rows reaching it, the pinned Required, the done clause reading the
# rows; and in the verb's replay, the checkpoint's point_seq, the unread final
# message, the spec mismatch line, the spec's trust (the store root discovery
# reads the bindings under) and a declared row's family (its gate id). The
# kill row is the named test's.
#
# "Scope lost" is the empty scope: every point kind maps to the session scope
# and cmp.Or falls back to it (obligations.go:92-93), so a lost lookup alone
# is equivalent. "Ledger from counted" was equivalent too (R12) and is
# replaced by the verb evaluating at the last seq.
O = "internal/verify/obligations.go"
S = "internal/verify/stop.go"
DN = "internal/recap/done.go"
LS = "cmd/rashomon/laststop.go"
LR = "cmd/rashomon/report_ledger.go"

# The live Stop path.
m("LW-1 the ledger's scope lost", O,
  'scope := cmp.Or(obligationScopes()[snap.Point.Kind], ScopeSession)\n',
  'scope := cmp.Or(obligationScopes()[""], "")\n', "TestLW1b_")
m("LW-1 the Stop's expected rows dropped from the ledger", S,
  '\tv.Obligations = ledgerOf(snap, &c, v.Expected)\n', '\tv.Obligations = ledgerOf(snap, &c, nil)\n', "TestLW1b_")
m("LW-1 the pinned Required ignored", S,
  '\tpinRequired(&snap, in.Required)\n', '', "TestLW1l_")
m("LW-1 the done clause reads v.Expected", DN,
  '\t\t\treturn verify.UnmetExpected(v.Expected, v.Obligations)\n',
  '\t\t\tvar out []verify.Expected\n\t\t\tfor _, e := range v.Expected {\n\t\t\t\tif e.State == verify.ObligationUnmet {\n'
  '\t\t\t\t\tout = append(out, e)\n\t\t\t\t}\n\t\t\t}\n\t\t\treturn out\n', "TestLW1k_")

# The verb's replay.
m("LW-1 the verb evaluates at the last seq instead of point_seq", LS,
  '\t\tpt.Seq, pt.PromptID, now = cp.PointSeq, cp.PromptID, time.UnixMilli(cp.RecordedAtUnixMS)\n',
  '\t\tpt.PromptID, now = cp.PromptID, time.UnixMilli(cp.RecordedAtUnixMS)\n', "TestLW1e_")
m("LW-1 MessageUnread left unset by the verb", LS,
  'in := verify.StopInput{Key: st.Key(), MessageUnread: true}', 'in := verify.StopInput{Key: st.Key()}', "TestLW1j_")
m("LW-1 the spec mismatch line dropped", LR,
  'if cp != nil && specText(cp.SpecDigest, cp.SpecTrust) != now {',
  'if false && cp != nil && specText(cp.SpecDigest, cp.SpecTrust) != now {', "TestLW1d_")
m("LW-1 the replay discovers the spec with no store root", LS,
  'hook.DiscoverSpec(root, projectDir, managedDir, st.Root())', 'hook.DiscoverSpec(root, projectDir, managedDir, "")', "TestLW1n_")
m("LW-1 a declared row's family matched over Family alone", LR,
  'for _, fam := range []string{g.Family, g.Gate} {', 'for _, fam := range []string{g.Family} {', "TestLW1o_")
