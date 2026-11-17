# KS-1: content is kept only where the record scope is v5. The hook's overlay
# gate (keptGoEdit) and both of its writers, and the prompt's two content
# writers (the unit briefs and the conflict base), take the scope their
# caller read once; a session-content marker set where the scope marker is
# absent or names v4 keeps nothing. Each break below keeps content below v5,
# or reads the scope a second time on the prompt path.
H = "internal/hook/overlaywrite.go"
G = "cmd/rashomon/graph.go"
m("KS-1 the overlay gate keeps content below v5", H,
  "\tif !v5 || !overlayTool(tool) ||",
  "\tif !overlayTool(tool) ||",
  "TestKeptGoEdit_ContentKeptOnlyAtScopeV5|TestPost_MarkerOnScopeMarkerAbsentKeepsNothing")
m("KS-1 the PreToolUse writer keeps content below v5", "internal/hook/handle.go",
  "\th.writeOverlay(&p, declared, scope == store.RecordScopeV5)\n",
  "\th.writeOverlay(&p, declared, true)\n",
  "TestKeptGoEdit_ContentKeptOnlyAtScopeV5|TestPost_MarkerOnScopeMarkerAbsentKeepsNothing")
m("KS-1 the PostToolUse writer keeps content below v5", H,
  "seq: p.landedSeq, v5: p.scopeV5, gate: &p.gate}",
  "seq: p.landedSeq, v5: true, gate: &p.gate}",
  "TestKeptGoEdit_ContentKeptOnlyAtScopeV5|TestPost_MarkerOnScopeMarkerAbsentKeepsNothing")
m("KS-1 the unit briefs are kept below v5", "cmd/rashomon/graph_unitbrief.go",
  "\tif storeRoot == \"\" || !v5 || !store.SessionContentMarker(storeRoot) {\n",
  "\tif storeRoot == \"\" || !store.SessionContentMarker(storeRoot) {\n",
  "TestPrompt_ContentKeptOnlyAtScopeV5")
m("KS-1 the conflict base is kept below v5", "cmd/rashomon/conflictbase.go",
  "\tif storeRoot == \"\" || specOf == nil || !v5 || !store.SessionContentMarker(storeRoot) {\n",
  "\tif storeRoot == \"\" || specOf == nil || !store.SessionContentMarker(storeRoot) {\n",
  "TestPrompt_ContentKeptOnlyAtScopeV5")
m("KS-1 the prompt keeps the unit briefs whatever the scope", G,
  "\twriteUnitBriefs(storeRoot, sessionID, promptID, root, h.Repo, s, rev, b, opt, v5)\n",
  "\twriteUnitBriefs(storeRoot, sessionID, promptID, root, h.Repo, s, rev, b, opt, true)\n",
  "TestPrompt_ContentKeptOnlyAtScopeV5")
m("KS-1 the prompt keeps the conflict base whatever the scope", G,
  "\twriteConflictBase(storeRoot, sessionID, root, h.Repo, s, rev, specOf, v5)\n",
  "\twriteConflictBase(storeRoot, sessionID, root, h.Repo, s, rev, specOf, true)\n",
  "TestPrompt_ContentKeptOnlyAtScopeV5")
m("KS-1 a prompt writer reads the scope a second time", G,
  "\twriteUnitBriefs(storeRoot, sessionID, promptID, root, h.Repo, s, rev, b, opt, v5)\n",
  "\twriteUnitBriefs(storeRoot, sessionID, promptID, root, h.Repo, s, rev, b, opt, scopeOf != nil && scopeOf() == store.RecordScopeV5)\n",
  "TestGraphEvidence_OnlyAtRecordScopeV5")
